# internal/app

这个目录是 Go 版本的应用装配层。

当前负责：

- 组装 store / auth / llm / tools / extensions / memory / context / multiagent
- 提供 chat、model switch、memory、multiagent 等统一入口
- 把 gateway 和 API 共用的业务逻辑收口到一个地方

设计原则：

- 上层只调用 `App`
- 各子系统保持独立模块
- 跨模块协调尽量在这里完成，而不是散落到 gateway 或 api
