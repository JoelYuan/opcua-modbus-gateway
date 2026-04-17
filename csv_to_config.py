#!/usr/bin/env python3
import csv
import json
import os

def generate_configs(csv_file="channels.csv"):
    # 初始化 Meta 数据结构，严格对应 Go 端的数组长度
    channel_meta = {
        "Reg16": [None] * 1000,  # 0-499 AI, 500-999 AO
        "Reg32": [None] * 500,   # 0-249 AI, 250-499 AO
        "Digital": [None] * 3000 # 0-1999 DI, 2000-2999 DO
    }
    
    tasks = []
    current_task = None

    if not os.path.exists(csv_file):
        print(f"❌ 找不到文件: {csv_file}")
        return

    with open(csv_file, 'r', encoding='utf-8') as f:
        reader = csv.DictReader(f)
        for row in reader:
            # 处理空行或注释
            if not row['name'] or row['name'].startswith('#') or not row['ip'].strip():
                current_task = None # 强制断开聚合
                continue

            # 数据解析
            try:
                ip_port = f"{row['ip'].strip()}:{row['port'].strip()}"
                slave = int(row['slave_id'])
                fc = int(row['mb_fc'])
                addr = int(row['mb_addr'])
                arr_idx = int(row['array_idx'])
                ch_type_raw = row['type'].upper()
                
                # 确定基础类型
                if "REG16" in ch_type_raw: base_type = "Reg16"
                elif "REG32" in ch_type_raw: base_type = "Reg32"
                else: base_type = "Digital"
                
                is_32bit = (base_type == "Reg32")
                step = 2 if is_32bit else 1

                # 任务聚合判定逻辑
                can_aggregate = (
                    current_task and 
                    current_task['ip_port'] == ip_port and
                    current_task['slave'] == slave and
                    current_task['fc'] == fc and
                    addr == current_task['start_addr'] + current_task['len']
                )

                if can_aggregate:
                    current_task['len'] += step
                else:
                    current_task = {
                        "ip_port": ip_port,
                        "slave": slave,
                        "fc": fc,
                        "start_addr": addr,
                        "len": step,
                        "base_array_idx": arr_idx
                    }
                    tasks.append(current_task)

                # 记录系数元数据
                channel_meta[base_type][arr_idx] = {
                    "name": row['name'],
                    "min_raw": float(row['min_raw']),
                    "max_raw": float(row['max_raw']),
                    "min_scaled": float(row['min_scaled']),
                    "max_scaled": float(row['max_scaled']),
                    "endian": row.get('endian', 'big').lower(),
                    "is_ao": "AO" in ch_type_raw or "DO" in ch_type_raw
                }
            except Exception as e:
                print(f"⚠️ 解析行出错 [{row.get('name')}]: {e}")

    # 按设备归类 modbus_config
    device_configs = defaultdict(lambda: {"read": [], "write": []})
    for t in tasks:
        dev_key = f"{t['ip_port']}_S{t['slave']}"
        # 简化判定：FC 1,2,3,4 为读取任务
        if t['fc'] in [1, 2, 3, 4]:
            device_configs[dev_key]["read"].append(t)
        else:
            device_configs[dev_key]["write"].append(t)

    # 写入文件
    with open('modbus_config.json', 'w', encoding='utf-8') as f:
        json.dump(device_configs, f, indent=2)
    with open('channel_config.json', 'w', encoding='utf-8') as f:
        json.dump(channel_meta, f, indent=2)

    print(f"✅ 转换成功! 生成了 {len(tasks)} 个聚合任务。")

from collections import defaultdict
if __name__ == "__main__":
    generate_configs()