# 分离基准（SEPARATION-BENCH）

本文把 CHG-0017 / CHG-0018 期间**实测**出来的分离性能与质量数字集中到一处，供选模型、
判断是否需要 GPU、以及评估下一步优化时使用。

口径先说清楚：只写实际跑过的数，没跑过的写「未测」并说明缺什么。跨素材的数字不合并比较，
所有倍数关系都标注素材与参数。

## 1. 测试台

| 项 | 值 |
|---|---|
| GPU | RTX 3060，12,288 MiB（`nvidia-smi` / NVML 读数） |
| 驱动 | 566.36（支持 CUDA 12.6） |
| 分离运行时 | audio-separator 0.48.0，onnxruntime-gpu 1.21.1，torch 2.14.1+cu126 |
| 素材 | `speech-30s.wav`（30 s 播客人声）、`speech-120s.wav`（120 s，同源）、`speech-1s.wav` |
| 参数 | Demucs 默认（`shifts=2`、`overlap=0.25`、`segment_size="Default"`），另有标注的除外 |

素材是**播客口播**，不是音乐。它的能量几乎全在人声，因此下文「Other/Bass/Drums 相关性低」
并不等于模型差，而是这些轨本身就是噪声底（见 §4）。

## 2. Demucs：同一素材 GPU vs CPU

模型 `htdemucs_ft`（4 轨，AC-04 主对象）。GPU 与 CPU 各跑同一份 30 s 素材：

| 运行 | 素材 | 设备 | 计时范围 | 分离耗时 | RTF | 峰值显存 | GPU util 峰值 |
|---|---|---|---|---|---|---|---|
| A | 120 s | cuda | 命令全程（外部计时） | 50.48 s | 0.42 | 3,794 MiB | 100% |
| B | 30 s | cuda | 命令全程 | 17.54 s | 0.58 | 3,650 MiB | 100% |
| C | 30 s | cpu | 命令全程 | 124.36 s | 4.15 | 0 | 0（CUDA 未参与） |
| D | 1 s | cuda | 命令全程 | 9.16 s | — | — | — |

- RTF = 分离耗时 / 素材时长。<1 表示比实时快。
- **同素材（30 s）GPU 加速比：124.36 / 17.54 = 7.09×**。
- 120 s 素材 RTF 0.42 比 30 s 的 0.58 更好，说明固定成本（进程启动、加载权重）在 30 s
  素材里占了可见比例；素材越长，固定成本摊得越薄。
- 1 s 素材 9.16 s 里绝大部分是固定成本，不是分离计算——这条只说明固定成本量级，
  不要拿它算 RTF。

## 3. 固定成本与可摊薄的优化空间

| 计法 | 数值 | 说明 |
|---|---|---|
| 加载模型（外部计时，从进程启动算到开始分离） | 1–2 s | 表 2 的「加载模型」列，不含 Python 解释器与 import torch |
| 整条命令的冷启动（1 s 素材） | 9.16 s | 含解释器启动、import torch、加载权重、分离 |
| 生产 venv 首次分离 30 s 素材 | 17.14 s | 与最小 venv 的 17.54 s 一致（M2） |

两个数字口径不同，不能相加。它们共同指向同一件事：**每次分离都要付一次进程冷启动**。
批量跑多集时这部分会重复 N 次，这就是 SPEC §5.3 常驻 helper 要摊薄的对象（尚未实现，见 §7）。

## 4. 数值一致性（GPU 与 CPU 是否算的是同一件事）

同 30 s 素材、同参数（`shifts=2` 带随机位移，故不要求逐位一致）：

| 轨 | corr | SNR | max\|diff\| | 该轨 rms |
|---|---|---|---|---|
| Vocals | 0.999973 | 42.7 dB | 0.0063 | 0.06699 |
| Other | 0.785 | 4.0 dB | 0.0036 | 5e-04 量级 |
| Bass | 0.665 | 1.5 dB | 0.0005 | 3e-05 量级 |
| Drums | 0.499 | −1.6 dB | 0.0012 | 噪声底 |

- 四轨求和与输入的残差 `residual_rms 0.000988`，`SNR 36.6 dB`（16-bit 量化 + 模型近似）。
- 人声轨 rms 0.06699 ≈ 输入 0.06678，符合「播客里几乎全是人声」。
- Other/Bass/Drums 的相关性低是**能量太低**导致的：这些轨本身接近 0，相关系数没有意义。
  验证 GPU 正确性应看 Vocals 的 0.999973 / 42.7 dB，而不是看全部四轨。

## 5. 多轨输出

| 模型 | 架构 | 轨 | 产物 |
|---|---|---|---|
| `uvr-mdx-kim-vocal-2` | mdx（ONNX） | 2 | `vocals.wav` + `instrumental.wav` |
| `demucs-htdemucs` | demucs（torch） | 4 | voc/inst 之外另有 `drums/bass/other.wav` |
| `demucs-htdemucs-6s` | demucs（torch） | 6 | 4 轨 + `guitar.wav` + `piano.wav` |
| `demucs-htdemucs-ft` | demucs（torch） | 4 | 同 htdemucs，fine-tuned 权重 |

转写始终只吃 `vocals16k.wav`（16 kHz 单声道），分离扩轨不改变转写契约。
同一段 30 s 素材：MDX 2 轨集转写出 1 段，Demucs 4 轨集的 vocals 转写出 11 段。
这**不**证明谁更准——差异更可能来自 vocals 里残留的伴奏/噪声被 whisper 当成语音切段。
要用「转写质量」选模型，必须抽听对照，本表不提供该结论。

## 6. 参数的影响（已知与未测）

| 参数 | 已验证的事实 | 未测 |
|---|---|---|
| `--shifts`（demucs） | 1 与 2 都能跑通；值越大计算量越大（上游按位移次数重复前向） | 1/2/4 的耗时与本机质量差异 |
| `--overlap`（demucs/mdx） | 0.5 与默认 0.25 都能跑通，参数会写进日志与 checkpoint | 耗时曲线、接缝听感 |
| `--segment`（demucs 为秒，mdx 为 STFT 帧） | 支持范围已按架构校验，不支持的值启动前拒绝 | 长素材上的分段边界听感 |
| `--fp16` | 只有 torch+cuda 生效，默认关闭 | **质量未测，因此不作为默认值** |

参数变化会让 `separate` 阶段重跑（参数哈希包含 model/segment/overlap/shifts/batch/fp16），
这是为了让「换参数却复用旧声部」不可能发生。

## 7. 未测与下一步

1. **30 min 长素材 RTF**：AC-04 的验收口径要求 30 min 素材、同 venv 只换设备档位。
   本轮只有 30 s 与 120 s，长素材的内存曲线与分段行为未验证。
2. **whisper 侧吞吐**：本表只覆盖分离。转写阶段的 RTF / 显存未测。
3. **逐模型耗时与峰值显存**：目前只有 htdemucs_ft 有峰值显存读数（3,794 / 3,650 MiB），
   其余模型用的是 registry 静态估算。要变成实测值，需要把 VRAM 采样接进 `run` 的执行路径
   （现在 `resource.Scheduler` 只在 `probe` 命令里用），属独立变更。
4. **热启动常驻 helper**：SPEC §5.3 的设计（JSON-lines、一次加载多次分离）。
   收益上限可估：每集省掉约 9 s 冷启动；100 集 ≈ 15 min。实现涉及进程生命周期与崩溃恢复，
   需与一次性模式逐样本 A/B，同样属独立变更。
5. **音乐素材质量**：本轮素材全是口播。Demucs 的卖点是音乐分轨，但没有音乐素材的
   相关性/抽听数据，所以「音乐用 htdemucs_ft」目前只是**建议**，不是实测结论。

## 8. 复现

```bash
# 单模型分离（GPU，Demucs）
stemscribe run --input speech-30s.wav --separator demucs-htdemucs-ft --workdir <workdir>

# 参数 A/B
stemscribe run --input speech-30s.wav --separator demucs-htdemucs-ft \
  --shifts 1 --overlap 0.5 --workdir <workdir>

# 设备/后端自述（看 engine.log）
grep 'separate backend' <workdir>/logs/engine.log
```

本文数字来自 CHG-0018 期间的本机实测；原始日志与截图留在该轮工作目录（未入库，`tmp/` 已被
`.gitignore` 忽略，因此不可作为仓库内证据被引用）。表中每个数字都能用 §8 的命令重跑复现；
重跑若不吻合，以重跑结果为准并更新本文，而不是沿用旧值。
