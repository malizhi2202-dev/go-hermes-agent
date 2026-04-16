# Hermes Go 版本任务拆解与 MR 文档

## 1. 文档目标

这份文档用于把当前 Go 迁移工作拆成可执行、可跟踪、可验收的步骤，也适合当作 MR/阶段汇报文档使用。

它回答 4 个问题：

1. 当前迁移已经推进到哪里
2. 每一段工作具体做了什么
3. 还剩哪些任务
4. 后续该如何继续提交和验收

---

## 2. 当前整体状态

当前 Go 版已经完成“安全主干版”的大部分关键闭环：

- 基础服务
- 登录鉴权
- 会话和搜索
- 模型切换
- 记忆和上下文
- Gateway
- 扩展面
- 多 Agent
- replay / resume
- 审计和 trace 视图

当前不是从 0 到 1，而是已经从“能跑”进入“收尾增强和补齐”的阶段。

---

## 3. 已完成阶段拆解

### 阶段 A：底座搭建

已完成：

- Go 工程结构
- `hermesd` / `hermesctl`
- 配置系统
- SQLite
- 安装卸载脚本

验收标准：

- 能构建
- 能启动
- 能初始化管理员

### 阶段 B：安全主干

已完成：

- 本地登录
- bcrypt
- JWT
- 登录失败限制
- 审计日志

验收标准：

- 能登录
- 能校验 token
- 审计落库

### 阶段 C：会话与搜索

已完成：

- sessions/messages
- history
- FTS search
- role/session/time 过滤

验收标准：

- 可以记录对话
- 可以分页读取
- 可以全文检索

### 阶段 D：模型体系

已完成：

- model profiles
- alias
- 本地模型发现
- 运行时切换

验收标准：

- 可切换远端模型
- 可接本地模型

### 阶段 E：记忆与上下文

已完成：

- 文件记忆
- recalled memory
- history window
- context budget
- summary persistence
- compressor

验收标准：

- 能读写 memory
- 能做上下文压缩

### 阶段 F：Gateway

已完成：

- webhook
- Telegram
- 去重
- 重试
- `/multiagent ...` 路由

验收标准：

- 外部消息能进主链
- Telegram 可稳定收发

### 阶段 G：扩展面

已完成：

- plugin
- skill
- MCP stdio / HTTP
- 启停状态
- hash 标记

验收标准：

- 扩展可发现
- 扩展可启停
- 扩展工具能注册

### 阶段 H：多 Agent

已完成：

- plan / run
- parent/child session
- child runtime
- native tool-calling 优先
- trace persistence
- replay
- resume
- summary / failures / hotspots

验收标准：

- 可以执行 delegated task
- 可以回放
- 可以恢复
- 可以做失败分析

---

## 4. 当前仍待完成任务

### 任务组 1：文档和注释收尾

目标：

- 所有核心模块都有 README
- 所有导出类型和函数有 GoDoc

当前状态：

- 已完成大部分
- 还需继续细补边角导出函数

优先级：

- 高

### 任务组 2：多 Agent 恢复链继续增强

目标：

- 不只恢复文本上下文
- 更完整保留 child loop 状态

当前状态：

- 已恢复最后成功 tool state
- 还没恢复更完整的 loop state

优先级：

- 高

### 任务组 3：trace 分析继续增强

目标：

- 支持更多审计、聚合、热点视图
- 为长期运维做准备

当前状态：

- 已有 traces / summary / failures / hotspots
- 已支持时间过滤

优先级：

- 中高

### 任务组 4：剩余平台与高级能力

目标：

- Slack 等 gateway
- 更完整 MCP
- 更复杂 delegated runtime

当前状态：

- 仍未完全迁移

优先级：

- 中

---

## 5. 建议的后续 MR 顺序

### MR-1：文档和注释最终补齐

内容：

- 补齐剩余 GoDoc
- 补齐 README
- 更新 gap checklist

输出：

- 文档完整版本

### MR-2：多 Agent 恢复链增强

内容：

- 恢复更多 child loop 状态
- 完善 replay/recover

输出：

- 更接近真实续跑的恢复链

### MR-3：trace 与运维视图增强

内容：

- 更细的聚合
- 更多热点分析
- 更强审计视图

输出：

- 更完整的排障和运维能力

### MR-4：平台和高动态能力补齐

内容：

- Slack
- 更完整 MCP transport
- 更复杂 delegated runtime

输出：

- 更接近 Python 高级形态

---

## 6. 每个 MR 的验收方式

所有后续 MR 都建议统一执行下面的验收动作：

1. `go test ./...`
2. `go build -o ./bin/hermesd ./cmd/hermesd`
3. `go build -o ./bin/hermesctl ./cmd/hermesctl`
4. 更新 README
5. 更新 gap checklist
6. 更新学习文档 / MR 文档

这样就能保证：

- 代码不是孤立推进
- 文档不会落后
- 管理者能看懂当前状态

---

## 7. 项目把控建议

如果要把控这个项目，不需要每天盯所有代码，只需要盯下面这些固定产物：

1. `README.md`
2. `go-gap-checklist.md`
3. 当前 MR 文档
4. `go test ./...` 是否持续通过
5. 是否每次改动都同步文档和注释

只要这几项一直同步，这个 Go 迁移就是可持续、可控的。

---

## 8. 当前结论

当前 Go 版本已经具备进入“可交付、可收尾、可继续深化”的条件。

最重要的不是再证明它“能不能做”，而是继续按步骤把：

- 恢复链
- 文档
- 注释
- 审计视图
- 差距清单

一起收完整。
