# internal/contextengine

这个目录负责上下文压缩。

当前能力：

- 历史窗口裁剪
- 规则型摘要压缩
- 持久化 summary 协作
- 可选 LLM 摘要扩展点

设计原则：

- 先做可解释、可审计的压缩
- 再逐步引入更复杂的 LLM summary 策略
