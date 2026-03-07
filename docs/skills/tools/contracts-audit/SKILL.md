---
name: contracts-audit
description: "合约审计：contract.list/get/audit 的巡检与核验流程"
---

# Contract 审计

用于合约索引、详情读取与审计条目核验。

## 覆盖方法
- `contract.list`
- `contract.get`
- `contract.audit`

## 推荐流程
1. 用 `contract.list` 获取全量清单。
2. 用 `contract.get` 查看单项合约详情。
3. 用 `contract.audit` 拉取审计条目并按时间排序核验。

## 风险控制
- 关键合约审计条目需留档，不只看最新一条。

## 成功判定
- 合约清单与审计条目可互相追溯。
