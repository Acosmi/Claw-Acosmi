#!/bin/bash
# macOS 双击启动入口
# 双击此文件会自动在 Terminal.app 中运行
cd "$(dirname "$0")" && exec ./linux-start.sh
