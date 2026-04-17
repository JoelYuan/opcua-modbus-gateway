package main

import (
	"log"
	"time"

	"github.com/goburrow/modbus"
)

// ModbusClient 代表一个基于附件架构的物理连接或逻辑从站
type ModbusClient struct {
	DeviceID  string
	Config    DeviceConfig
	Handler   *modbus.TCPClientHandler // 严格对接附件中的 TCP 模式
	Client    modbus.Client            // 严格对接附件中的 Client 接口
	DataStore *DataStore
}

// ModbusManager 管理所有从站的生命周期
type ModbusManager struct {
	Clients map[string]*ModbusClient
}

// NewModbusManager 根据附件架构初始化管理器
func NewModbusManager(conf map[string]DeviceConfig, ds *DataStore) *ModbusManager {
	mgr := &ModbusManager{Clients: make(map[string]*ModbusClient)}

	for devID, devConf := range conf {
		// 从第一个读任务中获取连接信息（假设同一从站的所有任务连接信息相同）
		if len(devConf.ReadTasks) == 0 && len(devConf.WriteTasks) == 0 {
			log.Printf("⚠️ 设备 %s 没有读写任务，跳过", devID)
			continue
		}

		var task ModbusTask
		if len(devConf.ReadTasks) > 0 {
			task = devConf.ReadTasks[0]
		} else {
			task = devConf.WriteTasks[0]
		}

		// 严格按照附件逻辑初始化 Handler
		handler := modbus.NewTCPClientHandler(task.IPPort)
		handler.Timeout = 5 * time.Second
		handler.SlaveId = byte(task.Slave)

		// 附件架构建议在多从站环境下管理连接状态
		mgr.Clients[devID] = &ModbusClient{
			DeviceID:  devID,
			Config:    devConf,
			Handler:   handler,
			Client:    modbus.NewClient(handler),
			DataStore: ds,
		}
	}
	return mgr
}

// Start 启动并发采集任务
func (m *ModbusManager) Start() {
	for _, client := range m.Clients {
		go client.workLoop()
	}
}

// workLoop 维护每个从站的独立生命周期
func (c *ModbusClient) workLoop() {
	// 500ms 采集周期
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		// 1. 处理读取任务块 (Bulk Read)
		c.processReadTasks()

		// 2. 处理写入任务块 (AO/DO 同步)
		c.processWriteTasks()
	}
}

func (c *ModbusClient) processReadTasks() {
	for _, task := range c.Config.ReadTasks {
		var results []byte
		var err error

		// 严格对接附件 api.go 中定义的函数
		switch task.FC {
		case 1:
			results, err = c.Client.ReadCoils(uint16(task.Addr), uint16(task.Len))
		case 2:
			results, err = c.Client.ReadDiscreteInputs(uint16(task.Addr), uint16(task.Len))
		case 3:
			results, err = c.Client.ReadHoldingRegisters(uint16(task.Addr), uint16(task.Len))
		case 4:
			results, err = c.Client.ReadInputRegisters(uint16(task.Addr), uint16(task.Len))
		}

		if err != nil {
			log.Printf("❌ [%s] 读取偏移 %d 出错: %v", c.DeviceID, task.Addr, err)
			c.Handler.Close() // 附件建议在出错时显式关闭以触发重连
			continue
		}

		// 核心：无损搬运原始字节到 DataStore 数组的对应索引
		c.DataStore.UpdateFromModbus(task, results)
	}
}

func (c *ModbusClient) processWriteTasks() {
	// 获取 DataStore 中处于 AO/DO 区段且需要更新的数据
	// 逻辑：如果 DataStore 的 Raw 数组在 AO 区段被 ProcessAllConversions 改变了，则下发
	for _, task := range c.Config.WriteTasks {
		// 此处根据 task.BaseArrayIdx 检查 DataStore 是否有待写指令
		// 示例逻辑：对于 AO (FC6)
		if task.FC == 6 {
			val := c.DataStore.RawReg16[task.BaseArrayIdx]
			// 执行写入
			_, err := c.Client.WriteSingleRegister(uint16(task.Addr), val)
			if err != nil {
				log.Printf("⚠️ [%s] 写入寄存器 %d 失败: %v", c.DeviceID, task.Addr, err)
			}
		}
	}
}
