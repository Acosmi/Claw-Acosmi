---
document_type: Tracking
status: In Progress
created: 2026-02-27
last_updated: 2026-02-27
audit_report: Pending
skill5_verified: false
---

# 全局 Boot 启动 + Qdrant 按需检索提取 + VFS 分级按需加载 — 实施跟踪

> 对应设计文档: `docs/claude/deferred/global-boot-skill-vectorized-loading.md`
>
> **核心目标**: 智能体运行时统一检索模式——需要什么就检索什么、需要多少就给多少。
> 技能加载从 O(n) 全量注入（~1500 tokens/次）→ O(1) 按需检索（~300 tokens/次），节省 80%。

---

## 当前状态摘要（分析基础）

### 已有基础（可复用）
| 能力 | 文件 | 状态 |
|------|------|------|
| VFS L0/L1/L2 读写 | `uhms/vfs.go` | ✅ 完整，支持 user 命名空间 |
| Manager 核心 | `uhms/manager.go` | ✅ 完整，但仅处理用户记忆 |
| VectorAdapter (Qdrant Segment) | `vectoradapter/adapter.go` | ✅ 完整，仅创建 mem_* collections |
| SQLite FTS5 搜索 | `uhms/store.go` | ✅ 完整，仅用于用户记忆 |
| 技能扫描 | `agents/skills/workspace_skills.go` | ✅ 完整，全量扫描（无缓存/索引） |
| 技能快照注入 | `agents/runner/attempt_runner.go:564-603` | ✅ 完整，`FormatSkillIndex` 紧凑格式（仍全量） |
| lookup_skill 工具 | `attempt_runner.go:630-636` | ✅ 完整，基于 `r.skillsCache` map |
| UHMS RPC | `gateway/server_methods_uhms.go` | ✅ 完整，无 skills.distribute |
| 技能 UI | `ui/src/ui/views/skills.ts` | ✅ 完整，无分级相关 |

### 关键缺口
1. **VFS `_system/` 命名空间**不存在 — 技能/插件无法存入 VFS
2. **Boot 文件**不存在 — 无全局系统地图，重启必须全量扫描
3. **Qdrant `sys_skills` collection** 不存在 — 无法按需检索技能
4. **skills.distribute RPC** 不存在 — 无入库触发点
5. **attempt_runner.go** 无 Boot 模式分支 — 仍全量加载所有技能
6. **前端** 无一键分级按钮和分级角标

---

## Skill 5: 在线验证需求清单（实施前必须完成）

> `skill5_verified: false` — **所有标 ⚠️ 项必须验证后再实施对应代码**

- [ ] ⚠️ **Qdrant Segment payload filter 查询** — 确认 `segment_cgo.go`/`segment_pure.go` 是否支持无向量 payload 搜索
  - 查询: `qdrant-engine segment scroll filter payload` + `man7.org FTS5`
  - 备选: 若 Qdrant 不支持，使用 SQLite FTS5 扩展表作为 `sys_skills` 索引
- [ ] ⚠️ **VFS `_system/` 路径安全** — 确认 `_system` 前缀不会与用户 ID 冲突（用户不能注册 `_system` 作为 userID）
- [ ] **Boot JSON 原子写入** — `os.WriteFile` 在多进程环境是否安全（是否需要 `os.Rename` 原子替换）
- [ ] **Lit 前端按钮状态管理** — 确认 `@state()` + `requestUpdate()` 在分级操作中的响应行为

---

## 总体任务分解

```
Phase 0: 基础设施准备（Foundation）
  P0.1  VFS _system/ 命名空间扩展
  P0.2  Boot 文件管理（uhms/boot.go 新建）
  P0.3  UHMS Config 扩展（BootFilePath）
  P0.4  Manager 统一系统检索接口
  P0.5  VectorAdapter sys_skills + payload 搜索

Phase 1: 技能 VFS 分级（Skills Distribution）
  P1.1  SkillDistributor 分发引擎（新建）
  P1.2  skills.distribute RPC（扩展 server_methods_uhms.go）
  P1.3  attempt_runner.go Boot 模式分支
  P1.4  workspace_skills.go Boot 分支（降级保障）
  P1.5  前端: types.ts SkillStatusEntry 扩展
  P1.6  前端: views/skills.ts UI 改造
  P1.7  前端: controllers/skills.ts 状态管理

Phase 2: 插件 VFS 分级（Plugins Distribution）
  P2.1  PluginVFSGenerator 插件入库（新建）
  P2.2  sys_plugins collection 注册
  P2.3  运行时插件检索集成

Phase 3: 会话上下文归档（Session Archive Search）
  P3.1  sys_sessions collection 注册
  P3.2  CommitSession 归档增强
  P3.3  运行时会话上下文检索

全局: 集成测试
  T0    Phase 0 单元测试
  T1    Phase 1 端到端测试（技能检索 + UI）
  T2    Phase 2 插件检索测试
  T3    Phase 3 会话检索测试
```

---

## Phase 0: 基础设施准备

### P0.1 VFS `_system/` 命名空间扩展

**文件**: `backend/internal/memory/uhms/vfs.go`

**目标**: 在现有 `LocalVFS` 上增加 `_system/` 命名空间，与用户记忆路径隔离。
路径格式: `{vfsRoot}/_system/{namespace}/{category}/{name}/`

**防护**: `_system` 前缀必须在用户 ID 校验时被拒绝（防止用户记忆路径冲突）。

**新增方法**:

```go
// WriteSystemEntry 将系统条目（技能/插件/会话）写入 _system/ 命名空间。
// namespace: "skills" | "plugins" | "sessions"
// category: 子分类，e.g. "tools", "providers"
// name: 条目名称，e.g. "browser", "feishu"
func (v *LocalVFS) WriteSystemEntry(namespace, category, name string, l0, l1, l2 string, meta map[string]interface{}) error

// ReadSystemEntry 按 VFS 相对路径读取 _system/ 条目。
// relPath: e.g. "_system/skills/tools/browser"
// 复用现有 ReadByVFSPath(relPath, level)
func (v *LocalVFS) ReadSystemEntry(relPath string, level int) (string, error)
// ↑ 注意: ReadByVFSPath 已存在 (vfs.go:198)，直接复用

// ListSystemEntries 列出 namespace/category 下的所有条目。
func (v *LocalVFS) ListSystemEntries(namespace, category string) ([]VFSDirEntry, error)

// SystemEntryExists 检查条目是否已存在。
func (v *LocalVFS) SystemEntryExists(relPath string) bool

// SystemEntryHash 读取条目 meta.json 中的 content_hash，用于增量跳过。
func (v *LocalVFS) SystemEntryHash(relPath string) string
```

**内部路径 helper**:
```go
func (v *LocalVFS) systemDir(namespace, category, name string) string {
    return filepath.Join(v.root, "_system", namespace, category, name)
}

func (v *LocalVFS) relativeSystemPath(namespace, category, name string) string {
    return filepath.Join("_system", namespace, category, name)
}
```

**meta.json schema（系统条目）**:
```json
{
  "name":         "browser",
  "namespace":    "skills",
  "category":     "tools",
  "description":  "Integrated browser control...",
  "tags":         "web,automation,chrome",
  "source_path":  "docs/skills/tools/browser/SKILL.md",
  "content_hash": "a1b2c3d4",
  "distributed":  true,
  "indexed_at":   "2026-02-27T10:30:00Z"
}
```

**校验**: 在 `WriteMemory` 等现有方法入口加 `_system` 前缀保护:
```go
// 在 WriteMemory 开头:
if userID == "_system" {
    return fmt.Errorf("uhms/vfs: userID '_system' is reserved")
}
```

- [ ] 实现 `WriteSystemEntry`
- [ ] 实现 `ListSystemEntries`
- [ ] 实现 `SystemEntryExists`（读 meta.json 判断）
- [ ] 实现 `SystemEntryHash`（读 meta.json content_hash）
- [ ] 在 `WriteMemory`/`WriteArchive` 入口加 `_system` 保护
- [ ] 单元测试: 写入/读取/列举 _system 条目

---

### P0.2 Boot 文件管理

**文件**: `backend/internal/memory/uhms/boot.go`（**新建**）

**路径**: `~/.openacosmi/memory/boot.json`（与 VFSPath 同级父目录）

**数据结构**:

```go
package uhms

// BootFile 全局 Boot 文件，记录系统地图和上次会话摘要。
// 路径: ~/.openacosmi/memory/boot.json
type BootFile struct {
    Version     string     `json:"version"`      // "1.0"
    UpdatedAt   string     `json:"updated_at"`   // ISO 8601
    LastSession *BootSession `json:"last_session,omitempty"`
    SystemMap   BootSystemMap `json:"system_map"`
    SearchGuide BootSearchGuide `json:"search_guide"`
}

// BootSession 上次会话摘要。
type BootSession struct {
    Summary     string   `json:"summary"`
    ActiveTasks []string `json:"active_tasks,omitempty"`
    EndedAt     string   `json:"ended_at"`
}

// BootSystemMap 系统地图，告诉系统一切在哪。
type BootSystemMap struct {
    Skills  BootSkillsMap  `json:"skills"`
    Plugins BootPluginsMap `json:"plugins,omitempty"`
    Memory  BootMemoryMap  `json:"memory"`
}

// BootSkillsMap 技能索引地图。
type BootSkillsMap struct {
    SourceDir        string `json:"source_dir"`          // "docs/skills/"
    VFSDir           string `json:"vfs_dir"`             // "_system/skills/"
    Categories       []string `json:"categories"`        // ["general","tools","official","providers"]
    TotalCount       int    `json:"total_count"`
    Indexed          bool   `json:"indexed"`
    QdrantCollection string `json:"qdrant_collection"`   // "sys_skills"
    LastIndexedAt    string `json:"last_indexed_at,omitempty"`
}

// BootPluginsMap 插件地图。
type BootPluginsMap struct {
    Registered []string `json:"registered,omitempty"`
    Active     []string `json:"active,omitempty"`
    Indexed    bool     `json:"indexed"`
}

// BootMemoryMap 记忆系统地图。
type BootMemoryMap struct {
    VFSRoot     string `json:"vfs_root"`
    SegmentData string `json:"segment_data,omitempty"`
}

// BootSearchGuide 按需检索指南（注入智能体系统提示）。
type BootSearchGuide struct {
    Pattern  string `json:"pattern"`
    Skills   string `json:"skills"`
    Memory   string `json:"memory"`
    Plugins  string `json:"plugins"`
    Sessions string `json:"sessions"`
}
```

**BootManager 方法**:

```go
// BootManager 管理 boot.json 的读写。
type BootManager struct {
    filePath string
    mu       sync.RWMutex
    current  *BootFile
}

func NewBootManager(memoryRootDir string) *BootManager
func (bm *BootManager) Load() (*BootFile, error)           // 读取并缓存
func (bm *BootManager) Save(boot *BootFile) error          // 原子写入（tmp → rename）
func (bm *BootManager) MarkSkillsIndexed(count int) error  // 更新 skills.indexed + last_indexed_at
func (bm *BootManager) UpdateLastSession(summary string, tasks []string) error
func (bm *BootManager) IsSkillsIndexed() bool              // 快速检查（不读文件）
func (bm *BootManager) IsValid() bool                      // 版本 + 基本字段校验
func (bm *BootManager) Reset() error                       // 删除 boot.json（触发全量重扫）
```

**启动流程逻辑**:
```go
// Gateway.Start() 中调用:
//   boot = BootManager.Load()
//   if boot.IsValid() && boot.IsSkillsIndexed():
//     → 快速启动: 不扫描文件系统
//   else:
//     → 全量扫描: LoadSkillEntries() → 生成 Boot（现有逻辑保留）
```

**原子写入**:
```go
func (bm *BootManager) Save(boot *BootFile) error {
    // 写入 .boot.json.tmp → os.Rename (原子替换)
    tmpPath := bm.filePath + ".tmp"
    // ... marshal + WriteFile → Rename
}
```

- [ ] 定义所有数据结构
- [ ] 实现 `NewBootManager`
- [ ] 实现 `Load`（文件读取 + 校验）
- [ ] 实现 `Save`（原子写入 via tmp + rename）
- [ ] 实现 `MarkSkillsIndexed`
- [ ] 实现 `UpdateLastSession`
- [ ] 实现 `IsSkillsIndexed`/`IsValid`/`Reset`
- [ ] 单元测试: Load（文件不存在/损坏/正常）、Save 原子性、MarkSkillsIndexed

---

### P0.3 UHMS Config 扩展

**文件**: `backend/internal/memory/uhms/config.go`

**新增字段**:
```go
// BootFilePath is the path to the global boot file.
// Default: ~/.openacosmi/memory/boot.json
BootFilePath string `json:"bootFilePath,omitempty"`

// SkillsVFSDistribution enables VFS tiered distribution for skills.
// When enabled, skills are indexed into _system/skills/ VFS + Qdrant.
// Default: false (backward compatible)
SkillsVFSDistribution bool `json:"skillsVFSDistribution,omitempty"`
```

**新增方法**:
```go
// ResolvedBootFilePath returns the absolute boot file path.
func (c *UHMSConfig) ResolvedBootFilePath() string {
    if c.BootFilePath != "" {
        return expandHome(c.BootFilePath)
    }
    return defaultBootFilePath()
}

func defaultBootFilePath() string {
    home, _ := os.UserHomeDir()
    return filepath.Join(home, ".openacosmi", "memory", "boot.json")
}
```

- [ ] 添加 `BootFilePath` 字段
- [ ] 添加 `SkillsVFSDistribution` 字段
- [ ] 实现 `ResolvedBootFilePath()`
- [ ] 实现 `defaultBootFilePath()`
- [ ] 更新 `DefaultUHMSConfig()` 默认值

---

### P0.4 Manager 统一系统检索接口

**文件**: `backend/internal/memory/uhms/manager.go`

**目标**: 添加 `DefaultManager` 的系统级检索接口，供 Phase 1-3 所有模块调用。

**新增类型**:
```go
// SystemHit 系统检索结果（技能/插件/会话归档）。
type SystemHit struct {
    ID          string  `json:"id"`
    Name        string  `json:"name"`
    Category    string  `json:"category"`
    Description string  `json:"description"`
    Tags        string  `json:"tags"`
    VFSPath     string  `json:"vfs_path"`
    Score       float64 `json:"score"`
}

// SystemIndexEntry 入库参数。
type SystemIndexEntry struct {
    Collection  string
    ID          string
    Name        string
    Category    string
    Description string
    Tags        string
    VFSPath     string
    ContentHash string
    L0, L1, L2  string
    Meta        map[string]interface{}
}
```

**新增方法**:
```go
// IndexSystemEntry 将系统条目写入 VFS + Qdrant。
// collection: "sys_skills" | "sys_plugins" | "sys_sessions"
func (m *DefaultManager) IndexSystemEntry(ctx context.Context, entry SystemIndexEntry) error

// SearchSystem 检索系统 collection（无向量，使用 payload/FTS5 关键词匹配）。
func (m *DefaultManager) SearchSystem(ctx context.Context, collection, query string, topK int) ([]SystemHit, error)

// ReadSystemL0 读取系统 VFS 条目的 L0 摘要。
func (m *DefaultManager) ReadSystemL0(vfsPath string) (string, error)

// ReadSystemL1 读取系统 VFS 条目的 L1 概览。
func (m *DefaultManager) ReadSystemL1(vfsPath string) (string, error)

// ReadSystemL2 读取系统 VFS 条目的 L2 完整内容。
func (m *DefaultManager) ReadSystemL2(vfsPath string) (string, error)

// SystemDistributionStatus 返回指定 collection 的分级状态。
func (m *DefaultManager) SystemDistributionStatus(collection string) SystemDistStatus
```

**`IndexSystemEntry` 实现逻辑**:
```
1. m.vfs.WriteSystemEntry(namespace, category, name, l0, l1, l2, meta)
2. 如果 vectorIndex != nil: m.vectorIndex.UpsertPayload("sys_skills", id, payload) [无向量]
3. 如果 vectorIndex == nil: 写入 SQLite FTS5 扩展表 system_entries (见 P0.5 备选方案)
```

**`SearchSystem` 实现逻辑**:
```
优先级:
  1. Qdrant payload 过滤（如果 vectorIndex != nil 且支持）
  2. SQLite system_entries FTS5 搜索
  3. VFS meta.json 扫描（兜底，O(n) 但只有 69-500 条）
```

- [ ] 定义 `SystemHit`/`SystemIndexEntry` 类型
- [ ] 实现 `IndexSystemEntry`
- [ ] 实现 `SearchSystem`（三级降级策略）
- [ ] 实现 `ReadSystemL0/L1/L2`（复用 `vfs.ReadByVFSPath`）
- [ ] 实现 `SystemDistributionStatus`
- [ ] 单元测试: IndexSystemEntry + SearchSystem 关键词匹配

---

### P0.5 VectorAdapter sys_skills Collection + Payload 搜索

**文件**: `backend/internal/memory/uhms/vectoradapter/adapter.go`

**目标**: 注册 `sys_skills`/`sys_plugins`/`sys_sessions` collections，
并添加 payload-only 搜索能力（不需要向量）。

**⚠️ 设计决策点（需在 Skill 5 验证后确定）**:

**方案 A（首选）**: 扩展 `SegmentStore` payload filter 查询
```go
// SearchByPayload 通过 payload 字段关键词匹配，无需向量。
// fields: 要搜索的 payload 字段，e.g. ["name","description","tags"]
func (s *SegmentVectorIndex) SearchByPayload(
    ctx context.Context, collection, query string,
    fields []string, topK int,
) ([]uhms.SystemHit, error)
```
- 需要确认 `segment_cgo.go`/`segment_pure.go` 是否支持 scroll/filter 操作
- Pure Go fallback: 内存 BM25 on payload strings

**方案 B（备选）**: 扩展 SQLite `store.go` 增加 `system_entries` FTS5 表
```sql
CREATE VIRTUAL TABLE IF NOT EXISTS system_fts USING fts5(
    id UNINDEXED, name, category, description, tags, vfs_path,
    content=system_entries
);
```
- 复用现有 FTS5 基础设施
- 不依赖 Qdrant，但失去统一检索优势
- 方案 B 作为方案 A 的兜底

**Collection 注册（在 `NewSegmentVectorIndex` 中）**:
```go
// 系统 collections（无向量维度，payload-only）
systemCols := []string{"sys_skills", "sys_plugins", "sys_sessions"}
for _, col := range systemCols {
    if err := store.CreateCollection(col, 0); err != nil { // dim=0 表示 payload-only
        // 忽略 dim=0 不支持的错误，回退到方案 B
    }
}
```

- [ ] ⚠️ Skill 5 验证: `segment_cgo.go` 是否支持 payload-only search
- [ ] 实现方案 A `SearchByPayload`（如 Qdrant 支持）
- [ ] 实现方案 B `system_entries` FTS5 表（作为兜底，无论方案 A 是否成功）
- [ ] 注册 `sys_skills` collection
- [ ] 注册 `sys_plugins` collection（Phase 2 使用）
- [ ] 注册 `sys_sessions` collection（Phase 3 使用）
- [ ] `UpsertPayload`（无向量版 Upsert）

---

## Phase 1: 技能 VFS 分级

### P1.1 SkillDistributor 分发引擎

**文件**: `backend/internal/agents/skills/skill_distributor.go`（**新建**）

**依赖**: P0.1 (WriteSystemEntry), P0.2 (BootManager), P0.4 (IndexSystemEntry)

**数据结构**:
```go
// DistributeResult 分发结果统计。
type DistributeResult struct {
    Indexed  int           `json:"indexed"`   // 新增
    Updated  int           `json:"updated"`   // 内容变更更新
    Skipped  int           `json:"skipped"`   // hash 未变化跳过
    Failed   int           `json:"failed"`    // 失败（仅 Warn，不中断）
    Duration time.Duration `json:"duration"`  // 总耗时
    Errors   []string      `json:"errors,omitempty"` // 失败摘要
}
```

**核心方法**:
```go
// SkillDistributor 将技能目录写入 VFS + 系统索引。
type SkillDistributor struct {
    vfs         *uhms.LocalVFS    // 注入
    manager     uhms.Manager      // 注入（用于 IndexSystemEntry）
    bootManager *uhms.BootManager // 注入
    workDir     string
}

func NewSkillDistributor(vfs *uhms.LocalVFS, mgr uhms.Manager, boot *uhms.BootManager, workDir string) *SkillDistributor

// Distribute 执行一键 VFS 分级。
// entries: 通过 LoadSkillEntries() 获取的技能列表
func (d *SkillDistributor) Distribute(ctx context.Context, entries []SkillEntry) (DistributeResult, error)
```

**`Distribute` 内部步骤**:
```
对每个 SkillEntry:
  1. 解析 frontmatter: name, description, tags, category
     → ParseSkillFrontmatter(content string) SkillMeta

  2. 计算 content_hash := sha256(entry.Skill.Content)[:16]

  3. 检查 VFS 是否已存在且 hash 未变:
     vfsPath := "_system/skills/{category}/{name}"
     existingHash := vfs.SystemEntryHash(vfsPath)
     if existingHash == contentHash → skipped++; continue

  4. 生成 L0/L1/L2:
     L0 = extractL0(entry) // ~100 tokens: frontmatter description 或前 100 tokens
     L1 = extractL1(entry) // ~2K: 完整 frontmatter + 使用场景章节
     L2 = entry.Skill.Content // 完整 SKILL.md 原文

  5. 写入 VFS:
     meta = { name, category, description, tags, source_path, content_hash, distributed:true, indexed_at }
     vfs.WriteSystemEntry("skills", category, name, L0, L1, L2, meta)

  6. 写入系统索引（Qdrant/FTS5）:
     manager.IndexSystemEntry(ctx, SystemIndexEntry{
       Collection:  "sys_skills",
       ID:          deterministicUUID(name),
       Name:        name,
       Category:    category,
       Description: description,
       Tags:        tags,
       VFSPath:     vfsPath,
       ContentHash: contentHash,
       L0:L1:L2:    已生成
     })

  7. indexed++ 或 updated++

完成后:
  bootManager.MarkSkillsIndexed(indexed+updated+skipped) // 更新 boot.json
```

**辅助函数**:
```go
// ParseSkillFrontmatter 解析 SKILL.md frontmatter。
// 返回 name, description, tags, category。
func ParseSkillFrontmatter(content string) SkillMeta

// SkillMeta frontmatter 解析结果。
type SkillMeta struct {
    Name        string
    Description string
    Tags        []string
    Category    string // 从目录结构推断（tools/providers/general/official）
}

// deterministicUUID 根据名称生成确定性 UUID（SHA1 namespace）。
func deterministicUUID(name string) string

// extractL0FromSkill 从 SKILL.md 提取 L0 摘要（~100 tokens）。
// 优先使用 frontmatter description，否则取正文前 100 tokens。
func extractL0FromSkill(entry SkillEntry, meta SkillMeta) string

// extractL1FromSkill 从 SKILL.md 提取 L1 概览（~2K tokens）。
// frontmatter + ## Overview/## Usage/## Examples 章节，不超过 2000 tokens。
func extractL1FromSkill(content string) string
```

**Category 推断逻辑**（从文件路径）:
```go
// 路径 "docs/skills/tools/browser" → category = "tools"
// 路径 "docs/skills/providers/feishu" → category = "providers"
// 无匹配子目录 → category = "general"
func inferCategory(skillDir string) string
```

- [ ] 定义 `DistributeResult`、`SkillMeta` 类型
- [ ] 实现 `ParseSkillFrontmatter`（扩展现有 `extractDescription` 逻辑）
- [ ] 实现 `deterministicUUID`（crypto/sha1 + UUID namespace）
- [ ] 实现 `extractL0FromSkill`
- [ ] 实现 `extractL1FromSkill`（章节提取）
- [ ] 实现 `inferCategory`
- [ ] 实现 `NewSkillDistributor`
- [ ] 实现 `Distribute` 核心循环
- [ ] 单元测试: 完整分发流程（mock VFS + mock Manager）
- [ ] 单元测试: hash 增量跳过
- [ ] 单元测试: frontmatter 解析（含各 category 样本）

---

### P1.2 skills.distribute RPC

**文件**: `backend/internal/gateway/server_methods_uhms.go`

**新增 RPC**: `skills.distribute`

**注册**（在 `UHMSHandlers()` 中添加）:
```go
"skills.distribute": handleSkillsDistribute,
"skills.distribution.status": handleSkillsDistributionStatus,
```

**`handleSkillsDistribute` 实现**:
```go
func handleSkillsDistribute(ctx *MethodHandlerContext) {
    // 1. 检查 UHMS Manager 可用性
    mgr := ctx.Context.UHMSManager
    if mgr == nil {
        ctx.Respond(false, nil, NewErrorShape(ErrCodeServiceUnavailable, "UHMS not enabled"))
        return
    }

    // 2. 加载技能列表
    workDir := ctx.Context.StorePath   // 或专用字段
    bundledDir := skills.ResolveBundledSkillsDir("")
    entries := skills.LoadSkillEntries(workDir, "", bundledDir, resolveConfigFromContext(ctx))

    // 3. 执行分发（同步，前端显示进度）
    distributor := skills.NewSkillDistributor(
        ctx.Context.UHMSManager.VFS(),
        ctx.Context.UHMSManager,
        ctx.Context.BootManager,
        workDir,
    )
    result, err := distributor.Distribute(request.Context(), entries)

    // 4. 返回结果
    if err != nil {
        ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, err.Error()))
        return
    }
    ctx.Respond(true, map[string]interface{}{
        "indexed":    result.Indexed,
        "updated":    result.Updated,
        "skipped":    result.Skipped,
        "failed":     result.Failed,
        "durationMs": result.Duration.Milliseconds(),
        "errors":     result.Errors,
    }, nil)
}
```

**`handleSkillsDistributionStatus` 实现**:
```go
// 返回当前分级状态（来自 boot.json）
// Response: { indexed: bool, totalCount: int, lastIndexedAt: string, vfsDir: "_system/skills/" }
```

**依赖**: P0.2 (BootManager), P1.1 (SkillDistributor)

**Gateway 状态注入**（`boot.go` 或 `server.go`）:
- `GatewayState` 需增加 `BootManager *uhms.BootManager` 字段
- 在 `Gateway.Start()` 初始化时创建 `BootManager`

- [ ] 在 `GatewayContext`/`GatewayState` 添加 `BootManager` 字段
- [ ] 在 `boot.go` 或 `server.go` Gateway 启动时初始化 `BootManager`
- [ ] 实现 `handleSkillsDistribute`
- [ ] 实现 `handleSkillsDistributionStatus`
- [ ] 注册两个 RPC 到 `UHMSHandlers()`
- [ ] 确认 `UHMSManager.VFS()` 方法是否存在（如不存在需在 manager.go 添加）

---

### P1.3 attempt_runner.go Boot 模式分支

**文件**: `backend/internal/agents/runner/attempt_runner.go`

**目标**: 在 `buildSystemPrompt` 中添加 Boot 模式分支。
当 Boot 文件存在且技能已分级时，改用 Qdrant 检索 + VFS L0 摘要注入（~300 tokens）。

**修改位置**: `buildSystemPrompt`（当前 L564-603）

**新增结构**:
```go
// EmbeddedAttemptRunner 新增字段：
UHMSManager uhms.Manager  // 注入（用于系统检索）
BootManager *uhms.BootManager // 注入（用于 Boot 状态检查）
```

**Boot 模式逻辑**:
```go
func (r *EmbeddedAttemptRunner) buildSkillsSection(params AttemptParams, userMessage string) (skillsPrompt string) {
    // 优先尝试 Boot 模式
    if r.BootManager != nil && r.BootManager.IsSkillsIndexed() &&
       r.UHMSManager != nil && params.WorkspaceDir != "" {

        // 检索 top-3 相关技能
        hits, err := r.UHMSManager.SearchSystem(context.Background(), "sys_skills", userMessage, 3)
        if err == nil && len(hits) > 0 {
            // 读取 L0 摘要并构建紧凑索引（~300 tokens）
            var sb strings.Builder
            sb.WriteString("<available_skills>\n")
            for _, hit := range hits {
                l0, _ := r.UHMSManager.ReadSystemL0(hit.VFSPath)
                if l0 == "" {
                    l0 = hit.Description
                }
                sb.WriteString(fmt.Sprintf("- %s: %s\n", hit.Name, l0))
            }
            sb.WriteString("</available_skills>")

            // 更新 skillsCache（lookup_skill 工具仍然可用）
            r.skillsCache = make(map[string]string, len(hits))
            for _, hit := range hits {
                r.skillsCache[hit.Name] = hit.VFSPath // L2 按需读取
            }

            return sb.String()
        }
        // Boot 模式检索失败 → 降级到全量扫描
    }

    // 降级: 现有全量扫描逻辑（完整保留）
    return r.buildSkillsSectionFallback(params)
}
```

**lookup_skill 工具增强**（支持 VFS L2 按需读取）:
```go
// 当前: skillsCache[name] = 完整 SKILL.md 内容（全量加载）
// Boot 模式: skillsCache[name] = VFS path
// lookup_skill 执行时:
//   若 value 是 VFS path（以 "_system/" 开头）→ ReadSystemL2(path)
//   否则 → 直接返回（向后兼容）
```

**对 `buildSystemPrompt` 的改动**:
```go
// 原: (L570-591) 直接全量扫描 + 全量缓存
// 改: 调用 buildSkillsSection(params, userMessageForContext)
//     (userMessageForContext 从最近用户消息提取)
```

- [ ] 给 `EmbeddedAttemptRunner` 添加 `UHMSManager` / `BootManager` 字段
- [ ] 提取 `buildSkillsSectionFallback` 函数（封装现有 L570-591 逻辑，原逻辑不变）
- [ ] 实现 `buildSkillsSection`（Boot 模式 + 降级）
- [ ] 修改 `buildSystemPrompt` 调用 `buildSkillsSection`
- [ ] 修改 `lookup_skill` 工具处理器支持 VFS path 延迟读取
- [ ] 在 attempt_runner 构造处注入 `UHMSManager`/`BootManager`
- [ ] 集成测试: Boot 模式路径 + 降级路径

---

### P1.4 workspace_skills.go Boot 分支

**文件**: `backend/internal/agents/skills/workspace_skills.go`

**目标**: `LoadSkillEntries` 支持 Boot 模式（当 Boot 可用时，从 VFS meta.json 加载而非扫描文件系统）。

**新增参数**:
```go
// BuildSnapshotParams 新增字段:
BootManager *uhms.BootManager // 可选，若非空且已索引则尝试 Boot 模式
VFSManager  uhms.Manager       // 可选，供 Boot 模式读取 VFS
```

**Boot 模式分支**（`LoadSkillEntries` 内）:
```go
func LoadSkillEntries(workspaceDir, managedDir, bundledDir string,
    cfg *types.OpenAcosmiConfig, opts ...LoadSkillOptions) []SkillEntry {

    // Boot 模式: 从 VFS meta.json 加载（更快，无 I/O 扫描）
    if len(opts) > 0 && opts[0].BootManager != nil {
        boot := opts[0].BootManager
        if boot.IsSkillsIndexed() && opts[0].VFSManager != nil {
            entries, err := loadEntriesFromVFS(opts[0].VFSManager, cfg)
            if err == nil && len(entries) > 0 {
                return deduplicateEntries(entries)
            }
            // 失败 → 降级到文件扫描（下面）
        }
    }

    // 现有全量扫描逻辑（完整保留）
    // ...
}
```

> **注意**: `LoadSkillOptions` 作为可选参数传入，保持现有调用方不需修改。

- [ ] 定义 `LoadSkillOptions` 结构
- [ ] 实现 `loadEntriesFromVFS`（读取 VFS `_system/skills/` meta.json 列表）
- [ ] 修改 `LoadSkillEntries` 函数签名（variadic opts）
- [ ] 更新 `BuildWorkspaceSkillSnapshot` 透传 opts

---

### P1.5 前端: types.ts SkillStatusEntry 扩展

**文件**: `ui/src/ui/types.ts`

**新增字段**（在 `SkillStatusEntry` 接口中）:
```typescript
// 已 VFS 分级
distributed?: boolean;
distributedAt?: string;  // ISO 8601
```

- [ ] 找到 `SkillStatusEntry` 接口定义
- [ ] 添加 `distributed` 和 `distributedAt` 字段

---

### P1.6 前端: views/skills.ts UI 改造

**文件**: `ui/src/ui/views/skills.ts`

**变更 1: `SkillsProps` 添加字段**
```typescript
export type SkillsProps = {
  // ... 现有字段 ...
  distributeLoading: boolean;
  distributeResult: string | null;   // 完成消息 e.g. "已完成 VFS 分级: 71 个技能"
  distributeError: string | null;
  onDistribute: () => void;
}
```

**变更 2: 顶部操作栏新增"一键 VFS 分级"按钮**
```typescript
// 在刷新按钮旁（renderSkills 函数的顶部 actions 区域）:
html`
  <button
    class="btn btn-secondary ${props.distributeLoading ? 'btn-loading' : ''}"
    ?disabled=${props.distributeLoading}
    @click=${props.onDistribute}
  >
    ${props.distributeLoading
      ? t("skills.distribute.loading")
      : t("skills.distribute.button")}
  </button>
  ${props.distributeResult
    ? html`<span class="distribute-result">${props.distributeResult}</span>`
    : nothing}
  ${props.distributeError
    ? html`<span class="distribute-error">${props.distributeError}</span>`
    : nothing}
`
```

**变更 3: 技能卡片 chip-row 添加分级角标**
```typescript
// 在 renderSkill() / chip-row 区域，紧跟现有 chip 后:
${skill.distributed
  ? html`<span class="chip chip-ok">${t("skills.distribute.badge.done")}</span>`
  : html`<span class="chip chip-warn">${t("skills.distribute.badge.pending")}</span>`
}
```

**i18n 新增 key**（i18n 文件路径待查找）:
```
skills.distribute.button = "一键 VFS 分级"
skills.distribute.loading = "分级中..."
skills.distribute.badge.done = "已VFS分级"
skills.distribute.badge.pending = "未分级"
```

- [ ] 修改 `SkillsProps` 类型
- [ ] 实现顶部"一键 VFS 分级"按钮渲染
- [ ] 实现技能卡片分级角标渲染
- [ ] 添加 i18n key（中文 + 英文）

---

### P1.7 前端: controllers/skills.ts 状态管理

**文件**: `ui/src/ui/controllers/skills.ts`

**`SkillsState` 新增字段**:
```typescript
distributeLoading: boolean;
distributeResult: string | null;
distributeError: string | null;
```

**新增函数**:
```typescript
export async function distributeSkills(
  state: SkillsState,
  requestUpdate: () => void
): Promise<void> {
  state.distributeLoading = true;
  state.distributeResult = null;
  state.distributeError = null;
  requestUpdate();

  try {
    const result = await state.client.request("skills.distribute", {});
    const { indexed, updated, skipped, durationMs } = result as any;
    const total = indexed + updated + skipped;
    state.distributeResult = `已完成 VFS 分级: ${total} 个技能 (新增 ${indexed}, 更新 ${updated}, 跳过 ${skipped})`;
    // 刷新技能列表（获取新的 distributed 状态）
    await loadSkills(state, requestUpdate);
  } catch (err) {
    state.distributeError = `分级失败: ${err instanceof Error ? err.message : String(err)}`;
  } finally {
    state.distributeLoading = false;
    requestUpdate();
  }
}
```

**初始化默认值**（在 `createSkillsState` 或构造函数中）:
```typescript
distributeLoading: false,
distributeResult: null,
distributeError: null,
```

- [ ] 添加 `distributeLoading`/`distributeResult`/`distributeError` 到 `SkillsState`
- [ ] 实现 `distributeSkills` 函数
- [ ] 初始化默认值
- [ ] 在宿主组件（App）中绑定 `onDistribute: () => distributeSkills(state, this.requestUpdate.bind(this))`

---

## Phase 2: 插件 VFS 分级

> **依赖**: Phase 0 全部完成，Phase 1 骨架可用

### P2.1 PluginVFSGenerator

**文件**: `backend/internal/channels/plugin_vfs_generator.go`（新建）

**目标**: 插件注册时自动生成 L0/L1/L2 写入 VFS，方便智能体检索。

**触发时机**: 插件注册（`ChannelManager.Register(plugin)`）+ 插件激活

**L0/L1/L2 格式**（以飞书为例）:
```
L0: "飞书（Feishu）- 企业即时通讯渠道，支持消息/群组/卡片/审批。"
L1: "飞书插件提供企业即时通讯功能。
    能力: 发送消息、群组管理、审批卡片、Bot @触发。
    账号: feishu:default (已激活)
    配置: appId=xxx, 支持 WebSocket 长连接。"
L2: 完整插件配置 JSON（隐去 secrets）
```

- [ ] 定义 `PluginVFSGenerator` 接口 + 实现
- [ ] 在 `ChannelManager.Register` 调用 `GenerateVFS`
- [ ] 测试: 飞书插件 VFS 写入验证

### P2.2 sys_plugins Collection 注册

- [ ] 在 P0.5 中已注册 `sys_plugins` collection（确认）
- [ ] Upsert 插件元数据到 Qdrant payload index

### P2.3 运行时插件检索

**文件**: `backend/internal/agents/runner/attempt_runner.go`

**目标**: 智能体需要调用外部通道时，检索 `sys_plugins` 获取可用插件 L0 摘要。

- [ ] 添加插件检索到 `buildSystemPrompt` 插件部分
- [ ] 实现 `lookup_plugin` 工具（类似 `lookup_skill`）

---

## Phase 3: 会话上下文归档检索

> **依赖**: Phase 0 全部完成，UHMS `CommitSession` 已有实现

### P3.1 sys_sessions Collection 注册

- [ ] 在 P0.5 中已注册 `sys_sessions` collection（确认）

### P3.2 CommitSession 归档增强

**文件**: `backend/internal/memory/uhms/session_committer.go`

**目标**: 在 `CommitSession` 完成后，将会话归档额外写入 `sys_sessions` Qdrant 索引。

```go
// 在 commitSession 末尾添加:
manager.IndexSystemEntry(ctx, SystemIndexEntry{
    Collection:  "sys_sessions",
    ID:          deterministicUUID(sessionKey),
    Name:        sessionKey,
    Category:    "archive",
    Description: l0Summary, // 会话摘要
    VFSPath:     archivePath,
    ContentHash: "",         // 会话无需 hash
})
```

- [ ] 修改 `session_committer.go` 在 CommitSession 成功后写入 sys_sessions

### P3.3 运行时会话上下文检索

**文件**: `backend/internal/memory/uhms/manager.go`

- [ ] `BuildContextBlock` 可选增加 sys_sessions 检索（"上次讨论了什么"类查询）

---

## 全局集成测试

### T0: Phase 0 单元测试

- [ ] `vfs_test.go`: `_system/` 写入/读取/列举/保护
- [ ] `boot_test.go`: Load/Save/Mark/Reset/IsValid
- [ ] `manager_system_test.go`: IndexSystemEntry + SearchSystem 三级降级

### T1: Phase 1 端到端测试

- [ ] 技能分发 → VFS 文件生成 → Boot 文件更新
- [ ] attempt_runner Boot 模式 → 检索 top-3 → L0 注入 → lookup_skill L2 按需读取
- [ ] 降级路径: Boot 不可用 → 全量扫描
- [ ] UI: 一键分级按钮 → RPC 调用 → 技能角标更新

### T2: Phase 2 插件检索测试

- [ ] 插件注册 → VFS 写入 → 检索
- [ ] lookup_plugin 工具

### T3: Phase 3 会话检索测试

- [ ] 会话结束 → sys_sessions 入库
- [ ] "上次讨论了什么" → 检索 sys_sessions → L0 摘要

---

## 文件改动总览

| 文件 | 类型 | 改动说明 |
|------|------|----------|
| `uhms/vfs.go` | 扩展 | `_system/` 命名空间：WriteSystemEntry/ListSystemEntries/SystemEntryExists/SystemEntryHash + 保护 |
| `uhms/boot.go` | **新建** | BootFile/BootManager 全部方法 |
| `uhms/config.go` | 扩展 | BootFilePath/SkillsVFSDistribution 字段 + ResolvedBootFilePath() |
| `uhms/manager.go` | 扩展 | SystemHit/SystemIndexEntry 类型 + IndexSystemEntry/SearchSystem/ReadSystemL0-L2 |
| `vectoradapter/adapter.go` | 扩展 | sys_skills/sys_plugins/sys_sessions collections + SearchByPayload（或方案 B FTS5） |
| `agents/skills/skill_distributor.go` | **新建** | SkillDistributor + DistributeResult + ParseSkillFrontmatter + L0/L1提取器 |
| `agents/skills/workspace_skills.go` | 改 | LoadSkillEntries variadic opts + Boot 模式分支 + loadEntriesFromVFS |
| `gateway/server_methods_uhms.go` | 扩展 | skills.distribute + skills.distribution.status RPC |
| `gateway/server.go` 或 `boot.go` | 扩展 | GatewayState 注入 BootManager |
| `agents/runner/attempt_runner.go` | 改 | buildSkillsSection Boot 模式分支 + lookup_skill VFS 延迟读取 |
| `channels/plugin_vfs_generator.go` | **新建** | Phase 2 PluginVFSGenerator |
| `memory/uhms/session_committer.go` | 改 | Phase 3 CommitSession sys_sessions 写入 |
| `ui/src/ui/types.ts` | 扩展 | SkillStatusEntry.distributed/distributedAt |
| `ui/src/ui/views/skills.ts` | 改 | 一键分级按钮 + 分级角标 |
| `ui/src/ui/controllers/skills.ts` | 扩展 | distributeSkills() + 状态字段 |

---

## 实施顺序（推荐）

```
第 1 批（并行可行）:
  P0.3 Config 扩展   → 无依赖，5 min
  P0.2 Boot 文件管理 → 无依赖，1-2h

第 2 批（依赖 P0.3）:
  P0.1 VFS _system/  → 依赖 P0.3，1-2h
  P0.5 Skill 5 验证  → 并行研究

第 3 批（依赖 P0.1 + P0.2）:
  P0.4 Manager 接口  → 依赖 P0.1 + P0.2，1-2h
  P0.5 VectorAdapter → 依赖 Skill 5 验证结果，1h

第 4 批（依赖 P0.x 全部）:
  P1.1 SkillDistributor → 依赖全 P0，2-3h
  P1.5 前端 types.ts    → 无后端依赖，30min

第 5 批（依赖 P1.1）:
  P1.2 skills.distribute RPC → 依赖 P1.1，1h
  P1.6 前端 views/skills.ts  → 依赖 P1.5，1h

第 6 批（依赖 P1.2 + P1.6）:
  P1.3 attempt_runner Boot 模式 → 依赖 P1.1+P0.4，1-2h
  P1.4 workspace_skills Boot 分支 → 依赖 P0.2，30min
  P1.7 前端 controllers → 依赖 P1.6，30min

第 7 批（依赖 Phase 1 全部）:
  T1 Phase 1 端到端测试
  (Phase 2/3 可同步并行启动)
```

---

## 延迟项与风险

### 风险 R1: Qdrant payload-only search 支持性不确定
- **影响**: P0.5 方案选择
- **缓解**: 方案 B（SQLite FTS5）作为充分备选，两方案底层接口相同（`SearchSystem` API 不变）
- **行动**: Skill 5 验证后立即决策

### 风险 R2: L0/L1 生成质量
- **影响**: 技能检索相关性（错误 L0 导致检索到不相关技能）
- **缓解**: Phase 1 先用 frontmatter description 作 L0，后续可用 LLM 异步升级
- **参考**: UHMS `upgradeVFSSummary` 的异步升级模式

### 风险 R3: 技能检索相关性（无向量）
- **影响**: 关键词搜索可能遗漏语义相关技能
- **缓解**: Boot 模式降级保障（全量扫描兜底）+ 后续可加向量增强
- **行动**: Phase 1 完成后用真实对话测试相关性，量化 top-3 召回率

### 延迟项 D1: 向量增强技能检索
- 当用户配置了 embeddingProvider 时，对技能 L0 生成向量
- 提升语义检索精度（"调试" → 命中 "debugger"/"browser-devtools" 技能）
- 加入 `deferred/skill-vector-enhancement.md`

### 延迟项 D2: 技能内容变更监听（文件 watch）
- 当 `docs/skills/` 下文件变更时自动触发增量更新
- 目前需要手动"一键 VFS 分级"
- 加入 `deferred/skill-auto-reindex.md`

---

## Online Verification Log（Skill 5）

> 验证项完成后在此记录，更新 `skill5_verified: true`

### Qdrant Segment Payload Search
- **Query**: 待查询
- **Source**: 待填写
- **Key finding**: 待填写
- **Verified date**: 待填写

### VFS _system/ path safety
- **Query**: 待查询
- **Source**: 待填写
- **Key finding**: 待填写
- **Verified date**: 待填写
