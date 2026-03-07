---
name: image-ops
description: "图像理解运维：image 配置、模型可用性与连通验证"
---

# Image 运维

用于图片理解模块配置和模型在线性验证。

## 覆盖方法
- `image.config.get`
- `image.config.set`
- `image.test`
- `image.models`
- `image.ollama.models`

## 推荐流程
1. 用 `image.config.get` 获取当前 provider/model。
2. 用 `image.config.set` 写入最小配置。
3. 用 `image.test` 验证端到端可用性。
4. 用 `image.models` / `image.ollama.models` 核对模型列表。

## 风险控制
- 模型切换后必须跑测试样本对比。
- 线上故障先确认 provider 在线状态再改配置。

## 成功判定
- 测试通过且目标模型可被枚举。
