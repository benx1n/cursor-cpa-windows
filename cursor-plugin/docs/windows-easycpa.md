# Windows EasyCPA 安装

Windows 版本是 CLIProxyAPI 原生 DLL，已在 EasyCPA 自带的 CLIProxyAPI v8.0.27 上验证加载、注册、管理页和 OAuth 登录入口。安装时不需要替换 `cli-proxy-api.exe`，也不需要启动额外服务。

当前 Windows 增强构建版本为 `0.6.6-windows.2`，保留 0.6.6 的资源限制，增加代理继承、授权重试、思考透传及客户端工具适配。变更和验证边界见[版本说明](releases/v0.6.6-windows.2.md)。

## 使用发布包

从[本仓库 CI](https://github.com/benx1n/cursor-cpa-windows/actions/workflows/cursor-plugin-build.yml) 的 `cursor` 分支成功构建下载 Windows artifact，解开后再解压其中的 `cursor-plugin-<version>-windows-amd64.zip`，也可在 Windows 本地构建。标签发布后可以从同一个 Release 下载包和 `checksums.txt` 并校验 SHA-256。上游项目的旧 Linux Release 没有 Windows 附件。安装脚本也会校验包内 `SHA256SUMS` 中的 DLL 摘要。

升级前先在 EasyCPA 中禁用 Cursor 插件；如果 DLL 仍被占用，先停止核心。用 PowerShell 7 在解压后的安装包目录执行，将示例路径替换为自己的 EasyCPA 安装目录：

```powershell
.\install.ps1 -PluginsDir 'D:\Apps\EasyCPA\cpa-core\plugins'
```

脚本把 DLL 安装到 `cpa-core\plugins\windows\amd64`，升级前的 DLL 会备份到 `plugins\.cursor-backups`。脚本不会修改 OAuth 文件。

也可以使用环境变量：

```powershell
$env:CLIPROXY_PLUGINS_DIR = 'D:\Apps\EasyCPA\cpa-core\plugins'
.\install.ps1
```

## 启用插件

在 EasyCPA 的插件管理页启用 `cursor`。如果直接编辑 `cpa-core\config.yaml`，确认配置包含：

```yaml
plugins:
  enabled: true
  dir: "plugins"
  configs:
    cursor:
      enabled: true
      priority: 1
```

保存后重启 EasyCPA 的 CLIProxyAPI 核心；v8.0.27 也支持通过管理 API 保存插件配置后加载。宿主会从 `plugins\windows\amd64` 加载 `cursor-v<version>.dll`，并按 DLL 内置的插件版本注册。首次使用 Cursor 时，必须在 EasyCPA 的认证页面由账号所有者完成 OAuth 登录；插件不会创建或导入 Cursor 凭据。

## 授权网络与代理

插件的授权轮询、令牌刷新、模型发现和推理请求会继承 EasyCPA 的上游代理设置，例如 `http://127.0.0.1:7890`。账号的 `proxy_url` 可以覆盖全局代理；设为 `direct` 或 `none` 时直连。全局代理未配置时，使用启动核心进程时的 HTTP(S) 代理环境变量。

`TLS handshake timeout` 表示连接 Cursor 上游时 TLS 握手超时。插件对连续的短暂网络错误、HTTP 408、429 和 5xx 最多额外重试 3 次；正常的等待响应会重置重试计数。证书校验失败、401、403 等错误仍会返回失败。错误信息不再包含授权轮询 URL 和 PKCE verifier。

如果旧版本已经把本次授权标记为失败，安装更新后需要重新发起 Cursor 授权。替换相同版本 DLL 后，应先禁用再启用插件，或重启核心，确保运行的是更新后的代码。

## Codex 工具与思考内容

思考事件单独输出到 Chat Completions 的 `reasoning_content` 字段，由宿主转换为 Responses 的思考摘要事件。思考文本也计入插件的估算用量和输出预算；这些估算不是 Cursor 账单金额。

带工具请求会在完整历史和检查点续轮中提示模型通过 `opencodex-responses` MCP 目录使用客户端工具。文件创建、编辑、读取、删除及终端命令应交给客户端执行；提示只推荐实际提供的 `apply_patch`、`exec_command` 等工具。模型仍可能尝试原生操作，因此插件也会对原生文本写入、删除、目录读取和原有原生读取/终端请求返回对应失败及客户端工具指引，不会在网关上执行这些文件操作。原生图片生成的隔离二进制写入路径保留。

无客户端工具时，普通文本聊天会收到避免原生编辑、终端和搜索的提示。提示本身不保证模型绝不尝试原生工具。

大文件工具参数可能在生成完成前不发送内容事件。带工具请求的默认无进展等待为 5 分钟，普通请求仍为 90 秒；两者都保留 30 秒连接数据/帧静默检查和 15 分钟总截止时间。工具参数和工具开始事件会重置进度计时，纯心跳不会无限延长请求。

结束标记延后到上游排空和清理完成后发送，以缩小客户端正常断开与 checkpoint 排空之间的竞态。CLIProxyAPI 8.0.27 自身的流收尾用量判定及流式 `error_details` 支持仍存在已记录的局限，本插件更新不替换核心程序。

## 从源码构建

需要 Go 1.25 或更高版本，以及支持 Windows amd64 的 MinGW-w64 GCC。PowerShell 7 示例：

```powershell
.\packaging\build-windows.ps1
.\packaging\build-release.ps1 0.6.6-windows.2
.\packaging\build-store-release.ps1 0.6.6-windows.2
```

便携式工具链不在 PATH 中时，可给 `build-windows.ps1` 传入 `-GoCommand '完整的 go.exe 路径'` 和 `-CCompiler '完整的 gcc.exe 路径'`。`-Version` 同时设置输出文件名及 DLL 内部注册版本。脚本只为当前构建设置 CGO、GOOS、GOARCH 和 CC，结束后恢复原来的进程环境变量。

商店包名为 `cursor_<version>_windows_amd64.zip`，ZIP 根目录包含版本化 DLL。手动安装包为 `cursor-plugin-<version>-windows-amd64.zip`。

## 卸载

先在 EasyCPA 中禁用插件并停止核心，然后执行：

```powershell
.\uninstall.ps1 -PluginsDir 'D:\Apps\EasyCPA\cpa-core\plugins'
```

DLL 会移动到 `plugins\.cursor-uninstalled` 以便恢复；OAuth 凭据仍保留在 `cpa-core\oauth`。
