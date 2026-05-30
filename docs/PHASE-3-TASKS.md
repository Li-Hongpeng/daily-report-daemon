# daily-report-daemon Phase 3 开发计划

版本：v0.1
日期：2026-05-30
作者：产品经理
前置：Phase 0 ✅、Phase 2 ✅（全部验收通过）

## 1. Phase 2 回顾与 Phase 3 定位

### 1.1 Phase 0 + Phase 2 已完成

| 阶段 | 核心交付 | 状态 |
|------|---------|------|
| Phase 0 | CLI 原型：scan → evidence → 单次 LLM → 日报 + agent context | ✅ |
| Phase 2 M1 | Agent 引擎：三步推理（Analyze→Investigate→Synthesize）+ 5 工具 + DeepSeek v4-pro tool calling | ✅ |
| Phase 2 M2 | Daemon：进程管理 + SQLite + 周期扫描 + 增量 | ✅ |
| Phase 2 M3 | Agent 驱动周报：多日聚合 + 跨日叙事 | ✅ |
| Phase 2 M4 | 组长版报告 + SMTP Email | ✅ |
| Phase 2 M5 | API key 加密 + Token 预算 + 基础 Web UI | ✅ |

### 1.2 Phase 2 M5 未完全覆盖的项（进入 Phase 3）

| 项 | 原因 | Phase 3 处理 |
|----|------|-------------|
| 普通目录扫描（非 Git workspace） | M5 范围过大，未明确交付 | M2 |
| Git stash 采集 | M5 未明确交付 | M2 |
| 6 个 P3 代码改进（Phase 0） | M5 未逐一验收 | M1 |
| Coding Conventions 去重 | Phase 2 agent 引擎已改善但未专门验证 | M1 |
| 4 个 Phase 2 P3（代码审查） | 低优先级，记 backlog | M1 |

### 1.3 Phase 3 定位

Phase 0 验证了"能做"，Phase 2 验证了"agent 做得更好"。Phase 3 的目标是**从个人工具升级为团队工具**——让组长看到多人聚合视图、让报告分发到 IM 和项目管理工具、让 agent 从结构化推理演进为更自由的智能体。

## 2. 里程碑总览

| 里程碑 | 内容 | 优先级 | 工期 | 依赖 |
|--------|------|--------|------|------|
| M1 | Backlog 清偿 + Agent 增强 | P0 | 2-3 天 | Phase 2 |
| M2 | 普通目录 + 更多信号采集 | P0 | 2-3 天 | M1 |
| M3 | 团队聚合与看板 | P0 | 3-5 天 | M2 |
| M4 | IM 集成（钉钉/飞书/企微） | P1 | 2-3 天 | M3 |
| M5 | Agent 自由推理 + 趋势分析 | P1 | 2-3 天 | M3 |

预估总工期：11-17 天（单人）

---

## M1：Backlog 清偿 + Agent 增强（P0，2-3 天）

### 目标

清偿 Phase 0 + Phase 2 所有遗留项，并增强 agent 可观测性。

### 任务

**Phase 0 P3 清偿（6 项）**：
- analyzer.go：collectUntrackedDiffs 去重调用
- analyzer.go：--since=today 依赖系统时区 → 支持 --since 参数
- report/generator.go：evidence index 始终为空 → 修复
- agentcontext/generator.go：filterByDate 把旧 commit 纳入 Today → 修复
- app/app.go：AgentContext 重扫文件而非加载已保存 metadata → 修复
- llm/client.go：中文 token 估算 chars/4 偏小 → 修正

**Phase 2 P3 清偿（4 项）**：
- daemon.go：isReportTime() 精度 1 分钟，ticker 漂移可能跳过 → 修复
- incremental.go：HasChanged 只用 size+mtime，同秒内改回原大小会漏 → 加 hash
- encrypt.go：机器密钥重建会 invalidate 已加密 key → 加注释
- webui.go：报告列表只渲染链接不展示内容 → 改进

**Agent 增强**：
- `--agent-trace` CLI 化：CLI run 模式下也能输出完整推理日志（当前仅 daemon 模式）
- Coding Conventions 去重：prompt 层面修复重复行问题
- Known Risks 分析改进：验证 agent 引擎已解决（对比 Phase 0 仅列 diff）

### 验收

- `go test ./...` 全绿
- `--agent-trace` 在 CLI run 和 daemon 模式均可输出
- Coding Conventions 不再出现重复行

---

## M2：普通目录 + 更多信号采集（P0，2-3 天）

### 目标

将采集范围从纯 Git 仓库扩展到普通目录，并补全 Git stash 等信号。

### 任务

**普通目录扫描**：
- 支持 `--workspace-type directory`（非 Git workspace）
- 文本文件读取内容摘要，按 mtime 检测变更
- 非文本文件只采集文件名、路径、扩展名、大小、mtime 等属性
- 增量扫描：基于 SQLite file_snapshots 表检测文件变化
- 普通目录的元信息纳入 agent 分析上下文

**Git stash 采集**：
- `git stash list` 读取 stash 记录
- stash 内容摘要纳入 evidence
- agent 可在分析阶段将 stash 作为"未完成工作"信号

### 验收

- 普通目录（如 `~/Documents/work`）可正常扫描并产出 evidence
- 文本文件变更被正确识别为当日活动
- `git stash list` 记录出现在 evidence 中
- Agent 日报能引用普通目录中的文本工作

---

## M3：团队聚合与看板（P0，3-5 天）

### 目标

从个人工具升级为团队工具——组长可查看多人聚合视图。

### 任务

**团队数据模型**：
- 团队配置：成员列表 + 各成员 workspace 路径
- 成员日报收集：从各成员 daemon SQLite 或共享目录读取
- 聚合数据结构：按日期/项目/成员维度汇总

**团队聚合功能**：
- 团队日报：聚合所有成员今日进展
- 团队周报：聚合本周项目进展 + 跨成员风险汇总
- 项目维度视图：按项目（而非按人）查看进展
- 成员负载感知：基于变更量和 commit 频率识别过载信号

**团队看板（Web UI 增强）**：
- 团队日报/周报查看页面
- 成员活动一览（非工时，仅进展摘要）
- 项目进展卡片
- 风险信号汇总

**隐私控制**：
- 成员可控制上报粒度（完整/摘要/仅进度）
- 组长不可远程扩大采集范围
- 团队模式提供隐私说明

### 验收

- 2 人以上团队的日报可成功聚合
- 周报能跨成员识别共用模块的风险
- 团队看板 Web UI 可正常访问
- 隐私控制开关生效

---

## M4：IM 集成（P1，2-3 天）

### 目标

报告自动推送到团队日常使用的 IM 工具。

### 任务

**钉钉集成**：
- 钉钉群机器人 Webhook
- 日报/周报格式化推送（Markdown → 钉钉 Markdown 格式转换）
- 手动确认 + 撤回窗口

**飞书集成**：
- 飞书群机器人 Webhook
- 飞书消息卡片格式
- 日报/周报推送

**企业微信集成**：
- 企微群机器人 Webhook
- 企微 Markdown 格式
- 日报/周报推送

**通用 IM 发送框架**：
- 统一的发送接口（Email / Webhook / IM 共用）
- 发送日志 + 重试机制
- 模板系统：不同 IM 使用不同格式模板

### 验收

- 至少一个 IM 渠道可成功推送日报
- 推送内容不乱码，格式正确
- 撤回窗口生效

---

## M5：Agent 自由推理 + 趋势分析（P1，2-3 天）

### 目标

Agent 从结构化三步推理演进为更自由的智能体，同时增加代码质量和风险的趋势分析。

### 任务

**Agent 自由推理**：
- 从固定的 Analyze→Investigate→Synthesize 演进为灵活的 Plan→Act→Observe→Reflect→Generate 循环
- Agent 自行决定何时停止追问、何时生成报告
- 最大迭代从 5 轮扩展到 10 轮（成本可控前提下）
- 跨天记忆增强：agent 能引用昨天的分析结论

**趋势分析**：
- 代码质量趋势：按周/月追踪 bug 密度、TODO 增长率、测试覆盖率变化
- 风险趋势：跨天追踪风险项的演变（是收敛还是恶化）
- 团队效率信号：PR 合并速度、commit 频率变化、跨模块耦合度

**Agent Prompt Pack 自动生成**：
- 基于项目特征自动生成推荐的 coding agent prompt
- 按项目类型（Go/Node/Python/前端等）定制
- 开发者可一键复制用于 Codex / Claude Code / Cursor

### 验收

- Agent 自由循环在真实仓库上运行不失控（有合理的终止条件）
- 趋势报告不空洞——有具体数字和变化方向
- Agent Prompt Pack 至少有 2 个可用的推荐 prompt
- Token 成本增幅在可控范围（≤ Phase 2 的 2 倍）

---

## 3. 与 PRD 原路线图对比

| 原 PRD Phase 3 | 新 Phase 3 | 变化说明 |
|---------------|-----------|---------|
| 组长汇总报告 | M3 团队聚合 | 从单人组长版升级为多人聚合 |
| 钉钉/飞书/企微 | M4 IM 集成 | 一致 |
| 团队模板和隐私策略 | M3 隐私控制 | 一致 |
| 多人项目进展聚合 | M3 项目维度视图 | 一致 |
| Agent prompt pack | M5 Agent Prompt Pack | 一致 |
| 代码质量/风险趋势 | M5 趋势分析 | 一致 |
| 企业级权限/审计 | **不做（Phase 4）** | 延后 |
| Agent 从三步到自由循环 | M5 Agent 自由推理 | Phase 2 架构文档已规划 |
| 普通目录 + stash | M2 | Phase 0/2 遗留 |
| Backlog 清偿 | M1 | Phase 0/2 遗留 |

## 4. 不做的事（Phase 3 非目标）

- 不做企业级权限、审计、策略下发（Phase 4）
- 不做 Jira/Linear/GitHub Issues 双向同步（Phase 4）
- 不做个人绩效排名、工时考勤（永久不做）
- 不做公有云 SaaS 模式（保持本地优先）
- 不做移动端 App

## 5. 实施顺序

```
M1（Backlog + Agent增强）→ M2（普通目录 + stash）→ M3（团队聚合）→ M4（IM集成）
                                                           ↘ M5（Agent自由推理 + 趋势）
```

M4 和 M5 可在 M3 完成后并行。

## 6. 关键产品决策（需 lee 确认）

1. **团队模式的部署方式**：各成员各自装 daemon，通过共享目录/内网同步日报？还是指定一台机器做聚合节点？
2. **IM 优先级**：钉钉、飞书、企业微信，先做哪个？建议按 lee 团队实际使用的 IM 优先。
3. **Agent 自由推理的深度**：先保持结构化三步（Phase 2 已稳定），用 M5 逐步放开 → 还是直接跳到自由 loop？推荐渐进式。
4. **普通目录扫描的隐私边界**：非 Git 目录的文本内容可能包含敏感信息（个人笔记、薪资文件等）。是否需要比 Git 仓库更严格的过滤策略？
5. **团队看板的访问控制**：谁可以看谁的日报？是否需要审批流程？
