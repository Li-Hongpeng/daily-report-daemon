# daily-report-daemon Phase 2 Agent 引擎技术架构

版本：v0.1  
日期：2026-05-29  
作者：技术负责人  
关联：Phase 0 SUMMARY、Phase 2 PRD（产品经理出）

## 1. 架构目标

Phase 0 的线性流水线（scan → sanitize → 单次 LLM → render）被 Phase 2 的 **agent 驱动多步推理**取代。核心变化：

| | Phase 0 | Phase 2 |
|---|---|---|
| 推理方式 | 单次 prompt，一次性输出 | 多步 Plan-Act-Observe-Reflect 循环 |
| 数据来源 | 被动采集的 evidence 快照 | agent 主动调用工具探索仓库 |
| 质量保证 | 依赖 prompt 工程 | agent 自检 + 交叉验证 + 修正 |
| 记忆 | 无 | SQLite 持久化上下文，跨天追踪 |
| 模型调用 | 1 次 | N 次（分步调用，每次聚焦一个子任务） |

## 2. Agent Loop 设计

### 2.1 核心循环（三步推理，与 PRD 对齐）

```
┌──────────────────────────────────────────────────┐
│                Agent 三步推理                      │
│                                                    │
│  ┌──────────┐     ┌──────────────┐    ┌──────────┐│
│  │ ANALYZE  │ ──→ │ INVESTIGATE  │ ──→│SYNTHESIZE││
│  │ 理解变更  │     │ 追问补全 gap  │    │ 综合生成  ││
│  └──────────┘     └──────────────┘    └──────────┘│
│       │                  │                  │      │
│       ↓                  ↓                  ↓      │
│  聚类 + 识别 gap    调 5 个工具追问     交叉验证 +   │
│  生成追问计划       收集追问结果         生成报告    │
└──────────────────────────────────────────────────┘
```

> 注：三步对应 Phase 0 架构设计中五步循环的精简版——
>   Analyze ≈ Plan，Investigate ≈ Act + Observe，Synthesize ≈ Reflect + Generate。
>   Phase 2 采用结构化三步，成本可控、行为可预测、可测试。Phase 3 可演进为更自由的五步 loop。

### 2.2 各阶段职责

**ANALYZE（理解）**：
- 输入：今日 evidence 快照 + 昨日报告 + 项目上下文
- 聚类变更按模块/功能，识别今日工作主题
- 识别信息缺口（如"这个文件改了三次但不知道为什么""commit message 太短需要看 diff"）
- 输出：追问计划——列出需要调用的工具和原因

**INVESTIGATE（追问）**：
- 按 ANALYZE 的追问计划调用工具（最多 5 次）
- 工具：git_log_explore、git_diff_detail、read_file、search_pattern、list_directory
- 工具返回原始结果，不经过 LLM 二次加工
- 收集追问结果，判断是否需要继续

**SYNTHESIZE（综合）**：
- 将原始 evidence + 追问发现综合成完整叙事
- 交叉验证：evidence ID 是否准确？结论是否有证据支撑？
- 标注置信度：high（直接 evidence）/ medium（推断但有工具支撑）/ low（纯推断）
- 生成结构化日报 JSON → 渲染 Markdown

### 2.3 循环控制

- 最大迭代次数：5 轮（PLAN→ACT→OBSERVE→REFLECT 算一轮）
- 每轮超时：60s
- 总超时：5 分钟
- token 预算：按配置的每日上限，agent 自行管理
- 提前终止条件：REFLECT 连续两轮无新 gap 发现

## 3. 工具集（与 PRD 对齐，M1 实现 5 个）

Agent 可调用的工具（比 Phase 0 的 evidence collector 更主动）：

| 工具 | 功能 | 对应操作 |
|------|------|---------|
| `git_log_explore` | 深挖特定文件的 commit 历史 | `git log -- <file>` |
| `git_diff_detail` | 获取特定文件的完整 diff | `git diff <file>` |
| `read_file` | 读取指定文件内容（裁剪） | 文件读取 |
| `search_pattern` | 扫描 TODO/FIXME/HACK/XXX | `grep -r` |
| `list_directory` | 浏览目录结构 | `ls` / `find` |

后续 M2-M3 按需扩展。

工具调用的安全约束：
- 只读，不修改任何文件
- 文件读取限制在 workspace 范围内
- 敏感文件仍然走 sanitizer 过滤
- 单次工具调用结果最大 16KB

## 4. Agent 记忆系统

基于 Phase 1 的 SQLite，增加 agent 专用表：

```sql
-- agent 推理过程记录
CREATE TABLE agent_sessions (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL,
    started_at TEXT NOT NULL,
    finished_at TEXT,
    iterations INTEGER DEFAULT 0,
    status TEXT DEFAULT 'running'  -- running/completed/failed
);

-- 每步推理记录
CREATE TABLE agent_steps (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    step_type TEXT NOT NULL,  -- plan/act/observe/reflect/generate
    step_order INTEGER NOT NULL,
    input_summary TEXT,       -- 输入摘要（不存完整 prompt）
    output_summary TEXT,      -- 输出摘要
    tool_calls TEXT,          -- JSON: 工具调用记录
    token_used INTEGER,
    duration_ms INTEGER,
    created_at TEXT NOT NULL
);

-- 跨天追踪
CREATE TABLE agent_memory (
    id TEXT PRIMARY KEY,
    key TEXT NOT NULL UNIQUE,  -- 如 "unfinished_tasks", "active_risks"
    value TEXT NOT NULL,       -- JSON
    updated_at TEXT NOT NULL
);
```

跨天记忆示例：
- `unfinished_tasks`：昨天未完成的 TODO 列表
- `active_risks`：持续跟踪的风险项
- `project_context`：项目结构变更记录

## 5. 与 Phase 1 的关系

Phase 1 是 Phase 2 的基础：

| Phase 1 交付 | Phase 2 如何使用 |
|-------------|----------------|
| SQLite 存储 | agent 记忆持久化 + 跨天追踪 |
| daemon 常驻 | agent loop 运行在 daemon 进程中 |
| 多 workspace | agent 可跨仓库分析 |
| 周报基础 | agent 驱动的多日聚合 + 趋势分析 |
| 组长版报告 | agent 按受众调整输出 |
| 普通目录扫描 | agent 可探索非代码工作痕迹 |

**建议**：Phase 1 和 Phase 2 不是严格串行——agent 引擎可以在 Phase 1 后期并行开发。Phase 1 先交付 daemon + SQLite + 日报增强（2-3 周），Phase 2 agent 引擎在 Phase 1 中期启动（第 2 周开始），两者在第 3-4 周合并。

## 6. 技术选型

| 组件 | 选择 | 理由 |
|------|------|------|
| Agent 框架 | 自建 Go 实现 | 轻量、可控、无重依赖 |
| 工具执行 | 复用 Phase 0 internal/git + 扩展 | 已有 Git CLI 封装 |
| LLM 调用 | 复用 Phase 0 internal/llm（OpenAI-compatible） | 支持 tool calling 的 model 即可 |
| 状态管理 | Phase 1 SQLite | 持久化 agent session |
| 并发 | Go goroutine + context 超时控制 | 天然适合 agent loop |

LLM Provider 要求：
- 支持 tool calling（function calling）
- DeepSeek 的 `deepseek-v4-pro` 已确认支持 tool calling ✓（deepseek-chat 同支持但将于 2026-07-24 废弃）
- OpenAI 全系列支持 ✓
- 内网 OpenAI-compatible 服务需要确认 tool calling 能力

## 7. Prompt 设计原则

Agent 的 prompt 和 Phase 0 不同——不再是一次性"根据 evidence 生成报告"，而是分步指令：

**PLAN prompt**：
- "你是一个开发活动分析 agent。这是今天的 evidence 快照。请分析哪些变更需要深入理解，列出需要调用的工具和原因。不要生成报告内容。"

**REFLECT prompt**：
- "这是当前的报告草稿。请逐条检查：每条结论是否有 evidence 支撑？推断是否标记？是否有重要变更被遗漏？列出需要补充调查的项。"

**GENERATE prompt**：
- 复用 Phase 0 的日报 prompt（已经过 PM review），但增加"基于前面收集的所有信息"的上下文。

## 8. 风险与应对

| 风险 | 应对 |
|------|------|
| Agent 推理轮次过多，耗 token | 最大 5 轮限制 + token 预算 |
| 工具调用超时 | 单工具 30s 超时，失败不阻断 loop |
| LLM tool calling 格式不稳定 | schema 校验 + 降级为单次生成 |
| Agent 幻觉式追问（追问不存在的问题） | 工具返回空时计入反思，连续空返回提前终止 |
| 本地 daemon 资源占用 | agent 推理异步执行，不阻塞扫描 |

## 9. 已确认决策（2026-05-29）

- [x] lee 确认：agent 推理使用云端 LLM tool calling（本地执行工具，云端做决策）
- [x] PM 确认：周报 agent 共用引擎，仅 prompt/工具策略不同
- [x] 架构确认：M1 开发时预留 SQLite agent_sessions/agent_steps/agent_memory 表
- [x] 成本确认：agent 多步推理 token 上限为 Phase 0 的 5 倍，超限降级
