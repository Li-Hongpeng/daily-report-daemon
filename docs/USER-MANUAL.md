# daily-report-daemon 用户手册

版本：v3.0（Phase 3）  
日期：2026-05-30

## 1. 产品简介

daily-report-daemon 是一个本地优先的开发活动观察与报告生成守护进程。它自动采集你的 Git 仓库和普通目录中的工作痕迹，使用 AI 生成日报、周报和项目上下文文档，并支持推送到钉钉群。

**核心能力**：
- 自动扫描 Git 仓库和普通目录，生成开发活动 evidence
- Agent 三步推理：分析 → 追问 → 综合，替代简单的一次性 AI 调用
- 生成开发者日报、团队周报、`AGENTS.generated.md`（给 AI 编程助手的项目上下文）
- 钉钉群机器人推送日报
- 团队模式：多人报告聚合 + 组长/管理员权限

## 2. 快速开始

### 2.1 安装

```bash
# 从源码构建
git clone https://github.com/Li-Hongpeng/daily-report-daemon.git
cd daily-report-daemon
make build

# 验证
./daily-report-daemon --version
```

### 2.2 初始化

```bash
# 设置 API Key
export DEEPSEEK_API_KEY=sk-your-key-here

# 在工作目录初始化
./daily-report-daemon init --workspace /path/to/your/project

# 输出：
# ✓  DEEPSEEK_API_KEY detected — using DeepSeek API
# Initialized daily-report-daemon
#   config:  .daily-report-daemon/config.yaml
#   runs:    .daily-report-daemon/runs
#   reports: .daily-report-daemon/reports
#   context: .daily-report-daemon/context
```

### 2.3 生成第一份日报

```bash
./daily-report-daemon run --workspace /path/to/your/project

# 输出：
# === Run Summary ===
# Files scanned:   113
# Diff files:      5
# Redactions:      0
# Evidence:        .daily-report-daemon/runs/2026-05-30-090000/evidence.jsonl
# Report:          .daily-report-daemon/reports/2026-05-30.md
# Agent context:   .daily-report-daemon/context/AGENTS.generated.md
```

## 3. 命令参考

### 3.1 核心命令

| 命令 | 功能 | 示例 |
|------|------|------|
| `init` | 初始化工作目录 | `daily-report-daemon init -w ./myproject` |
| `scan` | 扫描工作目录（不生成报告） | `daily-report-daemon scan -w ./myproject` |
| `run` | 一键：扫描 + 日报 + agent context | `daily-report-daemon run -w ./myproject` |
| `report today` | 仅生成日报 | `daily-report-daemon report today -w ./myproject` |
| `agent-context generate` | 仅生成 agent 上下文 | `daily-report-daemon agent-context generate -w ./myproject` |

### 3.2 Daemon 模式

```bash
# 启动后台守护进程（每 30 分钟扫描，定时生成日报）
./daily-report-daemon daemon start

# 查看状态
./daily-report-daemon daemon status

# 停止
./daily-report-daemon daemon stop

# 重启
./daily-report-daemon daemon restart
```

### 3.3 常用 Flag

| Flag | 功能 |
|------|------|
| `--workspace / -w` | 指定工作目录路径 |
| `--dry-run` | 预览：生成 evidence 但不调 AI |
| `--no-llm` | 只扫描，不调 AI |
| `--agent-trace` | 输出 agent 三步推理完整日志 |

## 4. 配置说明

配置文件：`.daily-report-daemon/config.yaml`

```yaml
llm:
  provider: openai-compatible
  base_url: https://api.deepseek.com/v1
  model: deepseek-chat         # 或 deepseek-v4-pro
  api_key_env: DEEPSEEK_API_KEY

workspace:
  path: /path/to/project
  type: git_repo               # git_repo 或 directory
  git_enabled: true
  docs_enabled: true

reports:
  output_dir: .daily-report-daemon/reports

publisher:
  enabled: false
  primary_channel: email
```

**支持的环境变量**：
- `DEEPSEEK_API_KEY` — DeepSeek API 密钥（推荐）
- `OPENAI_API_KEY` — OpenAI API 密钥（备选）

## 5. 钉钉推送

### 5.1 配置

1. 在钉钉群中：群设置 → 智能群助手 → 添加机器人 → 复制 Webhook URL
2. 机器人安全设置需添加关键词：`日报`、`周报` 或 `代码`
3. 配置 daemon 发送：

```bash
# 钉钉 Webhook 通过 daemon 配置使用
./daily-report-daemon daemon start --publish-dingtalk \
  --dingtalk-webhook "https://oapi.dingtalk.com/robot/send?access_token=xxx"
```

### 5.2 发送模式

- **手动确认**（推荐）：日报生成后需手动确认才推送
- **自动发送**：日报生成后自动推送（预留 N 分钟撤回窗口）

## 6. 团队模式

### 6.1 部署方式

各成员各自安装 daemon，将日报输出到内网共享目录，聚合节点读取生成团队视图。

```
/共享目录/
  dev-01/           # 成员 1 的报告
    reports/
  dev-02/           # 成员 2 的报告
    reports/
  team/             # 聚合节点读取
```

### 6.2 权限级别

| 角色 | 权限 |
|------|------|
| 成员 (member) | 查看自己的日报 |
| 组长 (lead) | 查看本团队所有成员日报 |
| 管理员 (admin) | 查看所有团队日报 |

## 7. Agent 引擎

### 7.1 三步推理

Phase 2+ 的 agent 引擎用三步推理替代一次性的 AI 调用：

1. **Analyze（分析）**：聚类今日变更，识别信息缺口
2. **Investigate（追问）**：调用工具（git log/diff/read file/search）填补缺口
3. **Synthesize（综合）**：交叉验证 + 标注置信度 + 生成报告

### 7.2 查看推理过程

```bash
./daily-report-daemon run -w ./myproject --agent-trace

# 输出示例：
# [analyze] 识别缺口：模块 A 连续修改 3 次但 commit message 仅 "fix"
# [investigate] read_file → 读取 diff 详情
# [investigate] git_log_explore → 查询历史变更
# [synthesize] 综合生成：4 sections + 3 gaps 已填补
```

## 8. 隐私与安全

### 8.1 数据保护

- **本地优先**：所有采集和存储在本地完成
- **敏感过滤**：自动过滤 API key、私钥、密码、身份证号、银行卡号、手机号
- **透明可控**：`--dry-run` 预览即将发送给 AI 的内容
- **不上传源码**：AI 调用只发送脱敏后的摘要

### 8.2 产品原则

- 定位为"开发者的自动工作秘书"，不是"管理者的监控器"
- 不做键盘/鼠标/截图监控
- 不做绩效排名和工时考勤
- 报告强调进展和阻塞，不强调工时

## 9. 产物说明

```
.daily-report-daemon/
  config.yaml                      # 配置文件
  runs/2026-05-30-090000/
    evidence.jsonl                 # 脱敏后的活动证据
    git-activity.json              # Git 活动原始数据
    project-metadata.json          # 项目结构元信息
    redaction-report.json          # 脱敏统计
  reports/
    2026-05-30.md                  # 开发者日报
    weekly/2026-W22.md             # 周报
  context/
    AGENTS.generated.md            # Agent 项目上下文
```

## 10. 常见问题

**Q: 支持哪些 AI 模型？**  
A: OpenAI 兼容协议均可。已测试 DeepSeek（v3/v4）、OpenAI。设置对应的 `API_KEY` 和 `base_url` 即可。

**Q: 日报生成需要多长时间？**  
A: 扫描 1-3 秒，AI 生成 15-60 秒（取决于变更量）。Agent 多步推理比单次生成慢 50% 但报告质量显著更好。

**Q: 会影响 Git 仓库吗？**  
A: 不会。工具只读不写，不修改任何文件，不创建 commit。

**Q: 没有 Git 的目录能用吗？**  
A: 可以。使用 `--workspace-type directory` 即可扫描普通目录中的文本文件变化。

**Q: 报告可以发给谁？**  
A: 个人模式：自己看。团队模式：组长看自己团队。钉钉模式：群内所有人。
