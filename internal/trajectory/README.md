# internal/trajectory

`internal/trajectory` 是 Go 版对 Python `agent/trajectory.py` 的轻量迁移。

当前职责：

- 从已保存的 chat session 构造轨迹
- 以 JSONL 形式保存到 `data/trajectories/`
- 支持按 `run_name / model / source / completed` 做轻量过滤
- 提供 trajectory summary 聚合，方便批处理排障和训练前检查
- 供 batch runner、调试、导出和训练前处理使用

当前设计：

- 不额外引入数据库表
- 直接使用文件系统 JSONL
- 一条 session 对应一条轨迹记录

这样做的优势：

- 更容易部署和排障
- 用户可以直接查看轨迹文件
- 保持单机版的简单性
- 不需要额外迁移数据库 schema，也方便 shell 和 Python 工具复用
