---
name: browser-login
description: "浏览器自动化的手动登录 + X/Twitter 发帖"
---

# 浏览器登录 + X/Twitter 发帖

## 手动登录（推荐）

当网站需要登录时，在**宿主**浏览器配置文件（openacosmi 浏览器）中**手动登录**。

**不要**将您的凭据提供给模型。自动化登录通常会触发反机器人防御，并可能导致账户被锁定。

返回主浏览器文档：[Browser](/tools/browser)。

## 使用哪个 Chrome 配置文件？

Claw Acosmi 控制一个**专用的 Chrome 配置文件**（名为 `openacosmi`，橙色调 UI）。这与您日常使用的浏览器配置文件是分开的。

两种简单的访问方式：

1. **让 Agent 打开浏览器** — Agent 使用 `browser` 工具的 `navigate` action 打开 URL。
2. **手动打开** — 通过 Rust CLI：参见 `docs/cli/browser.md` 中的 `openacosmi browser start` / `open` 命令。

## X/Twitter：推荐流程

- **阅读/搜索/话题：** 使用**宿主**浏览器（手动登录）。
- **发布更新：** 使用**宿主**浏览器（手动登录）。

## 沙箱 + 宿主浏览器访问

沙箱化的浏览器会话**更容易**触发机器人检测。对于 X/Twitter（及其他严格的网站），推荐使用**宿主**浏览器。

如果 Agent 处于沙箱中，浏览器工具默认使用沙箱。要允许宿主控制：

```json5
{
  agents: {
    defaults: {
      sandbox: {
        mode: "non-main",
        browser: {
          allowHostControl: true,
        },
      },
    },
  },
}
```

然后使用浏览器工具时带上 `target="host"` 参数。或者为发布更新的 Agent 禁用沙箱。
