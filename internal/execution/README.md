# internal/execution

这个目录负责受控执行链。

当前能力：

- 命令白名单
- 参数长度与数量限制
- 输出大小限制
- per-command rule

设计原则：

- 默认关闭
- 不走 shell 拼接
- 先收缩能力面，再考虑放开
