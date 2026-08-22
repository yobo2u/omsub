# Cursor for CPA/CLIProxyAPI

这是一个独立的 CLIProxyAPI 原生动态库插件，把用户本人授权的 Cursor 订阅接入 OpenAI 兼容的 `/v1/chat/completions` 接口。插件直接安装到 CLIProxyAPI 的插件目录，不要求部署或运行 opencodex 服务，也不修改 CPA Manager Plus 或 CLIProxyAPI 源码。

> [!IMPORTANT]
> 本项目是非官方社区插件，与 Cursor、Anysphere、CLIProxyAPI、CPA Manager Plus 或 opencodex 无隶属或授权关系。使用前请完整阅读[免责声明](DISCLAIMER.md)，并自行确认符合适用法律、服务条款及订阅限制。

## 已验证环境

- CLIProxyAPI `v7.2.131`，提交 `323b727`
- CPA Manager Plus `v1.11.10`
- Linux amd64
- Cursor OAuth、动态模型发现、非流式、SSE 流式和错误路径
- 插件自有 Cursor 管理页、受认证管理 API、模型禁用与本地估算用量

## 安装

### CLIProxyAPI 插件商店

官方插件商店收录后，可在 CPA Manager Plus / CLIProxyAPI 插件商店中选择
`cursor` 安装。商店会从最新的 `v<version>` GitHub Release 下载当前平台 ZIP，
并用同一 Release 中的 `checksums.txt` 校验文件。

当前商店发布物仅支持 Linux amd64：

```text
cursor_0.5.5_linux_amd64.zip
checksums.txt
```

### 手动安装

解压发布包，然后把 `--plugins-dir` 指向 CLIProxyAPI 配置中的 `plugins.dir`：

```sh
tar -xzf cursor-plugin-0.5.5-linux-amd64.tar.gz
cd cursor-plugin-0.5.5-linux-amd64
sudo ./install.sh --plugins-dir /opt/cpa-manager-plus/cliproxyapi/plugins
```

在 CPA Manager Plus 插件页面启用 `cursor`，或确保 CLIProxyAPI 配置包含：

```yaml
plugins:
  enabled: true
  dir: "plugins"
  configs:
    cursor:
      enabled: true
      priority: 1
```

重启 CLIProxyAPI 后，在 CPA Manager Plus 的认证页面发起 Cursor OAuth。浏览器登录和授权必须由 Cursor 账号本人完成。凭据由 CLIProxyAPI 的认证目录保存，插件不会把 token 写入自身配置。

## Cursor 管理

插件启用后会通过 CLIProxyAPI 的原生插件资源机制注册“Cursor 管理”菜单，不需要修改 CPA Manager Plus 页面。管理页提供：

- Cursor OAuth 账户状态和实时可用模型；
- 每个账户的模型禁用选择器，以及“保存设置 / Save settings”和需确认保存的“全部禁用 / Disable all”操作；
- 插件进程启动后的本地估算 Token 与请求计数；
- 明确的订阅额度不可用状态。

管理页完整支持中文和英文。首次打开时优先使用已保存的语言偏好，否则跟随浏览器语言，非中文环境默认英文；页面右上角可随时切换“中文 / English”。切换会同步更新静态文案、账户状态、用量指标、模型控制、操作提示、页面标题及无障碍语言标记。插件只保存语言偏好，管理密钥仍仅保留在当前页面内存中。

模型禁用规则保存在对应 Cursor OAuth 认证 JSON 的 `disabled_models` 字段中。插件会同时在模型发现和请求执行阶段应用规则，刷新 OAuth token 时也会保留规则。

账户列表只显示仍有物理文件的 Cursor OAuth 凭据，不显示 CLIProxyAPI 的 `runtime_only` 投影或文件删除后短暂残留的 `source: memory` 运行时记录；如果旧数据中多个条目解析到同一个 Cursor `account_id` 或邮箱，也只显示一个逻辑账户。

插件浏览器资源按 CLIProxyAPI 设计是未认证的静态入口，因此页面不会直接暴露账户数据；读取状态或保存规则时需要输入 CLIProxyAPI 管理密钥。密钥只保留在当前页面内存中，不写入浏览器存储。

## 调用

模型名使用 `cursor/` 前缀，例如：

```sh
curl https://your-cpa.example/v1/chat/completions \
  -H "Authorization: Bearer $CPA_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"cursor/default","messages":[{"role":"user","content":"hello"}],"stream":false}'
```

流式调用把 `stream` 改为 `true`，返回标准 SSE 数据帧和 `[DONE]`。

## 升级和卸载

再次运行新版本 `install.sh` 即可升级；旧二进制会备份到插件目录的 `.cursor-backups/`。

```sh
sudo ./uninstall.sh --plugins-dir /opt/cpa-manager-plus/cliproxyapi/plugins
```

卸载脚本不会直接删除二进制，而是移动到 `.cursor-uninstalled/`。随后在 CPA Manager Plus 中禁用 `cursor` 并重启 CLIProxyAPI。OAuth 凭据不会自动删除，避免误删账号数据。

## 能力与边界

- 支持 OpenAI `chat-completions` 文本消息、非流式和 SSE 流式响应，并兼容 OpenCode 发送的 `max_tokens`、`stream_options` 等扩展字段。
- 支持标准 function tools、`tool_choice`、多工具调用、assistant `tool_calls` 历史和 tool 结果续轮；工具目录通过 Cursor 原生 `mcp_tools` 注册，结果转换为 OpenAI 兼容 `tool_calls`。
- 支持 `image_url` / `input_image` 内联 data URL，以及 `file` / `input_file` 的图片或 UTF-8 文本附件；图片和文件通过 Cursor 原生 `selected_context` 发送，tool 结果中的图片也能随续轮送达。
- 为避免服务端请求伪造，远程图片 URL 不由插件下载；调用方应传内联 data URL。仅有 `file_id` 而没有 `file_data` 的附件无法由独立插件解析。
- v0.5.5 的会话检查点仅在插件进程内保存，并按账户、模型和会话严格隔离。仅追加式线性历史会尝试续传；分支、编辑、压缩、过期、重启或状态异常时会安全回退为完整重放。
- 检查点的上限为 15 分钟 TTL、64 条和 16 MiB；进程重启后不保留。原始检查点、凭据和管理密钥不会写入浏览器存储、宿主 metadata、日志或发布证据。
- 暂不直接实现 Responses API；CLIProxyAPI 可按其 executor 翻译能力把其他协议转换到插件声明的 `chat-completions` 输入输出格式。
- Cursor 没有公开、稳定的 OAuth 订阅剩余额度接口；管理页不会伪造百分比或余额。
- token usage 为插件运行期内的本地估算值，不代表 Cursor 账单或订阅额度，进程重启后重新计数。
- 当前发布包只提供 Linux amd64；其他平台需要对应平台的 CGO 工具链重新构建。

CLIProxyAPI 动态库插件是进程内受信代码。请只安装来自可信来源且校验过 `SHA256SUMS` 的构建。使用时应遵守 Cursor 的服务条款和可接受使用政策，不应共享账号、转售访问或规避配额与安全控制。

## 来源

Cursor 协议实现参考 [opencodex](https://github.com/lidge-jun/opencodex) 提交 `5840591322117f3ee9568b35b135a6d4339f7711`；插件 ABI 参考 [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) 提交 `85d2faddd17e6f4f8675a84ee28b131f702e8eaa`。两者均采用 MIT 许可证，详见 `THIRD_PARTY_NOTICES.md`。
