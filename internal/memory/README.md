# internal/memory

这个目录负责 Go 版本的内建记忆系统。

当前能力：

- `MEMORY.md / USER.md` 文件记忆
- recalled memory 注入
- memory read/write

设计原则：

- 先做简单可靠的文件记忆
- 后续再扩成可插拔 provider
