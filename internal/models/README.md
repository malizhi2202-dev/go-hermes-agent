# internal/models

这个目录负责模型目录、alias 和本地模型发现。

当前能力：

- 常用 profile alias
- Ollama / LM Studio 本地发现
- 远端与本地 profile 统一管理


## 新增轻量能力

- `metadata.go` 提供 Go 版轻量 `model_metadata / models_dev` 子集
- 重点提供 context window、max output、tool/vision/reasoning/prompt cache 能力说明
- 先采用内置 registry，后续如需联网再扩成远程刷新
