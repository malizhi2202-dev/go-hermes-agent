# Hermes Go 版本全方位学习与把控文档

## 1. 文档目标

这份文档的目标不是只解释“代码现在做了什么”，而是让管理者、架构负责人、研发负责人和接手开发者都能从同一份材料里看清楚：

- Go 版本为什么这样设计
- 它和 Python 原版的关系是什么
- 当前已经完成了什么
- 还有哪些能力没有完全迁完
- 后续应该按什么顺序推进
- 风险和边界在哪里

这份文档适合用于：

- 项目总体学习
- 架构评审
- 迁移把控
- 对外汇报
- 开发接手

---

## 2. 项目背景与迁移目标

Hermes 原本是一个以 Python 为核心的 Agent 工程，能力很强，但也带着 Python 生态常见的问题：

- 运行时动态装配较多
- 插件、技能、工具、MCP、gateway 等扩展面大
- 高动态能力很多，安全和治理成本也高

Go 版本的目标不是机械逐行翻译，而是把 Hermes 先重构成一条“安全主干”：

1. 可运行
2. 可安装
3. 可登录
4. 可审计
5. 可扩展
6. 可继续迁移

所以 Go 版的思路是：

- 先落地平台主干
- 再按能力切片逐步对齐 Python
- 对高风险动态能力先收口，不直接照搬

---

## 3. Go 版本总体架构

### 3.1 架构分层

Go 版本当前大致分为 7 层：

1. 入口层
   - `cmd/hermesd`
   - `cmd/hermesctl`

2. 应用装配层
   - `internal/app`

3. 配置与安全层
   - `internal/config`
   - `internal/auth`
   - `internal/security`

4. 数据与状态层
   - `internal/store`

5. Agent 运行边界层
   - `internal/llm`
   - `internal/api`
   - `internal/gateway`

6. 工具与动态扩展层
   - `internal/tools`
   - `internal/extensions`

7. 高风险能力收口层
   - `internal/execution`

### 3.2 核心设计原则

Go 版当前坚持 5 个原则：

1. 安全优先
   - 高风险执行链默认关闭
   - 动态扩展先受控接入

2. 结构化优先
   - 会话、审计、trace 都入库
   - 尽量避免只保留自然语言日志

3. 单一装配中心
   - 所有核心能力通过 `internal/app.App` 汇总

4. 多入口统一收敛
   - API、Webhook、Telegram 都尽量走统一主链

5. 分步骤迁移
   - 不追求一步全量等价
   - 每轮都要求代码、测试、文档同步推进

---

## 4. 当前已经完成的能力

### 4.1 基础平台能力

- Go 标准工程结构
- 一键安装 / 一键卸载
- `hermesd` / `hermesctl`
- 配置加载和配置样例
- 本地账号、安全登录、JWT

### 4.2 数据与会话能力

- SQLite
- `users / sessions / messages / audit_log`
- `messages_fts`
- session/history/search
- role/session/time 过滤

### 4.3 模型能力

- `model_profiles`
- 运行时切换 profile
- alias 解析
- 本地模型发现
- OpenAI-compatible 统一接入
- Ollama / LM Studio 示例支持

### 4.4 记忆与上下文能力

- 文件记忆
- recalled memory 注入
- 历史窗口
- context budget
- 规则型 context compressor
- 摘要持久化

### 4.5 Gateway 能力

- 通用 webhook
- Telegram webhook
- 去重
- 重试
- 会话隔离
- `/multiagent ...` 路由

### 4.6 工具与扩展能力

- 受限版 tool registry
- 安全 delegated tools
- plugin / skill / MCP stdio / HTTP
- 启停状态持久化
- hash 变化标记

### 4.7 多 Agent 能力

- plan / policy / orchestrator / aggregate
- parent / child session
- child trace
- replay
- resume
- trace summary / failures / hotspots
- 原生 tool-calling 优先，JSON 回退

---

## 5. 多 Agent 的架构理解

### 5.1 不是“放飞多个 Agent”

Hermes Go 的多 Agent 不是一群 Agent 自由聊天，而是：

1. parent 规划
2. child 受控执行
3. child 结果结构化回写
4. aggregate 汇总

### 5.2 为什么这样做

因为真正危险的不是“多 Agent”，而是：

- 越权
- 上下文串话
- 工具乱用
- 无法恢复
- 无法审计

所以 Go 版当前把多 Agent 做成了：

- 有 plan
- 有 policy
- 有 allowlist
- 有 trace
- 有 replay / resume

### 5.3 当前已经具备的多 Agent 关键闭环

- 任务规划
- 并发/串行判定
- 父子会话链
- child tool 执行
- 原生 tool-calling
- trace 落库
- trace summary / failures / hotspots
- replay / resume

### 5.4 当前还没完全做到的点

- 递归子代理
- 深度控制链
- 更完整的 child tool state 恢复
- 更复杂 delegated runtime

---

## 6. 理论、论文与工程决策

### 6.1 论文和理论不是装饰

Go 版当前的很多决策，已经和近两年多 Agent / Memory / Tool Use 的主流认识保持一致：

- 规划和执行分层
- 记忆不等于无限堆历史
- 轨迹必须结构化
- 工具调用尽量走原生协议

### 6.2 对当前工程最关键的理论结论

1. 规划层与执行层必须分开
   - 对应 Go 的 `planner / orchestrator / runtime / aggregator`

2. 记忆要做检索式注入
   - 对应当前的 `history_window + summary + memory`
   - 后续继续走 retrieval-style context

3. 轨迹要结构化
   - 对应 `multiagent_traces`

4. 工具协议要标准化
   - 对应当前 child runtime 原生 tool-calling 优先

5. 高风险能力要受控
   - 对应 `execution / extensions` 的收口策略

### 6.3 这些理论为什么重要

因为它们直接决定了：

- 这个系统是不是能审计
- 能不能恢复
- 能不能排障
- 能不能长期维护

---

## 7. 安全边界与治理策略

Go 版不是没有动态能力，而是把动态能力放进了边界里。

### 7.1 当前已经做的安全收口

- 登录鉴权
- JWT
- 登录失败限制
- 受控命令执行
- 插件/技能命令模板限制
- 参数限制
- shell 元字符拦截
- 审计日志
- child tool allowlist

### 7.2 为什么不直接照搬 Python 高动态能力

因为这些能力一旦直接放开，很容易出现：

- 命令执行风险
- 注入风险
- 不可控插件行为
- 无法回溯的 agent 操作

所以 Go 版优先做：

- 默认关闭
- 白名单
- 参数规则
- 结构化 trace
- 审计和恢复

---

## 8. 目前还差哪些

虽然 Go 版已经很深了，但还不是 Python 的全等价版。

当前主要还差：

1. 更完整的多 Agent 递归链
2. 更复杂 delegated runtime
3. 更完整 MCP transport
4. 更完整 gateway 平台族
5. 更完整 memory provider 生态
6. 更完整 plugin lifecycle
7. 更完整 prompt builder / prompt caching / AIAgent 主循环等价能力

一句话总结：

Go 版现在已经是“可运行、可治理、可审计、可恢复”的主干平台。  
剩下的主要是 Python 版那些更复杂、更动态、更开放的高级能力。

---

## 9. 建议的继续推进顺序

### 第 1 段：收尾当前 Go 主干

- 补齐剩余 GoDoc
- 补齐剩余模块 README
- 更新 gap checklist
- 统一最终文档

### 第 2 段：继续增强多 Agent

- 更完整的 child state 恢复
- 更完整的 delegated runtime
- 更细的恢复/重放机制

### 第 3 段：扩展平台能力

- Slack
- 其他 gateway
- 更完整 MCP transport

### 第 4 段：处理高动态能力

- browser automation
- 更完整 plugin lifecycle
- 更复杂 execution chain

---

## 10. 管理者如何把控这个项目

如果要“全方位学习和把控”，建议重点盯这 6 个问题：

1. 当前阶段目标是什么
2. 哪些能力已经可用
3. 哪些能力仍是骨架
4. 哪些能力因为安全原因故意没放开
5. 当前测试和构建是否持续通过
6. 文档是否和代码同步更新

只要这 6 个问题持续清楚，这个 Go 迁移项目就是可控的。

---

## 11. 交付建议

建议把当前 Go 版视为：

“Hermes 的可交付安全主干版”

而不是简单理解成：

“Python 的不完整翻译版”

因为它现在已经不是只会运行几个 demo，而是具备：

- 服务能力
- 登录能力
- 会话能力
- 搜索能力
- 记忆能力
- 多 Agent 能力
- 扩展能力
- 审计与恢复能力

这已经足够支撑后续进入更严肃的产品化阶段。
