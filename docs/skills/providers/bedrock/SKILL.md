---
name: bedrock
description: "在创宇太虚中使用 Amazon Bedrock（Converse API）模型"
---

# Amazon Bedrock

创宇太虚可通过 pi-ai 的 **Bedrock Converse** 流式供应商使用 **Amazon Bedrock** 模型。Bedrock 认证使用 **AWS SDK 默认凭据链**，无需 API 密钥。

## pi-ai 支持内容

- 供应商：`amazon-bedrock`
- API：`bedrock-converse-stream`
- 认证：AWS 凭据（环境变量、共享配置或实例角色）
- 区域：`AWS_REGION` 或 `AWS_DEFAULT_REGION`（默认：`us-east-1`）

## 自动模型发现

检测到 AWS 凭据后，创宇太虚可自动发现支持**流式**和**文本输出**的 Bedrock 模型。发现使用 `bedrock:ListFoundationModels` 并有缓存（默认：1 小时）。

配置选项位于 `models.bedrockDiscovery` 下：

```json5
{
  models: {
    bedrockDiscovery: {
      enabled: true,
      region: "us-east-1",
      providerFilter: ["anthropic", "amazon"],
      refreshInterval: 3600,
      defaultContextWindow: 32000,
      defaultMaxTokens: 4096,
    },
  },
}
```

备注：

- `enabled` 在 AWS 凭据存在时默认为 `true`。
- `region` 默认为 `AWS_REGION` 或 `AWS_DEFAULT_REGION`，然后 `us-east-1`。
- `providerFilter` 匹配 Bedrock 供应商名称（如 `anthropic`）。
- `refreshInterval` 以秒为单位；设为 `0` 禁用缓存。
- `defaultContextWindow`（默认：`32000`）和 `defaultMaxTokens`（默认：`4096`）用于发现的模型（如知道模型限制可覆盖）。

## 手动设置

1. 确保 AWS 凭据在**网关主机**上可用：

```bash
export AWS_ACCESS_KEY_ID="AKIA..."
export AWS_SECRET_ACCESS_KEY="..."
export AWS_REGION="us-east-1"
# 可选：
export AWS_SESSION_TOKEN="..."
export AWS_PROFILE="your-profile"
# 可选（Bedrock API 密钥/Bearer token）：
export AWS_BEARER_TOKEN_BEDROCK="..."
```

1. 在配置中添加 Bedrock 供应商和模型（无需 `apiKey`）：

```json5
{
  models: {
    providers: {
      "amazon-bedrock": {
        baseUrl: "https://bedrock-runtime.us-east-1.amazonaws.com",
        api: "bedrock-converse-stream",
        auth: "aws-sdk",
        models: [
          {
            id: "us.anthropic.claude-opus-4-6-v1:0",
            name: "Claude Opus 4.6 (Bedrock)",
            reasoning: true,
            input: ["text", "image"],
            cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
            contextWindow: 200000,
            maxTokens: 8192,
          },
        ],
      },
    },
  },
  agents: {
    defaults: {
      model: { primary: "amazon-bedrock/us.anthropic.claude-opus-4-6-v1:0" },
    },
  },
}
```

## EC2 实例角色

在附加了 IAM 角色的 EC2 实例上运行创宇太虚时，AWS SDK 将自动使用实例元数据服务（IMDS）进行认证。但创宇太虚的凭据检测目前仅检查环境变量，不检查 IMDS 凭据。

**解决方法：** 设置 `AWS_PROFILE=default` 以表示 AWS 凭据可用。实际认证仍通过 IMDS 使用实例角色。

```bash
# 添加到 ~/.bashrc 或 shell 配置
export AWS_PROFILE=default
export AWS_REGION=us-east-1
```

**EC2 实例角色所需 IAM 权限：**

- `bedrock:InvokeModel`
- `bedrock:InvokeModelWithResponseStream`
- `bedrock:ListFoundationModels`（用于自动发现）

或附加托管策略 `AmazonBedrockFullAccess`。

**快速设置：**

```bash
# 1. 创建 IAM 角色和实例配置
aws iam create-role --role-name EC2-Bedrock-Access \
  --assume-role-policy-document '{
    "Version": "2012-10-17",
    "Statement": [{
      "Effect": "Allow",
      "Principal": {"Service": "ec2.amazonaws.com"},
      "Action": "sts:AssumeRole"
    }]
  }'

aws iam attach-role-policy --role-name EC2-Bedrock-Access \
  --policy-arn arn:aws:iam::aws:policy/AmazonBedrockFullAccess

aws iam create-instance-profile --instance-profile-name EC2-Bedrock-Access
aws iam add-role-to-instance-profile \
  --instance-profile-name EC2-Bedrock-Access \
  --role-name EC2-Bedrock-Access

# 2. 附加到 EC2 实例
aws ec2 associate-iam-instance-profile \
  --instance-id i-xxxxx \
  --iam-instance-profile Name=EC2-Bedrock-Access

# 3. 在 EC2 实例上启用发现
openacosmi config set models.bedrockDiscovery.enabled true
openacosmi config set models.bedrockDiscovery.region us-east-1

# 4. 设置环境变量
echo 'export AWS_PROFILE=default' >> ~/.bashrc
echo 'export AWS_REGION=us-east-1' >> ~/.bashrc
source ~/.bashrc

# 5. 验证模型已发现
openacosmi models list
```

## 备注

- Bedrock 需要在你的 AWS 账户/区域中启用**模型访问**。
- 自动发现需要 `bedrock:ListFoundationModels` 权限。
- 若使用 profiles，在网关主机上设置 `AWS_PROFILE`。
- 创宇太虚按以下顺序查找凭据来源：`AWS_BEARER_TOKEN_BEDROCK`，然后 `AWS_ACCESS_KEY_ID` + `AWS_SECRET_ACCESS_KEY`，然后 `AWS_PROFILE`，然后默认 AWS SDK 链。
- 推理支持取决于模型；请查看 Bedrock 模型卡了解当前功能。
- 若偏好托管密钥流程，也可在 Bedrock 前放置 OpenAI 兼容代理，并将其配置为 OpenAI 供应商。
