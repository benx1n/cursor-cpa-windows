# Cursor CPA Windows / EasyCPA

将本人授权的 Cursor 账号接入 Windows EasyCPA / CLIProxyAPI，供支持 OpenAI 兼容接口的客户端使用。插件以原生 DLL 运行，安装到 EasyCPA 的插件目录，通过宿主提供 OAuth 登录、模型发现和请求转发。

当前 Windows 增强版本为 **`0.6.6-windows.2`**，基于上游 0.6.6，加入 Windows 打包安装、授权网络重试、思考内容透传和 Codex 客户端工具适配。完整变更见[版本说明](cursor-plugin/docs/releases/v0.6.6-windows.2.md)。

## 兼容与验证

| 项目 | 已验证环境或行为 |
| --- | --- |
| 系统 | Windows amd64 |
| EasyCPA | v0.2.101，内置 CLIProxyAPI v8.0.27 |
| 插件管理 | DLL 加载、注册、管理认证、Cursor 管理页及 OAuth |
| 调用 | 非流式、SSE、思考透传、客户端工具调用和工具结果续轮 |
| 真实文件创建 | 经宿主 `/v1/responses` 调用 custom `apply_patch`，创建 2266 字节 HTML 后回传工具结果，两轮正常完成 |

插件实现 `/v1/chat/completions` 格式；`/v1/responses` 由 CLIProxyAPI 宿主转换。真实验收覆盖小文件创建，原先约 43 KB HTML 的长文件流程尚未完整重放。

## 下载与安装

1. 打开本仓库 [Cursor plugin build](https://github.com/benx1n/cursor-cpa-windows/actions/workflows/cursor-plugin-build.yml)，选择 `cursor` 分支的成功构建，下载 Windows artifact。
2. 解开 artifact，再解压其中的 `cursor-plugin-0.6.6-windows.2-windows-amd64.zip`。包内包含 DLL、安装脚本和说明文档。
3. 升级前在 EasyCPA 中禁用 Cursor 插件；如果 DLL 仍被占用，先停止核心。在解压后的安装包目录，用 **PowerShell 7** 执行：

```powershell
.\install.ps1 -PluginsDir 'D:\Apps\EasyCPA\cpa-core\plugins'
```

将路径替换为自己的 EasyCPA 安装目录。脚本校验包内 `SHA256SUMS`，把 DLL 安装到 `plugins\windows\amd64`，旧 DLL 备份到 `plugins\.cursor-backups`。

4. 启动核心并在插件管理页启用 `cursor`，然后从认证页面完成 Cursor OAuth；已有账号可继续使用。
5. 在客户端选择宿主返回的 `cursor/` 模型，使用 EasyCPA 配置的 API 地址和密钥调用。

安装本插件不需要替换 `cli-proxy-api.exe`。详细配置、代理、升级和卸载步骤见 [Windows EasyCPA 安装说明](cursor-plugin/docs/windows-easycpa.md)。

构建产生以下 Windows 包：

| 文件 | 用途 |
| --- | --- |
| `cursor-plugin-0.6.6-windows.2-windows-amd64.zip` | 含 PowerShell 安装脚本的手动安装包 |
| `cursor_0.6.6-windows.2_windows_amd64.zip` | DLL 位于 ZIP 根目录的商店格式包 |

推送源码触发 CI 构建，标签发布流程才会创建 GitHub Release 和附件校验文件；请以成功构建或实际 Release 附件为准。上游旧 v0.6.6 Release 仅提供 Linux 包。

## Codex 工具适配

- 在完整请求和检查点续轮中，引导模型使用 `opencodex-responses` 暴露的客户端工具，例如 `apply_patch`、`exec_command`。文件创建和修改应由客户端工具实际执行。
- 对 Cursor 原生文本写入、删除、目录读取及已有的读取、搜索和终端请求返回明确失败，并推荐当前请求实际提供的客户端工具。原生图片生成的隔离二进制写入保留。
- 思考内容通过 `reasoning_content` 输出，由宿主转换为 Responses 思考摘要事件。
- 带工具请求的默认无进展等待为 **5 分钟**，普通请求仍为 90 秒；工具参数和工具开始事件会更新进度，纯心跳不会无限续期。保留 30 秒数据/帧静默检查和 15 分钟总截止时间。
- 等上游排空、清理完成后再发送最终结束标记，减少正常结束与客户端断开的竞态。

模型仍可能尝试原生工具，提示和运行时拒绝不能保证每次都会成功改用客户端工具。客户端未提供合适工具时，应返回能力限制，不应声称已经保存文件。

## 授权网络

授权轮询、令牌刷新、模型发现和推理继承 EasyCPA 的上游代理设置。账号的 `proxy_url` 可覆盖全局代理，`direct` / `none` 表示直连。

针对 OAuth 轮询中的暂时网络错误、HTTP 408、429 和 5xx，最多额外重试 3 次；证书错误、401 和 403 仍会明确失败。旧授权已经失败时，需要重新发起登录。详见[授权网络与代理](cursor-plugin/docs/windows-easycpa.md#授权网络与代理)。

## 已知限制

- CLIProxyAPI 8.0.27 的流结束用量判定存在竞态：客户端可能已收到完成事件，宿主仍记录 HTTP 499。此次插件更新未修复核心的记账逻辑。
- 该核心的流式桥接未完整保留插件 `error_details`，部分上游超时仍可能显示 HTTP 500。
- Token 为本地估算，不代表 Cursor 实际账单；插件不提供订阅剩余额度查询。
- 0.6.6 的上下文准入、会话排队和输出预算仍保留；容量及验收边界见[上游 0.6.6 说明](cursor-plugin/docs/releases/v0.6.6.md)。

## 从源码构建

需要 Go 1.25+、支持 Windows amd64 的 MinGW-w64 GCC 和 PowerShell 7。在 `cursor-plugin` 目录执行：

```powershell
.\packaging\build-windows.ps1
.\packaging\build-release.ps1 0.6.6-windows.2
.\packaging\build-store-release.ps1 0.6.6-windows.2
```

本地 Windows CGO 测试、`go vet`、安装脚本集成检查及工作流 actionlint 已通过。CI 另执行 Linux 和 Windows 构建检查。

## 文档与来源

- [完整插件说明与 Linux 安装](cursor-plugin/README.md)
- [Windows EasyCPA 安装](cursor-plugin/docs/windows-easycpa.md)
- [Windows 增强版本说明](cursor-plugin/docs/releases/v0.6.6-windows.2.md)
- [第三方来源与许可证](cursor-plugin/THIRD_PARTY_NOTICES.md)：思考透传及无客户端工具提示参考 [kilolonion/cursor-cpa-plugin](https://github.com/kilolonion/cursor-cpa-plugin)，保留 MIT 署名。
- [免责声明](cursor-plugin/DISCLAIMER.md)

这是非官方社区项目。仅可使用本人所有或已获明确授权的账号和订阅，使用前请阅读免责声明并确认符合适用条款。根目录采用 Apache-2.0 许可证，`cursor-plugin/` 子项目采用其目录内的 MIT 许可证。
