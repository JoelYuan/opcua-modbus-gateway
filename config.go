package main

import (
	"encoding/json"
	"io/ioutil"
)

// ModbusTask 对应聚合后的 Modbus 读写任务
type ModbusTask struct {
	FC           int    `json:"fc"`
	Addr         int    `json:"addr"`
	Len          int    `json:"len"`
	BaseArrayIdx int    `json:"base_array_idx"` // 该块在全局数组中的起始位置
	IPPort       string `json:"ip_port"`
	Slave        int    `json:"slave"`
}

// DeviceConfig 对应单个从站的任务列表
type DeviceConfig struct {
	ReadTasks  []ModbusTask `json:"read"`
	WriteTasks []ModbusTask `json:"write"`
}

// ChannelMeta 对应点位的量程和属性
type ChannelMeta struct {
	Name      string  `json:"name"`
	MinRaw    float64 `json:"min_raw"`
	MaxRaw    float64 `json:"max_raw"`
	MinScaled float64 `json:"min_scaled"`
	MaxScaled float64 `json:"max_scaled"`
	Endian    string  `json:"endian"`
	IsAO      bool    `json:"is_ao"` // 区分 AI/AO 或 DI/DO
}

// GlobalConfig 内存元数据结构
type GlobalConfig struct {
	Reg16   []*ChannelMeta `json:"Reg16"`
	Reg32   []*ChannelMeta `json:"Reg32"`
	Digital []*ChannelMeta `json:"Digital"`
}

func LoadModbusConfig(path string) map[string]DeviceConfig {
	data, _ := ioutil.ReadFile(path)
	var conf map[string]DeviceConfig
	json.Unmarshal(data, &conf)
	return conf
}

func LoadChannelConfig(path string) GlobalConfig {
	data, _ := ioutil.ReadFile(path)
	var conf GlobalConfig
	json.Unmarshal(data, &conf)
	return conf
}
