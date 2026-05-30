# AGENTS.md

> 本文件是人工维护的项目开发约束。`.daily-report-daemon/context/AGENTS.generated.md`
> 是工具生成的补充上下文，不应替代这里的长期工程原则。

## 项目定位

daily-report-daemon 是一个本地优先的开发活动观察与报告生成工具。核心链路是：

1. 只在用户授权的工作区内采集 Git、文件结构、关键文档等信号。
2. 先生成可审计、已脱敏的 evidence。
3. 再基于 evidence 生成日报、周报和 Agent Context。
4. 最后按配置保存、入库、发布到钉钉等渠道。

后续开发必须保护这条链路的可解释性和隐私边界。

## 通用开发经验

### 1. Evidence first, report second

所有日报、周报、Agent Context 都必须以 evidence 为事实源。不要让 LLM 直接读取未脱敏的仓库内容，也不要在报告生成阶段临时拼接新的原始数据。

改动 evidence schema、prompt 或报告结构时，要同步更新解析、Markdown 渲染、降级输出和测试。模型输出不可假设总是合法 JSON，必须有可保存、可排查的降级路径。

### 2. 隐私边界优先于功能便利

`.daily-report-daemon/` 是本地运行数据和本地配置目录，不能进入 Git Analyzer、Scanner、evidence、报告或发布内容。配置文件、model-io、runs、reports、context、baseline、daemon.db 都默认视为本地私有数据。

任何 API key、webhook、token、password、private key、连接串都必须在写入 evidence 前脱敏。不要在日志、错误信息、测试输出或报告中打印完整 webhook URL。

### 3. App 是唯一主业务管线

CLI 和 daemon 都应该尽量薄，只负责参数解析、调度和生命周期管理。扫描、日报、周报、Agent Context、发布等主流程应通过 `internal/app` 复用同一套实现。

不要在 daemon、CLI 或 report 包里复制一套“相似但不完全一致”的日报/周报逻辑。过去出现过 daemon 定时报表绕过 App 管线，导致报告不保存、SQLite 不入库、Agent Context 丢失的问题，后续要避免这种分叉。

### 4. Baseline 与“今日活动”语义要清晰

首次扫描已有项目时，当前 HEAD 之前的提交是 baseline，不应被当作当天工作成果。后续扫描只报告 baseline 之后的新提交和当前工作区 diff。

Agent Context 的 `Today's Activity` 应表达“本次扫描看到的当前活动”，不要靠日期字符串猜测。diff、file_change、commit、todo 这类动态 evidence 才适合进入当前活动；README、go.mod 等静态 doc snippet 不应混入。

### 5. 周报是跨日报综合，不是日报拼接

周报应从本周日报 Markdown 中抽取跨日模式、持续风险、质量趋势和下周计划。日报不足时要诚实说明信息有限，不要编造连续叙事。

周报输出目录、ISO week 命名、JSON schema、Markdown 渲染、钉钉发布都应和日报一样可测试、可降级、可配置。

### 6. Agent runtime 只有一条主线

Agent 核心采用 Eino ADK。不要恢复旧的文本 `parseToolCalls` 伪工具调用，也不要引入 `builtin|eino` 双轨兼容。重构旧逻辑时宁可移除历史包袱，也不要让 CLI、daemon、report 包各自保留一套相似管线。

日报和周报的最终叙事归 SupervisorAgent，WorkspaceAgent 只产出目录级调查、memory 更新和 Agent Context。每个 workspace 的 memory 必须独立 namespace，Supervisor 只能写全局聚合记忆。

Agent 工具默认只读，唯一可写面是 agent memory。所有工具都必须走路径范围校验、`.daily-report-daemon/` 阻断、敏感路径/内容脱敏和输出截断。

### 7. 配置必须真正驱动行为

`config.yaml` 中已有的字段不能只停留在文档或默认值里。输出目录、LLM provider、publisher、daemon 扫描间隔、日报时间、周报时间、baseline 行为都应由配置驱动。

新增配置时要考虑旧配置文件的零值兼容。默认值应保守，尤其是自动发送、历史 baseline、敏感路径扫描这类有风险的行为。

### 8. Daemon 必须可恢复、可审计

daemon 的定时扫描和定时报表必须保存可追溯数据：scan run、evidence、report、publish 状态等。run ID 不能只精确到秒，避免连续触发时撞库。

SQLite 中的历史 evidence 不能被稳定 evidence ID 覆盖。入库时要保留 scan run 维度，保证后续周报、趋势分析、回归排查能查询到真实历史。

### 9. 发布链路要可控

钉钉等外部发布必须尊重 `auto_send`。默认应支持“生成后待审阅”，自动发送必须显式配置。

HTTP 200 不代表发布成功。钉钉机器人要检查响应 body 的 `errcode`，并把失败作为 run warning 返回到 summary，而不是只写 stderr。

### 10. 测试要覆盖真实链路

普通代码改动至少跑：

- `go test ./...`
- `go test -race ./...`
- `go build ./cmd/daily-report-daemon`
- `bash scripts/smoke-test.sh`

触碰 LLM、发布、daemon、SQLite、脱敏、baseline、周报时，要补对应单测。合并前应至少做一次真实临时仓库回归：scan -> report/run -> weekly -> DingTalk publish -> daemon start/status/stop。

### 11. 为 Mac/Linux 稳定后再做 Windows

Windows 兼容前，先保证 Mac/Linux 主链路稳定。Go 代码中路径处理使用 `filepath`，不要写死 `/`。进程、信号、PID、daemon stop/start 这类逻辑要保持平台隔离。

跨平台改动至少做 Linux 交叉构建。Windows 适配时优先处理 daemon 生命周期、路径、文件锁、SQLite、shell 命令调用等系统差异。

## 推荐修改流程

1. 先画清楚改动影响哪段链路：scan、evidence、report、weekly、agent-context、publish、daemon、store。
2. 再修改最靠近事实源的模块，避免在上层用字符串补丁掩盖数据问题。
3. 修改后补单测，尤其是历史上出过问题的边界：空 evidence、旧 commit、非法 JSON、敏感路径、重复 scan run、发布失败。
4. 最后跑完整回归，并检查生成文件中没有真实 secret、webhook、本地私有路径泄漏。

## 近期工程方向

- 刷新 docs 中过期的命令示例，保持和实际 CLI 一致。
- 为 SQLite 增加更明确的迁移策略，避免未来 schema 变化只靠 `CREATE TABLE IF NOT EXISTS`。
- Windows 兼容前补一套 daemon 与路径相关的回归脚本。
- 长期把 publish events、report records、weekly trend analysis 做成可查询的产品能力，而不是只保存 Markdown 文件。
