package engine

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/hhx465453939/stemscribe/internal/registry"
)

// ParamKind is the value domain of one parameter.
type ParamKind string

const (
	KindInt    ParamKind = "int"
	KindFloat  ParamKind = "float"
	KindBool   ParamKind = "bool"
	KindEnum   ParamKind = "enum"
	KindString ParamKind = "string"
)

// Display groups, in order.
const (
	GroupLanguage     = "language"
	GroupQuality      = "quality"
	GroupSegmentation = "segmentation"
	GroupThreshold    = "threshold"
	GroupVAD          = "vad"
	GroupBackend      = "backend"
	GroupAdvanced     = "advanced"
	GroupOutput       = "output"
)

// ParamSpec declares one knob: its real CLI flag, meaning, default and domain.
//
// This table is the single source of truth for the UI, the CLI and the docs.
// Three hand-maintained copies drift within a week, so the UI renders from
// Specs() and a script compares the defaults against the actual binary output.
type ParamSpec struct {
	Key   string    // semantic key used by settings/UI, e.g. "input_language"
	Flag  string    // exact flag from whisper-cli --help
	Group string    // display group
	Kind  ParamKind // value domain
	Def   any       // default as reported by --help
	Min   float64
	Max   float64
	Enum  []string
	Unit  string
	Help  string
}

// WhisperParams is the typed form of every tunable whisper-cli option.
//
// Args() only emits flags whose value differs from the spec default, so a log
// line shows what the user changed instead of thirty identical defaults.
type WhisperParams struct {
	// language
	InputLanguage string // -l ("" or "auto" => auto-detect)
	DetectOnly    bool   // -dl
	Translate     bool   // -tr

	// quality
	BeamSize      int     // -bs
	BestOf        int     // -bo
	Temperature   float64 // -tp
	TempInc       float64 // -tpi
	NoFallback    bool    // -nf
	EntropyThold  float64 // -et
	LogprobThold  float64 // -lpt
	NoSpeechThold float64 // -nth
	WordThold     float64 // -wt

	// segmentation
	MaxContext   int  // -mc
	MaxLen       int  // -ml
	SplitOnWord  bool // -sow
	AudioCtx     int  // -ac
	NoTimestamps bool // -nt

	// vad
	VAD           bool    // --vad
	VADModelPath  string  // -vm
	VADThreshold  float64 // -vt
	VADMinSpeech  int     // -vspd
	VADMinSilence int     // -vsd
	VADPad        int     // -vp

	// backend
	Threads    int  // -t
	Processors int  // -p
	GPUDevice  int  // -dev
	NoGPU      bool // -ng
	FlashAttn  bool // -fa (default true; the off case needs -nfa)

	// advanced
	InitialPrompt string // --prompt
	CarryPrompt   bool   // --carry-initial-prompt
	SuppressNST   bool   // -sns
	SuppressRegex string // --suppress-regex

	// output
	OutSRT      bool // -osrt
	OutVTT      bool // -ovtt
	OutTXT      bool // -otxt
	OutJSON     bool // -oj (required by the pipeline)
	OutJSONFull bool // -ojf
	OutLRC      bool // -olrc
	OutCSV      bool // -ocsv
}

// Defaults returns the parameter set equal to the engine's own defaults.
//
// Used as the starting point for new tasks: it must not encode a preference,
// only what the binary would do on its own.
func Defaults() WhisperParams {
	return WhisperParams{
		InputLanguage: "en",
		BeamSize:      5,
		BestOf:        5,
		TempInc:       0.2,
		EntropyThold:  2.40,
		LogprobThold:  -1.00,
		NoSpeechThold: 0.60,
		WordThold:     0.01,
		MaxContext:    -1,
		VADThreshold:  0.50,
		VADMinSpeech:  250,
		VADMinSilence: 100,
		VADPad:        30,
		Threads:       4,
		Processors:    1,
		FlashAttn:     true,
		OutJSON:       true,
	}
}

// Specs returns the parameter table.
//
// Defaults are copied from `whisper-cli --help` of the pinned runtime (1.9.5);
// TRACEABILITY §3.1 records the same numbers together with that source.
func Specs() []ParamSpec {
	return []ParamSpec{
		{Key: "input_language", Flag: "-l", Group: GroupLanguage, Kind: KindEnum, Def: "en",
			Enum: []string{"auto", "zh", "en", "ja", "ko", "fr", "de", "es", "ru", "pt", "it"},
			Help: "Spoken language of the audio. auto detects it from the first 30s."},
		{Key: "detect_only", Flag: "-dl", Group: GroupLanguage, Kind: KindBool, Def: false,
			Help: "Exit after detecting the language; produces no transcript."},
		{Key: "translate", Flag: "-tr", Group: GroupLanguage, Kind: KindBool, Def: false,
			Help: "Translate into English. English is the only target whisper supports."},

		{Key: "beam_size", Flag: "-bs", Group: GroupQuality, Kind: KindInt, Def: 5, Min: 1, Max: 10,
			Help: "Beam size. Higher is slower and usually more accurate."},
		{Key: "best_of", Flag: "-bo", Group: GroupQuality, Kind: KindInt, Def: 5, Min: 1, Max: 10,
			Help: "Candidates kept when beam search is off."},
		{Key: "temperature", Flag: "-tp", Group: GroupQuality, Kind: KindFloat, Def: 0.0, Min: 0, Max: 1,
			Help: "Sampling temperature. 0 is deterministic."},
		{Key: "temperature_inc", Flag: "-tpi", Group: GroupQuality, Kind: KindFloat, Def: 0.2, Min: 0, Max: 1,
			Help: "Step added when decoding fails and falls back."},
		{Key: "no_fallback", Flag: "-nf", Group: GroupQuality, Kind: KindBool, Def: false,
			Help: "Disable temperature fallback while decoding."},
		{Key: "entropy_threshold", Flag: "-et", Group: GroupThreshold, Kind: KindFloat, Def: 2.40, Min: 0, Max: 10,
			Help: "Entropy above this counts as a decoding failure."},
		{Key: "logprob_threshold", Flag: "-lpt", Group: GroupThreshold, Kind: KindFloat, Def: -1.00, Min: -10, Max: 0,
			Help: "Log-probability below this counts as a decoding failure."},
		{Key: "no_speech_threshold", Flag: "-nth", Group: GroupThreshold, Kind: KindFloat, Def: 0.60, Min: 0, Max: 1,
			Help: "Above this, a window is treated as silence."},
		{Key: "word_threshold", Flag: "-wt", Group: GroupThreshold, Kind: KindFloat, Def: 0.01, Min: 0, Max: 1,
			Help: "Word timestamp probability threshold."},

		{Key: "max_context", Flag: "-mc", Group: GroupSegmentation, Kind: KindInt, Def: -1, Min: -1, Max: 4096,
			Help: "Text context tokens kept. -1 lets the engine decide."},
		{Key: "max_len", Flag: "-ml", Group: GroupSegmentation, Kind: KindInt, Def: 0, Min: 0, Max: 1000,
			Unit: "chars", Help: "Maximum segment length. 0 is unlimited."},
		{Key: "split_on_word", Flag: "-sow", Group: GroupSegmentation, Kind: KindBool, Def: false,
			Help: "Split on word rather than on token."},
		{Key: "audio_ctx", Flag: "-ac", Group: GroupSegmentation, Kind: KindInt, Def: 0, Min: 0, Max: 3000,
			Help: "Audio context size. 0 uses everything."},

		{Key: "vad", Flag: "--vad", Group: GroupVAD, Kind: KindBool, Def: false,
			Help: "Voice activity detection; skips silence on long recordings."},
		{Key: "vad_threshold", Flag: "-vt", Group: GroupVAD, Kind: KindFloat, Def: 0.50, Min: 0, Max: 1,
			Help: "Speech probability threshold for VAD."},
		{Key: "vad_min_speech_ms", Flag: "-vspd", Group: GroupVAD, Kind: KindInt, Def: 250, Min: 0, Max: 60000,
			Unit: "ms", Help: "Shortest speech run kept."},
		{Key: "vad_min_silence_ms", Flag: "-vsd", Group: GroupVAD, Kind: KindInt, Def: 100, Min: 0, Max: 60000,
			Unit: "ms", Help: "Shortest silence used to split segments."},
		{Key: "vad_pad_ms", Flag: "-vp", Group: GroupVAD, Kind: KindInt, Def: 30, Min: 0, Max: 5000,
			Unit: "ms", Help: "Padding around detected speech."},

		{Key: "threads", Flag: "-t", Group: GroupBackend, Kind: KindInt, Def: 4, Min: 1, Max: 128,
			Help: "CPU threads used during computation."},
		{Key: "processors", Flag: "-p", Group: GroupBackend, Kind: KindInt, Def: 1, Min: 1, Max: 8,
			Help: "Parallel workers the audio is split across."},
		{Key: "gpu_device", Flag: "-dev", Group: GroupBackend, Kind: KindInt, Def: 0, Min: 0, Max: 15,
			Help: "GPU index when several are present."},
		{Key: "no_gpu", Flag: "-ng", Group: GroupBackend, Kind: KindBool, Def: false,
			Help: "Disable the GPU even when the runtime has one."},
		{Key: "flash_attn", Flag: "-fa", Group: GroupBackend, Kind: KindBool, Def: true,
			Help: "Flash attention. Faster, slightly different numerics."},

		{Key: "initial_prompt", Flag: "--prompt", Group: GroupAdvanced, Kind: KindString, Def: "",
			Help: "Seed text: terminology, names, or a style hint."},
		{Key: "carry_initial_prompt", Flag: "--carry-initial-prompt", Group: GroupAdvanced, Kind: KindBool, Def: false,
			Help: "Prepend the seed text to every window, not just the first."},
		{Key: "suppress_nst", Flag: "-sns", Group: GroupAdvanced, Kind: KindBool, Def: false,
			Help: "Suppress non-speech tokens such as [music]."},
		{Key: "suppress_regex", Flag: "--suppress-regex", Group: GroupAdvanced, Kind: KindString, Def: "",
			Help: "Regex matching tokens to drop entirely."},

		{Key: "out_srt", Flag: "-osrt", Group: GroupOutput, Kind: KindBool, Def: false, Help: "Write SubRip subtitles."},
		{Key: "out_vtt", Flag: "-ovtt", Group: GroupOutput, Kind: KindBool, Def: false, Help: "Write WebVTT subtitles."},
		{Key: "out_txt", Flag: "-otxt", Group: GroupOutput, Kind: KindBool, Def: false, Help: "Write plain text."},
		{Key: "out_json", Flag: "-oj", Group: GroupOutput, Kind: KindBool, Def: true, Help: "Write JSON; the pipeline requires it."},
		{Key: "out_json_full", Flag: "-ojf", Group: GroupOutput, Kind: KindBool, Def: false, Help: "JSON with extra detail."},
		{Key: "out_lrc", Flag: "-olrc", Group: GroupOutput, Kind: KindBool, Def: false, Help: "Write LRC lyrics."},
		{Key: "out_csv", Flag: "-ocsv", Group: GroupOutput, Kind: KindBool, Def: false, Help: "Write CSV."},
	}
}

// SpecByKey looks up one spec.
func SpecByKey(key string) (ParamSpec, bool) {
	for _, s := range Specs() {
		if s.Key == key {
			return s, true
		}
	}
	return ParamSpec{}, false
}

// Groups returns group names in display order.
func Groups() []string {
	return []string{GroupLanguage, GroupQuality, GroupSegmentation, GroupThreshold,
		GroupVAD, GroupBackend, GroupAdvanced, GroupOutput}
}

// Args renders the parameter set as whisper-cli flags.
func (p WhisperParams) Args() []string {
	var a []string
	defInt := func(k string) int { s, _ := SpecByKey(k); return s.Def.(int) }
	defF := func(k string) float64 { s, _ := SpecByKey(k); return s.Def.(float64) }

	// language
	if p.InputLanguage != "" && p.InputLanguage != "en" {
		a = append(a, "-l", p.InputLanguage)
	}
	if p.DetectOnly {
		a = append(a, "-dl")
	}
	if p.Translate {
		a = append(a, "-tr")
	}

	// quality
	if p.BeamSize != 0 && p.BeamSize != defInt("beam_size") {
		a = append(a, "-bs", strconv.Itoa(p.BeamSize))
	}
	if p.BestOf != 0 && p.BestOf != defInt("best_of") {
		a = append(a, "-bo", strconv.Itoa(p.BestOf))
	}
	if p.Temperature != 0 {
		a = append(a, "-tp", fmt.Sprintf("%.2f", p.Temperature))
	}
	if p.TempInc != 0 && p.TempInc != defF("temperature_inc") {
		a = append(a, "-tpi", fmt.Sprintf("%.2f", p.TempInc))
	}
	if p.NoFallback {
		a = append(a, "-nf")
	}
	if p.EntropyThold != 0 && p.EntropyThold != defF("entropy_threshold") {
		a = append(a, "-et", fmt.Sprintf("%.2f", p.EntropyThold))
	}
	if p.LogprobThold != 0 && p.LogprobThold != defF("logprob_threshold") {
		a = append(a, "-lpt", fmt.Sprintf("%.2f", p.LogprobThold))
	}
	if p.NoSpeechThold != 0 && p.NoSpeechThold != defF("no_speech_threshold") {
		a = append(a, "-nth", fmt.Sprintf("%.2f", p.NoSpeechThold))
	}
	if p.WordThold != 0 && p.WordThold != defF("word_threshold") {
		a = append(a, "-wt", fmt.Sprintf("%.2f", p.WordThold))
	}

	// segmentation
	if p.MaxContext != 0 && p.MaxContext != defInt("max_context") {
		a = append(a, "-mc", strconv.Itoa(p.MaxContext))
	}
	if p.MaxLen != 0 {
		a = append(a, "-ml", strconv.Itoa(p.MaxLen))
	}
	if p.SplitOnWord {
		a = append(a, "-sow")
	}
	if p.AudioCtx != 0 {
		a = append(a, "-ac", strconv.Itoa(p.AudioCtx))
	}
	if p.NoTimestamps {
		a = append(a, "-nt")
	}

	// vad
	if p.VAD {
		a = append(a, "--vad")
		if p.VADModelPath != "" {
			a = append(a, "-vm", p.VADModelPath)
		}
		if p.VADThreshold != 0 && p.VADThreshold != defF("vad_threshold") {
			a = append(a, "-vt", fmt.Sprintf("%.2f", p.VADThreshold))
		}
		if p.VADMinSpeech != 0 && p.VADMinSpeech != defInt("vad_min_speech_ms") {
			a = append(a, "-vspd", strconv.Itoa(p.VADMinSpeech))
		}
		if p.VADMinSilence != 0 && p.VADMinSilence != defInt("vad_min_silence_ms") {
			a = append(a, "-vsd", strconv.Itoa(p.VADMinSilence))
		}
		if p.VADPad != 0 && p.VADPad != defInt("vad_pad_ms") {
			a = append(a, "-vp", strconv.Itoa(p.VADPad))
		}
	}

	// backend
	if p.Threads != 0 && p.Threads != defInt("threads") {
		a = append(a, "-t", strconv.Itoa(p.Threads))
	}
	if p.Processors != 0 && p.Processors != defInt("processors") {
		a = append(a, "-p", strconv.Itoa(p.Processors))
	}
	if p.GPUDevice != 0 {
		a = append(a, "-dev", strconv.Itoa(p.GPUDevice))
	}
	if p.NoGPU {
		a = append(a, "-ng")
	}
	if !p.FlashAttn {
		// -fa defaults to true, so the off case needs the negative flag.
		a = append(a, "-nfa")
	}

	// advanced
	if p.InitialPrompt != "" {
		a = append(a, "--prompt", p.InitialPrompt)
	}
	if p.CarryPrompt {
		a = append(a, "--carry-initial-prompt")
	}
	if p.SuppressNST {
		a = append(a, "-sns")
	}
	if p.SuppressRegex != "" {
		a = append(a, "--suppress-regex", p.SuppressRegex)
	}

	// The pipeline parses whisper's JSON, so it is always requested. The OutJSON
	// field exists only so the UI can show the switch as fixed-on; gating the flag
	// on it inverted the meaning and silently produced no transcript at all.
	a = append(a, "-oj")
	if p.OutJSONFull {
		a = append(a, "-ojf")
	}
	if p.OutSRT {
		a = append(a, "-osrt")
	}
	if p.OutVTT {
		a = append(a, "-ovtt")
	}
	if p.OutTXT {
		a = append(a, "-otxt")
	}
	if p.OutLRC {
		a = append(a, "-olrc")
	}
	if p.OutCSV {
		a = append(a, "-ocsv")
	}
	return a
}

// EnglishOnlyModel reports whether a model can only handle English.
//
// whisper ships ".en" variants with an English-only vocabulary. Feeding them
// Chinese yields fluent nonsense rather than an error, so this is checked before
// the run instead of being discovered in the output.
func EnglishOnlyModel(m registry.Model) bool {
	id := strings.ToLower(m.ID) + " " + strings.ToLower(m.DisplayName)
	return strings.Contains(id, ".en") || strings.Contains(id, "english only") ||
		strings.Contains(id, "english-only")
}

// ErrParamConflict marks a user-fixable parameter combination.
type ErrParamConflict struct {
	Key     string
	Message string
	Hint    string
}

func (e *ErrParamConflict) Error() string { return fmt.Sprintf("%s: %s", e.Key, e.Message) }

// Validate rejects combinations that cannot produce a meaningful result.
//
// Each check corresponds to a case that would otherwise fail silently: a
// wrong-language run reads as "bad model", and detect+translate reads as a broken
// pipeline, so both are refused up front with an actionable hint.
func (p WhisperParams) Validate(model registry.Model) error {
	if p.DetectOnly && p.Translate {
		return &ErrParamConflict{
			Key:     "translate",
			Message: "detect-only and translate cannot be enabled together",
			Hint:    "detect-only stops after identifying the language and produces no text to translate",
		}
	}

	if EnglishOnlyModel(model) {
		lang := strings.ToLower(strings.TrimSpace(p.InputLanguage))
		switch lang {
		case "auto", "":
			return &ErrParamConflict{
				Key:     "input_language",
				Message: fmt.Sprintf("model %q only supports English, so the language must be set to en explicitly", model.ID),
				Hint:    "pick a multilingual model, or set the language to en",
			}
		case "en":
			// fine
		default:
			return &ErrParamConflict{
				Key:     "input_language",
				Message: fmt.Sprintf("model %q only supports English and cannot transcribe %q", model.ID, p.InputLanguage),
				Hint:    "switch to a multilingual model (whisper-tiny, whisper-base, ...) or set the language to en",
			}
		}
	}

	if p.VAD && p.VADModelPath == "" {
		return &ErrParamConflict{
			Key:     "vad",
			Message: "voice activity detection needs a VAD model file",
			Hint:    "install a VAD model, or turn VAD off",
		}
	}

	if p.BeamSize < 0 || p.BestOf < 0 || p.MaxLen < 0 || p.Threads < 0 || p.Processors < 0 {
		return &ErrParamConflict{
			Key:     "quality",
			Message: "beam size, best-of, max length, threads and processors must not be negative",
			Hint:    "reset the offending field to its default (beam 5, best-of 5, max length 0, threads 4, processors 1)",
		}
	}
	if p.Temperature < 0 || p.Temperature > 1 {
		return &ErrParamConflict{
			Key:     "temperature",
			Message: "temperature must be within 0..1",
			Hint:    "use 0 for deterministic output; values above 0 trade accuracy for variety",
		}
	}
	return nil
}

// Snapshot records the non-default values for provenance.
//
// Only deviations are kept: the question a snapshot answers is "what made this
// run different", and thirty defaults per bundle bury that answer.
func (p WhisperParams) Snapshot() map[string]string {
	out := map[string]string{}
	for _, s := range Specs() {
		v, ok := p.valueByKey(s.Key)
		if !ok {
			continue
		}
		switch s.Kind {
		case KindFloat:
			f, err := strconv.ParseFloat(v, 64)
			d, ok2 := s.Def.(float64)
			if err == nil && ok2 && f == d {
				continue
			}
		case KindInt:
			i, err := strconv.Atoi(v)
			d, ok2 := s.Def.(int)
			if err == nil && ok2 && i == d {
				continue
			}
		default:
			if v == fmt.Sprint(s.Def) {
				continue
			}
		}
		if v == "" {
			continue
		}
		out[s.Key] = v
	}
	return out
}

// HashInput renders the parameters deterministically for the stage hash, so that
// changing any effective parameter invalidates exactly the stages that used it.
func (p WhisperParams) HashInput() string {
	snap := p.Snapshot()
	keys := make([]string, 0, len(snap))
	for k := range snap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+snap[k])
	}
	return strings.Join(parts, ";")
}

func (p WhisperParams) valueByKey(key string) (string, bool) {
	b := func(v bool) string { return strconv.FormatBool(v) }
	i := func(v int) string { return strconv.Itoa(v) }
	f := func(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) }
	switch key {
	case "input_language":
		return p.InputLanguage, true
	case "detect_only":
		return b(p.DetectOnly), true
	case "translate":
		return b(p.Translate), true
	case "beam_size":
		return i(p.BeamSize), true
	case "best_of":
		return i(p.BestOf), true
	case "temperature":
		return f(p.Temperature), true
	case "temperature_inc":
		return f(p.TempInc), true
	case "no_fallback":
		return b(p.NoFallback), true
	case "entropy_threshold":
		return f(p.EntropyThold), true
	case "logprob_threshold":
		return f(p.LogprobThold), true
	case "no_speech_threshold":
		return f(p.NoSpeechThold), true
	case "word_threshold":
		return f(p.WordThold), true
	case "max_context":
		return i(p.MaxContext), true
	case "max_len":
		return i(p.MaxLen), true
	case "split_on_word":
		return b(p.SplitOnWord), true
	case "audio_ctx":
		return i(p.AudioCtx), true
	case "vad":
		return b(p.VAD), true
	case "vad_threshold":
		return f(p.VADThreshold), true
	case "vad_min_speech_ms":
		return i(p.VADMinSpeech), true
	case "vad_min_silence_ms":
		return i(p.VADMinSilence), true
	case "vad_pad_ms":
		return i(p.VADPad), true
	case "threads":
		return i(p.Threads), true
	case "processors":
		return i(p.Processors), true
	case "gpu_device":
		return i(p.GPUDevice), true
	case "no_gpu":
		return b(p.NoGPU), true
	case "flash_attn":
		return b(p.FlashAttn), true
	case "initial_prompt":
		return p.InitialPrompt, true
	case "carry_initial_prompt":
		return b(p.CarryPrompt), true
	case "suppress_nst":
		return b(p.SuppressNST), true
	case "suppress_regex":
		return p.SuppressRegex, true
	case "out_srt":
		return b(p.OutSRT), true
	case "out_vtt":
		return b(p.OutVTT), true
	case "out_txt":
		return b(p.OutTXT), true
	case "out_json":
		return b(p.OutJSON), true
	case "out_json_full":
		return b(p.OutJSONFull), true
	case "out_lrc":
		return b(p.OutLRC), true
	case "out_csv":
		return b(p.OutCSV), true
	default:
		return "", false
	}
}
