# Cursor for CPA/CLIProxyAPI

`cursor` 分支用于发布可独立安装的 CPA/CLIProxyAPI Cursor 插件。插件直接作为 CLIProxyAPI 原生动态库运行，不需要安装或启动 opencodex 服务，也不修改 CPA Manager Plus 或 CLIProxyAPI 源码。

## 获取与安装

- [GitHub Releases](https://github.com/yobo2u/omsub/releases/tag/v0.5.9)
- [Linux amd64 发布包](https://github.com/yobo2u/omsub/releases/download/v0.5.9/cursor-plugin-0.5.9-linux-amd64.tar.gz)
- [安装和调用说明](cursor-plugin/README.md)
- [免责声明](cursor-plugin/DISCLAIMER.md)
- [第三方来源说明](cursor-plugin/THIRD_PARTY_NOTICES.md)

GitHub Release 同时提供符合 CLIProxyAPI 官方插件商店格式的
`cursor_0.5.9_linux_amd64.zip` 和 `checksums.txt`。ZIP 根目录只包含
`cursor.so`；TAR 包继续用于手动安装。
发布物校验值见同一 Release 中的 `checksums.txt`。

v0.5.9 支持 OpenAI Chat Completions 文本请求、非流式响应、SSE 流式响应、标准 function tools（包括多工具调用和结果续轮），以及内联图片和图片或 UTF-8 文本附件。该版本会在协议边界生成稳定、单行且不超过 64 字节的工具调用 ID，拒绝成功但真正为空的 assistant 响应，并修正管理页逻辑请求、宿主调度尝试与本地 Token 估算的统计口径。插件还提供 Cursor 管理页、账户去重、单账户“全部禁用 / Disable all”和模型禁用。Cursor 未公开稳定的 OAuth 订阅剩余额度接口，因此插件会明确显示额度不可用，不会伪造余额或百分比；Responses API 仍不由插件直接实现。

会话检查点仅在插件进程内保存，并按账户、模型和会话隔离；仅追加式线性历史会尝试续传，其他情况会安全回退为完整重放。检查点最多保留 15 分钟、64 条和 16 MiB，进程重启后不会保留。原始检查点不会写入浏览器存储、宿主 metadata、日志或发布证据。

> [!WARNING]
> 这是非官方社区项目。仅可使用本人所有或已获明确授权的账号和订阅；使用者必须自行确认符合适用法律、Cursor 服务条款、可接受使用政策和订阅限制。安装前请阅读完整[免责声明](cursor-plugin/DISCLAIMER.md)。

根目录 `LICENSE` 是 omsub 仓库的 Apache-2.0 许可证；`cursor-plugin/` 子项目采用其目录内的 MIT 许可证。
