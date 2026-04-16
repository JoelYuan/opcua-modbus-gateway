#!/usr/bin/env python3
"""
CSV to OPC UA & Modbus Config Converter
将规范的量程映射 CSV 转换为 channel_config.json 和 modbus_config.json
"""

import csv
import json
import sys
from pathlib import Path
from collections import defaultdict
from typing import Dict, List, Any

def csv_to_config(csv_file: str, output_dir: str = "."):
    """
    将 CSV 文件转换为 OPC UA 通道配置 JSON 和 Modbus 配置 JSON
    
    CSV 格式:
    modbus_type,modbus_addr,opcua_table,opcua_idx,name,unit,min_raw,max_raw,min_scaled,max_scaled,endian,modbus_fc
    
    参数:
        csv_file: 输入的 CSV 文件路径
        output_dir: 输出目录
    """
    
    # 默认配置
    default_min_raw = 0
    default_max_raw = 32767  # Scaled16 默认
    default_min_scaled = 0.0
    default_max_scaled = 100.0  # 100倍映射，5000表示50.00
    default_endian = "big"
    
    # 读取 CSV
    channels_scaled16: List[Dict[str, Any]] = []
    channels_scaled32: List[Dict[str, Any]] = []
    modbus_devices: Dict[str, Dict[str, Any]] = defaultdict(lambda: {"read": [], "write": [], "slave": 1})
    
    with open(csv_file, 'r', encoding='utf-8') as f:
        reader = csv.DictReader(f)
        
        for row in reader:
            modbus_type = row.get('modbus_type', '').strip().upper()
            modbus_addr = int(row.get('modbus_addr', 0))
            opcua_table = row.get('opcua_table', '').strip()
            opcua_idx = int(row.get('opcua_idx', 0))
            
            # 基本信息
            channel = {
                'index': opcua_idx,
                'name': row.get('name', f'Channel_{opcua_idx}').strip(),
                'unit': row.get('unit', '').strip()
            }
            
            # 量程配置
            if opcua_table == 'Scaled16':
                channel['min_raw'] = int(row.get('min_raw', default_min_raw))
                channel['max_raw'] = int(row.get('max_raw', default_max_raw))
                channel['min_scaled'] = float(row.get('min_scaled', default_min_scaled))
                channel['max_scaled'] = float(row.get('max_scaled', default_max_scaled))
                channel['endian'] = row.get('endian', default_endian).strip()
                channels_scaled16.append(channel)
                
            elif opcua_table == 'Scaled32':
                # Scaled32 默认最大值是 2147483647
                default_max_raw_32 = 2147483647
                channel['min_raw'] = int(row.get('min_raw', default_min_raw))
                channel['max_raw'] = int(row.get('max_raw', default_max_raw_32))
                channel['min_scaled'] = float(row.get('min_scaled', default_min_scaled))
                channel['max_scaled'] = float(row.get('max_scaled', default_max_scaled))
                channel['endian'] = row.get('endian', default_endian).strip()
                channels_scaled32.append(channel)
            
            # Modbus 配置
            modbus_fc = int(row.get('modbus_fc', 0))
            slave_id = int(row.get('slave_id', 1))
            ip_address = row.get('ip_address', '127.0.0.1').strip()
            len_val = int(row.get('len', 1))
            if modbus_fc in [3, 4]:  # 读操作
                device_key = f"modbus_slave_{slave_id}"
                modbus_devices[device_key]["read"].append({
                    "fc": modbus_fc,
                    "addr": modbus_addr,
                    "len": len_val,
                    "period_ms": 1000
                })
                modbus_devices[device_key]["slave"] = slave_id
                modbus_devices[device_key]["ip_address"] = ip_address
            elif modbus_fc in [5, 6]:  # 写操作
                device_key = f"modbus_slave_{slave_id}"
                modbus_devices[device_key]["write"].append({
                    "fc": modbus_fc,
                    "addr": modbus_addr,
                    "len": len_val,
                    "period": 1000
                })
                modbus_devices[device_key]["slave"] = slave_id
                modbus_devices[device_key]["ip_address"] = ip_address
    
    # 聚合连续地址的读写操作
    for device_key, device_data in modbus_devices.items():
        device_data["read"] = aggregate_modbus_ops(device_data["read"])
        device_data["write"] = aggregate_modbus_ops(device_data["write"])
    
    # 构建配置
    config = {
        'Scaled16': {
            'start_idx': 500,
            'end_idx': 999,
            'channels': channels_scaled16
        },
        'Scaled32': {
            'start_idx': 250,
            'end_idx': 499,
            'channels': channels_scaled32
        }
    }
    
    # 写入 OPC UA 配置 JSON
    output_file = Path(output_dir) / "channel_config.json"
    with open(output_file, 'w', encoding='utf-8') as f:
        json.dump(config, f, indent=2, ensure_ascii=False)
    
    print(f"✓ 已生成 OPC UA 配置: {output_file}")
    print(f"  - Scaled16: {len(channels_scaled16)} 个通道")
    print(f"  - Scaled32: {len(channels_scaled32)} 个通道")
    
    # 写入 Modbus 配置 JSON
    modbus_config = {"devices": []}
    for device_key, device_data in modbus_devices.items():
        device_name = device_key.replace("_", " ").title().replace(" ", "_")
        modbus_slave = device_data["slave"]
        ip_address = device_data.get("ip_address", "127.0.0.1")
        modbus_config["devices"].append({
            "name": device_name,
            "type": "tcp",
            "address": f"{ip_address}:502",
            "slave": modbus_slave,
            "unit": 1,
            "read": device_data["read"],
            "write": device_data["write"]
        })
    
    modbus_file = Path(output_dir) / "modbus_config.json"
    with open(modbus_file, 'w', encoding='utf-8') as f:
        json.dump(modbus_config, f, indent=2, ensure_ascii=False)
    
    print(f"✓ 已生成 Modbus 配置: {modbus_file}")
    print(f"  - 设备数量: {len(modbus_config['devices'])}")
    
    return config, modbus_config

def aggregate_modbus_ops(ops: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
    """
    聚合连续地址的 Modbus 操作
    
    例如:
    [
        {"fc": 4, "addr": 1, "len": 1},
        {"fc": 4, "addr": 2, "len": 1},
        {"fc": 4, "addr": 3, "len": 1}
    ]
    聚合为:
    [
        {"fc": 4, "addr": 1, "len": 3}
    ]
    """
    if not ops:
        return ops
    
    # 按功能码和地址分组
    grouped: Dict[tuple, List[Dict[str, Any]]] = {}
    for op in ops:
        key = (op["fc"], op["addr"])
        if key not in grouped:
            grouped[key] = []
        grouped[key].append(op)
    
    # 合并连续地址
    result: List[Dict[str, Any]] = []
    for fc, start_addr in sorted(grouped.keys()):
        ops_list = grouped[(fc, start_addr)]
        
        # 合并相同功能码和起始地址的操作
        total_len = sum(op["len"] for op in ops_list)
        result.append({
            "fc": fc,
            "addr": start_addr,
            "len": total_len,
            "period_ms": ops_list[0].get("period_ms", 1000),
            "period": ops_list[0].get("period", 1000)
        })
    
    # 进一步聚合连续地址
    if not result:
        return result
    
    final_result: List[Dict[str, Any]] = []
    current_group: List[Dict[str, Any]] = []
    
    for op in result:
        if not current_group:
            current_group.append(op)
        else:
            last_op = current_group[-1]
            # 检查是否连续（相同功能码，地址连续）
            if (op["fc"] == last_op["fc"] and 
                op["addr"] == last_op["addr"] + last_op["len"]):
                # 连续，合并
                current_group.append(op)
            else:
                # 不连续，保存当前组
                final_result.append(merge_group(current_group))
                current_group = [op]
    
    if current_group:
        final_result.append(merge_group(current_group))
    
    return final_result

def merge_group(group: List[Dict[str, Any]]) -> Dict[str, Any]:
    """
    合并一组连续的操作
    """
    if len(group) == 1:
        return group[0]
    
    fc = group[0]["fc"]
    start_addr = group[0]["addr"]
    total_len = sum(op["len"] for op in group)
    
    # 使用第一个操作的周期
    period_ms = group[0].get("period_ms", 1000)
    period = group[0].get("period", 1000)
    
    return {
        "fc": fc,
        "addr": start_addr,
        "len": total_len,
        "period_ms": period_ms,
        "period": period
    }

def create_example_csv(csv_file: str = "channel_mapping.csv"):
    """
    创建示例 CSV 文件
    """
    
    example_data = [
        ['modbus_type', 'modbus_addr', 'opcua_table', 'opcua_idx', 'name', 'unit', 'min_raw', 'max_raw', 'min_scaled', 'max_scaled', 'endian', 'modbus_fc', 'slave_id'],
        ['DI', '0', 'Digital', '0', 'Pressure_P1', 'MPa', '0', '1', '0.0', '1.0', '', '3', '1'],
        ['AI', '1', 'Scaled16', '500', 'Temperature_T1', '℃', '0', '32767', '-100.0', '200.0', 'big', '4', '1'],
        ['AI', '2', 'Scaled16', '501', 'Flow_F1', 'm³/h', '0', '32767', '0.0', '500.0', 'big', '4', '1'],
        ['AI', '3', 'Scaled32', '0', 'Level_L1', 'm', '0', '100000', '0.0', '100.0', 'big', '4', '1'],
        ['AO', '500', 'Scaled16', '999', 'Valve_V1', '%', '0', '32767', '0.0', '100.0', 'big', '6', '1'],
        ['DO', '2000', 'Digital', '2999', 'Control_C1', '', '0', '1', '0.0', '1.0', '', '5', '1'],
    ]
    
    with open(csv_file, 'w', encoding='utf-8', newline='') as f:
        writer = csv.writer(f)
        writer.writerows(example_data)
    
    print(f"✓ 已创建示例 CSV: {csv_file}")
    print("\nCSV 格式说明:")
    print("  modbus_type: DI (数字输入), AI (模拟输入), DO (数字输出), AO (模拟输出)")
    print("  modbus_addr: Modbus 寄存器地址")
    print("  opcua_table: OPC UA 表名 (Digital, Scaled16, Scaled32)")
    print("  opcua_idx: OPC UA 索引")
    print("  name: 通道名称（可选）")
    print("  unit: 单位（可选）")
    print("  min_raw: 原始值最小值（可选，默认0）")
    print("  max_raw: 原始值最大值（可选，默认Scaled16为32767，Scaled32为2147483647）")
    print("  min_scaled: 量程最小值（可选，默认0.0）")
    print("  max_scaled: 量程最大值（可选，默认100.0）")
    print("  endian: 字节序 big 或 little（可选，默认big）")
    print("  modbus_fc: Modbus 功能码 (3/4=读, 5/6=写)")
    print("  slave_id: Modbus 从站地址（可选，默认1）")
    print("\n示例:")
    print("  AI,1,Scaled16,500,Pressure_P1,MPa,0,32767,0.0,10.0,big,4,1")
    print("  → Modbus 地址 1 读取，映射到 Scaled16[500]，0-10MPa 量程，从站1")
    print("  AO,500,Scaled16,999,Valve_V1,%,0,32767,0.0,100.0,big,6,1")
    print("  → Modbus 地址 500 写入，映射到 Scaled16[999]，0-100% 量程，从站1")

def main():
    if len(sys.argv) < 2:
        print("OPC UA Channel Config CSV Converter")
        print("\n用法:")
        print("  python csv_to_config.py <csv_file> [output.json]")
        print("  python csv_to_config.py --example [csv_file]")
        print("\n示例:")
        print("  python csv_to_config.py channel_mapping.csv")
        print("  python csv_to_config.py --example")
        sys.exit(1)
    
    if sys.argv[1] == '--example' or sys.argv[1] == '-e':
        csv_file = sys.argv[2] if len(sys.argv) > 2 else "channel_mapping.csv"
        create_example_csv(csv_file)
    else:
        csv_file = sys.argv[1]
        output_dir = sys.argv[2] if len(sys.argv) > 2 else "."
        
        if not Path(csv_file).exists():
            print(f"错误: 文件不存在: {csv_file}")
            sys.exit(1)
        
        config, modbus_config = csv_to_config(csv_file, output_dir)

if __name__ == "__main__":
    main()
