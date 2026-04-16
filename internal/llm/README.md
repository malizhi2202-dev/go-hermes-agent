# internal/llm

这个目录封装 Go 版本的 LLM 调用边界。

当前职责：

- 提供 OpenAI-compatible `chat/completions` 客户端
- 支持普通聊天请求
- 支持 native tool-calling
- 在 provider 返回字符串或分段内容时做归一化

核心文件：

- `client.go`：客户端、消息结构、tool schema、completion 解析
- `client_test.go`：原生 tool-calling 解析测试

设计要点：

- 对上层暴露统一 `Chat / ChatWithContext / ChatWithMessages / ChatCompletion`
- tool-calling 优先使用标准协议，不依赖提示词约定
- 仍保留向后兼容，方便 child runtime 在不支持原生 tool-calling 的模型上回退
