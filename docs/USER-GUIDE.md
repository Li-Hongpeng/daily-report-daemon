# daily-report-daemon 用户手册

版本：Phase 3 | 日期：2026-05-30

## 1. 简介

daily-report-daemon 是一个本地优先的开发活动观察与报告生成工具。它在你授权的 Git 仓库内持续收集代码变更信号，使用 AI（DeepSeek/OpenAI）自动生成个人日报、Agent 上下文文档，并支持团队聚合与钉钉推送。

**核心能力**：
- 📊 自动日报：下班前自动生成，无需手动回忆今天做了什么
- 🤖 Agent 引擎：AI 主动分析代码变更、追问细节、交叉验证
- 📁 Agent 上下文：自动生成 AGENTS.generated.md，帮助 Claude Code/Cursor 等工具理解项目
- 👥 团队协作：内网共享目录聚合成员日报，钉钉推送
- 🔒 本地优先：采集和存储都在本机，支持脱敏过滤

## 2. 安装

### 前置要求
- Go 1.19+
- Git
- DeepSeek API Key（或 OpenAI API Key）

### 安装步骤

```bash
# 克隆仓库
git clone https://github.com/Li-Hongpeng/daily-report-daemon.git
cd daily-report-daemon

# 编译
go build -o daily-report-daemon ./cmd/daily-report-daemon

# 验证
./daily-report-daemon --version
```

## 3. 快速开始

### 3.1 初始化工作区

```bash
# 在你的项目根目录执行
cd /path/to/your/project
./daily-report-daemon init -w .
```

这会创建 `.daily-report-daemon/` 目录，包含配置文件。

### 3.2 配置 API Key

```bash
# DeepSeek（推荐）
export DEEPSEEK_API_KEY=sk-your-key

# 或 OpenAI
export OPENAI_API_KEY=sk-your-key
```

工具会自动检测环境变量，DeepSeek key 存在时优先使用 DeepSeek。

### 3.3 生成第一份日报

```bash
# 一键运行：扫描 + 生成日报 + Agent 上下文
./daily-report-daemon run -w .
```

输出位置：
- 日报：`.daily-report-daemon/reports/YYYY-MM-DD.md`
- Agent 上下文：`.daily-report-daemon/context/AGENTS.generated.md`
- 扫描数据：`.daily-report-daemon/runs/YYYY-MM-DD-HHMMSS/`

## 4. 命令参考

| 命令 | 说明 | 示例 |
|------|------|------|
| `init` | 初始化工作区 | `daily-report-daemon init -w .` |
| `scan` | 扫描工作区（不调用 AI） | `daily-report-daemon scan -w . --no-llm` |
| `report` | 生成日报 | `daily-report-daemon report -w .` |
| `agent-context` | 生成 Agent 上下文 | `daily-report-daemon agent-context -w .` |
| `run` | 一键：扫描+日报+上下文 | `daily-report-daemon run -w .` |
| `daemon start` | 启动后台服务 | `daily-report-daemon daemon start -w .` |
| `daemon stop` | 停止后台服务 | `daily-report-daemon daemon stop -w .` |
| `daemon status` | 查看服务状态 | `daily-report-daemon daemon status -w .` |

### 常用参数

| 参数 | 说明 |
|------|------|
| `-w, --workspace` | 指定工作区路径（默认当前目录） |
| `--dry-run` | 预览模式，不调用 AI |
| `--no-llm` | 跳过 AI 调用，只收集数据 |

## 5. 日报说明

### 日报结构

```
# 日报 — 2026-05-30
## 今日概览
- 完成 Phase 0 原型核心模块开发
## 完成事项
- 实现 Git Analyzer 模块（证据: diff:analyzer.go:abc123）
## 关键代码变更
- internal/git/analyzer.go: 新增 untracked 文件处理
## 风险与待确认
- [MEDIUM] LLM 超时可能导致报告生成失败
## 可能卡点
- 无
## 明日建议
- 完成端到端命令集成
## 证据索引
- diff:analyzer.go:abc123: 新增 untracked 文件处理
```

### 证据标记说明
- 带 `evidence_id` 的条目：有实际文件/commit 证据支撑
- 标记 `⚠推断` 的条目：AI 基于上下文推断，无直接证据
- 证据索引：报告末尾列出所有引用的证据来源

## 6. Agent 上下文使用

生成的 `AGENTS.generated.md` 可以直接用于 Claude Code、Cursor、Codex 等工具：

**Claude Code**：
```bash
# 在项目目录启动 Claude Code，它会自动读取
claude
```

**手动使用**：
```bash
cat .daily-report-daemon/context/AGENTS.generated.md
# 将内容粘贴到 AI 工具的"项目说明"或"自定义指令"中
```

### Agent 上下文内容
- 项目概览（从 README 提取）
- 目录结构和主要语言
- 构建/运行/测试命令
- 代码规范
- 近期活动摘要
- 已知风险和开放问题
- AI 工具的建议 prompt

## 7. 后台服务（Daemon）

### 启动后台服务

```bash
./daily-report-daemon daemon start -w .
```

启动后，daemon 会：
- 每 30 分钟自动扫描工作区
- 每天 17:30 自动生成日报
- 重启后从上次状态增量扫描

### 管理后台服务

```bash
./daily-report-daemon daemon status   # 查看状态
./daily-report-daemon daemon stop     # 停止
./daily-report-daemon daemon restart  # 重启
```

## 8. 团队协作

### 8.1 共享目录模式

1. **管理员**在内网创建共享目录（如 NAS 或共享文件夹）
2. **各成员**在自己的 daemon 配置中设置共享目录路径
3. **组长**在共享目录中查看聚合报告

### 8.2 配置团队功能

编辑 `.daily-report-daemon/config.yaml`：

```yaml
team:
  shared_dir: /mnt/nas/team-reports  # 内网共享目录
  role: member                         # member / lead / admin
  team_name: 研发一组
  members:                             # lead/admin 配置
    - 张三
    - 李四
```

### 8.3 权限说明

| 角色 | 可查看 |
|------|--------|
| member | 自己的报告 |
| lead | 所管团队所有成员的报告 |
| admin | 所有团队的报告 |

### 8.4 钉钉推送

1. 在钉钉群中添加自定义机器人，获取 webhook URL
2. 配置 webhook：

```yaml
dingtalk:
  webhook_url: https://oapi.dingtalk.com/robot/send?access_token=xxx
```

3. 报告会自动推送到钉钉群（标题包含"日报"关键词）
4. 支持推送模式：手动确认 / 自动发送 / dry-run 预览

## 9. 隐私与安全

### 自动过滤

工具默认过滤以下敏感信息：
- `.env` 及其变体（.env.local、.env.production 等）
- 私钥文件（*.pem、id_rsa 等）
- 包含 secret/token/password 的路径
- 代码中的 API key、密码、Token、AWS Key
- 身份证号、银行卡号、手机号
- JWT Token、数据库连接串

### 查看脱敏报告

```bash
# 每次扫描后查看脱敏统计
cat .daily-report-daemon/runs/*/redaction-report.json
```

### API Key 加密

API Key 使用 AES-256-GCM 本地加密存储，密钥绑定本机。

## 10. 常见问题

**Q: 日报内容不准确怎么办？**
A: 日报质量依赖 Git 活动质量。确保 commit message 描述清晰，定期提交代码。Agent 引擎会自动追问细节。

**Q: 生成太慢？**
A: 使用 `--dry-run` 预览即将发送给 AI 的内容。日报生成通常在 1-3 分钟内完成。

**Q: 如何排除某些文件？**
A: 编辑 `.daily-report-daemon/config.yaml` 中的 `exclude` 列表。

**Q: 支持哪些 AI 模型？**
A: DeepSeek（默认）、OpenAI 及任何 OpenAI-compatible API。

**Q: 数据存在哪里？**
A: 全部存储在工作区的 `.daily-report-daemon/` 目录下。不上传云端。

## 11. 升级与卸载

```bash
# 升级
git pull && go build -o daily-report-daemon ./cmd/daily-report-daemon

# 卸载
rm -rf .daily-report-daemon/
rm daily-report-daemon
```

## 12. 获取帮助

- 查看命令帮助：`./daily-report-daemon help`
- 查看子命令帮助：`./daily-report-daemon scan --help`
- GitHub Issues：https://github.com/Li-Hongpeng/daily-report-daemon/issues
