# Go 迁移：Session / History / Search

## 已迁移内容

- `sessions` 表：保存每次对话的主记录
- `messages` 表：保存 session 下的逐条消息
- `GET /v1/history`：返回最近会话及其消息
- `GET /v1/search?q=...`：按消息内容搜索当前用户历史
- SQLite FTS5：替代简单 `LIKE` 搜索
- 支持按 `role/session_id/from/to` 过滤
- `GET /v1/history`：支持 session 分页与每个 session 的消息分页/裁剪
- `session.history` 工具
- `session.search` 工具

## 与 Python 的差异

Python 版本的 `hermes_state.py` 是更完整的多字段会话库，包含：

- token 统计
- reasoning 存储
- FTS5
- parent session
- source 分类

Go 当前版本先保留最核心的一组：

- username
- model
- prompt / response
- message history
- message keyword search

## 当前实现说明

- 通过 `messages_fts` 虚表建立全文索引
- 写入 `messages` 时由 trigger 同步索引
- 查询时做了一个轻量的前缀 token 归一化
- 历史接口支持 `limit/offset/messages_limit/messages_offset`

## 后续可继续补

- FTS5 或倒排搜索
- reasoning / tool call 存储
- session metadata
- source / platform 维度过滤
