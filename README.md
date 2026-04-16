# OPC UA Modbus Gateway

[English](#english) | [中文](#中文)

## 中文

一个高性能的 OPC UA 与 Modbus 通信网关，用于连接 OPC UA 客户端与多个 Modbus TCP/RTU 从站设备。本项目旨在快速搭建工业自动化领域的 OPC UA 服务器，实现 Modbus 设备数据的统一访问和管理。

### 功能特性

- ✅ **OPC UA 服务器**：支持 OPC UA 协议标准，可通过 OPC UA 客户端（如 UA Expert）连接
- ✅ **多 Modbus 从站支持**：同时连接多个 Modbus TCP/RTU 从站设备
- ✅ **六种数据表**：RawReg16、RawReg32、Digital、Scaled16、Scaled32、Digital
- ✅ **量程标定**：支持线性量程转换（raw ↔ scaled），方便工程单位转换
- ✅ **双向数据同步**：OPC UA 写入操作可同步到 Modbus 从站
- ✅ **自动重连**：Modbus 连接断线自动重连，支持指数退避策略
- ✅ **Web 监控界面**：提供实时数据监控和调试界面
- ✅ **CSV 配置生成**：通过 CSV 文件快速生成配置，简化工程部署

### 技术架构

```
┌─────────────────┐      OPC UA      ┌──────────────────┐
│  OPC UA Client  │◄────────────────►│  OPC UA Server   │
│ (UA Expert, etc)│      协议         │   (本项目)        │
└─────────────────┘                   └────────┬─────────┘
                                               │
                                               │ 100ms 周期
                                               │
┌─────────────────┐     Modbus         ┌──────▼─────────┐
│  Modbus TCP/RTU │◄───────────────────│   Modbus 客户端  │
│   从站设备1-N   │      协议           │  (Go modbus 库) │
└─────────────────┘                    └──────────────────┘
```

### 数据模型

本项目定义了六种数据表，分为原始数据和缩放数据两类：

#### 原始数据表（Raw Registers）

| 数据表 | 类型 | 大小 | 说明 |
|--------|------|------|------|
| **RawReg16** | uint16 | 1000 | 16位原始寄存器，直接映射 Modbus 寄存器 |
| **RawReg32** | uint16 | 1000 | 32位原始寄存器（每32位占用2个16位寄存器） |
| **Digital** | bool | 3000 | 数字量（开关量），支持读写 |

#### 缩放数据表（Scaled Data）

| 数据表 | 类型 | 大小 | 说明 |
|--------|------|------|------|
| **Scaled16** | float32 | 1000 | 16位缩放数据，通过量程转换从 RawReg16 计算 |
| **Scaled32** | float32 | 500 | 32位缩放数据，通过量程转换从 RawReg32 计算 |

**数据流关系：**

```
Modbus 从站寄存器
       ↓ (读取)
    RawReg16 / RawReg32 (原始数据)
       ↓ (量程转换)
    Scaled16 / Scaled32 (工程单位)
       ↓ (OPC UA 映射)
    OPC UA 节点 (GlobalTables 命名空间)
```

**数据访问方式：**

- **RawReg16/RawReg32**: Modbus 通信使用，存储原始寄存器值
- **Scaled16/Scaled32**: OPC UA 访问使用，存储工程单位值
- **Digital**: OPC UA 和 Modbus 通信均使用

### 快速开始

项目采用**三步配置流程**：修改 CSV → 生成配置 → 运行程序

#### 1. 环境要求

- Go 1.18+
- Python 3.6+（用于配置转换）

#### 2. 第一步：配置 CSV 文件

编辑 [channel_mapping.csv](file:///home/yuan/文档/Go_Project/opcua-modbus-gateway/channel_mapping.csv)，定义 Modbus 通道与 OPC UA 节点的映射关系：

```csv
modbus_type,modbus_addr,opcua_table,opcua_idx,name,unit,min_raw,max_raw,min_scaled,max_scaled,endian,modbus_fc,slave_id,ip_address,len
AI,1,Scaled16,500,Temperature_T1,℃,0,32767,-100.0,200.0,big,4,1,192.168.2.12,1
AI,2,Scaled16,501,Flow_F1,m³/h,0,32767,0.0,500.0,big,4,1,192.168.2.12,1
AI,3,Scaled32,0,Level_L1,m,0,100000,0.0,100.0,big,4,1,192.168.2.12,1
AO,500,Scaled16,999,Valve_V1,%,0,32767,0.0,100.0,big,6,1,192.168.2.12,1
DI,0,Digital,0,Pressure_P1,MPa,0,1,0.0,1.0,,3,1,192.168.2.12,1
```

**字段说明**：

| 字段 | 说明 | 示例 |
|------|------|------|
| modbus_type | DI (数字输入), AI (模拟输入), DO (数字输出), AO (模拟输出) | AI |
| modbus_addr | Modbus 寄存器地址 | 1 |
| opcua_table | Digital/Scaled16/Scaled32 (或 RawReg16/RawReg32) | Scaled16 |
| opcua_idx | OPC UA 数据索引 | 500 |
| name | 通道名称 | Temperature_T1 |
| unit | 单位 | ℃ |
| min_raw | 原始值最小值 | 0 |
| max_raw | 原始值最大值 | 32767 |
| min_scaled | 量程最小值 | -100.0 |
| max_scaled | 量程最大值 | 200.0 |
| endian | 字节序（big/little） | big |
| modbus_fc | Modbus 功能码（3/4=读,5/6=写） | 4 |
| slave_id | Modbus 从站地址 | 1 |
| ip_address | Modbus 设备 IP 地址 | 192.168.2.12 |
| len | 读取长度 | 1 |

**工程配置建议**：
- 根据实际 Modbus 设备的寄存器地址修改 `modbus_addr`
- 根据设备量程设置 `min_raw`、`max_raw`、`min_scaled`、`max_scaled`
- 根据设备类型设置 `modbus_type` 和 `modbus_fc`
- 根据实际设备 IP 地址修改 `ip_address`

#### 3. 第二步：生成配置文件

运行 [csv_to_config.py](file:///home/yuan/文档/Go_Project/opcua-modbus-gateway/csv_to_config.py) 脚本，自动生成两个 JSON 配置文件：

```bash
cd /home/yuan/文档/Go_Project/opcua-modbus-gateway

# 生成配置文件
python3 csv_to_config.py channel_mapping.csv
```

执行后会生成：
- `channel_config.json` - OPC UA 通道配置（包含 Scaled16/Scaled32 量程参数）
- `modbus_config.json` - Modbus 设备配置（包含设备地址、读写参数）

**生成的配置示例**：

`channel_config.json`:
```json
{
  "Scaled16": {
    "start_idx": 500,
    "end_idx": 999,
    "channels": [
      {
        "index": 500,
        "name": "Temperature_T1",
        "unit": "℃",
        "min_raw": 0,
        "max_raw": 32767,
        "min_scaled": -100.0,
        "max_scaled": 200.0,
        "endian": "big"
      }
    ]
  },
  "Scaled32": {
    "start_idx": 250,
    "end_idx": 499,
    "channels": [...]
  }
}
```

`modbus_config.json`:
```json
{
  "devices": [
    {
      "name": "Device_1",
      "type": "tcp",
      "address": "192.168.2.12:502",
      "slave": 1,
      "unit": 1,
      "read": [
        {
          "fc": 4,
          "addr": 1,
          "len": 3,
          "period_ms": 1000
        }
      ],
      "write": [
        {
          "fc": 6,
          "addr": 500,
          "len": 1,
          "period": 1000
        }
      ]
    }
  ]
}
```

#### 4. 第三步：运行程序

编译并运行 Go 程序：

```bash
# 安装依赖
go mod download

# 编译
go build -o opcua-gateway

# 运行
./opcua-gateway
```

**启动日志示例**：
```
2024/01/01 10:00:00 Initializing Modbus clients...
2024/01/01 10:00:00 Modbus client initialized: Device_1 (type: tcp, address: 192.168.2.12:502, slave: 1)
2024/01/01 10:00:00 🚀 OPC UA Server starting on opc.tcp://0.0.0.0:4840
```

### CSV 格式说明

| 字段 | 说明 | 示例 |
|------|------|------|
| modbus_type | DI (数字输入), AI (模拟输入), DO (数字输出), AO (模拟输出) | AI |
| modbus_addr | Modbus 寄存器地址 | 1 |
| opcua_table | Digital/Scaled16/Scaled32 (或 RawReg16/RawReg32) | Scaled16 |
| opcua_idx | OPC UA 数据索引 | 500 |
| name | 通道名称 | Temperature_T1 |
| unit | 单位 | ℃ |
| min_raw | 原始值最小值 | 0 |
| max_raw | 原始值最大值 | 32767 |
| min_scaled | 量程最小值 | -100.0 |
| max_scaled | 量程最大值 | 200.0 |
| endian | 字节序（big/little） | big |
| modbus_fc | Modbus 功能码（3/4=读,5/6=写） | 4 |
| slave_id | Modbus 从站地址 | 1 |
| ip_address | Modbus 设备 IP 地址 | 192.168.2.12 |
| len | 读取长度 | 1 |

**完整配置流程**：
1. 编辑 [channel_mapping.csv](file:///home/yuan/文档/Go_Project/opcua-modbus-gateway/channel_mapping.csv) 定义通道映射
2. 运行 `python3 csv_to_config.py channel_mapping.csv` 生成配置文件
3. 编译运行 `go build -o opcua-gateway && ./opcua-gateway`

### 运行效果

#### 1. 启动日志
```
2024/01/01 10:00:00 Initializing Modbus clients...
2024/01/01 10:00:00 Modbus client initialized: Device_1 (type: tcp, address: 192.168.1.100:502, slave: 1)
2024/01/01 10:00:00 🚀 OPC UA Server starting on opc.tcp://0.0.0.0:4840
```

#### 2. Web 监控界面
访问 `http://localhost:8080` 查看实时数据监控界面。

#### 3. OPC UA 客户端连接
使用 UA Expert 或其他 OPC UA 客户端连接：
- **Endpoint**: `opc.tcp://localhost:4840`
- **Security**: Basic256Sha256
- **Mode**: Sign

连接后可浏览命名空间 "GlobalTables" 下的所有节点。

### 量程转换公式

本项目使用线性量程转换：

```
scaled = k * raw + b
```

其中：
- `k = (max_scaled - min_scaled) / (max_raw - min_raw)`
- `b = min_scaled - k * min_raw`

示例：温度量程 -100℃ ~ 200℃，对应原始值 0 ~ 32767
```
k = (200 - (-100)) / (32767 - 0) = 300 / 32767 ≈ 0.00915
b = -100 - 0.00915 * 0 = -100
```

### 项目结构

```
opcua-gateway/
├── main.go                 # 主程序入口
├── csv_to_config.py        # CSV 配置生成脚本
├── channel_mapping.csv     # 通道映射配置（工程配置入口）
├── channel_config.json     # OPC UA 通道配置（运行时生成）
├── modbus_config.json      # Modbus 设备配置（运行时生成）
├── go.mod                  # Go 依赖
└── README.md               # 项目说明
```

**配置流程**：
1. **工程配置**：编辑 [channel_mapping.csv](file:///home/yuan/文档/Go_Project/opcua-modbus-gateway/channel_mapping.csv) 定义 Modbus 通道与 OPC UA 节点的映射关系
2. **生成配置**：运行 `python3 csv_to_config.py channel_mapping.csv` 生成两个 JSON 配置文件
3. **运行程序**：编译并运行 `main.go` 启动 OPC UA 服务器

**数据流向**：Modbus 从站 → RawReg16/RawReg32 → Scaled16/Scaled32 → OPC UA 节点

### 依赖库

- [gopcua/opcua](https://github.com/gopcua/opcua) - OPC UA 协议实现
- [goburrow/modbus](https://github.com/goburrow/modbus) - Modbus 协议实现

### 常见问题

**Q: 如何添加新的 Modbus 从站？**  
A: 在 `modbus_config.json` 的 `devices` 数组中添加新设备配置即可。

**Q: 如何修改量程参数？**  
A: 编辑 `channel_config.json` 中对应通道的 `min_raw`、`max_raw`、`min_scaled`、`max_scaled` 参数。

**Q: 支持哪些 Modbus 功能码？**  
A: 支持 FC1（读线圈）、FC2（读离散输入）、FC3（读保持寄存器）、FC4（读输入寄存器）、FC5（写单个线圈）、FC6（写单个寄存器）、FC15（写多个线圈）、FC16（写多个寄存器）。数据通过 RawReg16/RawReg32 存储原始值，通过 Scaled16/Scaled32 提供工程单位值。

**Q: 数据更新频率是多少？**  
A: 数据读取和写入周期为 100ms，Web 界面刷新频率为 1000ms。

### 开发计划

- [ ] 支持更多 Modbus 功能码
- [ ] 添加日志记录功能
- [ ] 支持 SSL/TLS 加密
- [ ] 添加用户认证
- [ ] 支持 OPC UA 数据历史记录

### 许可证

MIT License

### 贡献

欢迎提交 Issue 和 Pull Request！

---

## English

A high-performance OPC UA and Modbus communication gateway that bridges OPC UA clients with multiple Modbus TCP/RTU slave devices. This project enables rapid deployment of OPC UA servers in industrial automation scenarios, providing unified access and management of Modbus device data.

### Features

- ✅ **OPC UA Server**: Standard OPC UA protocol support, connect via OPC UA clients (e.g., UA Expert)
- ✅ **Multi-Modbus Slave Support**: Connect multiple Modbus TCP/RTU slave devices simultaneously
- ✅ **Six Data Tables**: RawReg16, RawReg32, Digital, Scaled16, Scaled32, Digital
- ✅ **Scale Calibration**: Linear scale conversion (raw ↔ scaled) for engineering unit conversion
- ✅ **Bidirectional Sync**: OPC UA write operations sync to Modbus slaves
- ✅ **Auto-Reconnect**: Automatic reconnection with exponential backoff strategy
- ✅ **Web Monitoring**: Real-time data monitoring and debugging interface
- ✅ **CSV Configuration**: Quick configuration generation from CSV files

### Architecture

```
┌─────────────────┐      OPC UA      ┌──────────────────┐
│  OPC UA Client  │◄────────────────►│  OPC UA Server   │
│ (UA Expert, etc)│      Protocol    │   (This Project) │
└─────────────────┘                   └────────┬─────────┘
                                               │
                                               │ 100ms cycle
                                               │
┌─────────────────┐     Modbus         ┌──────▼─────────┐
│  Modbus TCP/RTU │◄───────────────────│   Modbus Client  │
│   Slave Device  │      Protocol      │  (Go modbus lib) │
└─────────────────┘                    └──────────────────┘
```

### Data Model

Six data tables defined, categorized into raw registers and scaled data:

#### Raw Registers

| Table | Type | Size | Description |
|-------|------|------|-------------|
| **RawReg16** | uint16 | 1000 | 16-bit raw registers, direct mapping of Modbus registers |
| **RawReg32** | uint16 | 1000 | 32-bit raw registers (each 32-bit value occupies 2 16-bit registers) |
| **Digital** | bool | 3000 | Digital/Discrete signals, read/write supported |

#### Scaled Data

| Table | Type | Size | Description |
|-------|------|------|-------------|
| **Scaled16** | float32 | 1000 | 16-bit scaled data, calculated from RawReg16 via scale conversion |
| **Scaled32** | float32 | 500 | 32-bit scaled data, calculated from RawReg32 via scale conversion |

**Data Flow:**

```
Modbus Slave Registers
       ↓ (read)
    RawReg16 / RawReg32 (raw data)
       ↓ (scale conversion)
    Scaled16 / Scaled32 (engineering units)
       ↓ (OPC UA mapping)
    OPC UA Nodes (GlobalTables namespace)
```

**Data Access:**

- **RawReg16/RawReg32**: Used for Modbus communication, stores raw register values
- **Scaled16/Scaled32**: Used for OPC UA access, stores engineering unit values
- **Digital**: Used for both OPC UA and Modbus communication

### Quick Start

The project uses a **three-step configuration workflow**: Modify CSV → Generate Config → Run Program

#### 1. Requirements

- Go 1.18+
- Python 3.6+ (for config conversion)

#### 2. Step 1: Configure CSV File

Edit [channel_mapping.csv](file:///home/yuan/文档/Go_Project/opcua-modbus-gateway/channel_mapping.csv) to define the mapping between Modbus channels and OPC UA nodes:

```csv
modbus_type,modbus_addr,opcua_table,opcua_idx,name,unit,min_raw,max_raw,min_scaled,max_scaled,endian,modbus_fc,slave_id,ip_address,len
AI,1,Scaled16,500,Temperature_T1,℃,0,32767,-100.0,200.0,big,4,1,192.168.2.12,1
AI,2,Scaled16,501,Flow_F1,m³/h,0,32767,0.0,500.0,big,4,1,192.168.2.12,1
AI,3,Scaled32,0,Level_L1,m,0,100000,0.0,100.0,big,4,1,192.168.2.12,1
AO,500,Scaled16,999,Valve_V1,%,0,32767,0.0,100.0,big,6,1,192.168.2.12,1
DI,0,Digital,0,Pressure_P1,MPa,0,1,0.0,1.0,,3,1,192.168.2.12,1
```

**Field Description**:

| Field | Description | Example |
|-------|-------------|---------|
| modbus_type | DI (Digital Input), AI (Analog Input), DO (Digital Output), AO (Analog Output) | AI |
| modbus_addr | Modbus register address | 1 |
| opcua_table | Digital/Scaled16/Scaled32 (or RawReg16/RawReg32) | Scaled16 |
| opcua_idx | OPC UA data index | 500 |
| name | Channel name | Temperature_T1 |
| unit | Unit | ℃ |
| min_raw | Minimum raw value | 0 |
| max_raw | Maximum raw value | 32767 |
| min_scaled | Minimum scaled value | -100.0 |
| max_scaled | Maximum scaled value | 200.0 |
| endian | Byte order (big/little) | big |
| modbus_fc | Modbus function code (3/4=read, 5/6=write) | 4 |
| slave_id | Modbus slave address | 1 |
| ip_address | Modbus device IP address | 192.168.2.12 |
| len | Read length | 1 |

**Engineering Configuration Tips**:
- Modify `modbus_addr` according to actual Modbus device register addresses
- Set `min_raw`, `max_raw`, `min_scaled`, `max_scaled` according to device range
- Set `modbus_type` and `modbus_fc` according to device type
- Modify `ip_address` according to actual device IP address

#### 3. Step 2: Generate Configuration Files

Run [csv_to_config.py](file:///home/yuan/文档/Go_Project/opcua-modbus-gateway/csv_to_config.py) to automatically generate two JSON configuration files:

```bash
cd /home/yuan/文档/Go_Project/opcua-modbus-gateway

# Generate config files
python3 csv_to_config.py channel_mapping.csv
```

After execution, two files will be generated:
- `channel_config.json` - OPC UA channel configuration (includes Scaled16/Scaled32 range parameters)
- `modbus_config.json` - Modbus device configuration (includes device addresses and read/write parameters)

**Generated Configuration Example**:

`channel_config.json`:
```json
{
  "Scaled16": {
    "start_idx": 500,
    "end_idx": 999,
    "channels": [
      {
        "index": 500,
        "name": "Temperature_T1",
        "unit": "℃",
        "min_raw": 0,
        "max_raw": 32767,
        "min_scaled": -100.0,
        "max_scaled": 200.0,
        "endian": "big"
      }
    ]
  },
  "Scaled32": {
    "start_idx": 250,
    "end_idx": 499,
    "channels": [...]
  }
}
```

`modbus_config.json`:
```json
{
  "devices": [
    {
      "name": "Device_1",
      "type": "tcp",
      "address": "192.168.2.12:502",
      "slave": 1,
      "unit": 1,
      "read": [
        {
          "fc": 4,
          "addr": 1,
          "len": 3,
          "period_ms": 1000
        }
      ],
      "write": [
        {
          "fc": 6,
          "addr": 500,
          "len": 1,
          "period": 1000
        }
      ]
    }
  ]
}
```

#### 4. Step 3: Run Program

Build and run the Go program:

```bash
# Install dependencies
go mod download

# Build
go build -o opcua-gateway

# Run
./opcua-gateway
```

**Startup Log Example**:
```
2024/01/01 10:00:00 Initializing Modbus clients...
2024/01/01 10:00:00 Modbus client initialized: Device_1 (type: tcp, address: 192.168.2.12:502, slave: 1)
2024/01/01 10:00:00 🚀 OPC UA Server starting on opc.tcp://0.0.0.0:4840
```

### CSV Format

| Field | Description | Example |
|-------|-------------|---------|
| modbus_type | DI (Digital Input), AI (Analog Input), DO (Digital Output), AO (Analog Output) | AI |
| modbus_addr | Modbus register address | 1 |
| opcua_table | Digital/Scaled16/Scaled32 (or RawReg16/RawReg32) | Scaled16 |
| opcua_idx | OPC UA data index | 500 |
| name | Channel name | Temperature_T1 |
| unit | Unit | ℃ |
| min_raw | Minimum raw value | 0 |
| max_raw | Maximum raw value | 32767 |
| min_scaled | Minimum scaled value | -100.0 |
| max_scaled | Maximum scaled value | 200.0 |
| endian | Byte order (big/little) | big |
| modbus_fc | Modbus function code (3/4=read, 5/6=write) | 4 |
| slave_id | Modbus slave address | 1 |
| ip_address | Modbus device IP address | 192.168.2.12 |
| len | Read length | 1 |

**Complete Configuration Workflow**:
1. Edit [channel_mapping.csv](file:///home/yuan/文档/Go_Project/opcua-modbus-gateway/channel_mapping.csv) to define channel mappings
2. Run `python3 csv_to_config.py channel_mapping.csv` to generate configuration files
3. Build and run `go build -o opcua-gateway && ./opcua-gateway`

### Usage

#### 1. Startup Log
```
2024/01/01 10:00:00 Initializing Modbus clients...
2024/01/01 10:00:00 Modbus client initialized: Device_1 (type: tcp, address: 192.168.1.100:502, slave: 1)
2024/01/01 10:00:00 🚀 OPC UA Server starting on opc.tcp://0.0.0.0:4840
```

#### 2. Web Monitoring
Access `http://localhost:8080` for real-time data monitoring.

#### 3. OPC UA Client Connection
Connect using UA Expert or other OPC UA client:
- **Endpoint**: `opc.tcp://localhost:4840`
- **Security**: Basic256Sha256
- **Mode**: Sign

Browse nodes under "GlobalTables" namespace.

### Scale Conversion Formula

Linear scale conversion is used:

```
scaled = k * raw + b
```

Where:
- `k = (max_scaled - min_scaled) / (max_raw - min_raw)`
- `b = min_scaled - k * min_raw`

Example: Temperature range -100°C to 200°C, raw value 0 to 32767
```
k = (200 - (-100)) / (32767 - 0) = 300 / 32767 ≈ 0.00915
b = -100 - 0.00915 * 0 = -100
```

### Project Structure

```
opcua-gateway/
├── main.go                 # Main entry point
├── csv_to_config.py        # CSV config generator
├── channel_mapping.csv     # Channel mapping config (engineering entry point)
├── channel_config.json     # OPC UA channel config (generated at runtime)
├── modbus_config.json      # Modbus device config (generated at runtime)
├── go.mod                  # Go dependencies
└── README.md               # This file
```

**Configuration Workflow**:
1. **Engineering Configuration**: Edit [channel_mapping.csv](file:///home/yuan/文档/Go_Project/opcua-modbus-gateway/channel_mapping.csv) to define mappings between Modbus channels and OPC UA nodes
2. **Generate Config**: Run `python3 csv_to_config.py channel_mapping.csv` to generate two JSON configuration files
3. **Run Program**: Build and run `main.go` to start the OPC UA server

**Data Flow**: Modbus Slave → RawReg16/RawReg32 → Scaled16/Scaled32 → OPC UA Nodes

### Dependencies

- [gopcua/opcua](https://github.com/gopcua/opcua) - OPC UA protocol implementation
- [goburrow/modbus](https://github.com/goburrow/modbus) - Modbus protocol implementation

### FAQ

**Q: How to add new Modbus slaves?**  
A: Add new device configurations to the `devices` array in `modbus_config.json`.

**Q: How to modify scale parameters?**  
A: Edit `min_raw`, `max_raw`, `min_scaled`, `max_scaled` for the corresponding channel in `channel_config.json`.

**Q: Which Modbus function codes are supported?**  
A: FC1 (Read Coils), FC2 (Read Discrete Inputs), FC3 (Read Holding Registers), FC4 (Read Input Registers), FC5 (Write Single Coil), FC6 (Write Single Register), FC15 (Write Multiple Coils), FC16 (Write Multiple Registers). Raw values are stored in RawReg16/RawReg32, while engineering unit values are provided via Scaled16/Scaled32.

**Q: What is the data update frequency?**  
A: Data read/write cycle is 100ms, web interface refresh is 1000ms.

### Roadmap

- [ ] Support more Modbus function codes
- [ ] Add logging functionality
- [ ] Support SSL/TLS encryption
- [ ] Add user authentication
- [ ] Support OPC UA historical data

### License

MIT License

### Contributing

Issues and Pull Requests are welcome!
