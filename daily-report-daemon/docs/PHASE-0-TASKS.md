# daily-report-daemon Phase 0 原型任务清单

版本：v0.2（2026-05-29 确认 scope 后修订）  
日期：2026-05-29  
关联文档：

- [PRD-v1.md](./PRD-v1.md)
- [TECHNICAL-ARCHITECTURE.md](./TECHNICAL-ARCHITECTURE.md)
- **已确认决策（2026-05-29）**：Go 技术栈、工程师 @user-6a164072 负责开发、普通目录不进 Phase 0（延后到 Phase 1）

## 1. Phase 0 目标

Phase 0 的目标不是做完整产品，而是验证 daily-report-daemon 的核心价值链是否成立：

1. 能否从一个本地 Git 仓库中提取足够好的开发活动 evidence。
2. 能否基于 evidence 生成可信、可直接复用的日报。
3. 能否生成对 Codex / Claude Code / Cursor 有帮助的 agent context。
4. 能否在不做 daemon、不做团队上报、不做 UI 的情况下，让用户看到产品价值。

## 2. Phase 0 范围

### 2.1 做什么

- 单 workspace CLI 原型，限定 Git 仓库（普通目录延后到 Phase 1）。
- 手动触发扫描。
- 读取 Git status、今日 commit、staged/unstaged diff、untracked 文本文件。
- 读取关键项目文件，例如 README、package scripts、Makefile、go.mod、pyproject、配置文件。
- 基础敏感信息过滤。
- 调用 OpenAI-compatible API，兼容公网云端模型和内网部署模型（单模型直出，分层 LLM 延后到 Phase 1）。
- 输出开发者版日报 Markdown（组长版延后到 Phase 1）。
- 输出 `.daily-report-daemon/context/AGENTS.generated.md`。
- 保存脱敏 evidence 和模型输入，便于调试。

### 2.2 不做什么

- 不做后台 daemon。
- 不做多 workspace。
- 不做普通目录扫描（延后到 Phase 1）。
- 不做 SQLite（延后到 Phase 1，Phase 0 用 JSONL + Markdown）。
- 不做周报（延后到 Phase 1）。
- 不做组长版报告（延后到 Phase 1）。
- 不做分层 LLM（延后到 Phase 1，Phase 0 单模型直出）。
- 不做真实 email/webhook 发送。
- 不做本地 Web UI。
- 不自动覆盖 `AGENTS.md`。
- 不做复杂团队权限、汇总看板和趋势分析。

## 3. Phase 0 成功标准

### 3.1 功能成功标准

- 在任意 Git 仓库运行一条命令后，能生成当天日报。
- 对有未提交 diff 的仓库，报告能解释主要变更点。
- 对有今日 commit 的仓库，报告能引用 commit 信息。
- 生成的 `AGENTS.generated.md` 能包含项目概览、目录结构、运行/测试命令、近期活动。
- 用户可以查看脱敏后的 evidence 和模型输入。

### 3.2 质量成功标准

- 报告中主要结论有 evidence id 或明确标注为推断。
- 敏感样例不会原文进入模型输入。
- 对 3000 行以内 diff，报告生成可在 3 分钟内完成。
- CLI 错误信息可读，不出现 panic 或难以理解的栈信息。

### 3.3 用户验证标准

找 2-3 个真实仓库试跑，回答三个问题：

- 开发者是否愿意直接复制日报，或只做少量编辑后提交？
- 组长版是否能帮助组长理解进展和风险？
- `AGENTS.generated.md` 是否能明显减少下一次使用 coding agent 的上下文准备成本？

## 4. 里程碑拆解

### M0：项目脚手架

目标：建立可运行的 CLI 工程骨架。

任务：

- [ ] 初始化 Go module。
- [ ] 建立推荐目录结构。
- [ ] 接入 CLI 框架。
- [ ] 实现 `daily-report-daemon --version`。
- [ ] 实现 `daily-report-daemon help`。
- [ ] 增加基础 README。
- [ ] 增加 `.gitignore`。
- [ ] 增加最小测试命令。

验收：

- [ ] 本地能执行 `go test ./...`。
- [ ] 本地能执行 `go run ./cmd/daily-report-daemon --version`。

建议工期：0.5 天。

### M1：配置与运行目录

目标：让工具知道当前工作区、输出目录和模型配置。

任务：

- [ ] 设计 Phase 0 配置结构。
- [ ] 支持从环境变量读取 `OPENAI_API_KEY`。
- [ ] 支持 `--workspace` 参数，默认当前目录。
- [ ] 支持 `--workspace-type` 参数，取值为 `auto`、`git_repo`、`directory`，默认 `auto`。
- [ ] 支持 `--output-dir` 参数，默认 `.daily-report-daemon`。
- [ ] 实现 `init` 命令，生成 `.daily-report-daemon/config.yaml`。
- [ ] 创建运行目录：`runs/`、`reports/`、`context/`。
- [ ] 输出配置加载日志。

验收：

- [ ] `init` 后能看到配置文件和目录。
- [ ] 未配置 API key 时，CLI 给出清晰提示。
- [ ] workspace 不存在或不是目录时，CLI 友好报错。

建议工期：0.5-1 天。

### M2：Git Analyzer

目标：把 Git 活动转成结构化数据。

任务：

- [ ] 识别 Git repo root。
- [ ] 读取当前 branch。
- [ ] 读取 remote 信息。
- [ ] 读取 HEAD commit。
- [ ] 读取今日 commit log。
- [ ] 读取 `git status --porcelain`。
- [ ] 读取 `git diff --stat` 和 `git diff --numstat`。
- [ ] 读取 unstaged diff。
- [ ] 读取 staged diff。
- [ ] 识别 untracked 文本文件路径。
- [ ] 对 diff 做最大字符数裁剪。
- [ ] 将结果序列化为 `git-activity.json`。

验收：

- [ ] 在干净仓库运行时，能输出空变更但不报错。
- [ ] 在有 unstaged diff 的仓库运行时，能看到文件级增删行。
- [ ] 在有 staged diff 的仓库运行时，能区分 staged/unstaged。
- [ ] 在非 Git 目录运行时，Git Analyzer 跳过并返回空 Git 活动，不阻断普通目录扫描。

建议工期：1 天。

### M3：File Scanner 与项目元信息

目标：采集生成报告和 agent context 所需的项目结构、文档内容。

任务：

- [ ] 枚举 workspace 目录结构。
- [ ] 默认跳过 `.git`、`node_modules`、`.venv`、`dist`、`build`、二进制文件。
- [ ] 读取 README、package.json、go.mod、pyproject.toml、Cargo.toml、Makefile、Taskfile、Dockerfile 等关键文件。
- [ ] 提取 package scripts / Makefile targets / 常见测试命令。
- [ ] 统计主要语言和文件类型。
- [ ] 对关键文件内容做长度裁剪。
- [ ] 输出 `project-metadata.json`。

验收：

- [ ] 对 Go/Node/Python 项目能识别基本运行或测试线索。
- [ ] 对没有 README 的项目不报错。
- [ ] 不读取默认忽略目录中的文件内容。
- [ ] 对二进制等非文本文件只输出 metadata。

建议工期：0.5-1 天（缩减自原 1 天，因移除普通目录文本摘要逻辑）。

### M4：Sanitizer 与 Evidence Builder

目标：在模型调用前形成安全、可引用的 evidence。

任务：

- [ ] 定义 evidence 数据结构。
- [ ] 将 commit、diff、file change、file metadata、doc snippet、project metadata 转成 evidence。
- [ ] 为每条 evidence 生成稳定 ID。
- [ ] 实现路径级敏感过滤。
- [ ] 实现基础 secret 正则过滤。
- [ ] 实现超长文本裁剪。
- [ ] 输出 `evidence.jsonl`。
- [ ] 输出 `redaction-report.json`。

验收：

- [ ] 假 API key、私钥、password 样例会被替换为 `[REDACTED]`。
- [ ] evidence id 能在报告中引用。
- [ ] 被裁剪的内容会记录裁剪原因和原始长度。

建议工期：1 天。

### M5：LLM Client 与 Prompt 模板

目标：打通 OpenAI-compatible 模型调用。

任务：

- [ ] 实现 OpenAI-compatible chat/completions 或 responses 兼容层。
- [ ] 支持 `base_url`、`model`、`api_key_env` 配置。
- [ ] 明确支持公网云端模型和内网 OpenAI-compatible 模型。
- [ ] 实现超时、重试、错误分类。
- [ ] 实现 `--dry-run`，只输出模型输入不调用模型。
- [ ] 编写日报 JSON prompt。
- [ ] 编写 agent context prompt。
- [ ] 保存脱敏后的 `model-input.json`。
- [ ] 保存原始模型响应 `model-output.json`。

验收：

- [ ] API key 缺失时报错清楚。
- [ ] 模型返回非 JSON 时能给出可排查日志。
- [ ] dry-run 不发起网络请求。
- [ ] 同一份 evidence 可分别生成日报和 agent context。

建议工期：1 天。

### M6：日报生成

目标：生成可读的开发者版日报（组长版延后到 Phase 1）。

任务：

- [ ] 定义日报结构化 schema。
- [ ] 校验模型输出字段。
- [ ] 渲染开发者版 Markdown。
- [ ] 添加 evidence 索引。
- [ ] 输出到 `reports/YYYY-MM-DD.md`。
- [ ] 在 CLI 结束时打印输出路径。

开发者版建议结构：

- 今日概览。
- 完成事项。
- 关键代码变更。
- 风险与待确认。
- 可能卡点。
- 明日建议。
- 证据索引。

验收：

- [ ] 报告 Markdown 可以直接复制到日报系统。
- [ ] 报告包含 evidence id。
- [ ] 空变更仓库能生成”今日无明显代码活动”的合理报告。

建议工期：0.5-1 天（缩减自原 1 天，因只出开发者版）。

### M7：Agent Context 生成

目标：生成能给 coding agent 使用的项目上下文。

任务：

- [ ] 定义 `AGENTS.generated.md` 模板。
- [ ] 从 project metadata 中填充项目结构和命令。
- [ ] 从 Git activity、文本摘要、普通目录 metadata 中填充近期活动。
- [ ] 从 evidence 中填充风险、开放问题、建议 prompt。
- [ ] 输出 `.daily-report-daemon/context/AGENTS.generated.md`。
- [ ] 如果根目录已有 `AGENTS.md`，读取并在输出中提示“已有人工上下文”。
- [ ] 不覆盖根目录 `AGENTS.md`。

建议结构：

- Project overview。
- Repository layout。
- Build, run, test commands。
- Coding conventions detected。
- Important files。
- Recent activity。
- Known risks and open questions。
- Suggested prompts for agents。

验收：

- [ ] 新开 Codex/Claude Code 会话时，可以直接把该文件作为项目上下文。
- [ ] 文件不超过 200 行，除非用户显式放宽限制。
- [ ] 近期活动和稳定规则区分清楚。

建议工期：1 天。

### M8：端到端命令

目标：用户用少量命令跑通完整链路。

任务：

- [ ] 实现 `scan --workspace .`。
- [ ] 实现 `report today --workspace .`。
- [ ] 实现 `agent-context generate --workspace .`。
- [ ] 实现 `run --workspace .`，一次性生成日报和 agent context。
- [ ] 增加 `--dry-run`。
- [ ] 增加 `--no-llm`，只生成 evidence。
- [ ] 输出运行摘要。

验收：

- [ ] `run --workspace .` 可从零生成完整产物。
- [ ] 失败时已生成的 evidence 不丢失。
- [ ] 运行摘要包含扫描文件数、diff 文件数、redaction 次数、输出路径。

建议工期：0.5 天。

### M9：测试与样例仓库

目标：让原型可维护、可演示。

任务：

- [ ] 为 Git Analyzer 增加 fixture repo 测试。
- [ ] 为 Sanitizer 增加敏感信息测试。
- [ ] 为 Markdown Renderer 增加 snapshot test。
- [ ] 为 LLM Client 增加 mock server 测试。
- [ ] 准备一个 `examples/` 样例输入目录。
- [ ] 准备一份样例输出报告。
- [ ] 增加 smoke test 脚本。

验收：

- [ ] `go test ./...` 通过。
- [ ] 无 API key 环境下也能跑单元测试。
- [ ] README 能指导用户跑通 dry-run。

建议工期：1-1.5 天。

### M10：真实仓库试跑与迭代

目标：验证报告质量，而不是只验证代码能跑。

任务：

- [ ] 选择 2-3 个真实 Git 仓库。
- [ ] 分别制造或选择有代表性的 diff：功能开发、bugfix、重构、文档更新。
- [ ] 生成日报和 agent context。
- [ ] 记录报告中的无用项、错误项、缺失项。
- [ ] 调整 prompt 和模板。
- [ ] 更新 README 中的限制说明。
- [ ] 写 Phase 0 验证结论。

验收：

- [ ] 至少 2 份报告达到“少量编辑即可使用”。
- [ ] 至少 1 份 `AGENTS.generated.md` 被用于真实 agent 会话。
- [ ] 得出 Phase 1 是否值得做 daemon 的明确结论。

建议工期：1-2 天。

## 5. 建议实施顺序

推荐顺序：

1. M0 项目脚手架。
2. M1 配置与运行目录。
3. M2 Git Analyzer。
4. M3 File Scanner 与项目元信息。
5. M4 Sanitizer 与 Evidence Builder。
6. M6 日报生成的离线 renderer。
7. M5 LLM Client 与 Prompt 模板。
8. M7 Agent Context 生成。
9. M8 端到端命令。
10. M9 测试与样例仓库。
11. M10 真实仓库试跑与迭代。

说明：M6 可以先用固定 fixture JSON 做 renderer，这样不会被模型调用阻塞。

## 6. Phase 0 命令设计

### 6.1 初始化

```bash
daily-report-daemon init --workspace . --workspace-type auto
```

输出：

```text
.daily-report-daemon/
  config.yaml
  runs/
  reports/
  context/
```

### 6.2 只扫描

```bash
daily-report-daemon scan --workspace . --workspace-type auto --no-llm
```

输出：

```text
.daily-report-daemon/runs/2026-05-29-183000/
  git-activity.json
  project-metadata.json
  file-activity.json
  evidence.jsonl
  redaction-report.json
```

### 6.3 生成日报

```bash
daily-report-daemon report today --workspace . --workspace-type auto
```

输出：

```text
.daily-report-daemon/reports/2026-05-29-developer.md
.daily-report-daemon/reports/2026-05-29-lead.md
```

### 6.4 生成 Agent Context

```bash
daily-report-daemon agent-context generate --workspace . --workspace-type auto
```

输出：

```text
.daily-report-daemon/context/AGENTS.generated.md
```

### 6.5 一键运行

```bash
daily-report-daemon run --workspace . --workspace-type auto
```

输出：

```text
Scan completed.
Evidence: .daily-report-daemon/runs/2026-05-29-183000/evidence.jsonl
Developer report: .daily-report-daemon/reports/2026-05-29-developer.md
Lead report: .daily-report-daemon/reports/2026-05-29-lead.md
Agent context: .daily-report-daemon/context/AGENTS.generated.md
```

## 7. 数据产物清单

Phase 0 每次运行建议产出：

```text
.daily-report-daemon/
  config.yaml
  runs/
    2026-05-29-183000/
      git-activity.json
      project-metadata.json
      evidence.jsonl
      redaction-report.json
      model-input.report.json
      model-output.report.json
      model-input.agent-context.json
      model-output.agent-context.json
  reports/
    2026-05-29.md
  context/
    AGENTS.generated.md
```

## 8. Prompt 设计要求

### 8.1 日报 Prompt 要求

必须要求模型：

- 基于 evidence 生成，不要编造外部信息。
- 每条关键结论引用 evidence id。
- 无证据但合理的内容必须标记为“推断”。
- 区分“已完成”“进行中”“风险”“卡点”“建议”。
- 输出结构化 JSON。
- 使用用户配置的语言。

### 8.2 Agent Context Prompt 要求

必须要求模型：

- 生成给 coding agent 看的上下文，不写营销文案。
- 优先提炼稳定信息，不把每日流水账塞进长期规则。
- 明确运行、测试、构建命令的证据来源。
- 不确定的命令标记为“需要确认”。
- 输出 Markdown。
- 限制长度，默认不超过 200 行。

## 9. 原型风险清单

### 9.1 Git diff 过大

应对：

- 按文件裁剪。
- 保留 diff stat 和 numstat。
- 让模型先处理摘要而不是完整 patch。

### 9.2 报告像流水账

应对：

- Prompt 中强制聚类。
- 限制摘要条数。
- 分开发者版和组长版。

### 9.3 敏感信息误传

应对：

- 默认跳过敏感文件。
- 模型输入落盘前也必须是脱敏结果。
- 增加 redaction test。

### 9.4 模型输出不稳定

应对：

- 结构化 JSON 输出。
- schema 校验。
- 失败时保存原始输出并提示重试。

### 9.5 Agent Context 过长

应对：

- 稳定上下文和近期活动分段。
- 默认 200 行限制。
- 超出时输出“建议拆分”。

## 10. Phase 0 交付物

代码交付：

- 可运行 CLI。
- Git Analyzer。
- File Scanner。
- Sanitizer。
- Evidence Builder。
- OpenAI-compatible LLM Client。
- Markdown Report Renderer。
- Agent Context Generator。
- 基础测试。

文档交付：

- README 快速开始。
- 配置说明。
- 隐私与数据说明。
- 样例报告。
- 样例 `AGENTS.generated.md`。
- Phase 0 验证结论。

## 11. Phase 0 预估周期

单人开发预估（scope 缩减后：仅 Git repo、仅开发者版日报、单模型直出）：

- 最快可演示：2-3 天。
- 稳定可试用：6-8 天。
- 带测试、样例、真实仓库迭代：8-10 天。

建议第一版演示路径：

1. 初始化项目。
2. 对当前仓库制造一次真实文档或代码变更。
3. 运行 `daily-report-daemon run --workspace .`。
4. 打开日报和 `AGENTS.generated.md`。
5. 用生成的 agent context 开一个 Codex/Claude Code 会话，观察是否减少上下文解释成本。
