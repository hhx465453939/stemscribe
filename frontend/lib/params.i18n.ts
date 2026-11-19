// Bilingual copy for the parameter panel.
//
// The structure (keys, groups, flags, defaults, ranges) comes from
// params.generated.json, which is generated from the Go spec table and guarded by
// a drift test. Only the human wording lives here, because that is the one part
// a generator cannot produce.
//
// If a key is missing the panel falls back to the engine's own English help text
// rather than showing an empty control.

export interface Copy {
  zh: string;
  en: string;
}

export const paramLabels: Record<string, Copy> = {
  input_language: { zh: "识别语言", en: "Spoken language" },
  detect_only: { zh: "仅探测语言", en: "Detect language only" },
  translate: { zh: "翻译成英文", en: "Translate to English" },

  beam_size: { zh: "束搜索宽度", en: "Beam size" },
  best_of: { zh: "候选数", en: "Best of" },
  temperature: { zh: "采样温度", en: "Temperature" },
  temperature_inc: { zh: "温度回退增量", en: "Temperature fallback step" },
  no_fallback: { zh: "关闭温度回退", en: "Disable fallback" },
  entropy_threshold: { zh: "熵阈值", en: "Entropy threshold" },
  logprob_threshold: { zh: "对数概率阈值", en: "Log-prob threshold" },
  no_speech_threshold: { zh: "静音判定阈值", en: "No-speech threshold" },
  word_threshold: { zh: "词时间戳阈值", en: "Word timestamp threshold" },

  max_context: { zh: "文本上下文", en: "Text context" },
  max_len: { zh: "单段最大长度", en: "Max segment length" },
  split_on_word: { zh: "按词切分", en: "Split on word" },
  audio_ctx: { zh: "音频上下文", en: "Audio context" },

  vad: { zh: "语音活动检测", en: "Voice activity detection" },
  vad_threshold: { zh: "VAD 阈值", en: "VAD threshold" },
  vad_min_speech_ms: { zh: "最短语音", en: "Min speech" },
  vad_min_silence_ms: { zh: "最短静音", en: "Min silence" },
  vad_pad_ms: { zh: "语音前后留白", en: "Speech padding" },

  threads: { zh: "CPU 线程", en: "CPU threads" },
  processors: { zh: "并行处理器", en: "Processors" },
  gpu_device: { zh: "GPU 序号", en: "GPU device" },
  no_gpu: { zh: "禁用 GPU", en: "Disable GPU" },
  flash_attn: { zh: "Flash Attention", en: "Flash attention" },

  initial_prompt: { zh: "初始提示词", en: "Initial prompt" },
  carry_initial_prompt: { zh: "每窗都带提示词", en: "Carry prompt every window" },
  suppress_nst: { zh: "抑制非语音标记", en: "Suppress non-speech tokens" },
  suppress_regex: { zh: "抑制命中正则的标记", en: "Suppress regex tokens" },

  out_srt: { zh: "输出 SRT 字幕", en: "Write SRT" },
  out_vtt: { zh: "输出 WebVTT 字幕", en: "Write WebVTT" },
  out_txt: { zh: "输出纯文本", en: "Write plain text" },
  out_json: { zh: "输出 JSON（管线必需）", en: "Write JSON (required)" },
  out_json_full: { zh: "输出详细 JSON", en: "Write detailed JSON" },
  out_lrc: { zh: "输出 LRC 歌词", en: "Write LRC" },
  out_csv: { zh: "输出 CSV", en: "Write CSV" },
};

export const paramHelps: Record<string, Copy> = {
  input_language: {
    zh: "音频里说的是什么语言。auto 会先自动探测再转写。",
    en: "What language the audio is in. auto detects it before transcribing.",
  },
  detect_only: {
    zh: "只报告探测到的语言，不产出逐字稿。与「翻译」互斥。",
    en: "Only report the detected language. Cannot be combined with translate.",
  },
  translate: { zh: "把结果翻译成英文。whisper 只支持英文这一个目标语言。", en: "Translate into English, the only target whisper supports." },
  beam_size: { zh: "越大越慢，通常更准。默认 5。", en: "Higher is slower and usually more accurate. Default 5." },
  best_of: { zh: "不开束搜索时保留的候选数。", en: "Candidates kept when beam search is off." },
  temperature: { zh: "0 为确定性输出；大于 0 会引入变化。", en: "0 is deterministic; above 0 introduces variation." },
  temperature_inc: { zh: "解码失败回退时每次增加的温度。", en: "Step added each time decoding falls back." },
  no_fallback: { zh: "解码失败时不做温度回退。", en: "Do not fall back on failure." },
  entropy_threshold: { zh: "熵高于此值视为解码失败。", en: "Above this, decoding counts as failed." },
  logprob_threshold: { zh: "对数概率低于此值视为解码失败。", en: "Below this, decoding counts as failed." },
  no_speech_threshold: { zh: "高于此值的时间窗按静音处理。", en: "Windows above this are treated as silence." },
  word_threshold: { zh: "词级时间戳的概率阈值。", en: "Probability threshold for word timestamps." },
  max_context: { zh: "保留的上下文 token 数，-1 交给引擎决定。", en: "Context tokens kept; -1 lets the engine decide." },
  max_len: { zh: "单段最大字符数，0 表示不限制。", en: "Max characters per segment; 0 is unlimited." },
  split_on_word: { zh: "按词而不是按 token 切分。", en: "Split on word rather than token." },
  audio_ctx: { zh: "音频上下文长度，0 使用全部。", en: "Audio context size; 0 uses everything." },
  vad: { zh: "跳过静音、加速长录音。需要先装 VAD 模型。", en: "Skip silence on long files. Needs a VAD model installed." },
  vad_threshold: { zh: "判定为语音的概率阈值。", en: "Speech probability threshold." },
  vad_min_speech_ms: { zh: "短于此长度的语音段会被丢弃。", en: "Speech runs shorter than this are dropped." },
  vad_min_silence_ms: { zh: "用于切分段落的最短静音长度。", en: "Shortest silence used to split segments." },
  vad_pad_ms: { zh: "在语音前后补的留白，避免切掉首尾字。", en: "Padding around speech, so first and last syllables survive." },
  threads: { zh: "计算使用的 CPU 线程数。", en: "CPU threads used for computation." },
  processors: { zh: "音频被拆给几个并行处理器（会改变上下文）。", en: "How many workers the audio is split across." },
  gpu_device: { zh: "多显卡时选择哪一张。", en: "Which GPU to use when several exist." },
  no_gpu: { zh: "即使运行时带 GPU 也强制走 CPU。", en: "Force CPU even when the runtime has a GPU." },
  flash_attn: { zh: "更快，但数值结果略有差异。", en: "Faster, with slightly different numerics." },
  initial_prompt: { zh: "术语、人名、风格提示。会作为解码的起点。", en: "Terminology, names or style hints used to seed decoding." },
  carry_initial_prompt: { zh: "每个音频窗都带上提示词，而不只是第一段。", en: "Prepend the seed text to every window, not only the first." },
  suppress_nst: { zh: "去掉 [音乐] 之类的非语音标记。", en: "Drop non-speech tokens such as [music]." },
  suppress_regex: { zh: "命中正则的 token 会被整体去掉。", en: "Tokens matching the regex are dropped entirely." },
  out_srt: { zh: "生成 SubRip 字幕。", en: "Write SubRip subtitles." },
  out_vtt: { zh: "生成 WebVTT 字幕。", en: "Write WebVTT subtitles." },
  out_txt: { zh: "生成纯文本。", en: "Write plain text." },
  out_json: { zh: "管线靠它解析结果，始终开启。", en: "The pipeline parses this, so it is always on." },
  out_json_full: { zh: "JSON 里包含更多细节。", en: "Include extra detail in the JSON." },
  out_lrc: { zh: "生成 LRC 歌词文件。", en: "Write LRC lyrics." },
  out_csv: { zh: "生成 CSV。", en: "Write CSV." },
};

export const groupLabels: Record<string, Copy> = {
  language: { zh: "语言", en: "Language" },
  quality: { zh: "质量", en: "Quality" },
  segmentation: { zh: "分段", en: "Segmentation" },
  threshold: { zh: "阈值", en: "Thresholds" },
  vad: { zh: "语音活动检测", en: "Voice activity" },
  backend: { zh: "算力与后端", en: "Compute and backend" },
  advanced: { zh: "高级", en: "Advanced" },
  output: { zh: "输出格式", en: "Output formats" },
};

export function pick(c: Copy | undefined, lang: "zh" | "en", fallback: string): string {
  if (!c) return fallback;
  return lang === "zh" ? c.zh : c.en;
}
