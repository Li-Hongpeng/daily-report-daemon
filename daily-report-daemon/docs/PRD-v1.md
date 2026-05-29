# daily-report-daemon V1 PRD 初稿

版本：v0.1  
日期：2026-05-29  
阶段：概念验证 / MVP 定义  
作者：产品初稿 by Codex  

## 1. 产品一句话

daily-report-daemon 是一个本地优先的开发活动观察与报告生成守护进程：它在开发者授权的工作空间内持续收集代码与文档变更信号，使用大模型自动生成个人日报、周报、代码变更分析、风险提示，并为 Codex、Claude Code、Cursor 等编程 agent 自动维护项目上下文文档。

## 2. 背景与问题

团队管理者和开发者在 AI 编程时代同时遇到三类高频痛点：

1. 日报/周报编写困难：开发者需要回忆、检索、组织一天的工作内容；表达能力差异导致报告质量不稳定，且这件事被感知为“额外行政负担”。
2. 代码编写情况分析困难：管理者很难仅凭 commit、PR、issue 状态判断真实开发进展、风险、卡点和质量；AI 生成代码增加后，代码量和变更速度更快，传统 review 与追踪更吃力。
3. Agent 编程上下文组织困难：很多开发者不知道如何描述项目、拆任务、提供约束、维护 `AGENTS.md` / `CLAUDE.md` / Cursor rules / Copilot instructions 等上下文资产，导致 agent 经常误解项目结构、测试方式和编码规范。

核心机会：把“开发者真实工作痕迹”转化为“人能读的进展报告”和“agent 能用的项目上下文”，并且尽量不打断开发者。

## 3. 市场与竞品调研

### 3.1 工程效能 / Engineering Intelligence 平台

代表产品：LinearB、Swarmia、Jellyfish、Pluralsight Flow、Waydev。

观察：

- LinearB 聚焦 AI 与开发者生产力洞察、交付速度、代码质量、团队健康、成本资本化、AI 工具影响衡量等管理视角。
- Swarmia 提供工程指标、DORA/SPACE、AI 影响、投资分布、开发者体验调查、工作日志与 Slack/Teams 提醒。
- Jellyfish 强调把工程工具数据与业务目标、资源分配、AI 投资 ROI 连接。
- Pluralsight Flow 把 commit、ticket、PR 数据转成工程流程洞察，覆盖瓶颈、交付趋势、daily standup 到 executive updates。

可借鉴点：

- 管理者需要的不是“更多 raw data”，而是进展、风险、瓶颈、资源投入和质量趋势。
- 指标必须能落到行动建议，否则容易变成低信任度的绩效看板。

差异与空白：

- 这类产品主要从 GitHub/GitLab/Jira/Slack 等云端系统接入数据，对本地未提交变更、桌面工作目录、临时文档、agent prompt 草稿覆盖不足。
- 它们偏组织级管理平台，不是开发者个人本地工作记忆，也不直接解决 `AGENTS.md` 等 agent 上下文自动生成问题。

### 3.2 自动 standup / Git 报告工具

代表产品：AutoStandup、Gitrecap、DevTrack、GitPulse、ProjectRecap。

观察：

- AutoStandup 将 GitHub/Bitbucket 代码贡献与 Slack 连接，自动生成每日 standup，并能按开发者、组长、干系人调整技术细节。
- Gitrecap 关注 GitHub activity tracking，把 commits、PR、issues 做成 daily/weekly report，推送 Slack 或 email。
- DevTrack 值得重点参考：它强调 local-first、no cloud required、open source，用 git hooks、SQLite、fsnotify、Go daemon、本地/云端 LLM 生成 standup 或同步 PM 系统。

可借鉴点：

- “从 commit 自动生成 standup”已经被市场验证。
- 报告应支持不同受众版本：开发者技术版、组长进展版、非技术干系人版。
- Local-first 是信任建设的强卖点。

差异与空白：

- 多数工具以 GitHub/commit/PR 事件为主，难以覆盖未提交代码、跨目录文档、工作区草稿、临时调研材料。
- 报告通常止步于 standup，不持续沉淀项目结构、卡点、约束、测试命令和 agent 可复用上下文。

### 3.3 AI 代码评审产品

代表产品：CodeRabbit、Qodo Merge、Graphite Diamond、GitHub Copilot code review。

观察：

- CodeRabbit 覆盖 PR、IDE、CLI review，提供 diff summary、walkthrough、架构图、bug 检测、规则定制、每日 standup 和 sprint review 报告。
- GitHub Copilot code review 能在 PR、IDE、未提交变更场景提供 review，支持 repository custom instructions。

可借鉴点：

- 代码质量分析应该落到具体文件、具体变更、可能风险和建议动作。
- 自定义规则与路径级规则非常重要，因为不同仓库和模块有不同规范。

差异与空白：

- 代码评审产品主要围绕 PR 或显式 review 触发，不负责全天候工作汇总、日报归档、周报、管理者上报和 agent 上下文资产维护。
- 它们可以成为未来集成或能力参考，但 daily-report-daemon 的核心不是替代 PR review，而是“持续观察 + 报告 + 上下文沉淀”。

### 3.4 活动追踪 / 开发者记忆

代表产品：WakaTime、RescueTime、Pieces。

观察：

- WakaTime 通过 IDE/编辑器/终端插件自动追踪开发活动，近年也开始强调 AI-generated code、AI adoption、model efficiency、per-agent breakdown、prompt length。
- Pieces 强调 OS-level memory，自动记录开发者在各种 app 中的上下文，支持长期记忆、MCP、云/本地模型、私有本地默认。

可借鉴点：

- 自动化采集必须尽量无感，但必须可解释、可查看、可关闭。
- 个人工作记忆与 LLM 上下文连接是一个明确方向。

差异与空白：

- WakaTime 更偏时间和活动指标，Pieces 更偏个人记忆与检索；二者都不以“团队日报/周报 + 代码风险 + agent 项目文档自动维护”为主线。

### 3.5 Agent 上下文与项目规则生态

代表机制：AGENTS.md、Claude Code 的 `CLAUDE.md` / auto memory、GitHub Copilot repository instructions、Cursor rules。

观察：

- `AGENTS.md` 已成为一个面向 coding agent 的开放 Markdown 约定，用来描述项目概览、构建/测试命令、代码风格、测试说明、安全注意事项等。
- Claude Code 支持 `CLAUDE.md`，并建议记录项目架构、构建命令、调试经验、编码规范，同时也支持 auto memory。
- GitHub Copilot code review 支持 `.github/copilot-instructions.md` 和路径级 instructions。

可借鉴点：

- 市场已经接受“为 agent 准备专门上下文文件”的思路。
- 上下文文件需要短、具体、可验证、可维护，不能无限膨胀。

差异与空白：

- 现有机制大多要求开发者手写、维护和周期性清理。daily-report-daemon 的机会是根据真实活动自动发现项目结构、命令、约束、常见坑和近期变更，并生成可审阅的上下文更新建议。

### 3.6 竞品结论

目前没有发现一个成熟产品同时覆盖以下三点：

- 本地常驻、面向个人电脑和本地工作空间的开发活动采集。
- 自动生成日报、周报、代码变更分析、风险/卡点分析，并能向组长或邮箱上报。
- 自动生成和维护面向编程 agent 的项目上下文文档。

最接近的产品是 DevTrack、AutoStandup、Gitrecap、CodeRabbit、Pieces 的交叉区域。daily-report-daemon 应避免与大型工程效能平台正面竞争，先从“开发者本地工作记忆 + 自动报告 + agent 上下文维护”切入。

## 4. 产品定位

### 4.1 定位

daily-report-daemon 是“开发者自己的自动工作秘书”，不是“管理者的监控器”。

它默认站在开发者一侧：帮助开发者减少日报/周报负担、复盘自己的工作、把项目上下文组织好；在开发者授权后，再把合适粒度的报告同步给组长或团队系统。

### 4.2 核心价值主张

- 对开发者：不用每天回忆自己做了什么；自动获得清晰日报、周报、变更解释、风险提示、明日建议；agent 更容易理解项目。
- 对组长：更早看到项目进展、卡点、风险和质量信号，减少反复追问和低质量周报。
- 对团队：把散落在本地文件、git diff、commit、PR、issue、prompt 和上下文文件里的知识沉淀成可检索、可复用、可审计的工作资产。

### 4.3 产品原则

1. 本地优先：采集、索引、归档默认保存在本机；云模型调用和上报必须显式配置。
2. 透明可控：开发者能看到采集了什么、分析了什么、将上报什么。
3. 最小必要：默认只扫描用户授权目录，尊重 `.gitignore`，过滤密钥、二进制、大文件和个人隐私目录。
4. 证据优先：报告中的结论要尽量链接到 commit、diff、文件路径、任务编号或日志片段。
5. 不做绩效排名：V1 不提供个人排行榜、键盘活动监控、屏幕截图、鼠标轨迹等容易破坏信任的能力。

## 5. 目标用户与使用场景

首批目标用户：研发小组。产品应同时服务组员个人报告生成和组长项目进展汇总，不优先面向大企业全量工程效能平台。

### 5.1 用户角色

- 开发者：被要求写日报/周报、使用 AI 编程工具、维护项目上下文的人。
- 组长 / Tech Lead：需要掌握组员进展、风险、代码质量、卡点的人。
- 项目负责人 / PM：需要非技术版本进展说明、里程碑风险、延期原因的人。
- 团队管理员：负责部署、配置上报渠道、设置隐私边界和模板的人。

### 5.2 核心场景

1. 开发者下班前自动生成日报，无需手动回忆。
2. 周五自动汇总本周工作，生成周报和项目进展摘要。
3. 守护进程发现今天某个仓库有大量未提交变更，生成变更说明和风险提醒。
4. 守护进程根据项目结构、README、package scripts、Makefile、测试命令、近期修复经验生成 `AGENTS.generated.md`。
5. 组长每天早上收到团队成员授权后的摘要，快速识别卡点和风险。
6. 开发者启动 Codex / Claude Code 前，可以把最新 agent context 作为项目说明使用。
7. 对非 Git 工作目录，例如桌面、文档目录或需求资料目录，系统读取文本内容和非文本文件属性，帮助补全当天工作证据。

## 6. V1 目标与非目标

### 6.1 V1 目标

- 支持配置多个本地工作空间，周期性扫描文件和 Git 变更。
- 自动生成个人日报、项目日报、周报初稿，并按日期归档。
- 对代码变更进行摘要、质量风险、潜在 bug、测试建议和卡点分析。
- 为每个 Git 仓库生成 `AGENTS.generated.md`，包括项目概览、结构、构建/测试命令、编码规范、近期活动、常见坑。
- 支持 Git 仓库和普通目录采集：文本文件可读取内容摘要，非文本文件只采集文件名、路径、大小、mtime 等属性。
- 支持个人日报和项目维度汇总；项目维度汇总基于成员个人日报和项目 activity 聚合。
- 支持本地查看报告，并优先配置 email 上报；webhook 和 IM 进入后续扩展。
- 提供强隐私控制：include/exclude、密钥过滤、上报预览、本地数据清理。

### 6.2 V1 非目标

- 不做完整工程效能 BI 平台。
- 不做个人绩效排名、工时考勤、键盘/鼠标/截图监控。
- 不替代 PR review 工具，也不在 V1 中自动修改业务代码。
- 不自动创建、提交或覆盖项目根目录 `AGENTS.md`；V1 默认只生成 `AGENTS.generated.md`。
- 不在 V1 中支持复杂组织权限、HR 系统、财务成本资本化等企业级管理能力。

## 7. 功能需求

### 7.1 工作空间配置与守护进程

优先级：P0

需求：

- 用户可通过 CLI 配置一个或多个工作空间路径。
- 每个工作空间支持 include/exclude glob、扫描间隔、最大文件大小、是否启用 Git 分析、是否启用文档内容分析。
- 默认遵守 `.gitignore`，跳过 `node_modules`、`.venv`、`dist`、`build`、二进制文件、图片、压缩包、大型数据文件。
- 支持手动触发：`daily-report-daemon scan`、`daily-report-daemon report today`、`daily-report-daemon agent-context generate`。
- 本期实验优先支持 macOS/Linux；设计上保留跨平台抽象，后期兼容 Windows service、路径规则、文件监听和凭据存储。

验收标准：

- 用户能在 5 分钟内完成单 Git 仓库或普通目录初始化。
- 守护进程重启后能继续从上次状态增量扫描。
- 扫描失败不会影响用户代码文件。

### 7.2 本地活动采集

优先级：P0

采集信号：

- Git：branch、remote、commit log、author、status、staged/unstaged diff、untracked files、stash、PR/issue ID 线索。
- 文件：新增/修改/删除文件路径、扩展名、大小、mtime、内容摘要 hash。
- 文档：Markdown、txt、配置文件、需求文档、prompt 草稿等文本内容的增量摘要。
- 普通目录：支持桌面、文档目录、资料目录等非 Git workspace；文本内容可读取摘要，非文本文件只读取文件名、路径、扩展名、大小、mtime 等属性。
- 可选：编辑器/终端活动插件、issue tracker、日历、IM 消息不进入 V1 核心。

存储：

- 本地 SQLite 或 JSONL event log。
- 原文内容保留策略可配置：默认保留摘要和 diff，不长期保存完整文件快照。

验收标准：

- 能识别一天内每个仓库的提交、未提交 diff、主要文件变更，也能识别普通目录中的文本变化和非文本文件属性变化。
- 对密钥和敏感配置做过滤，不把 `.env`、私钥、token 原文送入模型。

### 7.3 LLM 分析流水线

优先级：P0

分析任务：

- Diff 摘要：解释改了什么、为什么可能改、影响哪些模块。
- 工作内容摘要：按项目/任务聚类生成“今天完成了什么”。
- 风险与质量：识别潜在 bug、缺少测试、接口兼容风险、配置/迁移风险、复杂度异常。
- 卡点推断：从反复修改、回滚、TODO、错误日志、未完成测试、长时间未提交等信号推断可能卡点。
- 明日建议：基于未完成变更、风险项、TODO 生成下一步建议。
- 证据绑定：输出中尽量包含文件路径、commit hash、任务号、变更片段摘要。

模型策略：

- 支持 OpenAI / OpenAI-compatible API。
- 本期允许云端 LLM 处理大部分代码 diff 和文本摘要，但所有内容必须先经过脱敏、裁剪和审计记录。
- 支持内网部署的 OpenAI-compatible LLM 作为“本地/私有模型”形态；V1 不强依赖桌面本地模型。
- 分层调用：小模型做文件分类和摘要，大模型做最终报告与风险分析。
- 支持 token/cost 预算：按日、按仓库、按用户设置上限。

验收标准：

- 一天 5000 行以内 diff 的日报生成时间小于 5 分钟。
- 报告中的每条主要结论至少有一个证据来源或标记为“推断”。
- 模型失败时保留原始采集数据，可重试生成。

### 7.4 日报与周报生成

优先级：P0

日报模板：

- 今日摘要：3-5 条自然语言总结。
- 按项目/仓库分组的工作内容。
- 关键变更：commit、文件、模块、功能点。
- 代码质量与风险。
- 遇到的卡点 / 未完成事项。
- 明日建议。
- 给组长版本：更少技术细节，突出进展、风险、是否需要协助。
- 给开发者版本：包含更多文件路径、测试命令、具体建议。

周报模板：

- 本周完成。
- 本周重点项目进展。
- 风险与延期原因。
- 质量与测试情况。
- 下周计划建议。
- 可选：需要组长决策的问题。

归档：

- 默认输出到 `.daily-report-daemon/reports/YYYY-MM-DD.md` 或用户指定目录。
- 支持按周/月索引。
- 支持全文搜索后续版本实现，V1 可先输出 Markdown 文件。

验收标准：

- 单日多仓库活动能合并成一份个人日报。
- 多名成员的个人日报能聚合成项目维度汇总，个人日报是项目汇总的基础数据来源。
- 报告可直接复制到团队日报系统。
- 同一份数据能生成开发者版和组长版。

### 7.5 Agent 上下文生成

优先级：P0

输出文件：

- `AGENTS.generated.md`：自动生成，不建议人工编辑。
- `AGENTS.md`：V1 不自动创建、不自动覆盖；若已有文件，仅作为输入参考并在报告中提示人工上下文存在。
- `CLAUDE.md` / `.github/copilot-instructions.md` / Cursor rules：V1 可生成建议，不默认写入。
- `.daily-report-daemon/context/YYYY-MM-DD.md`：活动上下文快照。

内容结构：

- Project overview。
- Build/test/lint/run commands。
- Repo layout。
- Key modules and ownership hints。
- Code style and conventions。
- Testing instructions。
- Security considerations。
- Recent activity summary。
- Current open questions / known blockers。
- Useful prompts for coding agents。

更新策略：

- 稳定上下文与近期活动分离：`AGENTS.generated.md` 也不应每天被大量噪音污染。
- 近期活动进入 `.daily-report-daemon/context/`，并在 `AGENTS.generated.md` 中只保留短摘要和链接。
- 每次更新给出变更原因；如未来支持写入 `AGENTS.md`，必须另行设计人工确认流程。

验收标准：

- 对一个已有项目，首次生成的 agent context 能让 coding agent 知道如何安装、测试、运行和修改代码。
- 生成文件少于 200 行或有拆分建议，避免占用过多上下文。
- 不覆盖用户手写内容。

### 7.6 上报与通知

优先级：P1

V1 支持：

- Email SMTP，作为首选上报渠道。
- Webhook。
- 本地文件目录同步。

后续支持：

- 钉钉、飞书、企业微信。
- Slack、Microsoft Teams。
- Jira、Linear、GitHub Issues、GitLab Issues、Azure DevOps work items。

上报模式：

- 手动确认后发送。
- 自动发送，但给开发者预留 N 分钟撤回窗口。
- 完全自动发送，仅用于团队已明确同意的低敏摘要。

验收标准：

- 组长能收到开发者授权的日报摘要。
- 上报内容可配置模板和脱敏级别。

### 7.7 本地查看与配置界面

优先级：P1

MVP 可先 CLI + Markdown 文件；V1 增强本地 Web UI 或桌面托盘。

视图：

- 今日活动时间线。
- 今日报告预览。
- 风险列表。
- Agent context 更新建议。
- 采集范围与隐私设置。
- 上报历史。

验收标准：

- 开发者能清楚看到“系统知道了什么、会发什么、能删什么”。
- 能一键禁用某个 workspace 或删除本地历史。

## 8. 关键用户流程

### 8.1 开发者首次安装

1. 安装 CLI。
2. 执行 `daily-report-daemon init`。
3. 选择工作空间路径。
4. 预览默认排除规则。
5. 配置模型 provider 和 API key，或选择本地模型。
6. 选择报告语言、上报模式、报告目录。
7. 启动 daemon。
8. 生成首份 baseline project context。

### 8.2 每日自动报告

1. Daemon 按间隔采集活动。
2. 下班前或用户设定时间生成日报。
3. 本地保存开发者版。
4. 若开启上报，生成组长版摘要。
5. 根据上报模式等待确认或自动发送。
6. 更新活动上下文快照。

### 8.3 Agent context 更新

1. Daemon 检测到项目结构、命令、测试方式、主要模块或规范变化。
2. 生成 `AGENTS.generated.md`。
3. 若项目根目录已有 `AGENTS.md`，仅读取其稳定规则作为输入参考，不生成覆盖 patch。
4. 后续 Codex / Claude Code / Cursor 可直接读取这些上下文。

## 9. 数据与隐私设计

### 9.1 数据分类

- 低敏：文件路径、commit hash、branch、任务号、非敏感 commit message。
- 中敏：代码 diff、文档片段、TODO、错误日志。
- 高敏：密钥、token、私钥、客户数据、个人聊天、未公开产品策略、薪资/人事信息。

### 9.2 默认保护

- 默认不扫描用户 Home 全目录，只扫描显式授权 workspace。
- 默认遵守 `.gitignore` 和内置敏感文件规则。
- 默认过滤 `.env`、`*.pem`、`id_rsa`、credentials、secret、token 等文件。
- 默认本地加密存储 API key。
- 默认不上传原始文件到团队服务器。
- 模型调用前进行 redaction，并记录发送摘要。

### 9.3 可信上报

- 开发者可查看每次上报内容。
- 管理者不能远程扩大采集范围，除非开发者确认或组织明确部署策略。
- 团队模式必须提供隐私说明和采集范围说明。

## 10. 指标体系

### 10.1 产品成功指标

- 激活率：安装后 24 小时内成功配置至少一个 workspace 的比例。
- 首份报告成功率：首次生成日报成功比例。
- 日报采纳率：开发者直接使用或少量修改后使用日报的比例。
- 节省时间：开发者自评日报/周报耗时下降。
- 上报满意度：组长对报告可读性、完整性、风险识别的评分。
- Agent context 使用率：生成上下文后被 agent 引用或被开发者保留的比例。

### 10.2 质量指标

- 报告幻觉率：无证据结论占比。
- 敏感信息泄漏率：redaction 漏检事件。
- 噪音率：开发者删除或标记无用的报告条目比例。
- 风险建议采纳率：代码风险提示被确认有用的比例。
- 生成成本：每人每天 token 成本和耗时。

## 11. MVP 范围建议

### 11.1 MVP P0

- CLI 初始化与配置文件。
- 单机 daemon。
- 多 workspace 扫描，支持 Git 仓库和普通目录。
- Git diff / commit / status 分析。
- 普通目录文本内容摘要和非文本文件属性采集。
- Markdown 日报、周报、项目报告归档。
- `AGENTS.generated.md` 生成。
- OpenAI-compatible provider。
- 本地 SQLite 存储。
- 敏感文件和密钥过滤。
- 手动确认 email 上报。

### 11.2 P1

- 本地 Web UI / 托盘。
- Webhook 上报。
- 钉钉/飞书/企业微信。
- Jira/Linear/GitHub Issues 关联。
- 内网 OpenAI-compatible LLM 配置增强。
- 报告全文搜索。
- 规则模板市场：前端、后端、移动端、数据工程、AI 项目等。

### 11.3 P2

- 团队汇总看板。
- 多人、多仓库项目进展聚合。
- Agent prompt pack 自动生成。
- 代码质量趋势和风险趋势。
- 企业级权限、审计、策略下发。

## 12. 技术架构初稿

### 12.1 模块

- Config Manager：管理 workspace、排除规则、模型、上报渠道。
- Watcher：文件系统监听和周期性扫描。
- Git Analyzer：读取 Git 状态、diff、commit、branch、remote。
- File Analyzer：分析普通目录、文本文件内容和非文本文件属性。
- Sanitizer：敏感信息识别、脱敏、内容裁剪。
- Event Store：本地 SQLite / JSONL。
- Summarizer：增量摘要、diff 摘要、项目摘要。
- Report Generator：日报、周报、组长版、开发者版。
- Agent Context Generator：生成 `AGENTS.generated.md` 和 context snapshot。
- Publisher：优先 email 上报，后续扩展 webhook/IM。
- UI/API：CLI、本地 Web UI。

### 12.2 分析流程

1. Watcher 收集文件、普通目录和 Git 事件。
2. Git Analyzer 生成结构化代码变更对象。
3. File Analyzer 生成文本摘要和非文本文件属性变更对象。
4. Sanitizer 过滤敏感内容。
5. Summarizer 对文件、diff、commit、文档活动做增量摘要。
6. Report Generator 聚合当天活动，生成个人日报和项目维度汇总。
7. Agent Context Generator 检测项目上下文变化，生成 `AGENTS.generated.md`。
8. Event Store 归档报告和摘要。
9. Publisher 根据策略优先通过 email 上报。

## 13. 风险与应对

### 13.1 员工信任风险

风险：产品被理解为“监控员工”。  
应对：定位为开发者个人工具；默认本地优先；上报可预览；不做排名/截图/键盘监控；报告强调进展和阻塞，不强调工时。

### 13.2 LLM 幻觉与误判

风险：错误总结、错误风险提示影响管理判断。  
应对：证据绑定；推断标记；开发者确认；报告置信度；高风险结论不自动上报为事实。

### 13.3 代码与密钥泄漏

风险：扫描和模型调用可能泄露敏感信息。  
应对：本地存储、脱敏、include/exclude、敏感文件默认忽略、本地模型选项、审计日志。

### 13.4 报告噪音

风险：日报太长、太泛、像流水账。  
应对：模板分层；面向受众生成；限制条目数；合并相似变更；支持用户反馈训练。

### 13.5 Agent context 膨胀

风险：`AGENTS.generated.md` 每天膨胀，反而影响 agent。  
应对：稳定规则与活动快照分离；限制长度；周期性清理；如未来支持写入 `AGENTS.md`，必须单独设计人工确认流程。

## 14. 路线图

### Phase 0：验证原型

- 单 workspace CLI 扫描，优先 Git 仓库，同时支持普通目录 metadata/text 扫描。
- 从 Git diff/commit 和普通目录文本活动生成 Markdown 日报。
- 生成 `AGENTS.generated.md`。
- 手动运行，不做 daemon。

### Phase 1：本地 daemon MVP

- 多 workspace 配置。
- 周期扫描和本地 SQLite。
- 每日自动生成报告。
- 敏感信息过滤。
- email 手动确认上报。

### Phase 2：开发者体验增强

- 本地 Web UI / tray。
- 报告编辑、确认、撤回窗口。
- Ollama 支持。
- Agent context patch 审阅。

### Phase 3：团队试点

- 组长汇总报告。
- 钉钉、飞书、企业微信等 IM 集成。
- 团队模板和隐私策略。
- 多人项目进展聚合。

## 15. 已确认产品决策

1. 首批目标用户：研发小组。
2. 平台优先级：本期在 macOS/Linux 上实验，设计时保留 Windows 跨平台兼容性。
3. 模型策略：允许云端 LLM 处理大部分内容；“本地模型”优先理解为部署在内网的 OpenAI-compatible LLM。
4. 上报渠道：优先 email；后续接入钉钉、飞书、企业微信，再考虑 Slack、Teams。
5. Agent context：默认只生成 `AGENTS.generated.md`，不自动创建或覆盖项目根目录 `AGENTS.md`。
6. 采集范围：不限于 Git 仓库，也包括桌面、文档目录等普通目录；文本文件读取内容摘要，非文本文件读取文件名、路径、大小、mtime 等属性。
7. 管理视角：个人日报和项目维度汇总都需要；个人日报是项目维度汇总的基础。

## 16. 调研来源

- LinearB: https://linearb.io/platform/software-engineering-intelligence
- Swarmia: https://www.swarmia.com/product/
- Jellyfish: https://jellyfish.co/
- Pluralsight Flow: https://www.pluralsight.com/product/flow
- AutoStandup: https://autostandup.tech/
- Gitrecap: https://www.gitrecap.com/features/git-reports
- DevTrack: https://devtrack.cloud/
- CodeRabbit: https://www.coderabbit.ai/
- GitHub Copilot code review: https://docs.github.com/en/copilot/how-tos/use-copilot-agents/request-a-code-review/use-code-review
- WakaTime: https://wakatime.com/
- Pieces: https://pieces.app/
- AGENTS.md: https://agents.md/
- Claude Code memory: https://docs.anthropic.com/en/docs/claude-code/memory
