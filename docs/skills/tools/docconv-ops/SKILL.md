---
name: docconv-ops
description: "文档转换运维：docconv 配置、连通测试与格式能力核验"
---

# DocConv 运维

用于文档转换模块配置与格式能力验证。

## 覆盖方法
- `docconv.config.get`
- `docconv.config.set`
- `docconv.test`
- `docconv.formats`

## 推荐流程
1. 用 `docconv.config.get` 确认 provider/endpoint。
2. 用 `docconv.config.set` 更新最小必要参数。
3. 先跑 `docconv.test` 验证可用性。
4. 用 `docconv.formats` 核对目标格式是否受支持。

## 风险控制
- 变更 provider 后必须重跑 test。
- 非兼容格式不要直接上线批量任务。

## 成功判定
- `docconv.test` 成功。
- 目标输入输出格式在 `docconv.formats` 可见。
