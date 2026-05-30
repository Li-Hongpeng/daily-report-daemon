# daily-report-daemon

本地优先的开发活动观察与报告生成工具。自动扫描 Git 仓库变更，使用 AI 生成日报、Agent 上下文，支持团队协作与钉钉推送。

当前版本：Phase 3

## 安装

```bash
# 克隆仓库
git clone https://github.com/Li-Hongpeng/daily-report-daemon.git
cd daily-report-daemon

# 安装到系统 PATH（推荐，安装后直接使用 daily-report-daemon 命令）
go install ./cmd/daily-report-daemon
# 确保 $GOPATH/bin 在 PATH 中: export PATH=$PATH:$(go env GOPATH)/bin

# 或者本地编译（生成 ./daily-report-daemon 文件）
make build
# 或: go build -o daily-report-daemon ./cmd/daily-report-daemon
```

## 快速开始

```bash
# 验证安装
daily-report-daemon --version

# 配置 API Key（DeepSeek 推荐）
export DEEPSEEK_API_KEY=sk-your-key

# 在项目目录初始化
cd /path/to/your/project
daily-report-daemon init -w .

# 一键生成日报 + Agent 上下文
daily-report-daemon run -w .
```

详细操作见 [用户手册](./docs/USER-GUIDE.md)。

## 核心功能

- 📊 **自动日报**：扫描 Git 活动，AI 生成结构化日报（概览/完成/变更/风险/建议）
- 🤖 **Agent 引擎**：AI 主动分析代码变更、追问细节、交叉验证，非被动生成
- 📁 **Agent 上下文**：自动生成 AGENTS.generated.md，供 Claude Code/Cursor 等工具使用
- 🔄 **后台服务**：daemon 常驻，定时扫描 + 自动日报
- 👥 **团队协作**：内网共享目录聚合成员日报，钉钉推送
- 🔒 **隐私保护**：本地优先，敏感信息自动过滤，API Key 加密存储

## 命令速查

| 命令 | 说明 |
|------|------|
| `init -w .` | 初始化工作区 |
| `run -w .` | 一键：扫描 + 日报 + Agent 上下文 |
| `scan -w . --no-llm` | 仅扫描（不调用 AI） |
| `report -w .` | 生成日报 |
| `agent-context -w .` | 生成 Agent 上下文 |
| `daemon start -w .` | 启动后台服务 |
| `daemon status -w .` | 查看服务状态 |
| `daemon stop -w .` | 停止后台服务 |

## 环境要求

- Go 1.19+
- Git
- DeepSeek API Key（或 OpenAI-compatible API）
- 完整用户手册：[docs/USER-GUIDE.md](./docs/USER-GUIDE.md)

## 文档

- [用户手册](./docs/USER-GUIDE.md)
- [PRD v1](./docs/PRD-v1.md)
- [Phase 2 PRD](./docs/PRD-PHASE-2.md)
- [技术架构](./docs/TECHNICAL-ARCHITECTURE.md)
- [Phase 0 任务](./docs/PHASE-0-TASKS.md)
- [Phase 2 任务](./docs/PHASE-2-TASKS.md)
- [Phase 3 任务](./docs/PHASE-3-TASKS.md)

## 样例

- [样例日报](./examples/sample-report.md)
- [样例 Agent 上下文](./examples/sample-agents-generated.md)
- [样例 Evidence](./examples/evidence.jsonl)
