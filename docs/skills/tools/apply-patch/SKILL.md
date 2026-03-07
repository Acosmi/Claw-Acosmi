---
name: apply-patch
description: "使用 apply_patch 工具应用多文件补丁"
tools: apply_patch
---

# apply_patch 工具

使用结构化补丁格式应用文件更改。适用于多文件或多段编辑，在单次 `edit` 调用不够可靠时使用。

该工具接受一个 `input` 字符串，包含一个或多个文件操作：

```
*** Begin Patch
*** Add File: path/to/file.txt
+line 1
+line 2
*** Update File: src/app.ts
@@
-old line
+new line
*** Delete File: obsolete.txt
*** End Patch
```

## 参数

- `input`（必填）：完整的补丁内容，包含 `*** Begin Patch` 和 `*** End Patch`。

## 说明

- 路径相对于工作区根目录解析。
- 在 `*** Update File:` 段中使用 `*** Move to:` 来重命名文件。
- `*** End of File` 在需要时标记仅追加到文件末尾的插入。
- 实验性功能，默认禁用。通过 `tools.exec.applyPatch.enabled` 启用。
- 仅限 OpenAI（包括 OpenAI Codex）。可选通过 `tools.exec.applyPatch.allowModels` 按模型控制。
- 配置仅在 `tools.exec` 下。

## 示例

```json
{
  "tool": "apply_patch",
  "input": "*** Begin Patch\n*** Update File: src/index.ts\n@@\n-const foo = 1\n+const foo = 2\n*** End Patch"
}
```
