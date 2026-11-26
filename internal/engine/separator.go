package engine

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hhx465453939/stemscribe/internal/pipeline"
)

// The separation stack is deliberately not part of the Go binary: it is the UVR5
// inference code (Python + onnxruntime) running as a child process, which is the
// only way Go can stay free of tensor math (ADR-002).
//
//go:embed separator_helper.py
var separatorHelperPy []byte

//go:embed separator_meta/*.json
var separatorMetaFS embed.FS

// SeparatorRuntimeID is the id accepted by "runtime install".
const SeparatorRuntimeID = "separator-python"

// separatorDir is <workdir>/runtimes/separator.
func separatorDir(workdir string) string {
	return filepath.Join(workdir, "runtimes", "separator")
}

// FindSeparatorPython resolves the Python interpreter that owns audio-separator.
//
// Order matters: an explicit environment override wins so a user can point at an
// existing environment, then the managed venv, then nothing. It never falls back
// to a bare "python" on PATH: that interpreter almost certainly lacks
// audio-separator, and a run that fails halfway through is worse than a run that
// never starts (ADR-037: separation is mandatory, not best-effort).
func FindSeparatorPython(workdir string) (string, bool) {
	if p := strings.TrimSpace(os.Getenv("STEMSCRIBE_SEPARATOR_PYTHON")); p != "" {
		if fileExists(p) {
			return p, true
		}
	}
	venv := filepath.Join(separatorDir(workdir), "venv")
	for _, rel := range []string{
		filepath.Join("Scripts", "python.exe"),
		filepath.Join("bin", "python"),
	} {
		p := filepath.Join(venv, rel)
		if fileExists(p) {
			return p, true
		}
	}
	return "", false
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// SeparatorVersion reports the installed audio-separator version.
//
// It reads the install record first and only spawns Python when the record is
// absent (an environment provisioned before the record existed). Provenance used
// to record whisper.cpp's version under "separate", which is a false statement
// about how the stems were produced.
func SeparatorVersion(workdir string) string {
	record := filepath.Join(separatorDir(workdir), "INSTALLED.json")
	if raw, err := os.ReadFile(record); err == nil {
		var v struct {
			Version string `json:"version"`
		}
		if json.Unmarshal(raw, &v) == nil && strings.TrimSpace(v.Version) != "" {
			return strings.TrimSpace(v.Version)
		}
	}
	python, ok := FindSeparatorPython(workdir)
	if !ok {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, python, "-c",
		"import importlib.metadata as m; print(m.version('audio-separator'))")
	cmd.Env = append(os.Environ(), "PYTHONUTF8=1", "PYTHONIOENCODING=utf-8")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// SeparatorRuntimeInfo is the acceleration record written at install time.
//
// The installer probes onnxruntime once and stores what it found; the run path
// re-reads it so the requested device and its recorded availability cannot drift
// apart. An install made before the field existed reads as empty -- which means
// "CPU-only", the truth for those environments.
type SeparatorRuntimeInfo struct {
	Accel     string   `json:"accel"`
	Providers []string `json:"providers"`
	Reason    string   `json:"accel_reason"`
	// TorchBackend / TorchReason describe the PyTorch half of the same
	// environment: "cuda", "cpu", "absent" or "" for records written before the
	// field existed. Keeping it separate from Accel is what lets a run say
	// "ONNX is on CUDA but Demucs would fall back to the CPU".
	TorchBackend string `json:"torch_backend,omitempty"`
	TorchReason  string `json:"torch_reason,omitempty"`
	TorchVersion string `json:"torch_version,omitempty"`
}

// ReadSeparatorRuntime returns the acceleration record, or a zero value when the
// runtime is missing or was installed by an older build.
func ReadSeparatorRuntime(workdir string) SeparatorRuntimeInfo {
	raw, err := os.ReadFile(filepath.Join(separatorDir(workdir), "INSTALLED.json"))
	if err != nil {
		return SeparatorRuntimeInfo{}
	}
	var info SeparatorRuntimeInfo
	if json.Unmarshal(raw, &info) != nil {
		return SeparatorRuntimeInfo{}
	}
	return info
}

// EnsureSeparatorAssets writes the helper script and the pinned UVR metadata into
// the runtime directory and returns their paths.
//
// The metadata files are the exact upstream lists that audio-separator would
// otherwise fetch from raw.githubusercontent.com, which is unreachable from the
// target network. Without them separation dies with a connection reset before it
// ever touches the model.
func EnsureSeparatorAssets(workdir string) (helperPath, metaDir string, err error) {
	dir := separatorDir(workdir)
	metaDir = filepath.Join(dir, "meta")
	if err = os.MkdirAll(metaDir, 0o755); err != nil {
		return "", "", fmt.Errorf("create separator runtime dir: %w", err)
	}

	helperPath = filepath.Join(dir, "helper.py")
	if err = writeIfChanged(helperPath, separatorHelperPy); err != nil {
		return "", "", fmt.Errorf("write separation helper: %w", err)
	}

	entries, err := separatorMetaFS.ReadDir("separator_meta")
	if err != nil {
		return "", "", fmt.Errorf("read embedded separator metadata: %w", err)
	}
	for _, e := range entries {
		raw, readErr := separatorMetaFS.ReadFile("separator_meta/" + e.Name())
		if readErr != nil {
			return "", "", fmt.Errorf("read embedded %s: %w", e.Name(), readErr)
		}
		if err = writeIfChanged(filepath.Join(metaDir, e.Name()), raw); err != nil {
			return "", "", fmt.Errorf("write separator metadata %s: %w", e.Name(), err)
		}
	}
	return helperPath, metaDir, nil
}

// writeIfChanged rewrites a managed file only when its bytes differ, so a
// re-build does not invalidate anything on every run.
func writeIfChanged(path string, want []byte) error {
	if cur, err := os.ReadFile(path); err == nil && string(cur) == string(want) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, want, 0o644)
}

// Separator adapts audio-separator (the UVR5 inference stack) to the pipeline
// Engine port. It produces the two stems and the 16 kHz mono copy that whisper
// actually consumes.
type Separator struct {
	Python    string // python.exe of the environment that has audio-separator
	Helper    string // helper.py written by EnsureSeparatorAssets
	MetaDir   string
	ModelDir  string // directory holding the weights and the UVR metadata
	ModelFile string // weight filename, e.g. Kim_Vocal_2.onnx
	// Arch is the registry architecture of ModelFile (mdx/vr/demucs/mdxc/
	// roformer). It selects which parameter dictionary the helper fills in.
	Arch      string
	FFmpegExe string
	Version   string // audio-separator version, recorded in provenance
	// Accel is the execution device requested from the Python stack: "cuda12",
	// "cpu", or "" for whatever the environment supports. It is requested, not
	// assumed -- LastBackend carries what actually happened.
	Accel string
	Log   *os.File

	// LastBackend / LastBackendReason / LastProviders describe the most recent
	// separation. Provenance records Backend: hardcoding "cpu" in the bundle
	// metadata meant a GPU run and a CPU run were indistinguishable afterwards.
	LastBackend       string
	LastBackendReason string
	LastProviders     []string
	// LastRuntime is "onnx" or "torch": which stack actually produced the stems.
	LastRuntime string

	exec execWithEnvFunc
}

type execWithEnvFunc func(ctx context.Context, exe string, args, env []string, onLine func(string)) (stdout []byte, stderr string, err error)

func (s *Separator) runner() execWithEnvFunc {
	if s.exec != nil {
		return s.exec
	}
	return runWithEnv
}

// runWithEnv is defaultExec plus a child environment. audio-separator shells out
// to ffmpeg by name, so the ffmpeg directory has to be on the child's PATH.
func runWithEnv(ctx context.Context, exe string, args, env []string, onLine func(string)) ([]byte, string, error) {
	cmd := exec.CommandContext(ctx, exe, args...)
	// Python spawns ffmpeg; stopping the run must take the whole tree down.
	wireCancel(cmd)
	cmd.Env = env

	var stdout strings.Builder
	cmd.Stdout = &stdout

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, "", fmt.Errorf("stderr pipe: %w", err)
	}
	if err = cmd.Start(); err != nil {
		return nil, "", fmt.Errorf("start %s: %w", filepath.Base(exe), err)
	}

	var stderr strings.Builder
	buf := make([]byte, 4096)
	var pending strings.Builder
	for {
		n, readErr := stderrPipe.Read(buf)
		if n > 0 {
			chunk := string(buf[:n])
			stderr.WriteString(chunk)
			pending.WriteString(chunk)
			for {
				line, rest, found := strings.Cut(pending.String(), "\n")
				if !found {
					pending.Reset()
					pending.WriteString(rest)
					break
				}
				if onLine != nil {
					onLine(line)
				}
				pending.Reset()
				pending.WriteString(rest)
			}
		}
		if readErr != nil {
			break
		}
	}
	if rest := strings.TrimSpace(pending.String()); rest != "" && onLine != nil {
		onLine(rest)
	}
	waitErr := cmd.Wait()
	return []byte(stdout.String()), stderr.String(), waitErr
}

type separatorResult struct {
	Stems     map[string]string `json:"stems"`
	Backend   string            `json:"backend"`
	Runtime   string            `json:"runtime"`
	Device    string            `json:"device"`
	Providers []string          `json:"providers"`
	// Arch / Params / FP16 / IgnoredParams make the run log answer "which knobs
	// actually took effect" instead of leaving it to the reader's guess.
	Arch          string         `json:"arch"`
	Params        map[string]any `json:"params"`
	FP16          bool           `json:"fp16"`
	IgnoredParams []string       `json:"ignored_params"`
	Reason        string         `json:"reason"`
}

// Separate runs UVR5 separation, then derives the 16 kHz mono vocal track that
// whisper-cli consumes.
//
// The transcription input is produced here rather than in the pipeline because
// only this stage knows the output is 44.1 kHz stereo; handing that to
// whisper-cli would silently change playback speed instead of failing.
func (s *Separator) Separate(ctx context.Context, inPath, outDir string, opts pipeline.SeparateOptions) (map[string]string, error) {
	if s.Python == "" || !fileExists(s.Python) {
		return nil, fmt.Errorf("vocal separation runtime is missing: install it with \"stemscribe runtime install %s\"", SeparatorRuntimeID)
	}
	if s.ModelFile == "" {
		return nil, fmt.Errorf("no separation model configured")
	}
	if !fileExists(filepath.Join(s.ModelDir, s.ModelFile)) {
		return nil, fmt.Errorf("separation model %s is missing from %s: install it with \"stemscribe model install\"", s.ModelFile, s.ModelDir)
	}
	if !fileExists(inPath) {
		return nil, fmt.Errorf("separation input missing: %s", filepath.Base(inPath))
	}

	args := []string{
		s.Helper,
		"--input", inPath,
		"--out-dir", outDir,
		"--model", s.ModelFile,
		"--model-dir", s.ModelDir,
		"--meta-dir", s.MetaDir,
	}
	if s.Accel != "" {
		args = append(args, "--accel", s.Accel)
	}
	if s.Arch != "" {
		args = append(args, "--arch", s.Arch)
	}
	// Only send a knob when the caller actually set one: the helper keeps the
	// architecture default for everything it is not told about, which is the
	// only way "unset" and "set to the default value" stay distinguishable.
	if opts.Segment > 0 {
		args = append(args, "--segment", strconv.Itoa(opts.Segment))
	}
	if opts.Overlap > 0 {
		args = append(args, "--overlap", strconv.FormatFloat(opts.Overlap, 'f', -1, 64))
	}
	if opts.Shifts > 0 {
		args = append(args, "--shifts", strconv.Itoa(opts.Shifts))
	}
	if opts.BatchSize > 0 {
		args = append(args, "--batch", strconv.Itoa(opts.BatchSize))
	}
	if opts.HalfPrecision {
		args = append(args, "--fp16")
	}
	env := os.Environ()
	env = append(env, "PYTHONUTF8=1", "PYTHONIOENCODING=utf-8")
	if s.FFmpegExe != "" {
		dir := filepath.Dir(s.FFmpegExe)
		env = append(env, "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	}

	onLine := func(line string) {
		if s.Log != nil && strings.TrimSpace(line) != "" {
			_, _ = fmt.Fprintln(s.Log, line)
		}
	}
	stdout, stderr, err := s.runner()(ctx, s.Python, args, env, onLine)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("separation canceled: %w", ctxErr)
		}
		detail := lastLine(stderr)
		if detail == "" {
			detail = "separation helper printed nothing to stderr"
		}
		return nil, fmt.Errorf("separation failed: %w: %s", err, detail)
	}

	var res separatorResult
	for _, line := range strings.Split(strings.TrimSpace(string(stdout)), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "{") {
			if jErr := json.Unmarshal([]byte(line), &res); jErr == nil {
				break
			}
		}
	}

	// Record what the helper actually did. The engine log line is the only place a
	// user can afterwards answer "did this run use the GPU?", so it is written for
	// every run, not only when something went wrong. Runtime and device are part of
	// it because an ONNX run and a torch run now share this adapter.
	s.LastBackend = strings.TrimSpace(res.Backend)
	s.LastRuntime = strings.TrimSpace(res.Runtime)
	s.LastProviders = res.Providers
	s.LastBackendReason = strings.TrimSpace(res.Reason)
	if s.LastBackend == "" {
		s.LastBackend = "cpu"
		s.LastBackendReason = "the separation helper did not report a backend"
	}
	if s.Log != nil {
		_, _ = fmt.Fprintf(s.Log, "separate backend=%s runtime=%s device=%s providers=%v requested=%q arch=%s params=%v fp16=%v\n",
			s.LastBackend, res.Runtime, res.Device, s.LastProviders, s.Accel, res.Arch, res.Params, res.FP16)
		if len(res.IgnoredParams) > 0 {
			_, _ = fmt.Fprintf(s.Log, "WARN separation parameters ignored by arch %s: %v\n",
				res.Arch, res.IgnoredParams)
		}
		if s.Accel == "cuda12" && s.LastBackend != "cuda12" {
			_, _ = fmt.Fprintf(s.Log, "WARN separation requested %s but ran on %s: %s\n",
				s.Accel, s.LastBackend, s.LastBackendReason)
		}
	}
	if len(res.Stems) == 0 {
		return nil, fmt.Errorf("separation helper reported no stems")
	}
	for stem, p := range res.Stems {
		info, statErr := os.Stat(p)
		if statErr != nil || info.Size() == 0 {
			return nil, fmt.Errorf("separation produced no %s track at %s", stem, filepath.Base(p))
		}
	}

	// 16 kHz mono copy of the vocals: this, not the 44.1 kHz stem, is what
	// whisper-cli reads.
	vocals16k := filepath.Join(outDir, "vocals16k.wav")
	ff := &FFmpeg{Exe: s.FFmpegExe}
	if err = ff.ToWhisperWAV(ctx, res.Stems["vocals"], vocals16k); err != nil {
		return nil, fmt.Errorf("convert vocals for transcription: %w", err)
	}
	return res.Stems, nil
}

// Composite is one engine port that transcribes with whisper.cpp and separates
// with the UVR5 stack. The pipeline asks for both on the same value, so the two
// adapters are glued here instead of leaking a second port into every caller.
type Composite struct {
	*Whisper
	Separator *Separator
}

// Separate delegates to the UVR5 process.
func (c *Composite) Separate(ctx context.Context, inPath, outDir string, opts pipeline.SeparateOptions) (map[string]string, error) {
	if c.Separator == nil {
		return nil, fmt.Errorf("no separator configured")
	}
	return c.Separator.Separate(ctx, inPath, outDir, opts)
}
