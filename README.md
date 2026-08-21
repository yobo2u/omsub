# Cursor for CPA/CLIProxyAPI

`cursor` 分支用于发布可独立安装的 CPA/CLIProxyAPI Cursor 插件。插件直接作为 CLIProxyAPI 原生动态库运行，不需要安装或启动 opencodex 服务。

## 获取与安装

- [Linux amd64 发布包](cursor-plugin/release/cursor-plugin-0.1.0-linux-amd64.tar.gz)
- [安装和调用说明](cursor-plugin/README.md)
- [免责声明](cursor-plugin/DISCLAIMER.md)
- [第三方来源说明](cursor-plugin/THIRD_PARTY_NOTICES.md)

发布包 SHA-256：

```text
d2f9d45b925115572f5556aeb2e8d3e987adfa4a4257580f90299ec4cf6d7535  cursor-plugin-0.1.0-linux-amd64.tar.gz
```

当前版本支持 OpenAI Chat Completions 文本请求、非流式响应和 SSE 流式响应。暂不支持 tools、图片及 Responses API。

> [!WARNING]
> 这是非官方社区项目。仅可使用本人所有或已获明确授权的账号和订阅；使用者必须自行确认符合适用法律、Cursor 服务条款、可接受使用政策和订阅限制。安装前请阅读完整[免责声明](cursor-plugin/DISCLAIMER.md)。

根目录 `LICENSE` 是 omsub 仓库的 Apache-2.0 许可证；`cursor-plugin/` 子项目采用其目录内的 MIT 许可证。
