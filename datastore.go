package main

import (
	"encoding/binary"
	"math"
	"sync"
)

type DataStore struct {
	mu sync.RWMutex

	// 1. 原始数据数组 (Southbound Raw)
	RawDigital [3000]bool
	RawReg16   [1000]uint16
	RawReg32   [500]uint32

	// 2. 工程值数组 (Northbound Scaled/Real)
	ScaledDigital [3000]bool
	Scaled16      [1000]float64
	Scaled32      [500]float64

	// 3. 配置元数据 (只读)
	Meta GlobalConfig
}

func NewDataStore(meta GlobalConfig) *DataStore {
	return &DataStore{Meta: meta}
}

// UpdateFromModbus 由南向驱动调用，搬运原始字节块
func (ds *DataStore) UpdateFromModbus(task ModbusTask, data []byte) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	idx := task.BaseArrayIdx
	switch task.FC {
	case 1, 2: // Digital
		for i := 0; i < task.Len; i++ {
			// Modbus bit 打包逻辑略，此处假设驱动已转为 bool 序列
		}
	case 3, 4: // Register
		if task.Len+idx > 1000 && task.Len <= 2000 { // 假设 Reg16 处理
			for i := 0; i < task.Len; i++ {
				ds.RawReg16[idx+i] = binary.BigEndian.Uint16(data[i*2 : i*2+2])
			}
		} else { // Reg32 处理
			for i := 0; i < task.Len/2; i++ {
				ds.RawReg32[idx+i] = binary.BigEndian.Uint32(data[i*4 : i*4+4])
			}
		}
	}
}

// ProcessAllConversions 核心计算引擎：AI/AO 分段处理
func (ds *DataStore) ProcessAllConversions() {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	// --- 1. 处理 Reg16 (0-499 AI, 500-999 AO) ---
	for i := 0; i < 1000; i++ {
		m := ds.Meta.Reg16[i]
		if m == nil { continue }
		if !m.IsAO { // AI: Raw -> Scaled
			ds.Scaled16[i] = ds.linearTransform(float64(ds.RawReg16[i]), m.MinRaw, m.MaxRaw, m.MinScaled, m.MaxScaled)
		} else {     // AO: Scaled -> Raw (来自北向写入)
			ds.RawReg16[i] = uint16(ds.linearTransform(ds.Scaled16[i], m.MinScaled, m.MaxScaled, m.MinRaw, m.MaxRaw))
		}
	}

	// --- 2. 处理 Reg32 (0-249 AI, 250-499 AO) ---
	for i := 0; i < 500; i++ {
		m := ds.Meta.Reg32[i]
		if m == nil { continue }
		
		// 字节序预处理 (如果是 Little Endian，在计算前进行调换)
		rawVal := ds.RawReg32[i]
		if m.Endian == "little" {
			rawVal = (rawVal << 16) | (rawVal >> 16) // 简单示例：字翻转
		}

		if !m.IsAO { // AI
			ds.Scaled32[i] = ds.linearTransform(float64(rawVal), m.MinRaw, m.MaxRaw, m.MinScaled, m.MaxScaled)
		} else {     // AO
			scaledVal := ds.linearTransform(ds.Scaled32[i], m.MinScaled, m.MaxScaled, m.MinRaw, m.MaxRaw)
			ds.RawReg32[i] = uint32(scaledVal)
			// AO 写回时若需要 Little Endian 需在此逆向翻转
		}
	}
}

// 线性转换公式：y = (x - minX) * (maxY - minY) / (maxX - minX) + minY
func (ds *DataStore) linearTransform(x, minX, maxX, minY, maxY float64) float64 {
	if maxX == minX { return minY }
	// 钳位检查 (Clamping)
	if x < minX { x = minX }
	if x > maxX { x = maxX }
	val := (x-minX)*(maxY-minY)/(maxX-minX) + minY
	if math.IsNaN(val) { return minY }
	return val
}