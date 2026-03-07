---
name: claude-max-api-proxy
description: "将 Claude Max/Pro 订阅用作 OpenAI 兼容的 API 端点"
---

# Claude Max API Proxy

**claude-max-api-proxy** 是一个社区工具，将你的 Claude Max/Pro 订阅暴露为 OpenAI 兼容的 API 端点。这样你可以用任何支持 OpenAI API 格式的工具使用订阅。

## 为什么使用？

| 方案 | 费用 | 适用场景 |
| --- | --- | --- |
| Anthropic API | 按 token 计费（Opus 约 $15/M 输入, $75/M 输出） | 生产应用, 高流量 |
| Claude Max 订阅 | $200/月固定费 | 个人使用, 开发, 无限量 |

如果你有 Claude Max 订阅并想用 OpenAI 兼容工具使用，这个代理可以节省大量费用。

## 工作原理

```text
你的应用 → claude-max-api-proxy → Claude Code CLI → Anthropic（通过订阅）
     (OpenAI 格式)              (转换格式)              (使用你的登录)
```

代理：

1. 在 `http://localhost:3456/v1/chat/completions` 接受 OpenAI 格式请求
2. 将其转换为 Claude Code CLI 命令
3. 以 OpenAI 格式返回响应（支持流式）

## 安装

```bash
# 需要 Node.js 20+ 和 Claude Code CLI
npm install -g claude-max-api-proxy

# 验证 Claude CLI 已认证
claude --version
```

## 使用方法

### 启动服务器

```bash
claude-max-api
# 服务器运行在 http://localhost:3456
```

### 测试

```bash
# 健康检查
curl http://localhost:3456/health

# 列出模型
curl http://localhost:3456/v1/models

# 聊天补全
curl http://localhost:3456/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-opus-4",
    "messages": [{"role": "user", "content": "你好！"}]
  }'
```

### 与创宇太虚配合

将创宇太虚指向代理作为自定义 OpenAI 兼容端点：

```json5
{
  env: {
    OPENAI_API_KEY: "not-needed",
    OPENAI_BASE_URL: "http://localhost:3456/v1",
  },
  agents: {
    defaults: {
      model: { primary: "openai/claude-opus-4" },
    },
  },
}
```

## 可用模型

| 模型 ID | 映射到 |
| --- | --- |
| `claude-opus-4` | Claude Opus 4 |
| `claude-sonnet-4` | Claude Sonnet 4 |
| `claude-haiku-4` | Claude Haiku 4 |

## macOS 自启动

创建 LaunchAgent 自动运行代理：

```bash
cat > ~/Library/LaunchAgents/com.claude-max-api.plist << 'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.claude-max-api</string>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>ProgramArguments</key>
  <array>
    <string>/usr/local/bin/node</string>
    <string>/usr/local/lib/node_modules/claude-max-api-proxy/dist/server/standalone.js</string>
  </array>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key>
    <string>/usr/local/bin:/opt/homebrew/bin:~/.local/bin:/usr/bin:/bin</string>
  </dict>
</dict>
</plist>
EOF

launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.claude-max-api.plist
```

## 链接

- **npm:** [https://www.npmjs.com/package/claude-max-api-proxy](https://www.npmjs.com/package/claude-max-api-proxy)
- **GitHub:** [https://github.com/atalovesyou/claude-max-api-proxy](https://github.com/atalovesyou/claude-max-api-proxy)
- **Issues:** [https://github.com/atalovesyou/claude-max-api-proxy/issues](https://github.com/atalovesyou/claude-max-api-proxy/issues)

## 备注

- 这是一个**社区工具**，非 Anthropic 或创宇太虚官方支持
- 需要已认证 Claude Code CLI 的有效 Claude Max/Pro 订阅
- 代理在本地运行，不向任何第三方服务器发送数据
- 完全支持流式响应

## 另请参见

- [Anthropic 供应商](/providers/anthropic) - 使用 Claude setup-token 或 API 密钥的原生创宇太虚集成
- [OpenAI 供应商](/providers/openai) - 用于 OpenAI/Codex 订阅
