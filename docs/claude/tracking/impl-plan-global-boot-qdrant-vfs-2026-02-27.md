---
document_type: Tracking
status: Draft
created: 2026-02-27
last_updated: 2026-02-27
audit_report: Pending
skill5_verified: false
source_plan: docs/claude/deferred/global-boot-skill-vectorized-loading.md
---

# 全局 Boot + Qdrant 按需检索 + VFS 分级加载 — 实施跟踪计划

> **核心循环**: 智能体需要信息 → Qdrant 检索定位 → VFS 提取 L0 → 需要更多 → L1/L2
>
> 分三个 Phase 落地，Phase 0 为共享基础设施。

---

## Phase 0: 共享基础设施

> 所有 Phase 复用的底层能力。Phase 1 前必须完成。

### 0.1 VFS `_system/` 命名空间支持

**目标**: 让 VFS 支持系统级数据（技能/插件），与用户记忆隔离。

**文件**: `backend/internal/memory/uhms/vfs.go`

- [ ] 0.1.1 新增 `WriteSystemEntry(namespace, category, id string, l0, l1, l2 string, meta map[string]interface{}) error`
  - 路径: `{vfsRoot}/_system/{namespace}/{category}/{id}/l0.txt|l1.txt|l2.txt|meta.json`
  - namespace 示例: `skills`, `plugins`, `sessions`
  - 复用现有 `writeFile()` 辅助函数
- [ ] 0.1.2 新增 `ReadSystemL0(namespace, category, id string) (string, error)`
- [ ] 0.1.3 新增 `ReadSystemL1(namespace, category, id string) (string, error)`
- [ ] 0.1.4 新增 `ReadSystemL2(namespace, category, id string) (string, error)`
- [ ] 0.1.5 新增 `ReadSystemMeta(namespace, category, id string) (map[string]interface{}, error)`
- [ ] 0.1.6 新增 `BatchReadSystemL0(namespace string, ids []SystemEntryRef) ([]SystemL0Entry, error)`
  - `SystemEntryRef` 包含 category + id
  - `SystemL0Entry` 包含 id + abstract + meta
- [ ] 0.1.7 新增 `ListSystemEntries(namespace, category string) ([]SystemEntryRef, error)`
  - 列出 `_system/{namespace}/{category}/` 下所有条目
- [ ] 0.1.8 新增 `DeleteSystemEntry(namespace, category, id string) error`

**文件**: `backend/internal/memory/uhms/interfaces.go`

- [ ] 0.1.9 `VFS` 接口新增 `_system/` 方法签名
  - `WriteSystemEntry(namespace, category, id, l0, l1, l2 string, meta map[string]interface{}) error`
  - `ReadSystemL0/L1/L2(namespace, category, id string) (string, error)`
  - `ReadSystemMeta(namespace, category, id string) (map[string]interface{}, error)`
  - `BatchReadSystemL0(namespace string, ids []SystemEntryRef) ([]SystemL0Entry, error)`
  - `ListSystemEntries(namespace, category string) ([]SystemEntryRef, error)`
  - `DeleteSystemEntry(namespace, category, id string) error`

**文件**: `backend/internal/memory/uhms/types.go`

- [ ] 0.1.10 新增类型定义:
  - `SystemEntryRef { Category, ID string }`
  - `SystemL0Entry { ID, Category, Abstract string; Meta map[string]interface{} }`

### 0.2 Boot 文件管理

**目标**: 单文件记录系统全局地图 + 上次会话 + 检索指南，启动时 ~50ms 就绪。

**文件**: `backend/internal/memory/uhms/boot.go` (**新建**)

- [ ] 0.2.1 定义 `BootFile` 结构体
  ```go
  type BootFile struct {
      Version     string          `json:"version"`
      UpdatedAt   time.Time       `json:"updated_at"`
      LastSession *BootSession    `json:"last_session,omitempty"`
      SystemMap   BootSystemMap   `json:"system_map"`
      SearchGuide BootSearchGuide `json:"search_guide"`
  }
  ```
- [ ] 0.2.2 定义 `BootSession` 结构体
  - `Summary string`, `ActiveTasks []string`, `EndedAt time.Time`
- [ ] 0.2.3 定义 `BootSystemMap` 结构体
  - `Skills BootSkillsInfo`
  - `Plugins BootPluginsInfo`
  - `Memory BootMemoryInfo`
  - `Sessions BootSessionsInfo` (预留)
- [ ] 0.2.4 定义 `BootSkillsInfo` 结构体
  - `SourceDir string`, `VFSDir string`, `Categories []string`
  - `TotalCount int`, `Indexed bool`, `QdrantCollection string`
  - `LastIndexedAt time.Time`
- [ ] 0.2.5 定义 `BootPluginsInfo` 结构体
  - `Registered []string`, `Active []string`
- [ ] 0.2.6 定义 `BootMemoryInfo` 结构体
  - `VFSRoot string`, `SegmentData string`
- [ ] 0.2.7 定义 `BootSearchGuide` 结构体
  - `Pattern string`, `Skills string`, `Memory string`, `Plugins string`, `Sessions string`
- [ ] 0.2.8 实现 `LoadBootFile(path string) (*BootFile, error)`
  - 读取 JSON → 反序列化 → 校验 version
  - 文件不存在或损坏 → 返回 nil, nil（让调用方走全量扫描路径）
- [ ] 0.2.9 实现 `SaveBootFile(path string, boot *BootFile) error`
  - 原子写: 写临时文件 → os.Rename
  - 更新 `UpdatedAt`
- [ ] 0.2.10 实现 `UpdateBootSession(path string, session *BootSession) error`
  - 读取 → 更新 LastSession → 保存
- [ ] 0.2.11 实现 `UpdateBootSkillsInfo(path string, info BootSkillsInfo) error`
  - 读取 → 更新 SystemMap.Skills → 保存
- [ ] 0.2.12 实现 `DefaultSearchGuide() BootSearchGuide`
  - 返回默认的搜索指南字符串（中文描述各数据类型的检索模式）

**文件**: `backend/internal/memory/uhms/config.go`

- [ ] 0.2.13 `UHMSConfig` 新增 `BootFilePath string` 字段
- [ ] 0.2.14 `DefaultUHMSConfig()` 设置默认值: `~/.openacosmi/memory/boot.json`
- [ ] 0.2.15 新增 `ResolvedBootFilePath() string` 方法

**文件**: `backend/pkg/types/types_memory.go`

- [ ] 0.2.16 `MemoryUHMSConfig` 同步新增 `BootFilePath` 字段

### 0.3 Qdrant Segment 统一检索接口

**目标**: 提供统一的 `Index()` / `Search()` / `Delete()` 跨所有 Collection 使用。

**文件**: `backend/internal/memory/uhms/vectoradapter/adapter.go`

- [ ] 0.3.1 `memoryCollections()` 重构 → `allCollections()` 同时返回 `mem_*` + `sys_*` collections
  - sys_skills (Phase 1)
  - sys_plugins (Phase 2 预留)
  - sys_sessions (Phase 3 预留)
  - 现有 mem_* collections 不变
- [ ] 0.3.2 `NewSegmentVectorIndex()` 中循环 `allCollections()` 创建所有 collection

**文件**: `backend/internal/memory/uhms/vectoradapter/segment_pure.go` (或对应 FFI 版)

- [ ] 0.3.3 确认 `Upsert()` 对 `sys_*` collection 的 payload 兼容性
  - payload 中含 `vfs_path`, `name`, `category`, `tags` 等字符串字段
  - 验证 payload JSON 序列化/反序列化正确

**文件**: `backend/internal/memory/uhms/interfaces.go`

- [ ] 0.3.4 `VectorHit` 新增 `Payload map[string]interface{}` 字段（可选，用于返回检索元数据）
- [ ] 0.3.5 考虑是否需要新增 `VectorIndex.SearchWithPayload()` 或在现有 `Search()` 中增加 payload 返回

**文件**: `backend/internal/memory/uhms/manager.go`

- [ ] 0.3.6 新增统一方法 `IndexSystemEntry(ctx, collection, id string, payload map[string]interface{}) error`
  - 调用 VectorIndex.Upsert，无需 embedding vector（payload-only 检索）
- [ ] 0.3.7 新增统一方法 `SearchSystem(ctx, collection, query string, topK int) ([]SystemSearchHit, error)`
  - payload filter 检索（Qdrant segment payload index）
  - 返回 `SystemSearchHit { ID, Payload, VFSPath string, Score float64 }`
- [ ] 0.3.8 新增统一方法 `DeleteSystemEntry(ctx, collection, id string) error`

### 0.4 Qdrant Segment Payload 检索能力

**目标**: 确保 Qdrant Segment 支持 payload field index + filter 检索（不依赖 embedding vector）。

**文件**: `backend/internal/memory/uhms/vectoradapter/segment_pure.go`

- [ ] 0.4.1 验证 `create_field_index()` 对 Keyword 类型字段的支持
  - 需要的 index: `category` (Keyword), `name` (Keyword), `tags` (Keyword)
- [ ] 0.4.2 验证 / 实现 payload filter 查询
  - 按 category 过滤: `filter: { must: [{ key: "category", match: { value: "tools" } }] }`
  - 按 tags 包含: `filter: { should: [{ key: "tags", match: { value: "web" } }] }`
- [ ] 0.4.3 验证 / 实现全文 payload 搜索（name + description 匹配 query）
  - 如 Qdrant segment 不直接支持 full-text payload search，则在 Go 层做二次过滤
- [ ] 0.4.4 实现 `SearchByPayload(collection string, filters map[string]interface{}, topK int) ([]PayloadHit, error)`
  - 返回 `PayloadHit { ID string, Payload map[string]interface{}, Score float32 }`

### 0.5 Gateway Boot 集成

**目标**: Gateway 启动时读取 Boot 文件，决定走快速路径还是全量扫描。

**文件**: `backend/internal/gateway/boot.go`

- [ ] 0.5.1 `initState()` 或 `NewGatewayState()` 中增加 Boot 文件加载逻辑
  - 读取 boot.json → 成功则设置 `state.Boot = boot`
  - 失败/不存在 → `state.Boot = nil`（后续走全量扫描）
- [ ] 0.5.2 Gateway shutdown 时更新 `Boot.LastSession`

**文件**: `backend/internal/gateway/server.go`

- [ ] 0.5.3 `GatewayState` 新增 `Boot *uhms.BootFile` 字段
- [ ] 0.5.4 `GatewayState` 新增 `BootFilePath string` 字段（从 config 读取）
- [ ] 0.5.5 Segment store 初始化：确保 `sys_*` collections 在启动时创建

---

## Phase 1: 技能分级加载

> 核心功能。将 69+ 技能从全量加载转为 Boot + Qdrant 检索 + VFS 分级提取。

### 1.1 技能解析 + VFS 分级写入

**目标**: 解析 SKILL.md → 生成 L0/L1/L2 → 写入 VFS `_system/skills/`。

**文件**: `backend/internal/agents/skills/skill_distributor.go` (**新建**)

- [ ] 1.1.1 定义 `SkillDistributeResult` 结构体
  - `Indexed int`, `Skipped int`, `Errors []string`, `Duration time.Duration`
- [ ] 1.1.2 实现 `DistributeSkills(ctx, vfs VFS, entries []SkillEntry) (*SkillDistributeResult, error)`
  - 遍历所有 SkillEntry
  - 对每个技能调用 `distributeOneSkill()`
  - 返回汇总结果
- [ ] 1.1.3 实现 `distributeOneSkill(vfs VFS, entry SkillEntry) error`
  - 解析 frontmatter 获取 name, description, category, tags
  - 计算 content_hash (MD5 of Content)
  - 检查 meta.json → content_hash 未变则 skip（增量）
  - 生成 L0: 摘要（name + description + tags，~100 tokens）
  - 生成 L1: 概览（frontmatter + 使用场景 + 关键命令，~2K tokens）
  - 生成 L2: 完整 SKILL.md 原文
  - 调用 `vfs.WriteSystemEntry("skills", category, name, l0, l1, l2, meta)`
- [ ] 1.1.4 实现 `generateSkillL0(entry SkillEntry) string`
  - 格式: `{name}: {description} [tags: {tags}]`
  - 控制在 ~100 tokens
- [ ] 1.1.5 实现 `generateSkillL1(entry SkillEntry) string`
  - 格式: frontmatter 完整信息 + SKILL.md 前 2K tokens
  - 包含使用场景、触发条件、关键配置
- [ ] 1.1.6 实现 `generateSkillL2(entry SkillEntry) string`
  - 完整 SKILL.md 内容
- [ ] 1.1.7 实现 `computeContentHash(content string) string`
  - MD5 hex string

### 1.2 技能 Qdrant 索引入库

**目标**: 每个技能写入 Qdrant `sys_skills` collection，建立 field index。

**文件**: `backend/internal/agents/skills/skill_distributor.go` (续)

- [ ] 1.2.1 `DistributeSkills()` 新增参数: `vectorIndex VectorIndex`
  - 对每个技能: Upsert payload 到 `sys_skills`
- [ ] 1.2.2 实现 `indexSkillToQdrant(ctx, vectorIndex, entry SkillEntry, vfsPath string) error`
  - 生成确定性 UUID (基于 skill name hash)
  - payload: `{ name, category, description, tags, vfs_path, content_hash, distributed: true }`
  - 调用 `vectorIndex.Upsert(ctx, "sys_skills", uuid, nil, payload)` (无 embedding vector)
- [ ] 1.2.3 首次分级时创建 field index
  - `create_field_index("sys_skills", "category", Keyword)`
  - `create_field_index("sys_skills", "name", Keyword)`
  - `create_field_index("sys_skills", "tags", Keyword)`
- [ ] 1.2.4 分级完成后调用 `flush()` 持久化

### 1.3 `skills.distribute` RPC 方法

**目标**: 前端 "一键 VFS 分级" 按钮的后端入口。

**文件**: `backend/internal/gateway/server_methods_skills.go`

- [ ] 1.3.1 `SkillsHandlers()` 新增 `"skills.distribute": handleSkillsDistribute`
- [ ] 1.3.2 实现 `handleSkillsDistribute(ctx *MethodHandlerContext)`
  - 参数: 无（分级所有技能）
  - 调用 `skills.LoadSkillEntries()` 获取全部技能
  - 调用 `skills.DistributeSkills(ctx, vfs, vectorIndex, entries)`
  - 更新 Boot 文件: `uhms.UpdateBootSkillsInfo(bootPath, info)`
  - 返回 `{ indexed: N, skipped: M, duration: "Xs" }`
- [ ] 1.3.3 错误处理: 分级失败不影响现有功能

**文件**: `backend/internal/gateway/server_methods.go`

- [ ] 1.3.4 将 `"skills.distribute"` 加入权限列表（与 `skills.install` 同级）

### 1.4 `skills.status` 返回分级状态

**目标**: 前端能知道每个技能是否已 VFS 分级。

**文件**: `backend/internal/gateway/server_methods_skills.go`

- [ ] 1.4.1 `handleSkillsStatus()` 中为每个技能检查 VFS 分级状态
  - 检查 `_system/skills/{category}/{name}/meta.json` 是否存在
  - 如果存在，读取 `distributed: true` + `distributedAt` 时间戳
- [ ] 1.4.2 响应中每个 skill entry 新增 `distributed` 和 `distributedAt` 字段

### 1.5 运行时: Boot 模式分支 (attempt_runner)

**目标**: 启动时如果 Boot 存在，跳过全量缓存构建，改用 Qdrant 检索 + VFS 按需加载。

**文件**: `backend/internal/agents/runner/attempt_runner.go`

- [ ] 1.5.1 `buildSystemPrompt()` 中新增 Boot 模式判断
  - 有 Boot 且 `Boot.SystemMap.Skills.Indexed == true` → Boot 模式
  - 否则 → 现有逻辑（全量 skillsCache + FormatSkillIndex）
- [ ] 1.5.2 Boot 模式: 不构建 `skillsCache` map
  - 不调用 `snap.ResolvedSkills` 遍历构建 map
  - 改为: 注入 Boot 的 `search_guide` 到 system prompt
- [ ] 1.5.3 Boot 模式: system prompt 注入 Qdrant 检索指令
  - 替换原 69 条索引 → 注入检索模式描述 + `search_skills(query)` tool 说明
  - 固定 ~300 tokens（不随技能数增长）
- [ ] 1.5.4 新增 `r.bootMode bool` 字段标记当前模式
  - 影响 `lookup_skill` 行为（见 1.6）

### 1.6 运行时: lookup_skill VFS 集成 (tool_executor)

**目标**: Boot 模式下 `lookup_skill` 从 VFS 读取而非内存 cache。

**文件**: `backend/internal/agents/runner/tool_executor.go`

- [ ] 1.6.1 `executeLookupSkill()` 新增 Boot 模式分支
  - 非 Boot 模式: 现有逻辑（从 `params.SkillsCache` 读取）
  - Boot 模式: 从 VFS 读取
- [ ] 1.6.2 Boot 模式 lookup 逻辑:
  - 先读 L1（概览 ~2K tokens）→ 判断是否够用
  - 如需完整内容 → 读 L2
  - 路径: `vfs.ReadSystemL1("skills", category, name)` / `ReadSystemL2(...)`
- [ ] 1.6.3 错误兜底: VFS 读取失败 → 回退到全量 skillsCache（如果存在）

### 1.7 运行时: search_skills 新 tool

**目标**: LLM 通过 Qdrant 检索技能而非从 69 条索引中匹配。

**文件**: `backend/internal/agents/runner/tool_executor.go`

- [ ] 1.7.1 新增 `search_skills` tool 定义
  - 输入: `{ query: string, top_k?: number }`
  - 输出: 匹配的技能 L0 摘要列表
- [ ] 1.7.2 实现 `executeSearchSkills(ctx, params, input) (string, error)`
  - 调用 `manager.SearchSystem(ctx, "sys_skills", input.Query, topK)`
  - 对每个 hit: `vfs.ReadSystemL0("skills", category, name)`
  - 格式化返回: `[1] browser: Integrated browser control... [2] ...`
- [ ] 1.7.3 在 Boot 模式下注册 `search_skills` tool
  - 非 Boot 模式不注册（保持现有行为）
- [ ] 1.7.4 `search_skills` 与 `lookup_skill` 协作:
  - `search_skills` → L0 摘要列表 → LLM 选择
  - `lookup_skill` → L1/L2 详情 → LLM 使用

### 1.8 降级策略

**目标**: Qdrant 不可用时优雅降级，最差回到现有逻辑。

**文件**: `backend/internal/agents/runner/attempt_runner.go`

- [ ] 1.8.1 Boot 模式启动检测:
  - Qdrant segment store 可用 → Level 0 (正常)
  - Qdrant 不可用但 VFS 存在 → Level 1 (VFS 直读 + FTS5)
  - VFS 也不存在 → Level 2 (全量扫描，现有逻辑)
- [ ] 1.8.2 Level 1 降级: `search_skills` 回退到 FTS5 关键词匹配
  - 使用 SQLite FTS5 搜索技能元数据（需入库时同步写 FTS5）
- [ ] 1.8.3 Level 2 降级: 完全回退到现有 `LoadSkillEntries()` + `FormatSkillIndex()`
  - 代码路径保持不变，无需修改

### 1.9 前端: 技能中心 UI 改造

**目标**: 新增 "一键 VFS 分级" 按钮 + VFS 分级状态胶囊。

#### 1.9.1 类型定义

**文件**: `ui/src/ui/types.ts`

- [ ] 1.9.1.1 `SkillStatusEntry` 新增:
  - `distributed?: boolean` — 是否已 VFS 分级
  - `distributedAt?: string` — 分级时间 ISO string

#### 1.9.2 控制器

**文件**: `ui/src/ui/controllers/skills.ts`

- [ ] 1.9.2.1 `SkillsState` 新增:
  - `distributeLoading: boolean` — 分级进行中
  - `distributeResult: string | null` — 结果消息
- [ ] 1.9.2.2 新增 `distributeSkills(state: SkillsState)` 函数
  - 调用 `state.client.request("skills.distribute", {})`
  - 进度态: `state.distributeLoading = true`
  - 完成后: `loadSkills(state)` 刷新 + 设置 `distributeResult`
  - 错误: 设置 `state.skillsError`
- [ ] 1.9.2.3 `SkillsState` 初始值添加 `distributeLoading: false, distributeResult: null`

#### 1.9.3 视图

**文件**: `ui/src/ui/views/skills.ts`

- [ ] 1.9.3.1 `SkillsProps` 新增:
  - `distributeLoading: boolean`
  - `distributeResult: string | null`
  - `onDistribute: () => void`
- [ ] 1.9.3.2 `renderSkills()` 顶部按钮区:
  - 在 `[刷新]` 按钮旁新增 `[一键 VFS 分级]` 按钮
  - disabled 态: `props.distributeLoading`
  - 加载中文字: "分级中..."
  - 点击回调: `props.onDistribute()`
- [ ] 1.9.3.3 分级结果提示:
  - `props.distributeResult` 非空时显示 callout 提示
  - "已完成 VFS 分级: 71 个技能" (success 样式)
- [ ] 1.9.3.4 `renderSkill()` chip-row 新增 VFS 分级胶囊:
  - `skill.distributed === true` → `<span class="chip chip-ok">已VFS分级</span>`
  - `skill.distributed !== true` → `<span class="chip chip-warn">未分级</span>`
  - 位置: 在 `eligible/blocked` chip 之后

#### 1.9.4 主应用集成

**文件**: `ui/src/ui/app.ts`

- [ ] 1.9.4.1 `@state()` 新增: `distributeLoading`, `distributeResult`
- [ ] 1.9.4.2 技能中心渲染时传递新 props

**文件**: `ui/src/ui/app-view-state.ts`

- [ ] 1.9.4.3 `AppViewState` 类型同步新增字段

### 1.10 Boot 文件更新时机

**目标**: 保持 Boot 文件与系统状态同步。

**文件**: `backend/internal/gateway/boot.go` + 各调用点

- [ ] 1.10.1 一键分级完成后: 更新 `Boot.SystemMap.Skills` (indexed=true, count, categories, lastIndexedAt)
- [ ] 1.10.2 技能变更检测: 如 SKILL.md 内容变化 (content_hash)，增量更新 VFS + Qdrant + Boot
- [ ] 1.10.3 会话结束时: 更新 `Boot.LastSession` (summary, activeTasks, endedAt)
- [ ] 1.10.4 Boot 文件校验失败时: 删除 → 下次启动走全量扫描 → 重新生成

### 1.11 测试

- [ ] 1.11.1 单元测试: `vfs_test.go` — `_system/` 命名空间 CRUD
- [ ] 1.11.2 单元测试: `boot_test.go` — Boot 文件读写 + 更新 + 损坏恢复
- [ ] 1.11.3 单元测试: `skill_distributor_test.go` — 解析 + L0/L1/L2 生成 + 增量跳过
- [ ] 1.11.4 集成测试: `skills.distribute` RPC 端到端
- [ ] 1.11.5 集成测试: Boot 模式下 `search_skills` → L0 → `lookup_skill` → L1/L2 完整流水线
- [ ] 1.11.6 降级测试: Qdrant 不可用时自动回退到 FTS5 → 全量扫描

---

## Phase 2: 插件分级

> 在 Phase 1 基础设施上扩展，将插件信息纳入全局检索提取循环。

### 2.1 插件 VFS 分级写入

**文件**: 待定（可能新建 `plugin_distributor.go` 或集成到现有插件注册流程）

- [ ] 2.1.1 插件注册时自动生成 L0/L1/L2
  - L0: 插件名 + 描述 + 支持的操作
  - L1: 配置详情 + API 使用方法
  - L2: 完整插件文档
- [ ] 2.1.2 写入 VFS `_system/plugins/{pluginID}/l0.txt|l1.txt|l2.txt|meta.json`
- [ ] 2.1.3 Qdrant Upsert 到 `sys_plugins` collection
  - payload: `{ name, type, description, capabilities, vfs_path }`

### 2.2 插件运行时检索

- [ ] 2.2.1 智能体需要外部通道时 → 检索 `sys_plugins` → L0 → 选择 → L1
- [ ] 2.2.2 新增 `search_plugins` tool (与 `search_skills` 同模式)
- [ ] 2.2.3 Boot 文件 `SystemMap.Plugins` 更新

### 2.3 测试

- [ ] 2.3.1 插件分级写入 + 检索端到端测试
- [ ] 2.3.2 降级测试

---

## Phase 3: 会话上下文归档

> 利用现有 `WriteArchive()` 基础，将会话归档纳入全局检索提取循环。

### 3.1 会话归档入库

**文件**: `backend/internal/memory/uhms/session_committer.go` + `manager.go`

- [ ] 3.1.1 会话结束时自动生成 L0/L1/L2 摘要
  - L0: "本次讨论了 X, Y, Z" (~100 tokens)
  - L1: 每个话题的摘要 + 关键结论 (~2K tokens)
  - L2: 完整对话记录 (已有 `WriteArchive()`)
- [ ] 3.1.2 Qdrant Upsert 到 `sys_sessions` collection
  - payload: `{ session_id, user_id, topics, ended_at, vfs_path }`
- [ ] 3.1.3 VFS 路径: `{userID}/archives/{sessionID}/` (已有结构)

### 3.2 会话归档运行时检索

- [ ] 3.2.1 "上次讨论了什么" → 检索 `sys_sessions` → L0 摘要 → L1 详情
- [ ] 3.2.2 新增 `search_sessions` tool (与 `search_skills` 同模式)
- [ ] 3.2.3 Boot 文件 `LastSession` 自动更新

### 3.3 测试

- [ ] 3.3.1 会话归档入库 + 检索端到端测试
- [ ] 3.3.2 降级测试

---

## 依赖关系

```
Phase 0.1 (VFS _system)  ──┐
Phase 0.2 (Boot 文件)      ├──→ Phase 1 (技能) ──→ Phase 2 (插件) ──→ Phase 3 (会话)
Phase 0.3 (Qdrant 统一接口) │
Phase 0.4 (Payload 检索)   ─┘
Phase 0.5 (Gateway 集成)   ──→ Phase 1
```

Phase 0 所有子任务必须在 Phase 1 开始前完成。
Phase 2 和 Phase 3 可独立进行，但都依赖 Phase 1 验证统一接口可行性。

---

## 改动文件汇总

### 新建文件

| 文件 | Phase | 说明 |
|------|-------|------|
| `uhms/boot.go` | 0.2 | Boot 文件管理 |
| `agents/skills/skill_distributor.go` | 1.1-1.2 | 技能解析 + VFS/Qdrant 入库 |

### 修改文件

| 文件 | Phase | 改动 |
|------|-------|------|
| `uhms/vfs.go` | 0.1 | `_system/` 命名空间 CRUD |
| `uhms/interfaces.go` | 0.1, 0.3 | VFS 接口 + VectorHit 扩展 |
| `uhms/types.go` | 0.1 | SystemEntryRef, SystemL0Entry |
| `uhms/config.go` | 0.2 | BootFilePath 字段 |
| `uhms/manager.go` | 0.3 | IndexSystemEntry / SearchSystem / DeleteSystemEntry |
| `vectoradapter/adapter.go` | 0.3 | sys_* collections 注册 |
| `vectoradapter/segment_pure.go` | 0.4 | payload filter 查询 |
| `gateway/boot.go` | 0.5 | Boot 加载 + shutdown 更新 |
| `gateway/server.go` | 0.5 | GatewayState Boot 字段 |
| `gateway/server_methods_skills.go` | 1.3, 1.4 | skills.distribute + status 扩展 |
| `gateway/server_methods.go` | 1.3 | 权限列表 |
| `agents/runner/attempt_runner.go` | 1.5, 1.8 | Boot 模式分支 + 降级 |
| `agents/runner/tool_executor.go` | 1.6, 1.7 | lookup_skill VFS + search_skills |
| `pkg/types/types_memory.go` | 0.2 | BootFilePath 字段 |
| `ui/src/ui/types.ts` | 1.9 | distributed 字段 |
| `ui/src/ui/views/skills.ts` | 1.9 | 一键分级按钮 + 胶囊角标 |
| `ui/src/ui/controllers/skills.ts` | 1.9 | distributeSkills 函数 |
| `ui/src/ui/app.ts` | 1.9 | 新 state + props 传递 |
| `ui/src/ui/app-view-state.ts` | 1.9 | AppViewState 类型同步 |

### 测试文件

| 文件 | Phase | 说明 |
|------|-------|------|
| `uhms/vfs_test.go` | 1.11 | _system CRUD |
| `uhms/boot_test.go` | 1.11 | Boot 文件 |
| `agents/skills/skill_distributor_test.go` | 1.11 | 分级逻辑 |
| 集成测试 (位置待定) | 1.11 | 端到端 |

---

## 风险与缓解

| 风险 | 级别 | 缓解措施 |
|------|------|---------|
| Qdrant Segment payload 检索不支持 full-text | 中 | Go 层二次过滤 / FTS5 兜底 |
| Boot 文件与实际状态不一致 | 低 | content_hash 校验 + 损坏即删 |
| 技能数增长超预期 (>500) | 低 | Qdrant segment mmap 水平扩展 |
| VFS 磁盘占用 | 低 | 69 技能 ~2.2MB，可忽略 |
| 降级路径测试不足 | 中 | 明确 3 级降级 + 每级独立测试 |
| 前端改动破坏现有技能中心 | 低 | 新增字段均 optional，不影响未分级状态 |

---

## 进度概览

| Phase | 状态 | 进度 |
|-------|------|------|
| Phase 0: 共享基础设施 | ❌ 待实现 | 0/27 |
| Phase 1: 技能分级加载 | ❌ 待实现 | 0/40 |
| Phase 2: 插件分级 | ❌ 待规划 | 0/6 |
| Phase 3: 会话归档 | ❌ 待规划 | 0/6 |
| **总计** | | **0/79** |
