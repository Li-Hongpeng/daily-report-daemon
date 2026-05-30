# daily-report-daemon Phase 2 PRD

版本：v0.1
日期：2026-05-29
阶段：Agent 驱动的智能报告生成
作者：产品经理
前置：Phase 0 验收通过（2026-05-29）

## 1. Phase 0 回顾与 Phase 2 动机

### 1.1 Phase 0 已验证

- ✅ 本地 Git 仓库提取开发活动 evidence：可行
- ✅ 基于 evidence 生成可信日报：可行，报告"可直接使用"
- ✅ 生成 coding agent 可用的 agent context：可行
- ✅ 单次 LLM 调用的线性流水线（scan → evidence → prompt → render）能跑通

### 1.2 Phase 0 暴露的天花板

M10 真实仓库试跑暴露了单次 prompt 调用的固有限制：

1. **Known Risks 只能列 diff，做不了真正的风险分析**——模型看到了 diff 内容，但不知道"这个改动为什么反复出现""关联模块在哪"
2. **Coding Conventions 输出重复**——模型无法在生成后自检
3. **local-daemon-agent 的 metadata 被误纳入当日概览**——模型缺乏"这条信息跟今天的工作有什么关系"的判断力
4. **所有结论的质量完全依赖单次 prompt 工程**——没有追问、没有交叉验证、没有自我纠错

根因：**Phase 0 的报告生成是"被动翻译"（evidence → report），不是"主动分析"（evidence → 理解 → 追问 → 综合分析 → report）。**

### 1.3 Phase 2 核心命题

把报告生成从 **"一次性 prompt 调用"** 升级为 **"agent 驱动的多步推理过程"**。

不是让 prompt 更聪明，而是让生成引擎有了"追问的能力"。

## 2. 产品定位更新

### 2.1 不变的部分

- 本地优先、透明可控、最小必要、证据优先
- 定位为"开发者的自动工作秘书"，不是"管理者的监控器"
- 首批目标用户：研发小组

### 2.2 新增的产品原则

6. **主动推理优于被动生成**：agent 应该主动分析、追问、交叉验证，而不是把 evidence 灌进 prompt 等输出。
7. **可解释的推理过程**：开发者能看到 agent 做了什么分析、调了什么工具、为什么得出这个结论——不是黑箱。

## 3. Agent 引擎设计

### 3.1 推理 Pipeline

```
Phase 0（线性）:
  evidence → [单次 LLM] → structured JSON → Markdown

Phase 2（Agent 多步）:
  evidence → [Step 1: 理解] → [Step 2: 追问] → [Step 3: 综合] → structured JSON → Markdown
                ↓                  ↓                 ↓
          分析今天的变更       发现 gap 后调工具   交叉验证 + 生成报告
          在讲什么故事         深挖细节            标注置信度
```

### 3.2 三步推理详解

**Step 1 — 理解（Analyze）**：
- Agent 接收 evidence pack
- 识别今日工作主题：功能开发？bugfix？重构？文档？
- 按模块/功能聚类变更
- 标记需要追问的 gap（如"这个文件改了三次但不知道为什么""这个模块的测试没变但逻辑改了"）

**Step 2 — 追问（Investigate）**：
- Agent 调用工具填补 gap
- 可用工具：git log explore、git diff detail、read file、search patterns、list directory
- 追问有预算限制（最多 N 次工具调用，防止无限循环）
- 追问过程记录在推理日志中

**Step 3 — 综合（Synthesize）**：
- Agent 将原始 evidence + 追问发现综合成完整叙事
- 交叉验证：evidence ID 是否准确？结论是否有证据支撑？
- 生成结构化日报 JSON
- 标注置信度：高（有直接 evidence）/ 中（推断但有工具调用支撑）/ 低（纯推断）

### 3.3 Agent 工具集（Phase 2 v1）

| 工具 | 功能 | 触发条件 |
|------|------|---------|
| `git_log_explore` | 深挖特定文件的 commit 历史 | 文件被多次修改，需理解演变 |
| `git_diff_detail` | 获取特定文件的完整 diff | summary diff 不够，需细节 |
| `read_file` | 读取项目文件内容 | 需要文件上下文理解变更 |
| `search_pattern` | 扫描 TODO/FIXME/HACK/XXX | 识别技术债务信号 |
| `list_directory` | 浏览目录结构 | 理解模块关系 |

### 3.4 多步推理 vs 自由 Agent Loop

Phase 2 采用 **结构化多步推理**，不是完全自由的 agent loop：

- **结构化**：3 个预定义步骤（理解/追问/综合），每步有明确的输入/输出 schema
- **有限工具调用**：追问阶段最多 5 次工具调用
- **可预测成本**：每份报告 3-8 次 LLM 调用（vs Phase 0 的 2 次）
- **可测试**：每步输出可独立验证

原因：完全自由的 agent loop 在 V1 阶段成本不可控、行为不可预测、难以测试。结构化多步推理在灵活性和可控性之间取了平衡。Phase 3 可以演进为更自由的 agent。

### 3.5 Agent 运行位置

Agent loop 运行在本地 daemon 进程内。LLM 调用走配置的 provider（DeepSeek/OpenAI/Ollama）。

此设计保持本地优先的产品原则——采集、存储、推理编排都在本机，只有 LLM 调用出站。

## 4. 里程碑与交付计划

### M1：Agent 引擎核心（P0，Phase 2 的核心交付）

**目标**：实现多步推理 pipeline，替代 Phase 0 的单次 LLM 调用。

**具体**：
- 实现三步推理引擎（Analyze → Investigate → Synthesize）
- 实现 5 个 agent 工具
- 工具调用预算控制（最多 5 次追问）
- 推理日志记录（每次工具调用 + 输入输出）
- 结构化日报 JSON schema 扩展（新增 confidence 字段）
- DeepSeek tool-calling API 适配
- `--agent-trace` flag：输出完整推理过程供调试

**验收**：
- 同一份 evidence，agent 生成的报告在风险分析维度明显优于 Phase 0 baseline
- agent 至少触发 1 次工具调用来填补 evidence gap
- 推理日志完整可读
- 报告成本不超过 Phase 0 的 5 倍

**工期**：核心引擎 2-3 天，工具 + 调试 1-2 天

### M2：Daemon 化（P0）

**目标**：从手动 CLI 演进为后台常驻服务。

**具体**：
- 周期扫描引擎（configurable interval，默认 30 分钟）
- SQLite 存储（替代 Phase 0 的 JSONL/Markdown 文件）
- 多 workspace 配置
- Daemon 进程管理（start / stop / status / restart）
- 定时自动日报生成（用户配置触发时间，如下午 5:30）
- 异步生成 + 桌面通知

**验收**：
- daemon 能在后台持续运行，不阻塞终端
- 重启后从上次状态增量扫描
- 日报在预设时间自动生成，无需手动触发
- SQLite 可查询历史 runs 和 reports

**工期**：3-4 天

### M3：Agent 驱动的周报（P0）

**目标**：多日聚合 + 趋势分析，同样由 agent 生成。

**具体**：
- Agent 读取本周每日 evidence 和日报
- 识别跨日模式：连续修改的模块、反复出现的问题、逐日增长的风险
- 周报结构：本周完成 / 重点项目进展 / 风险与延期 / 质量趋势 / 下周计划
- 与日报共享 agent 引擎，仅 prompt 和工具策略不同

**验收**：
- 周报不是"7 天日报拼凑"，而是有跨日叙事
- 能识别本周内反复出现的风险信号
- 能生成"下周建议"且不空洞

**工期**：1-2 天

### M4：组长版报告 + Email 发送（P1）

**具体**：
- 组长版日报：技术细节精简、突出进展/风险/需要协助
- SMTP email 发送（配置 SMTP server + 收件人）
- 手动确认发送或 N 分钟撤回窗口
- Webhook 发送（备用渠道）

**工期**：1-2 天

### M5：Phase 0 Backlog 清偿 + 基础 Web UI（P1-P2）

**具体**：
- 普通目录扫描（非 Git workspace）
- 分层 LLM（小模型做摘要，大模型做报告——注意与 agent 引擎的协同）
- API key 加密存储
- Token 预算控制（按日 / 按仓库上限）
- Git stash 采集
- 6 个 P3 代码改进
- 本地 Web UI 基础版：今日报告查看、历史浏览、配置管理

**工期**：3-5 天

## 5. 与原 PRD 路线图的差异

| 项目 | 原 PRD Phase 1 | 新 Phase 2 | 理由 |
|------|---------------|-----------|------|
| 报告生成方式 | 单次 LLM prompt | Agent 多步推理 | Phase 0 验证了单次 prompt 的天花板 |
| 优先级 | daemon 排第一 | agent 引擎排第一 | 报告质量是根基，daemon 是分发层 |
| 周报 | "Phase 1" 有但未详述 | agent 驱动，跨日叙事 | 周报同样需要 agent 能力 |
| 分层 LLM | Phase 1 做 | Phase 2 M5 再评估 | agent 模式可能改变分层策略 |
| 本地 Web UI | Phase 2 | M5 P2 | daemon + agent 优先于 UI |

## 6. 关键产品决策（需 lee 确认）

1. **Agent 推理深度**：结构化三步推理（推荐） vs 自由 agent loop？——推荐结构化，成本可控、可测试
2. **工具调用预算**：每份报告最多几次工具调用？——建议 5 次，M10 后根据实际 gap 数量调
3. **Agent 推理日志**：开发者是否能看到完整的 agent 推理过程？——建议默认可见（`--agent-trace`），这是"透明可控"产品原则的延伸
4. **周报 agent 是否独立**：日报和周报共用同一套 agent 引擎还是各自独立？——建议共用引擎，仅 prompt/工具策略不同
5. **DeepSeek tool-calling 兼容性**：需要技术负责人验证 DeepSeek API 的 tool calling 支持程度

## 7. 风险与应对

| 风险 | 影响 | 应对 |
|------|------|------|
| Agent 成本失控 | 每次报告 token 消耗远超预期 | M1 就有 token 预算控制；结构化步骤天然限制调用次数 |
| Agent 幻觉放大 | 多步推理可能产生更严重的幻觉 | 每步输出独立验证；推理日志可审计；evidence 回溯 |
| DeepSeek tool calling 不成熟 | 工具调用不稳定 | M1 先验证 tool calling；如有问题回退到 prompt-based 工具模拟 |
| 生成延迟 | agent 模式比单次 prompt 慢 3-10 倍 | 异步生成 + 通知；用户不需要等待 |
| Agent 行为不可预测 | 同一 evidence 不同 run 结果差异大 | 结构化步骤限定行为空间；temperature 可配 |

## 8. 成功指标

### 8.1 功能指标

- Agent 每份报告至少触发 1 次主动工具调用
- 周报能识别跨日模式（非简单拼接）
- Daemon 在后台持续运行 24 小时不崩溃

### 8.2 质量指标（vs Phase 0 baseline）

- 报告"推断"标记占比下降 50% 以上（agent 能追到 evidence）
- 风险分析准确率提升（lee 主观评分 1-5，目标 ≥ 4）
- 日报采纳率：开发者直接使用或少量修改比例 ≥ 80%

### 8.3 成本指标

- 单份报告平均 token 消耗 ≤ Phase 0 的 5 倍
- Token 预算控制生效：达到上限后降级为简化报告而非崩溃

## 9. 不做的事（Phase 2 非目标）

- 不做完全自由的 agent loop（留给 Phase 3）
- 不做团队聚合看板和趋势大盘（Phase 3）
- 不做钉钉/飞书/企业微信 IM 集成（Phase 3）
- 不做 Jira/Linear/GitHub Issues 双向同步（Phase 3）
- 不做企业级权限、审计、策略下发（Phase 4）
- 不做个人绩效排名、工时考勤（永久不做）
