# daily-report-daemon Phase 2 任务拆解

版本：v0.1  
日期：2026-05-29  
关联：PRD-PHASE-2.md、PHASE-2-AGENT-ARCHITECTURE.md  
状态：已确认，待开工

## 已确认决策

| # | 决策 | 结论 |
|---|------|------|
| 1 | Agent 推理深度 | 结构化三步推理（Analyze → Investigate → Synthesize） |
| 2 | 工具调用预算 | 每份报告最多 5 次工具调用 |
| 3 | Agent 推理日志 | 默认可见（`--agent-trace`） |
| 4 | 周报 agent | 共用引擎，仅 prompt/工具策略不同 |
| 5 | DeepSeek tool calling | v4-pro 已验证，完全兼容 |

## 里程碑总览

| 里程碑 | 内容 | 优先级 | 工期 | 依赖 |
|--------|------|--------|------|------|
| M1 | Agent 引擎核心 | P0 | 3-5 天 | Phase 0 代码 |
| M2 | Daemon 化 | P0 | 3-4 天 | M1 |
| M3 | Agent 驱动周报 | P0 | 1-2 天 | M1 |
| M4 | 组长版 + Email | P1 | 1-2 天 | M3 |
| M5 | Backlog 清偿 + Web UI | P1-P2 | 3-5 天 | M2 |

预估总工期：11-18 天（单人）

---

## M1：Agent 引擎核心（P0，3-5 天）

Phase 2 的核心交付。用三步推理替代 Phase 0 单次 LLM 调用。

### 任务

- [ ] 实现 `internal/agent/` 包：engine.go（三步推理 loop）、tools.go（工具注册与调用）、session.go（推理日志）
- [ ] 实现 Analyze 步骤：接收 evidence → 聚类变更 → 识别 gap → 生成追问计划
- [ ] 实现 Investigate 步骤：按计划调工具（最多 5 次）→ 收集追问结果
- [ ] 实现 Synthesize 步骤：原始 evidence + 追问发现 → 交叉验证 → 结构化日报 JSON
- [ ] 实现 5 个 agent 工具：git_log_explore、git_diff_detail、read_file、search_pattern、list_directory
- [ ] 实现 `--agent-trace` flag：输出完整推理过程（每步输入/输出/token 消耗）
- [ ] 扩展日报 JSON schema：新增 `confidence` 字段（high/medium/low）
- [ ] SQLite schema 预留：agent_sessions、agent_steps、agent_memory 三张表
- [ ] DeepSeek v4-pro tool calling API 适配（OpenAI-compatible 格式，复用 internal/llm）
- [ ] Token 预算控制：单份报告上限 5× Phase 0 baseline
- [ ] 降级路径：tool calling 失败 → 回退到 Phase 0 单次生成

### 验收

- [ ] 同一份 evidence，agent 报告的风险分析优于 Phase 0 baseline
- [ ] agent 至少触发 1 次主动工具调用
- [ ] `--agent-trace` 输出完整可读
- [ ] `go test ./internal/agent/...` 通过（mock LLM + fixture evidence）
- [ ] token 预算超限时降级而非崩溃

---

## M2：Daemon 化（P0，3-4 天）

从手动 CLI 到后台常驻。依赖 M1 的 agent 引擎。

### 任务

- [ ] 实现周期扫描引擎：configurable interval（默认 30 分钟）
- [ ] SQLite 完整迁移：workspaces、scan_runs、file_snapshots、git_events、evidence、reports 表
- [ ] 实现 daemon 进程管理：`daemon start/stop/status/restart`
- [ ] 实现多 workspace 配置（复用 Phase 0 config 结构，扩展为数组）
- [ ] 定时自动日报：用户配置触发时间（如下午 5:30），到点自动跑 agent 引擎
- [ ] 异步生成 + 桌面通知：agent 推理不阻塞终端，完成后 notify
- [ ] 增量扫描：基于 SQLite 上次扫描状态，只处理变更文件
- [ ] 数据迁移工具：Phase 0 JSONL → Phase 2 SQLite

### 验收

- [ ] daemon 后台持续运行，不阻塞终端
- [ ] 重启后从上次状态增量扫描
- [ ] 日报在预设时间自动生成
- [ ] SQLite 可查询历史 runs/reports

---

## M3：Agent 驱动周报（P0，1-2 天）

共用 M1 agent 引擎，生成跨日叙事周报。依赖 M1。

### 任务

- [ ] 周报 prompt + 工具策略：读取本周每日 evidence → 识别跨日模式
- [ ] 周报结构化 schema：本周完成 / 重点项目进展 / 风险趋势 / 质量趋势 / 下周计划
- [ ] Agent trace 适配周报上下文（本周 vs 今日）
- [ ] 周报归档：`reports/weekly/YYYY-WXX.md`

### 验收

- [ ] 周报不是"7 天日报拼凑"，有跨日叙事
- [ ] 能识别本周反复出现的风险信号
- [ ] "下周建议"不空洞

---

## M4：组长版报告 + Email 发送（P1，1-2 天）

依赖 M3（报告体系完整后）。

### 任务

- [ ] 组长版日报 prompt：精简技术细节、突出进展/风险/需协助
- [ ] SMTP email 发送：配置 SMTP server + 收件人列表
- [ ] 手动确认发送模式（默认）+ 撤回窗口
- [ ] Webhook 发送（备用渠道）

### 验收

- [ ] 组长版报告可直接转发给管理者
- [ ] email 发送成功，内容不乱码

---

## M5：Phase 0 Backlog 清偿 + 基础 Web UI（P1-P2，3-5 天）

### 任务

- [ ] 普通目录扫描（非 Git workspace）
- [ ] API key 加密存储（AES 本地加密）
- [ ] Token 预算控制面板（按日/按仓库）
- [ ] Git stash 采集
- [ ] 6 个 P3 代码改进（Phase 0 code review backlog）
- [ ] Coding Conventions 去重（prompt 改进）
- [ ] Known Risks 风险分析改进（agent 引擎已解决，验证即可）
- [ ] 本地 Web UI 基础版：今日报告查看、历史浏览、配置管理、agent trace 查看

### 验收

- [ ] `go test ./...` 全绿
- [ ] Web UI 可在浏览器打开，核心页面可用

---

## 推荐实施顺序

```
M1（Agent 引擎）→ M2（Daemon）→ M3（周报）→ M4（组长版+Email）→ M5（Backlog+WebUI）
                    ↘ M3 可与 M2 并行（M1 完成后）
```

## 关键风险

| 风险 | 应对 |
|------|------|
| DeepSeek v4-pro tool calling 不稳定 | M1 早期验证；失败回退 Phase 0 单次生成 |
| Agent token 成本超预算 | M1 内置 5× 硬上限 + 降级策略 |
| SQLite 迁移复杂度 | M2 提供 Phase 0 JSONL → SQLite 迁移脚本 |
| Daemon 跨平台兼容 | macOS/Linux 优先，Windows service 后续 |
