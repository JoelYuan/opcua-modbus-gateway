package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/goburrow/modbus"
	"github.com/gopcua/opcua/id"
	"github.com/gopcua/opcua/server"
	"github.com/gopcua/opcua/ua"
)

const (
	DigitalSize = 3000
	Reg16Size   = 1000
	Reg32Size   = 500
)

type ChannelConfig struct {
	Index     int     `json:"index"`
	Name      string  `json:"name"`
	Unit      string  `json:"unit"`
	MinRaw    int     `json:"min_raw"`
	MaxRaw    int     `json:"max_raw"`
	MinScaled float64 `json:"min_scaled"`
	MaxScaled float64 `json:"max_scaled"`
	Endian    string  `json:"endian"`
}

type ScaleConfig struct {
	StartIdx int             `json:"start_idx"`
	EndIdx   int             `json:"end_idx"`
	Channels []ChannelConfig `json:"channels"`
}

type Config struct {
	Scaled16 ScaleConfig `json:"Scaled16"`
	Scaled32 ScaleConfig `json:"Scaled32"`
}

type ChannelMapping struct {
	ModbusType string  `csv:"modbus_type"`
	ModbusAddr int     `csv:"modbus_addr"`
	OPCUATable string  `csv:"opcua_table"`
	OPCUAIdx   int     `csv:"opcua_idx"`
	Name       string  `csv:"name"`
	Unit       string  `csv:"unit"`
	MinRaw     int     `csv:"min_raw"`
	MaxRaw     int     `csv:"max_raw"`
	MinScaled  float64 `csv:"min_scaled"`
	MaxScaled  float64 `csv:"max_scaled"`
	Endian     string  `csv:"endian"`
	ModbusFC   int     `csv:"modbus_fc"`
}

var channelMappings []ChannelMapping

type ModbusDevice struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Address string `json:"address"`
	Port    string `json:"port"`
	Baud    int    `json:"baud"`
	Slave   int    `json:"slave"`
	Unit    int    `json:"unit"`
	Read    []struct {
		FC       int `json:"fc"`
		Addr     int `json:"addr"`
		Len      int `json:"len"`
		PeriodMs int `json:"period_ms"`
	} `json:"read"`
	Write []struct {
		FC     int `json:"fc"`
		Addr   int `json:"addr"`
		Len    int `json:"len"`
		Period int `json:"period"`
	} `json:"write"`
}

type ModbusClient struct {
	client    modbus.Client
	device    *ModbusDevice
	backoff   time.Duration
	lastError error
}

var (
	Digital  [DigitalSize]bool
	RawReg16 [Reg16Size]uint16
	Scaled16 [Reg16Size]float32
	RawReg32 [Reg16Size]uint16
	Scaled32 [Reg32Size]float32
)

var mu sync.RWMutex
var config Config

const webHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <title>OPC UA 监控</title>
    <style>
        body { font-family: sans-serif; margin: 20px; background: #f5f5f5; }
        h1 { text-align: center; color: #333; }
        table { width: 100%; border-collapse: collapse; margin: 20px 0; }
        th, td { padding: 8px; text-align: center; border: 1px solid #ddd; }
        th { background: #4CAF50; color: white; }
        tr:hover { background: #f5f5f5; }
        .status { padding: 10px; background: #e8f5e9; text-align: center; margin: 20px 0; font-weight: bold; }
    </style>
</head>
<body>
    <h1>OPC UA 监控系统</h1>
    <div class="status" id="status">状态: 等待数据...</div>
    <table id="mainTable"><thead><tr><th>索引</th><th>Scaled16</th><th>Scaled32</th><th>RawReg16</th><th>RawReg32</th><th>Digital</th></tr></thead><tbody></tbody></table>
    <script>
        let data = null;
        async function fetchData() {
            try {
                const response = await fetch('/api/data');
                data = await response.json();
                updateMainTable();
                document.getElementById('status').textContent = '状态: 最后更新 ' + new Date().toLocaleTimeString();
            } catch (error) {
                document.getElementById('status').textContent = '状态: 连接失败';
            }
        }
        function updateMainTable() {
            const tbody = document.querySelector('#mainTable tbody');
            tbody.innerHTML = '';
            const maxSize = Math.max(data.Scaled16.length, data.Scaled32.length, data.RawReg16.length, data.RawReg32.length, data.Digital.length);
            for (let i = 0; i < maxSize; i++) {
                const row = document.createElement('tr');
                row.innerHTML = '<td>' + i + '</td><td>' + (data.Scaled16[i] !== undefined ? parseFloat(data.Scaled16[i]).toFixed(4) : '-') + '</td><td>' + (data.Scaled32[i] !== undefined ? parseFloat(data.Scaled32[i]).toFixed(4) : '-') + '</td><td>' + (data.RawReg16[i] !== undefined ? data.RawReg16[i] : '-') + '</td><td>' + (data.RawReg32[i] !== undefined ? data.RawReg32[i] : '-') + '</td><td>' + (data.Digital[i] !== undefined ? (data.Digital[i] ? '✓' : '✗') : '-') + '</td>';
                tbody.appendChild(row);
            }
        }
        setInterval(fetchData, 1000);
        fetchData();
    </script>
</body>
</html>`

func loadConfig(filename string) error {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &config)
}

func findChannelConfig(idx int, scaleConfig *ScaleConfig) *ChannelConfig {
	for i := range scaleConfig.Channels {
		if scaleConfig.Channels[i].Index == idx {
			return &scaleConfig.Channels[i]
		}
	}
	return nil
}

func calcK(minRaw, maxRaw int, minScaled, maxScaled float64) float64 {
	if maxRaw == minRaw {
		return 0
	}
	return (maxScaled - minScaled) / float64(maxRaw-minRaw)
}

func calcB(k float64, minRaw int, minScaled float64) float64 {
	return minScaled - k*float64(minRaw)
}

func rawToScaled(raw int, k, b float64) float64 {
	return k*float64(raw) + b
}

func scaledToRaw(scaled float64, k, b float64) int {
	if k == 0 {
		return 0
	}
	return int((scaled - b) / k)
}

func runBackgroundTasks() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		mu.Lock()
		for i := 0; i < Reg16Size; i++ {
			channel := findChannelConfig(i, &config.Scaled16)
			if channel != nil {
				k := calcK(channel.MinRaw, channel.MaxRaw, channel.MinScaled, channel.MaxScaled)
				b := calcB(k, channel.MinRaw, channel.MinScaled)
				Scaled16[i] = float32(rawToScaled(int(RawReg16[i]), k, b))
			} else {
				Scaled16[i] = float32(float64(RawReg16[i]) / 327.67)
			}
		}

		for i := 0; i < Reg32Size; i++ {
			channel := findChannelConfig(i, &config.Scaled32)
			if channel != nil {
				high := RawReg32[i*2]
				low := RawReg32[i*2+1]
				var bits uint32
				if channel.Endian == "little" {
					bits = (uint32(low) << 16) | uint32(high)
				} else {
					bits = (uint32(high) << 16) | uint32(low)
				}
				raw := int32(bits)
				k := calcK(channel.MinRaw, channel.MaxRaw, channel.MinScaled, channel.MaxScaled)
				b := calcB(k, channel.MinRaw, channel.MinScaled)
				Scaled32[i] = float32(rawToScaled(int(raw), k, b))
			} else {
				high := RawReg32[i*2]
				low := RawReg32[i*2+1]
				bits := (uint32(high) << 16) | uint32(low)
				Scaled32[i] = float32(int32(bits)) / 21474.83647
			}
		}
		mu.Unlock()
	}

	readModbusData()
	writeModbusData()
}

func loadModbusConfig(filename string) error {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}

	var modbusConfig struct {
		Devices []ModbusDevice `json:"devices"`
	}
	if err := json.Unmarshal(data, &modbusConfig); err != nil {
		return err
	}

	for _, device := range modbusConfig.Devices {
		var handler modbus.ClientHandler
		var err error

		if device.Type == "tcp" {
			handler = modbus.NewTCPClientHandler(device.Address)
			if err = handler.(*modbus.TCPClientHandler).Connect(); err != nil {
				log.Printf("Modbus TCP device %s (%s) initial connection failed: %v - will retry on operations", device.Name, device.Address, err)
			}
		} else if device.Type == "rtu" {
			handler = modbus.NewRTUClientHandler(device.Port)
		} else {
			log.Printf("Unknown device type: %s", device.Type)
			continue
		}

		client := modbus.NewClient(handler)
		modbusClients = append(modbusClients, &ModbusClient{client: client, device: &device, backoff: 1 * time.Second})
		log.Printf("Modbus client initialized: %s (type: %s, address: %s, slave: %d)", device.Name, device.Type, device.Address, device.Slave)
	}

	return nil
}

var modbusClients []*ModbusClient

func initModbusClients() {
	log.Println("Initializing Modbus clients...")
}

func readModbusData() {
	mu.Lock()
	defer mu.Unlock()

	var wg sync.WaitGroup
	for _, client := range modbusClients {
		if client == nil || client.client == nil {
			continue
		}

		wg.Add(1)
		go func(client *ModbusClient) {
			defer wg.Done()

			for _, readConfig := range client.device.Read {
				var values []byte
				var err error
				retries := 3
				reconnectFailed := false

				for attempt := 0; attempt <= retries; attempt++ {
					if client.client == nil || client.device == nil {
						if err = reconnectClient(client); err != nil {
							client.lastError = err
							log.Printf("Modbus reconnect failed for %s (attempt %d/%d): %v", client.device.Name, attempt+1, retries+1, err)
							reconnectFailed = true
							time.Sleep(client.backoff)
							if client.backoff < 30*time.Second {
								client.backoff *= 2
							}
							continue
						}
						reconnectFailed = false
					}

					switch readConfig.FC {
					case 1:
						values, err = client.client.ReadCoils(uint16(readConfig.Addr), uint16(readConfig.Len))
					case 2:
						values, err = client.client.ReadDiscreteInputs(uint16(readConfig.Addr), uint16(readConfig.Len))
					case 3:
						values, err = client.client.ReadHoldingRegisters(uint16(readConfig.Addr), uint16(readConfig.Len))
					case 4:
						values, err = client.client.ReadInputRegisters(uint16(readConfig.Addr), uint16(readConfig.Len))
					case 15:
						values, err = client.client.WriteMultipleCoils(uint16(readConfig.Addr), uint16(readConfig.Len), make([]byte, (readConfig.Len+7)/8))
					case 16:
						values, err = client.client.WriteMultipleRegisters(uint16(readConfig.Addr), uint16(readConfig.Len), make([]byte, readConfig.Len*2))
					default:
						log.Printf("Unsupported function code: %d", readConfig.FC)
						break
					}

					if err != nil {
						client.lastError = err
						if reconnectFailed {
							log.Printf("Modbus reconnect failed for %s (attempt %d/%d): %v", client.device.Name, attempt+1, retries+1, err)
						} else {
							log.Printf("Modbus read error (FC%d @ %d, len=%d, attempt %d/%d): %v", readConfig.FC, readConfig.Addr, readConfig.Len, attempt+1, retries+1, err)
						}
						_ = closeClient(client)
						time.Sleep(20 * time.Millisecond)
						continue
					}
					reconnectFailed = false

					for i := 0; i < readConfig.Len; i++ {
						if readConfig.FC == 1 || readConfig.FC == 2 || readConfig.FC == 15 {
							if (readConfig.Addr + i) >= DigitalSize {
								continue
							}
							Digital[readConfig.Addr+i] = values[i%len(values)] != 0
						} else {
							if (readConfig.Addr + i) >= Reg16Size {
								continue
							}
							RawReg16[readConfig.Addr+i] = uint16(values[i*2])<<8 | uint16(values[i*2+1])
						}
					}
					client.backoff = 1 * time.Second
					break
				}
			}
		}(client)
	}
	wg.Wait()
}

func writeModbusData() {
	mu.Lock()
	defer mu.Unlock()

	var wg sync.WaitGroup
	for _, client := range modbusClients {
		if client == nil || client.client == nil {
			continue
		}

		wg.Add(1)
		go func(client *ModbusClient) {
			defer wg.Done()

			for _, writeConfig := range client.device.Write {
				var err error
				retries := 3
				reconnectFailed := false

				for attempt := 0; attempt <= retries; attempt++ {
					if client.client == nil || client.device == nil {
						if err = reconnectClient(client); err != nil {
							client.lastError = err
							log.Printf("Modbus reconnect failed for %s (attempt %d/%d): %v", client.device.Name, attempt+1, retries+1, err)
							reconnectFailed = true
							time.Sleep(client.backoff)
							if client.backoff < 30*time.Second {
								client.backoff *= 2
							}
							continue
						}
						reconnectFailed = false
					}

					switch writeConfig.FC {
					case 1:
						if writeConfig.Addr >= DigitalSize {
							continue
						}
						var value uint16 = 0
						if Digital[writeConfig.Addr] {
							value = 0xFF00
						}
						_, err = client.client.WriteSingleCoil(uint16(writeConfig.Addr), value)
					case 5:
						if writeConfig.Addr >= DigitalSize {
							continue
						}
						var value uint16 = 0
						if Digital[writeConfig.Addr] {
							value = 0xFF00
						}
						_, err = client.client.WriteSingleCoil(uint16(writeConfig.Addr), value)
					case 6:
						if writeConfig.Addr >= Reg16Size {
							continue
						}
						value := RawReg16[writeConfig.Addr]
						_, err = client.client.WriteSingleRegister(uint16(writeConfig.Addr), value)
					case 15:
						if writeConfig.Addr >= DigitalSize {
							continue
						}
						values := make([]bool, writeConfig.Len)
						for i := 0; i < writeConfig.Len; i++ {
							if (writeConfig.Addr + i) < DigitalSize {
								values[i] = Digital[writeConfig.Addr+i]
							}
						}
						_, err = client.client.WriteMultipleCoils(uint16(writeConfig.Addr), uint16(writeConfig.Len), make([]byte, (writeConfig.Len+7)/8))
					case 16:
						if writeConfig.Addr >= Reg16Size {
							continue
						}
						values := make([]uint16, writeConfig.Len)
						for i := 0; i < writeConfig.Len; i++ {
							if (writeConfig.Addr + i) < Reg16Size {
								values[i] = RawReg16[writeConfig.Addr+i]
							}
						}
						_, err = client.client.WriteMultipleRegisters(uint16(writeConfig.Addr), uint16(writeConfig.Len), make([]byte, writeConfig.Len*2))
					default:
						log.Printf("Unsupported function code: %d", writeConfig.FC)
						break
					}

					if err != nil {
						client.lastError = err
						if reconnectFailed {
							log.Printf("Modbus reconnect failed for %s (attempt %d/%d): %v", client.device.Name, attempt+1, retries+1, err)
						} else {
							log.Printf("Modbus write error (addr %d, attempt %d/%d): %v", writeConfig.Addr, attempt+1, retries+1, err)
						}
						_ = closeClient(client)
						time.Sleep(20 * time.Millisecond)
						continue
					}
					reconnectFailed = false
					client.backoff = 1 * time.Second
					break
				}
			}
		}(client)
	}
	wg.Wait()
}

func reconnectClient(client *ModbusClient) error {
	if client.device.Type == "tcp" {
		handler := modbus.NewTCPClientHandler(client.device.Address)
		handler.Timeout = 500 * time.Millisecond
		handler.SlaveId = byte(client.device.Slave)
		if err := handler.Connect(); err != nil {
			return err
		}
		client.client = modbus.NewClient(handler)
		client.backoff = 1 * time.Second
		return nil
	} else if client.device.Type == "rtu" {
		handler := modbus.NewRTUClientHandler(client.device.Port)
		handler.BaudRate = client.device.Baud
		handler.DataBits = 8
		handler.Parity = "N"
		handler.StopBits = 1
		handler.Timeout = 500 * time.Millisecond
		handler.SlaveId = byte(client.device.Slave)
		if err := handler.Connect(); err != nil {
			return err
		}
		client.client = modbus.NewClient(handler)
		client.backoff = 1 * time.Second
		return nil
	}
	return fmt.Errorf("unsupported device type: %s", client.device.Type)
}

func closeClient(client *ModbusClient) error {
	if client.client != nil {
		client.client = nil
	}
	return nil
}

func runOPCUAServer() {
	opts := []server.Option{
		server.EnableSecurity("Basic256Sha256", ua.MessageSecurityModeSign),
		server.EnableAuthMode(ua.UserTokenTypeAnonymous),
		server.EndPoint("0.0.0.0", 4840),
	}

	s := server.New(opts...)

	mapNS := server.NewMapNamespace(s, "GlobalTables")
	log.Printf("Map Namespace added at index %d", mapNS.ID())

	rootNs, _ := s.Namespace(0)
	rootObjNode := rootNs.Objects()
	rootObjNode.AddRef(mapNS.Objects(), id.HasComponent, true)

	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			mu.Lock()
			for i := 0; i < DigitalSize; i++ {
				mapNS.Mu.Lock()
				mapNS.Data[fmt.Sprintf("Digital_%04d", i)] = Digital[i]
				mapNS.Mu.Unlock()
			}

			for i := 0; i < Reg16Size; i++ {
				Scaled16[i] = float32(float64(RawReg16[i]) / 327.67)
				mapNS.Mu.Lock()
				mapNS.Data[fmt.Sprintf("Scaled16_%04d", i)] = Scaled16[i]
				mapNS.Mu.Unlock()
			}

			for i := 0; i < Reg32Size; i++ {
				high := RawReg32[i*2]
				low := RawReg32[i*2+1]
				bits := (uint32(high) << 16) | uint32(low)
				Scaled32[i] = float32(int32(bits)) / 21474.83647
				mapNS.Mu.Lock()
				mapNS.Data[fmt.Sprintf("Scaled32_%04d", i)] = Scaled32[i]
				mapNS.Mu.Unlock()
			}
			mu.Unlock()
			mapNS.ChangeNotification("Digital_0000")
		}
	}()

	go func() {
		for {
			key := <-mapNS.ExternalNotification
			log.Printf("Node %s changed", key)
			parts := strings.Split(key, "_")
			if len(parts) < 2 {
				continue
			}

			prefix := parts[0]
			idx, err := strconv.Atoi(parts[1])
			if err != nil {
				continue
			}

			value := mapNS.GetValue(key)
			switch prefix {
			case "Digital":
				if b, ok := value.(bool); ok && idx >= 2000 && idx < DigitalSize {
					mu.Lock()
					Digital[idx] = b
					mu.Unlock()
					fmt.Printf("[OPC UA Write] Digital[%d] = %v\n", idx, b)
				}
			case "Scaled16":
				if f, ok := value.(float64); ok && idx >= config.Scaled16.StartIdx && idx < config.Scaled16.EndIdx {
					mu.Lock()
					channel := findChannelConfig(idx, &config.Scaled16)
					if channel != nil {
						k := calcK(channel.MinRaw, channel.MaxRaw, channel.MinScaled, channel.MaxScaled)
						b := calcB(k, channel.MinRaw, channel.MinScaled)
						rawVal := scaledToRaw(f, k, b)
						RawReg16[idx] = uint16(rawVal)
						Scaled16[idx] = float32(f)
					} else {
						RawReg16[idx] = uint16(f * 327.67)
						Scaled16[idx] = float32(f)
					}
					mu.Unlock()
					fmt.Printf("[OPC UA Write] Scaled16[%d] = %.2f (Raw: %d)\n", idx, f, RawReg16[idx])
				}
			case "Scaled32":
				if f, ok := value.(float64); ok && idx >= config.Scaled32.StartIdx && idx < config.Scaled32.EndIdx {
					mu.Lock()
					channel := findChannelConfig(idx, &config.Scaled32)
					if channel != nil {
						k := calcK(channel.MinRaw, channel.MaxRaw, channel.MinScaled, channel.MaxScaled)
						b := calcB(k, channel.MinRaw, channel.MinScaled)
						rawVal := scaledToRaw(f, k, b)
						bits := uint32(rawVal)
						high := uint16(bits >> 16)
						low := uint16(bits & 0xFFFF)
						if channel.Endian == "little" {
							RawReg32[idx*2] = low
							RawReg32[idx*2+1] = high
						} else {
							RawReg32[idx*2] = high
							RawReg32[idx*2+1] = low
						}
						Scaled32[idx] = float32(f)
					} else {
						bits := uint32(f * 21474.83647)
						RawReg32[idx*2] = uint16(bits >> 16)
						RawReg32[idx*2+1] = uint16(bits & 0xFFFF)
						Scaled32[idx] = float32(f)
					}
					mu.Unlock()
					fmt.Printf("[OPC UA Write] Scaled32[%d] = %.2f (Raw: %08X)\n", idx, f, uint32(RawReg32[idx*2])<<16|uint32(RawReg32[idx*2+1]))
				}
			}
		}
	}()

	log.Printf("🚀 OPC UA Server starting on opc.tcp://0.0.0.0:4840")
	if err := s.Start(context.Background()); err != nil {
		log.Fatal(err)
	}
}

func runWebServer() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(webHTML))
		} else {
			http.ServeFile(w, r, "."+r.URL.Path)
		}
	})

	http.HandleFunc("/api/data", func(w http.ResponseWriter, r *http.Request) {
		mu.RLock()
		data := make(map[string]interface{})
		data["Digital"] = Digital[:]
		data["RawReg16"] = make([]string, len(RawReg16))
		data["Scaled16"] = Scaled16[:]
		data["RawReg32"] = make([]string, len(RawReg32))
		data["Scaled32"] = Scaled32[:]

		for i := range RawReg16 {
			data["RawReg16"].([]string)[i] = fmt.Sprintf("0x%04X", RawReg16[i])
		}
		for i := range RawReg32 {
			data["RawReg32"].([]string)[i] = fmt.Sprintf("0x%04X", RawReg32[i])
		}
		mu.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(data)
	})

	http.HandleFunc("/api/write", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Type  string  `json:"type"`
			Index int     `json:"index"`
			Value float64 `json:"value"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		mu.Lock()
		var result string

		switch req.Type {
		case "Digital":
			if req.Index >= 2000 && req.Index < DigitalSize {
				Digital[req.Index] = req.Value != 0
				result = fmt.Sprintf("Digital[%d] = %v", req.Index, Digital[req.Index])
			} else {
				http.Error(w, "Digital index out of range", http.StatusBadRequest)
				return
			}
		case "Scaled16":
			if req.Index >= config.Scaled16.StartIdx && req.Index < config.Scaled16.EndIdx {
				channel := findChannelConfig(req.Index, &config.Scaled16)
				if channel != nil {
					k := calcK(channel.MinRaw, channel.MaxRaw, channel.MinScaled, channel.MaxScaled)
					b := calcB(k, channel.MinRaw, channel.MinScaled)
					rawVal := scaledToRaw(req.Value, k, b)
					RawReg16[req.Index] = uint16(rawVal)
					Scaled16[req.Index] = float32(req.Value)
					result = fmt.Sprintf("Scaled16[%d] = %.2f (Raw: 0x%04X)", req.Index, req.Value, RawReg16[req.Index])
				} else {
					RawReg16[req.Index] = uint16(req.Value * 327.67)
					Scaled16[req.Index] = float32(req.Value)
					result = fmt.Sprintf("Scaled16[%d] = %.2f (Raw: 0x%04X)", req.Index, req.Value, RawReg16[req.Index])
				}
			} else {
				http.Error(w, "Scaled16 index out of range", http.StatusBadRequest)
				return
			}
		case "Scaled32":
			if req.Index >= config.Scaled32.StartIdx && req.Index < config.Scaled32.EndIdx {
				channel := findChannelConfig(req.Index, &config.Scaled32)
				if channel != nil {
					k := calcK(channel.MinRaw, channel.MaxRaw, channel.MinScaled, channel.MaxScaled)
					b := calcB(k, channel.MinRaw, channel.MinScaled)
					rawVal := scaledToRaw(req.Value, k, b)
					bits := uint32(rawVal)
					high := uint16(bits >> 16)
					low := uint16(bits & 0xFFFF)
					if channel.Endian == "little" {
						RawReg32[req.Index*2] = low
						RawReg32[req.Index*2+1] = high
					} else {
						RawReg32[req.Index*2] = high
						RawReg32[req.Index*2+1] = low
					}
					Scaled32[req.Index] = float32(req.Value)
					result = fmt.Sprintf("Scaled32[%d] = %.2f (Raw: 0x%08X)", req.Index, req.Value, uint32(RawReg32[req.Index*2])<<16|uint32(RawReg32[req.Index*2+1]))
				} else {
					bits := uint32(req.Value * 21474.83647)
					RawReg32[req.Index*2] = uint16(bits >> 16)
					RawReg32[req.Index*2+1] = uint16(bits & 0xFFFF)
					Scaled32[req.Index] = float32(req.Value)
					result = fmt.Sprintf("Scaled32[%d] = %.2f (Raw: 0x%08X)", req.Index, req.Value, uint32(RawReg32[req.Index*2])<<16|uint32(RawReg32[req.Index*2+1]))
				}
			} else {
				http.Error(w, "Scaled32 index out of range", http.StatusBadRequest)
				return
			}
		default:
			http.Error(w, "Invalid type", http.StatusBadRequest)
			return
		}
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"result": result})
	})

	log.Println("🌐 Web Server starting on http://127.0.0.1:8080/")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}

func main() {
	logFile, err := os.OpenFile("opcua-gateway.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	defer logFile.Close()
	log.SetOutput(logFile)

	if err := loadConfig("channel_config.json"); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if err := loadModbusConfig("modbus_config.json"); err != nil {
		log.Printf("Failed to load modbus config: %v", err)
	}

	go runBackgroundTasks()
	go runOPCUAServer()
	go runWebServer()

	sigch := make(chan os.Signal, 1)
	signal.Notify(sigch, os.Interrupt)
	<-sigch
	log.Println("Shutting down...")
}
