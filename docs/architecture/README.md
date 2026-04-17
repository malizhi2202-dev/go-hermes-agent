# Go Hermes 架构文档

这一组文档专门回答 4 个问题：

1. `go-hermes-agent` 整体由哪些部分组成
2. 每个部分采用了什么方案，为什么这么设计
3. 各部分之间如何协作，运行时序是什么
4. Python 原版 Agent 的关键知识点，在 Go 版里怎么映射、保留、简化或延期

推荐阅读顺序：

1. [总体架构与时序](./overall-architecture-and-sequences.md)
2. [模块架构与时序](./module-architecture-and-sequences.md)
3. [Python Agent 知识映射](./python-agent-knowledge-map.md)
4. [轻量版迁移蓝图](../migration/lightweight-migration-blueprint.md)

这套文档的目标不是“介绍 Go 代码”，而是让人或 AI 工具能够：

- 快速理解当前 Go 版设计边界
- 快速定位每个模块的职责
- 知道哪些能力已经等价、哪些是轻量化裁剪
- 按照文档继续补齐后续迁移切片
