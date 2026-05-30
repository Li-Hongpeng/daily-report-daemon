# daily-report-daemon Phase 3 任务拆解

版本：v0.1  
日期：2026-05-30  
关联：docs/PHASE-3-TASKS.md（PM）  
状态：已确认，开工

## 已确认决策

| # | 决策 | 结论 |
|---|------|------|
| 1 | 团队部署方式 | 成员各自安装 daemon，内网共享目录同步报告 |
| 2 | IM 优先级 | 只做钉钉，其他延后 |
| 3 | Agent 自由推理 | 渐进式——先保持三步，M5 逐步放开 |
| 4 | 普通目录隐私 | 规则过滤高敏信息（身份证号、银行卡号等），不过度复杂 |
| 5 | 团队看板访问 | 组长→自己团队，管理员→所有人，无需审批 |

## 推荐实施顺序

```
M1（Backlog + Agent增强）→ M2（目录 + stash）→ M3（团队聚合）→ M4（钉钉）
                                                         ↘ M5（Agent自由 + 趋势）
```

## M1：Backlog 清偿 + Agent 增强（P0，2-3 天）

- [ ] Phase 0 P3 × 6：collectUntrackedDiffs 去重、--since 参数、evidence index 修复、filterByDate 修复、metadata 复用、中文 token 估算修正
- [ ] Phase 2 P3 × 4：isReportTime 精度修复、HasChanged 加 hash、encrypt 注释、webui 内容展示
- [ ] --agent-trace CLI 化：run 模式也能输出完整推理日志
- [ ] Coding Conventions 去重验证

验收：go test 全绿，--agent-trace CLI+daemon 均可输出

## M2：普通目录 + Git Stash（P0，2-3 天）

- [ ] 普通目录扫描：--workspace-type directory，文本内容摘要 + 非文本 metadata
- [ ] 增量检测：基于 SQLite file_snapshots 的 mtime 变更
- [ ] 高敏过滤：身份证号、银行卡号、手机号等规则过滤
- [ ] git stash 采集：stash list → evidence
- [ ] agent 将 stash 作为"未完成工作"信号

验收：普通目录正常扫描，stash 进入 evidence

## M3：团队聚合与看板（P0，3-5 天）

- [ ] 团队配置：成员列表 + 各成员共享目录路径
- [ ] 报告同步：daemon 产出到共享目录，聚合节点读取
- [ ] 团队日报/周报聚合
- [ ] 项目维度视图
- [ ] 团队看板 Web UI：日报/周报查看、成员活动一览、项目进展卡片
- [ ] 权限：组长看自己团队，admin 看全部
- [ ] 隐私：成员控制上报粒度（完整/摘要/仅进度）

验收：2 人以上团队聚合成功，Web 看板可用

## M4：钉钉集成（P1，2-3 天）

- [ ] 钉钉群机器人 Webhook
- [ ] 日报/周报 Markdown → 钉钉格式转换
- [ ] 手动确认 + 撤回窗口
- [ ] 统一 IM 发送框架（预留飞书/企微扩展点）

验收：钉钉群成功收到日报

## M5：Agent 自由推理 + 趋势分析（P1，2-3 天）

- [ ] 三步→五步循环：Plan→Act→Observe→Reflect→Generate，最大 10 轮
- [ ] Agent 自行决定终止条件
- [ ] 跨天记忆增强
- [ ] 代码质量趋势：bug 密度、TODO 增长率
- [ ] 风险趋势：跨天风险项演变追踪
- [ ] Agent Prompt Pack：按项目类型推荐 prompt

验收：自由循环不失控，趋势有具体数字，Prompt Pack ≥ 2 个
