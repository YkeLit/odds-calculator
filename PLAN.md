## 德州个人决策模式重设计（基于下注行为与赔率）

### Summary
- 将德州从“已知所有玩家手牌的复盘工具”改为“Hero 个人实时决策工具”，前端完全替换为个人模式。
- 核心逻辑改为：对手未知手牌 + 基于下注行为动态收缩范围 + 多街近似全树求解，输出 `Top3动作 + EV + 置信区间`。
- 覆盖四街（Preflop/Flop/Turn/River），现金局优先，最多 6 人，支持简化输入和结构化动作序列两种录入方式。

### Key Changes
- **API / 类型（新增主接口）**
  - 新增 `POST /api/v1/holdem/decision` 作为德州主接口，前端仅使用该接口。
  - 请求采用统一“结构化场景”：
  - `hero`: `holeCards[2]`, `position`, `stack`
  - `table`: `playerCount(2..6)`, `positions`, `effectiveStacks`, `rakeConfig`
  - `street`: `preflop|flop|turn|river`
  - `boardCards(0..5)`, `deadCards[]`
  - `potState`: `potSize`, `toCall`, `minRaiseTo`, `blinds`
  - `actionHistory[]`: `{street, actor, action(fold/call/check/bet/raise/allin), amount}`
  - `opponents[]`: `{id, position, stylePreset, rangeOverride?}`
  - `solverConfig`: `{branchCount(2..5), timeoutMs, rolloutBudget}`
  - 响应：
  - `topActions[3]`: `{action, amount?, ev, ciLow, ciHigh, frequency}`
  - `heroMetrics`: `{equity, tieRate, potOdds, requiredEquity}`
  - `opponentRangeSummary[]`: 每位对手当前范围覆盖率与Top组合
  - `treeStats`: `{nodes, rollouts, depthReached, elapsedMs, convergence}`
- **对手范围与动作建模**
  - 初始范围支持两种来源并可混用：
  - 内置位置默认范围（UTG/MP/CO/BTN/SB/BB）
  - 手动覆盖范围 `rangeOverride`（语法：`AA,AKs,AQo,JJ:0.7`，权重 0~1，默认 1）
  - 行为收缩机制：对 `actionHistory` 逐条应用街道+动作+下注尺度的似然权重并归一化，形成后验范围。
  - 同时提供简化输入模式（当前底池/需跟注/最近动作/对手风格），前端先映射为结构化场景再调用统一接口。
- **多街近似全树求解器**
  - 使用“采样 Expectimax 全街树”：
  - Hero 节点：`fold/call` + 可配置 `N` 档加注（始终包含 all-in）
  - 加注档位生成规则（确定性）：
    - `N=2`: `[1.0x pot, all-in]`
    - `N=3`: `[0.5x, 1.0x, all-in]`
    - `N=4`: `[0.5x, 0.75x, 1.0x, all-in]`
    - `N=5`: `[0.5x, 0.75x, 1.0x, 1.5x, all-in]`
  - 对手节点：按后验范围+策略模型做期望动作，不做“完美对抗求解”。
  - 机会节点：抽样未来公共牌与对手手牌；默认预算 `8k~30k rollouts`，`timeoutMs` 默认 `5000`。
  - 置信区间：基于 rollout EV 分布做 bootstrap（固定样本数）输出 `ciLow/ciHigh`。
- **前端重构（完全替换旧德州流程）**
  - 新德州页分两层输入：
  - `快速模式`：Hero 手牌、公共牌、人数、底池、需跟注、有效筹码、对手风格。
  - `专业模式`：结构化动作时间线编辑器（按街道逐条录入 bet/call/raise/fold）。
  - 对手区不再录入具体手牌，改为“位置 + 风格 + 可选手动范围覆盖”。
  - 结果区展示：
  - Top3 动作卡片（EV/置信区间/推荐强度）
  - 核心指标（Hero equity、pot odds、required equity）
  - 可视化（动作EV柱状图 + 对手范围覆盖/权重图）
  - 旧“已知全部手牌”的德州页面入口移除；历史记录改写为 `holdem_decision`。

### Test Plan
- **单元测试**
  - 范围解析与权重合法性（含 `AA:0.7`、非法 token、重复覆盖规则）。
  - 行为收缩（不同街道/动作/下注尺度）后范围概率归一化与单调性。
  - 分支生成（`N=2..5`）与动作合法性校验。
  - Top3 排序、EV 置信区间计算、pot odds / required equity 正确性。
- **求解器回归**
  - 四街各选典型场景做 golden cases：强牌价值下注、听牌跟注、边缘牌弃牌、河牌薄价值。
  - 多人（2..6）场景稳定性与收敛性检查（convergence 不足时需降级提示）。
- **API 集成**
  - 快速模式映射请求与专业模式结构化请求都能产出一致语义结果。
  - 历史记录写入/查询 `holdem_decision` 正常。
- **前端交互与E2E**
  - 快速/专业模式切换、动作编辑、范围覆盖、错误提示、结果刷新链路。
  - 移动端验证：输入、结果卡片与图表可完整阅读和操作。
- **性能验收**
  - 默认配置（6人、N=3）`p95 <= 5s`；超时返回可解释降级结果而非失败。

### Assumptions
- 现金局模型优先，不做 ICM。
- 首版目标是“近似全树可用决策”，不是 GTO 精确求解器。
- 最大总玩家数（含 Hero）为 6。
- 旧德州个人界面完全替换为新模式；后端旧接口可保留一版但不在前端暴露。
