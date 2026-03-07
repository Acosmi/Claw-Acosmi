---
name: stt-ops
description: "语音转写运维：stt.config/test/models/transcribe 工作流"
---

# STT 运维

用于语音转写配置、联调、模型可用性检查和转写回归。

## 覆盖方法
- `stt.config.get`
- `stt.config.set`
- `stt.test`
- `stt.models`
- `stt.transcribe`

## 推荐流程
1. 用 `stt.config.get` 获取当前 provider 与参数。
2. 小步更新走 `stt.config.set`。
3. 用 `stt.test` 先验证服务可连通。
4. 用 `stt.models` 确认可用模型。
5. 用标准音频样本执行 `stt.transcribe` 回归。

## 风险控制
- 不同 provider 的采样率和语言参数需显式核对。
- 转写基准样本应固定，避免回归噪声。

## 成功判定
- 测试通过，转写文本稳定可复现。
