# OPC UA Gateway 量程配置说明

## 配置文件位置
`channel_config.json`

## 配置文件结构

```json
{
  "Scaled16": {
    "start_idx": 500,
    "end_idx": 999,
    "channels": [
      {
        "index": 500,
        "name": "Pressure_P1",
        "unit": "MPa",
        "min_raw": 0,
        "max_raw": 32767,
        "min_scaled": 0.0,
        "max_scaled": 10.0,
        "endian": "big"
      }
    ]
  },
  "Scaled32": {
    "start_idx": 250,
    "end_idx": 499,
    "channels": [
      {
        "index": 250,
        "name": "Flow_F1",
        "unit": "m³/h",
        "min_raw": 0,
        "max_raw": 2147483647,
        "min_scaled": 0.0,
        "max_scaled": 1000.0,
        "endian": "big"
      }
    ]
  }
}
```

## 配置参数说明

### Scaled16 / Scaled32
- `start_idx`: 该类型数据的起始索引
- `end_idx`: 该类型数据的结束索引（不包含）
- `channels`: 通道配置数组

### Channel 配置
- `index`: 通道索引
- `name`: 通道名称（可选）
- `unit`: 单位（可选）
- `min_raw`: 原始值最小值（对于 Scaled16 是 0-32767，对于 Scaled32 是 0-2147483647）
- `max_raw`: 原始值最大值
- `min_scaled`: 量程最小值（例如 0.0）
- `max_scaled`: 量程最大值（例如 10.0）
- `endian`: 字节序，"big" 或 "little"（仅对 Scaled32 有效）

## 量程映射公式

```
ratio = (scaled - min_scaled) / (max_scaled - min_scaled)
raw = min_raw + ratio * (max_raw - min_raw)
```

## 示例

### 示例1: 压力变送器 0-10MPa
```json
{
  "index": 500,
  "name": "Pressure_P1",
  "unit": "MPa",
  "min_raw": 0,
  "max_raw": 32767,
  "min_scaled": 0.0,
  "max_scaled": 10.0,
  "endian": "big"
}
```

- 输入 5.0 MPa → 原始值 = 16383
- 原始值 32767 → 输出 10.0 MPa

### 示例2: 温度变送器 -100~200℃
```json
{
  "index": 501,
  "name": "Temperature_T1",
  "unit": "℃",
  "min_raw": 0,
  "max_raw": 32767,
  "min_scaled": -100.0,
  "max_scaled": 200.0,
  "endian": "big"
}
```

- 输入 0℃ → 原始值 = 10922
- 输入 200℃ → 原始值 = 32767

### 示例3: 大量程流量计 0-1000m³/h
```json
{
  "index": 250,
  "name": "Flow_F1",
  "unit": "m³/h",
  "min_raw": 0,
  "max_raw": 2147483647,
  "min_scaled": 0.0,
  "max_scaled": 1000.0,
  "endian": "big"
}
```

## 字节序说明

- **Big Endian**: 高位字节在前（网络字节序）
- **Little Endian**: 低位字节在前（x86 架构）

对于 Scaled32（32位浮点数），根据 PLC 或设备的字节序选择。

## 注意事项

1. 配置文件修改后需要重启 OPC UA 服务器
2. 量程映射在写入时自动计算
3. 读取时直接返回Scaled值，无需转换
4. 索引范围必须在配置的 start_idx 和 end_idx 之间
