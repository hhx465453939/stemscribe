# 安全策略

## 报告漏洞

请**不要**开公开 issue。用 GitHub 的 [Private vulnerability reporting](https://docs.github.com/en/code-security/security-advisories/guidance-on-reporting-and-writing-information-about-vulnerabilities/privately-reporting-a-security-vulnerability)，
或邮件至仓库 owner（见 GitHub 个人主页）。

请在报告中包含：
- 影响版本
- 复现步骤（最小可复现）
- 影响面判断（密钥泄漏 / 任意文件写 / 远程代码执行 / 信息泄露）
- 是否已公开

**响应承诺**：72 小时内确认收到；高危 7 天内给修复计划；修复后发布公告并致谢（除非你要求匿名）。

## 安全边界（本项目的设计前提）

| 面 | 设计 |
|---|---|
| 网络 | 主进程只出站到 registry 声明的下载域名 + 用户配置的 LLM 端点。**默认不监听任何本地端口**（MCP HTTP 模式例外：仅 `127.0.0.1` + 随机 token，默认关闭） |
| 媒体隐私 | 音视频字节**永不外发**；仅转写文本可发往用户指定的 LLM，且首次启用需显式确认 |
| 密钥 | Windows DPAPI（`CryptProtectData`，用户作用域）加密存 `config/secrets.bin`；明文不落盘、不进日志、不进崩溃报告、不进产物 bundle |
| 引擎进程 | 只允许访问 workdir 内路径；通过 Windows Job Object 绑定，主进程退出时强制回收，防显存僵尸 |
| 下载供应链 | 每个下载物 sha256 强校验（流式计算）；解压前校验归档、解压后比对文件指纹；`dest` 路径禁止 `..` 与绝对路径（防目录穿越）；`.part` → `fsync` → rename 原子落盘 |
| 发布产物 | Release 附 `SHA256SUMS.txt`；构建未代码签名（会在 README 明示 SmartScreen 警告） |
| 提示词注入 | 转写文本作为**数据**传入 user 消息，不拼进 system 指令；润色输出后置校验时间戳锚点，防改写污染档案 |

## 已知的非安全项（不是漏洞）

| 现象 | 说明 |
|---|---|
| SmartScreen「未知发布者」 | 开源构建未购买代码签名证书，属预期行为；用 `SHA256SUMS.txt` 自校验 |
| 换机后需重新输入 API Key | DPAPI 密文与 Windows 用户绑定，属设计行为 |
| 首次运行需联网下载运行时 | 模型/引擎不随包分发（体积与许可原因） |
| 本地 LLM 端点用 HTTP 明文 | 回环通信，不出网卡；远程端必须 HTTPS（`insecure_tls` 默认关） |
| 无沙箱隔离引擎进程 | 引擎是可执行代码，用户从官方源安装；不提供第三方 registry 源 |

## 不在威胁模型内

- 已取得本机管理员权限的攻击者（DPAPI 用户态加密在管理员面前不构成边界）
- 恶意构造的视频文件触发的引擎内存破坏（依赖上游 whisper.cpp / ONNX Runtime 修复）
- 供应链上游（whisper.cpp、FFmpeg、onnxruntime）自身的 0day
- 用户主动把自己的 API Key 分享给他人

## 支持的版本

仅最新 release 接受安全修复。预发布版本（`-rc`）不承诺回溯修复。
