# TRACEABILITY · CHG-0002

> 规则：无对应关系必须写 `TBD` 并说明由哪个里程碑回填；「实测数据」只登记真跑出来的数值。
> 本 CHG 尚未开工，因此**除契约文档外全部为 ⬜**——这不是遗漏，是真实状态。

---

## 1. 需求 → 判据 → 验证 → 归属

| 需求（PRD） | 目标 | AC | 验证方式 | 交付文件 | 里程碑 |
|---|---|---|---|---|---|
| F-01 自建运行时 | G-2 | AC-01、AC-02 | `runtime.yml` 构建日志 + `runtime-verify.ps1` | `.github/workflows/runtime.yml`、`scripts/runtime-build.ps1` | M1 |
| F-02 运行时校验 | G-2 | AC-03、AC-04 | 资产 hash vs registry 比对 | `internal/registry/data/registry.json` | M1/M2 |
| F-03 模型下载 | G-3 | AC-04 | 下载 + sha256 + `.part` → rename 观察 | `internal/downloader`（复用，不改） | M2 |
| F-04 ffmpeg 运行时 | G-5 | AC-05 | `ffprobe` 对比源/产物时长 | `internal/engine/ffmpeg.go` | M3 |
| F-05 引擎适配器 | G-1 | AC-07 | 编译期接口断言 | `internal/engine/whisper.go` | M4 |
| F-06 后端选择 | G-4 | AC-09 | 去掉 Vulkan 运行时后重跑 | `cmd/stemscribe/cli.go`、`internal/resource`（接线） | M5 |
| F-07 降级可见 | G-4 | AC-09 | 事件日志含 `reason` | `internal/engine/whisper.go` | M5 |
| F-08 真实 e2e | G-1 | AC-06、AC-12 | `smoke-test.ps1 -Real` | `scripts/smoke-test.ps1` | M6 |
| F-09 合成引擎退位 | G-1 | AC-12 | 默认路径不含 synthetic 字样 | `cmd/stemscribe/cli.go` | M5 |
| F-10 未发布不可装 | G-3 | AC-11 | registry 断言 + UI 手工 | `internal/registry`、`frontend`（后置联调） | M2 |
| 取消/孤儿进程 | G-6 | AC-08 | 取消后 `tasklist` 断言 | `internal/engine/whisper.go` | M4 |
| 缺失项一次列全 | G-1 | AC-10 | 空目录下 `run` | `cmd/stemscribe/cli.go` | M5 |

## 2. 交付物 → 判据 → 状态

| # | 交付物 | 判据 | 状态 |
|---|---|---|---|
| 1 | `tasks/CHG-0002/PRD.md` | 三件套 | ✅ 已交付 |
| 2 | `tasks/CHG-0002/SPEC.md` | 三件套 | ✅ 已交付 |
| 3 | `tasks/CHG-0002/TRACEABILITY.md`（本文件） | 三件套 | ✅ 已交付 |
| 4 | `tasks/CHG-0002/CHG.md` | 流程记录 | ✅ 已交付 |
| 5 | `.github/workflows/runtime.yml` | AC-01/02 | ⬜ M1 |
| 6 | `scripts/runtime-build.ps1` / `runtime-verify.ps1` | AC-02/03 | ⬜ M1 |
| 7 | registry 实测回填（whisper + ffmpeg） | AC-04 | ⬜ M2 |
| 8 | `internal/engine/ffmpeg.go` | AC-05 | ⬜ M3 |
| 9 | `internal/engine/whisper.go` + `progress.go` | AC-06/07/08 | ⬜ M4 |
| 10 | `cmd/stemscribe/cli.go` 引擎接线 + 缺失项一次列全 | AC-09/10/12 | ⬜ M5 |
| 11 | `scripts/smoke-test.ps1 -Real` 分支 | AC-12 | ⬜ M6 |
| 12 | `THIRD-PARTY-NOTICES.md`（whisper.cpp MIT / ffmpeg LGPL） | 合规 | ⬜ M6 |
| 13 | `docs/MODEL-REGISTRY.md` 运行时章节 + `docs/UPSTREAM-RESEARCH.md` 实测对照 | 文档同步 | ⬜ M6 |

## 3. 实测数据登记（禁止编造）

### 3.1 已确认的事实（有来源）

| 事实 | 来源 | 日期 |
|---|---|---|
| 模型官方下载源为 `https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-*.bin` | HF 仓库页（`ggml-tiny.bin` 直链已核实） | 2026-11-17 |
| whisper.cpp 的 Vulkan 后端产物运行时**不需厂商 SDK**，NVIDIA/AMD/Intel 通用 | 上游构建说明 + 社区 Vulkan 构建实践 | 2026-11-17 |
| CUDA 后端需要 CUDA Toolkit 构建，且运行时依赖 CUDA DLL | 上游构建说明 | 2026-11-17 |
| `whisper-cli` 只接受 **16 kHz 16-bit WAV** | 上游 README | 2026-11-17 |
| 二进制名为 `whisper-cli.exe` | 上游 README / 构建产物 | 2026-11-17 |
| 无 GPU 时 CPU 路径可用（本机实测：`probe` 返回 `cpu` 或 `vulkan` 的理由串） | 本机 `stemscribe probe` | 2026-11-17 |

### 3.2 待实测回填（**不得猜**）

| 项 | 谁填 | 里程碑 |
|---|---|---|
| cpu / vulkan 两个 zip 的**实测字节数** | `runtime-verify.ps1` | M1 |
| cpu / vulkan 两个 zip 的**实测 sha256** | 同上 | M1 |
| Vulkan SDK 在 CI 的安装耗时 | Actions 日志 | M1 |
| whisper.cpp 构建耗时（两变体） | Actions 日志 | M1 |
| 量化档位文件是否存在（Q-1） | `HEAD` 探测脚本 | M1 |
| Vulkan 构建的 DLL 依赖清单（确认无 CUDA） | `dumpbin` | M1 |
| 各模型档位的实测字节数 + sha256 | 下载后实测 | M2 |
| 30 分钟音频：`medium` 的 RTF 与峰值显存 | `probe` + NVML 采样 | M4 |
| `tiny` / `small` / `medium` 在 CPU 上的 RTF | 同上 | M4 |
| Vulkan 相对 CPU 的加速比 | 同上 | M4 |
| 适配器层额外开销（JSON 落盘+读取） | 计时 | M4 |
| 真实 30s 语音样本的生成方式与 hash（Q-3） | 生成脚本 | M6 |

## 4. 缺口登记（诚实）

| 缺口 | 影响 | 计划 |
|---|---|---|
| **人声分离不在本 CHG** | v0.1 的「人声分离」勾选框需置灰并说明"运行时未就绪" | CHG-0003 |
| CUDA 后端缺失 | NVIDIA 用户拿不到 CUDA 级性能 | 后续可选资产（ADR-013 同流程） |
| 无 DirectML | 老 Intel 核显可能只有 CPU 可用 | 不做（上游无成熟后端） |
| 模型量化档位命名未逐一核实 | registry 可能引用不存在的文件 | M1 阻塞项 Q-1 |
| Vulkan SDK 若导致 CI 超时 | 只能自建 CPU 变体 | Q-2 的降级路径已写明 |
| 真语音样本来源 | 不能把版权音频塞进仓库 | Q-3：TTS/合成 + 仅存 hash |
| AC-11（UI 不可安装态）依赖前端 | 本 CHG 只到后端契约与数据 | 与 CHG-0005 联调时收口 |

## 5. 里程碑状态（按实收）

| 里程碑 | 状态 | 证据 |
|---|---|---|
| M1 运行时自建流水线 | ✅ cpu + vulkan | run 44232117583：两变体构建成功、`--help` 自检通过、非 CUDA 包断言无 cudart/cublas；cuda12 为 job 级 opt-in（托管 runner 无 CUDA toolkit） |
| M2 registry 实测回填 | 🟡 cpu/vulkan 已填 | 实测 cpu 892,232 B / `d4a658f4…`、vulkan 14,997,989 B / `be16e210…`；**cuda12 仍 tbd**（无产物，不编造）；同时修正了 URL 与实际资产名不符会 404 的缺陷 |
| M3 ffmpeg + demux | 🟡 代码与单测完，未真跑 | `ffmpeg.go` + `ToWhisperWAV` 参数有断言；本机无 ffmpeg 二进制，未做真实转码验证 |
| M4 whisper 适配器 | 🟡 代码与单测完，未真跑 | `whisper.go` 实现 `pipeline.Engine`；进度/JSON 解析 12 个测试绿；尚未用真二进制跑通 |
| M5 CLI 接线与降级 | ✅ | `wire.go` 与 `probe` 共用后端选择；`install` 命令已实现，tbd 条目拒绝安装（实测）；缺项一次列全 |
| M6 真 e2e + 文档 | ⬜ 未开始 | 需真实音频 + 已发布运行时 + 模型；HF 与 github.com 本会话多次不可达 |

### 5.1 本 CHG 当前的真实阻塞点

| 阻塞 | 影响 | 解法 |
|---|---|---|
| 运行时未发布为 release 资产 | `install` 能下到 404（资产目前只在 Actions artifacts 里） | 打 `runtime-v1.9.5` tag 触发 publish job，或 `gh release create` 后重跑 |
| cuda12 无产物 | N 卡只能走 vulkan | 在带 CUDA toolkit 的 runner（自建/本地）上以 `with_cuda=true` 跑一次 |
| 无真实音频与模型 | AC-06/AC-12 无法出证据 | 网络恢复后下 `ggml-tiny`（77 MB）+ 生成 TTS 语音 |
| ffmpeg 未安装 | AC-05 只能靠注入式单测 | 下载静态构建放入 `runtimes/ffmpeg/<ver>/` |

### 5.2 本会话的实测证据（真跑，不是推论）

| 项 | 命令 | 结果 | 对应 AC |
|---|---|---|---|
| 三变体自建 | `runtime.yml` run 44232117583 | cpu ✅ / vulkan ✅ / cuda12 正确 skipped（opt-in）；非 CUDA 包断言无 cudart/cublas | AC-01、AC-02 |
| 实测值产出 | 同上，`registry-values` job | cpu 892,232 B / `d4a658f4…`；vulkan 14,997,989 B / `be16e210…` | AC-03 |
| 真实下载 + 校验 | `stemscribe runtime install whispercpp-cpu-win-x64` | 从 release 下载 → `verified sha256 d4a658f47e8c` → 解开出 `whisper-cli.exe` + `VERIFIED.json` + `PROVENANCE.txt` | AC-04 |
| 后端回退 | 只装 cpu 运行时（本机有 RTX 3060）后 `run` | 不再报 missing runtime，自动选 cpu | AC-09 |
| 缺项一次列全 | 同上 | 一次输出 model + ffmpeg 两条，各带安装命令 | AC-10 |
| tbd 不可装 | `runtime install` / `model install` 于 tbd 条目 | 明确拒绝：`refusing to install unverified bytes` | F-10 |

**过程中修掉的三个真缺陷**（都是跑起来才暴露的）：

1. job 级 `if` 用了 `matrix` 上下文 → 整个 workflow 解析失败、零 job；已拆成独立 CUDA job。
2. 只 checkout 了 whisper.cpp，覆盖掉本仓库 → 打包脚本不存在；已补本仓库 checkout。
3. 后端选择只看硬件能力 → 有 N 卡但只装 cpu 运行时则拒绝运行；已改为“按偏好顺序返回第一个已安装的后端”。

外加一处数据缺陷：registry 的下载 URL 与 CI 实际资产名不一致（会 404），已改为真实资产路径。

### 5.3 第三轮实测（原生客户端 + 真转写 + 目录规范化）

| 项 | 命令 / 产物 | 结果 | 对应 AC |
|---|---|---|---|
| 原生窗口客户端 | `CGO_ENABLED=1 go build -tags 'gui desktop production' -H windowsgui -o dist/Stemscribe.exe ./cmd/stemscribe-gui` | 进程 `MainWindowTitle=Stemscribe`、`Responding=True`、**18 个 `msedgewebview2` 子进程** → 内嵌 WebView2 的真窗口，不是外部浏览器 | AC-07 |
| 发行产物含客户端 | 下载 `Stemscribe-v0.1.0-rc1-portable.zip` 并解开 | **2 个 exe**：`Stemscribe.exe` 11.55 MB + `stemscribe-cli.exe` 8.19 MB | AC-07 |
| 模型源走 hf-mirror | `stemscribe model install whisper-tiny` | `from https://hf-mirror.com/...` → `verified sha256 be07e048e1e5` → 落在 `dist/workdir/models/whisper/whisper-tiny/` | AC-03、AC-04 |
| **真转写** | `stemscribe run --input tts.wav --model whisper-tiny` | 阶段全绿、`segments: 2`；逐字稿为音频原话：<br>`[00:00:00] The Quick Brown Fox jumps over the lazy dog.`<br>`[00:00:03] Stemscribe transcripts audio locally on your own graphics card.` | **AC-06、AC-12** |
| 工作目录不污染系统 | 不带 `--workdir` 运行 `probe` | `workdir : E:\Development\stemscribe\dist\workdir`（exe 同级，项目内） | F-09 |
| tbd 拒绝安装 | 对未实测条目 `install` | `refusing to install unverified bytes` | F-10 |

### 5.4 第三轮修掉的真缺陷

| # | 缺陷 | 怎么暴露的 |
|---|---|---|
| 1 | **Windows 文件名大小写不敏感**：`stemscribe.exe` 与 `Stemscribe.exe` 是同一个文件，桌面构建静默覆盖 CLI，发布包里只剩 1 个 exe | 解开实际 zip 数文件，而不是看构建日志（日志完全“正常”） |
| 2 | `store.stageOutputs` 声明 probe/demux 产出 `audio/full.wav`，真实路径从未写它 → 每次运行都报 `MISSING_OUTPUT`，且给不存在的文件记指纹，导致下次续跑无谓重跑 | 跑真转写时看到告警，追到 checkpoint 语义 |
| 3 | `run` 没有 `--model`，装了 tiny 也选不了，只能吃 settings 默认值 | 真跑时被 “missing model: whisper-medium” 拦住 |
| 4 | 默认工作目录是 `%USERPROFILE%\Stemscribe`，模型会散落到用户目录 | owner 指出：必须落在项目/安装目录内 |
| 5 | `SECURITY.md` 叫用户“不要开公开 issue” | owner 指出：开源项目就该开 issue，只有未修复漏洞才走私下渠道 |
| 6 | 后端选择只看硬件能力，不看已安装 → 有 N 卡但只装 cpu 运行时则拒绝运行 | 装完 cpu 运行时后 `probe` 报 cuda12，`run` 找不到 cuda12 |
| 7 | 提交不完整：改了 `wire.go`/`locate.go` 却没 stage，本地绿而 CI 红 | CI 报 `undefined: inputNeedsDemux` |
| 8 | 验证方法本身错：改了 `bundle.go` 没重建就重跑，拿旧二进制得出“修了没用” | 自查发现二进制时间戳早于源码 |

> 第 1、7、8 三条都是**“看起来成功了”的失败**：构建日志正常、本地编译通过、重跑有输出。
> 它们的共同修法是——**去看真实产物（zip 内容、二进制 mtime、CI 结论）**，而不是看流程输出。

### 5.5 仍未完成（不掩饰）

| 项 | 状态 | 阻塞 |
|---|---|---|
| AC-05 真实 demux | 代码 + 注入式单测绿；未真跑 | 本机无 ffmpeg 二进制 |
| AC-04 其余模型 sha256 | `whisper-tiny` 已实测；base/small/medium/large-v3 仍 `tbd` | 未下载即不编造（铁律 2） |
| cuda12 运行时产物 | 仍 `tbd` | CI 无 CUDA-capable runner；已做 job 级 opt-in |
| 界面与后端接线 | 窗口已开，但界面仍展示 demo 数据 | 属 CHG-0005（前端），需绑定 `ListTasks`/`ProbeHardware` |

## 6. 与既有 CHG 的关系

| CHG | 关系 |
|---|---|
| CHG-0001 | 提供 `internal/downloader`（续传/校验/原子落盘）与 `pipeline.Engine` 端口 → 本 CHG 直接复用，不改 |
| CHG-0003 | 接收本 CHG 移出的「人声分离」；本 CHG 的 `Separate` 返回 `ErrNotImplemented` |
| CHG-0006 | 本 CHG 只**接线** `internal/resource.SelectBackend`，不改预算算法；算法改动静走 CHG-0006 |
| CHG-0007 | 无交集（纯前端主题/i18n） |
