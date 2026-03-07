---
name: web-artifacts-builder
description: 使用现代前端 Web 技术（React、Tailwind CSS、shadcn/ui）创建精细多组件 HTML 制品的工具套件。用于需要状态管理、路由或 shadcn/ui 组件的复杂制品——不适用于简单的单文件 HTML/JSX 制品。
license: Complete terms in LICENSE.txt
---

# Web 制品构建器

构建强大的前端 HTML 制品，按以下步骤进行：

1. 使用 `scripts/init-artifact.sh` 初始化前端仓库
2. 编辑生成的代码开发制品
3. 使用 `scripts/bundle-artifact.sh` 将所有代码打包为单个 HTML 文件
4. 向用户展示制品
5. （可选）测试制品

**技术栈**：React 18 + TypeScript + Vite + Parcel（打包） + Tailwind CSS + shadcn/ui

## 设计与样式指南

**非常重要**：为避免"AI 流水线"风格，避免使用过多居中布局、紫色渐变、统一圆角和 Inter 字体。

## 快速开始

### 步骤 1：初始化项目

运行初始化脚本创建新的 React 项目：

```bash
bash scripts/init-artifact.sh <project-name>
cd <project-name>
```

创建的项目包含：

- ✅ React + TypeScript（通过 Vite）
- ✅ Tailwind CSS 3.4.1 含 shadcn/ui 主题系统
- ✅ 路径别名（`@/`）已配置
- ✅ 40+ shadcn/ui 组件已预装
- ✅ 所有 Radix UI 依赖已包含
- ✅ Parcel 打包已配置（通过 .parcelrc）
- ✅ Node 18+ 兼容（自动检测并锁定 Vite 版本）

### 步骤 2：开发制品

编辑生成的文件构建制品。参见下方**常见开发任务**获取指导。

### 步骤 3：打包为单个 HTML 文件

将 React 应用打包为单个 HTML 制品：

```bash
bash scripts/bundle-artifact.sh
```

生成 `bundle.html`——一个自包含的制品，所有 JavaScript、CSS 和依赖都已内联。此文件可直接作为制品分享。

**要求**：项目根目录必须有 `index.html`。

**脚本功能**：

- 安装打包依赖（parcel、@parcel/config-default、parcel-resolver-tspaths、html-inline）
- 创建 `.parcelrc` 配置（含路径别名支持）
- 使用 Parcel 构建（无 source map）
- 使用 html-inline 将所有资源内联为单个 HTML

### 步骤 4：向用户分享制品

将打包后的 HTML 文件在对话中分享，用户即可作为制品查看。

### 步骤 5：测试/预览制品（可选）

注意：此步骤完全可选。仅在必要时或用户要求时执行。

使用可用工具（包括其他技能或 Playwright/Puppeteer 等内置工具）测试/预览制品。一般避免提前测试制品，因为这会增加请求和最终制品之间的延迟。展示后再测试（若需要或出现问题）。

## 参考

- **shadcn/ui 组件**：<https://ui.shadcn.com/docs/components>
