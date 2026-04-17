package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	log.Println("--- OPC UA Modbus 稳健型网关启动中 ---")

	// 1. 加载配置（由 python 脚本生成的两个 JSON）
	channelConf := LoadChannelConfig("channel_config.json")
	modbusConf := LoadModbusConfig("modbus_config.json")
	log.Println("✅ 配置加载成功")

	// 2. 初始化核心 DataStore (内存布局)
	ds := NewDataStore(channelConf)

	// 3. 初始化南向 Modbus 管理器 (严格对接附件架构)
	modbusMgr := NewModbusManager(modbusConf, ds)

	// 4. 启动向量化计算引擎 (100ms 周期)
	// 该协程负责全量 AI 转换、AO 逆向转换、字节序调换和钳位保护
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			ds.ProcessAllConversions()
		}
	}()
	log.Println("📐 100ms 向量化计算引擎已就绪")

	// 5. 启动南向采集协程池
	modbusMgr.Start()
	log.Println("📡 南向采集服务已启动")

	// 6. 启动北向接口 (OPC UA + Web)
	go RunOPCUAServer(ds, modbusMgr)
	go RunWebServer(ds)

	// 7. 优雅停机信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Println("🚀 网关全模块运行中。按 Ctrl+C 停止。")
	<-sigChan

	log.Println("🛑 正在关闭网关并清理资源...")
	// 此处可扩展对各 Modbus Client 的 Close 操作
	log.Println("👋 网关已安全停止")
}
