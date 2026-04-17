# Go 轻量版迁移蓝图

这份文档回答后续迁移必须先说清楚的 4 个问题：

1. 要迁移什么
2. 为什么迁移
3. Go 里放到哪个模块
4. 每个模块开发时，代码注释、单元测试、文档要达到什么要求

## 1. 版本目标

Go 版不是 Python 全量平台版，而是：

- 轻量版
- 容易部署
- 容易接入
- 容易理解

所以迁移策略必须遵循：

- 先迁移主链，不先迁移重平台依赖
- 先迁移 CLI 能力，不先迁移 Web UI
- 先迁移插件化接入，不先堆平台适配
- 先迁移可测试、可审计能力，不先迁移高风险黑盒能力

## 2. 当前优先迁移项

| 迁移项 | 为什么迁移 | Go 模块落点 | 推荐优先级 |
|---|---|---|---|
| CLI 能力增强 | 让 Go 版在不做 Web 的情况下仍可管理系统 | `cmd/hermesctl` `internal/app` `internal/api` | P0 |
| Prompt Builder | 当前 prompt 装配分散，后续能力会越来越难维护 | `internal/promptengine` 或 `internal/app` + 新包 | P0 |
| Prompt Caching | 降成本、保稳定 | `internal/promptengine` | P1 |
| Auxiliary Client | 把摘要、分类、视觉等副任务从主模型分离 | `internal/llm` `internal/models` | P1 |
| Models 元数据 | 为上下文预算、provider 路由、辅助模型选择提供依据 | `internal/models` | P1 |
| Weixin 平台接入 | 验证“平台适配插件格式”是否成立 | `internal/gateway` + 平台抽象子层 | P1 |
| Batch Runner | 支持批量评测与轨迹产出 | `cmd/` + `internal/batch` | P1 |
| Trajectory 产出链 | 让 Go 版支持训练/评测数据沉淀 | `internal/trajectory` | P1 |
| Cron / Scheduler | 增加自动化能力 | `internal/cron` | P2 |
| ACP / Editor 集成 | 扩大入口 | `internal/acp` | P2 |
| RL / Atropos | 研究链路 | `internal/rl` 或独立模块 | P3 |

## 3. 重点迁移路线

### 3.1 CLI 能力增强

迁移什么：

- 让 `hermesctl` 覆盖更多现在 Web/API 才能做到的能力
- 增加会话查看、审计查询、扩展管理、模型管理、多 Agent 运行/恢复等命令

为什么迁移：

- 轻量版最适合 CLI first
- 用户不需要浏览器也能完成大多数操作

Go 落点：

- `cmd/hermesctl`
- `internal/app`
- `internal/api` 作为复用边界

### 3.2 Prompt Builder / Prompt Caching

迁移什么：

- 系统块装配
- context files / memory / platform / skill / task prompt 的拼装
- 稳定前缀缓存

为什么迁移：

- 当前 Go prompt 逻辑分散
- 后续补更多能力时会越来越难控制

Go 落点：

- 新包 `internal/promptengine`

### 3.3 Auxiliary Client / Models 元数据

迁移什么：

- 辅助模型
- 模型元数据
- provider-aware 预算与选择

为什么迁移：

- 不能让所有摘要、分类、检索辅助都挤在主模型上

Go 落点：

- `internal/llm`
- `internal/models`

### 3.4 Weixin 与平台插件格式

迁移什么：

- 不是先硬写多个平台
- 而是先定义平台适配模板
- 再迁一个 `weixin`

为什么迁移：

- 你的目标是“个人添加平台也方便”
- 所以必须先做可复用的接入格式

Go 落点：

- `internal/gateway`
- 后续可扩成 `internal/gateway/platforms`

### 3.5 Batch Runner / Trajectory

迁移什么：

- 批量运行 prompt
- 结果收集
- 轨迹导出

为什么迁移：

- 这是 Python 版研究和批处理价值很高的一条线
- 比 RL/Atropos 更适合作为先行切片

Go 落点：

- `cmd/hermes-batch`
- `internal/batch`
- `internal/trajectory`

## 4. 每个模块开发必须满足的规范

### 4.1 代码注释要求

必须有：

- 包级 `doc.go`
- 导出类型和导出函数的 GoDoc
- 复杂逻辑前的短注释
- 明确写出和 Python 对应能力的设计说明

不建议：

- 冗长废话注释
- 变量级机械注释

### 4.2 单元测试要求

每个迁移模块至少要有：

- 正常路径测试
- 关键边界测试
- 错误路径测试
- 兼容/恢复/审计相关测试（如果模块涉及状态）

推荐按目录放：

- `xxx_test.go`
- 使用 table-driven tests

### 4.3 文档要求

每个模块至少要更新：

- 模块 `README.md`
- 总体架构文档中对应部分
- Python 知识映射文档中的对应条目

### 4.4 迁移交付模板

每次迁移完成后，都应该能回答：

- 迁了什么
- 为什么这样迁
- 哪些和 Python 保持一致
- 哪些做了轻量化调整
- 哪些故意延期
- 测了什么

## 5. 推荐开发顺序

1. CLI 能力增强
2. Prompt Builder
3. Prompt Caching
4. Auxiliary Client
5. Models 元数据
6. 平台插件格式
7. Weixin
8. Batch Runner
9. Trajectory
10. Cron / Scheduler

## 6. 这份蓝图怎么用

使用方式很简单：

- 先看 [总体架构与时序](../architecture/overall-architecture-and-sequences.md)
- 再看 [模块架构与时序](../architecture/module-architecture-and-sequences.md)
- 再看 [Python Agent 知识映射](../architecture/python-agent-knowledge-map.md)
- 最后按本蓝图逐项迁移

这样可以保证 Go 版始终围绕“轻量、容易部署、容易接入、容易理解”这条主线演进，而不会重新变成一个难以理解的巨型动态系统。
