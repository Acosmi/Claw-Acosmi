---
name: system-config
description: "系统配置全链路：通用 config.* 与专项配置 RPC 的安全读取、写入、应用与验证"
tools: gateway
---

# 系统配置技能

## 可执行主链路

```
config.get → config.schema → 选择一个写路径 → 检查 validation/verification → 必要时再读一次 config.get
```

## 1. 读取当前配置

使用 `gateway(action="config.get")`。

重点读取这些字段：
- `hash`: 后续 `config.set` / `config.patch` / `config.apply` 的 `baseHash`
- `raw`: 当前脱敏后的 JSON5 文本
- `config`: 当前脱敏后的结构化配置
- `issues` / `warnings`: 当前配置是否已经有问题

没有最新 `hash` 不要写配置。

## 2. 读取 Schema / UI hints

使用 `gateway(action="config.schema")`。

用途：
- 确认字段类型
- 确认可选值 / UI hints
- 在生成 patch 前先确认目标字段存在且语义正确

## 3. 先判断走通用链路还是专项链路

### 通用 `config.*`

适用于：
- 需要查看完整脱敏配置
- 需要生成最小 JSON5 merge patch
- 需要做整份配置替换

### 专项配置动作

适用于只改一个窄域：
- 顶层专项工具（如果当前 runtime 已暴露）：
  - `browser_config`
  - `remote_approval_config`
  - `image_config`
  - `stt_config`
  - `docconv_config`
  - `media_config`
- 浏览器：`tools.browser.get` → `tools.browser.set`
- 远程审批：`security.remoteApproval.config.get` → `security.remoteApproval.config.set` → `security.remoteApproval.test`
- 图片理解：`image.config.get` → `image.models` / `image.ollama.models` → `image.config.set` → `image.test`
- STT：`stt.config.get` → `stt.models` → `stt.config.set` → `stt.test`
- DocConv：`docconv.config.get` → `docconv.formats` → `docconv.config.set` → `docconv.test`
- Media：`media.config.get` → `media.config.update`

规则：
- 只改单一窄域时，优先走专项工具；如果当前回合没有专项顶层工具，再走 `gateway` 的专项动作
- 不要为了一个字段去改整份 `config.*`
- `tools.browser.*` / `image.*` / `stt.*` / `docconv.*` / `media.config.*` 使用主配置文件的 `hash`
- `security.remoteApproval.config.*` 使用它自己返回的专用 `hash`，不是 `config.get` 的 `hash`

## 4. 通用链路的写路径

### 默认路径：最小变更

使用 `gateway(action="config.patch", raw="<JSON5 merge patch>", baseHash="<latest hash>")`。

规则：
- `raw` 必须是 JSON5 对象，且只包含最小 diff
- 一次只改一个主题块
- 优先用于 `models.*`、`providers.*`、`tools.*`、`security.*` 这类局部修改

### 整份配置替换并立即生效

使用 `gateway(action="config.apply", raw="<full JSON5 config>", baseHash="<latest hash>")`。

适用场景：
- 需要整份替换
- 需要 gateway 写盘并安排 restart

### 整份配置替换但不重启

使用 `gateway(action="config.set", raw="<full JSON5 config>", baseHash="<latest hash>")`。

适用场景：
- 只需要写盘
- 由上层另行决定何时重启 / 应用

不要对同一变更先 `config.patch` 再 `config.apply`。
`config.patch` 本身已经会写入并安排 restart。

## 5. 专项链路的写法

### 浏览器

先 `tools.browser.get` 取最新 `hash`，再 `tools.browser.set(baseHash=...)`。

适合：
- `enabled`
- `cdpUrl`
- `evaluateEnabled`
- `headless`

### 远程审批

先 `security.remoteApproval.config.get` 取最新专用 `hash`，再 `security.remoteApproval.config.set(baseHash=...)`，最后必要时 `security.remoteApproval.test(provider=...)`。

适合：
- 总开关
- 回调地址
- Feishu / DingTalk / WeCom 子配置

### 图片理解 / STT / DocConv

共同模式：
- 先读 `*.config.get`
- 需要枚举能力时读 `image.models` / `image.ollama.models` / `stt.models` / `docconv.formats`
- 再用 `*.config.set(baseHash=...)`
- 写后可执行 `*.test`

### Media

先 `media.config.get`，再 `media.config.update(baseHash=...)`。

适合：
- `autoSpawnEnabled`
- `maxAutoSpawnsPerDay`
- `hotKeywords`
- `monitorIntervalMin`
- `trendingThreshold`
- 发布渠道 / 热点源子配置

## 6. 验证结果

每次写入后都检查：
- `result.validation.ok`
- `result.validation.issues`
- `result.verification.runtimeEffect`
- `result.verification.restartScheduled`
- `result.hash` 或 `result.snapshot.hash`
- `result.restart` / `result.sentinel`（如果有）

写入成功后，把新的 `hash` 作为下一次变更的 `baseHash`。

对专项动作，额外检查：
- `security.remoteApproval.test` / `image.test` / `stt.test` / `docconv.test` 是否成功
- `result.verification.baseHashChecked`
- `result.verification.legacyUnsafeWrite` 不应在 Agent 正常路径出现

## 7. 常用 patch 示例

### 调整 gateway 端口

```json5
{
  gateway: {
    port: 26223
  }
}
```

### 调整浏览器 profile

```json5
{
  browser: {
    profiles: {
      default: {
        cdpPort: 9222
      }
    }
  }
}
```

### 调整执行安全级别

```json5
{
  tools: {
    exec: {
      security: "allowlist",
      ask: "on-miss"
    }
  }
}
```

## 8. 安全规则

- 每次写入前先做一次新的 `config.get`
- 专项配置先做对应的 `*.config.get`
- 高风险块一次只改一个：`security.*`、`tools.exec.*`、`models.*`、`providers.*`
- 如果用户没有明确要求写入，只读取 `config.get` / `config.schema`
- `baseHash` 失败时，立即重新 `config.get`，不要猜
- 专项配置的 `baseHash` 失败时，立即重新跑对应的 `*.config.get`
- validation 失败时，直接看错误里的 `details.issues`
- 对窄域修改，优先使用专项 setter，不要绕回通用 `config.patch`

## 9. 边界

- 通用系统配置走 `gateway` 的 `config.*`
- 窄域配置优先走已暴露的专项动作：
  - `browser_config`
  - `remote_approval_config`
  - `image_config`
  - `stt_config`
  - `docconv_config`
  - `media_config`
  - 如果当前 runtime 没有这些顶层工具，则回退到 `gateway` 的窄域动作
  - `tools.browser.get` / `tools.browser.set`
  - `security.remoteApproval.config.get` / `security.remoteApproval.config.set` / `security.remoteApproval.test`
  - `image.config.get` / `image.config.set` / `image.test` / `image.models` / `image.ollama.models`
  - `stt.config.get` / `stt.config.set` / `stt.test` / `stt.models`
  - `docconv.config.get` / `docconv.config.set` / `docconv.test` / `docconv.formats`
  - `media.config.get` / `media.config.update`
- 当前主 Agent 的 `gateway` 工具已经同时支持通用配置动作、专项配置动作、`restart` 和 `update.run`
