# internal/batch

`internal/batch` 是 Go 版对 Python `batch_runner.py` 的轻量迁移。

当前职责：

- 读取 JSONL prompt 数据集
- 顺序调用 `App.ChatDetailed`
- 对成功项保存 trajectory
- 保存 batch checkpoint，支持按 run name 恢复
- 输出批处理结果和汇总

当前设计取舍：

- checkpoint 先做成单机 JSON 文件，不引入额外队列和数据库表
- 先不做训练专用统计聚合
- 优先保证单机易用、易调试、易复现

后续如果需要，可以再加：

- 并发 worker
- 更完整的 tool / reasoning 统计
- 更细粒度的 checkpoint 清理和失败重试策略
