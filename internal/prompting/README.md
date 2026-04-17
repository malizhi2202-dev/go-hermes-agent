# internal/prompting

`internal/prompting` 是 Go 版对 Python `prompt_builder + prompt_caching` 的轻量实现。

当前职责：

- 汇总 memory / persisted summary / recent history
- 接入已有 context compressor
- 在字符预算内裁剪 history
- 用短 TTL 的本地缓存缓存“已组装的 prompt plan”

方案优势：

- 不引入额外外部依赖
- 不耦合具体 store / memory / compressor 实现
- 方便 CLI 和 API 做 prompt 可观测能力
- 保持单机版的简单性，同时给后续 prompt caching 扩展留入口
