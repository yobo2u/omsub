# Cursor for CPA/CLIProxyAPI

这是一个独立的 CLIProxyAPI 原生动态库插件，把用户本人授权的 Cursor 订阅接入 OpenAI 兼容的 `/v1/chat/completions` 接口。插件直接安装到 CLIProxyAPI 的插件目录，不要求部署或运行 opencodex 服务。

> [!IMPORTANT]
> 本项目是非官方社区插件，与 Cursor、Anysphere、CLIProxyAPI、CPA Manager Plus 或 opencodex 无隶属或授权关系。使用前请完整阅读[免责声明](DISCLAIMER.md)，并自行确认符合适用法律、服务条款及订阅限制。

## 已验证环境

- CLIProxyAPI `v7.2.131`，提交 `323b727`
- CPA Manager Plus `v1.11.10`
- Linux amd64
- Cursor OAuth、204 个 Cursor 模型发现、非流式、SSE 流式和错误路径

## 安装

解压发布包，然后把 `--plugins-dir` 指向 CLIProxyAPI 配置中的 `plugins.dir`：

```sh
tar -xzf cursor-plugin-0.1.0-linux-amd64.tar.gz
cd cursor-plugin-0.1.0-linux-amd64
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

## 当前边界

- 支持 OpenAI `chat-completions` 文本消息、非流式和流式响应。
- 暂不支持 tools、图片和 Responses API；这些请求返回明确的客户端错误。
- token usage 为本地估算值，不代表 Cursor 账单或订阅额度。
- 当前发布包只提供 Linux amd64；其他平台需要对应平台的 CGO 工具链重新构建。

CLIProxyAPI 动态库插件是进程内受信代码。请只安装来自可信来源且校验过 `SHA256SUMS` 的构建。使用时应遵守 Cursor 的服务条款和可接受使用政策，不应共享账号、转售访问或规避配额与安全控制。

## 来源

Cursor 协议实现参考 [opencodex](https://github.com/lidge-jun/opencodex) 提交 `5840591322117f3ee9568b35b135a6d4339f7711`；插件 ABI 参考 [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) 提交 `85d2faddd17e6f4f8675a84ee28b131f702e8eaa`。两者均采用 MIT 许可证，详见 `THIRD_PARTY_NOTICES.md`。
