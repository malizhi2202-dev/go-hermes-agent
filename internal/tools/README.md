# internal/tools

这个目录负责 Go 版本的受限工具注册表。

当前能力：

- registry
- 内建工具注册
- child delegated tool 执行入口

设计原则：

- 编译期与启动期白名单优先
- 不直接开放任意动态工具执行
