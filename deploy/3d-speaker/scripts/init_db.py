#!/usr/bin/env python3
"""
数据库与目录初始化脚本
"""

import os
import json
import sys


def init():
    """初始化数据目录和声纹数据库"""
    dirs = [
        "data",
        "data/voiceprint_embeddings",
        "data/enrollment_audio",
        "data/results",
        "data/temp",
        "logs",
    ]

    for d in dirs:
        os.makedirs(d, exist_ok=True)
        print(f"  目录已就绪: {d}")

    # 初始化声纹数据库（空 JSON）
    db_path = "data/voiceprint.db"
    if not os.path.exists(db_path):
        with open(db_path, "w", encoding="utf-8") as f:
            json.dump({}, f)
        print(f"  声纹库已初始化: {db_path}")
    else:
        with open(db_path, "r", encoding="utf-8") as f:
            data = json.load(f)
        print(f"  声纹库已存在: {db_path} ({len(data)} 条记录)")

    print("\n初始化完成")


if __name__ == "__main__":
    print("初始化数据目录和声纹数据库...\n")
    init()
