package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// FFmpeg wraps the ffmpeg runtime used for demuxing.
//
// whisper-cli only accepts 16 kHz 16-bit mono WAV, so demux is a hard
// prerequisite, not an optimisation (PRD F-04). Using one tool for probe and
// convert keeps the runtime promise to a single binary.
type FFmpeg struct {
	Exe string

	run func(ctx context.Context, exe string, args []string) (stdout, stderr string, err error)
}

func (f *FFmpeg) runner() func(ctx context.Context, exe string, args []string) (string, string, error) {
	if f.run != nil {
		return f.run
	}
	return defaultRun
}

func defaultRun(ctx context.Context, exe string, args []string) (string, string, error) {
	cmd := exec.CommandContext(ctx, exe, args...)
	wireCancel(cmd)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// durationRe matches ffmpeg's "Duration: 00:00:30.05," banner line.
var durationRe = regexp.MustCompile(`Duration:\s*(\d+):(\d{2}):(\d{2})\.(\d{2})`)

// ProbeDuration reads the container duration.
//
// ffmpeg exits non-zero when given only `-i` (no output), which is expected: the
// banner is still printed on stderr, so the exit code is ignored.
func (f *FFmpeg) ProbeDuration(ctx context.Context, path string) (time.Duration, error) {
	if _, err := os.Stat(path); err != nil {
		return 0, fmt.Errorf("probe input: %w", err)
	}
	_, stderr, _ := f.runner()(ctx, f.Exe, []string{"-hide_banner", "-i", path})
	m := durationRe.FindStringSubmatch(stderr)
	if m == nil {
		return 0, fmt.Errorf("could not read duration from %s", filepath.Base(path))
	}
	h, _ := strconv.Atoi(m[1])
	min, _ := strconv.Atoi(m[2])
	sec, _ := strconv.Atoi(m[3])
	cs, _ := strconv.Atoi(m[4])
	return time.Duration(h)*time.Hour +
		time.Duration(min)*time.Minute +
		time.Duration(sec)*time.Second +
		time.Duration(cs)*10*time.Millisecond, nil
}

// ToWhisperWAV converts any input into 16 kHz 16-bit mono PCM WAV.
func (f *FFmpeg) ToWhisperWAV(ctx context.Context, inPath, outPath string) error {
	if _, err := os.Stat(inPath); err != nil {
		return fmt.Errorf("demux input: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("demux output dir: %w", err)
	}

	args := []string{
		"-hide_banner", "-nostdin", "-y",
		"-i", inPath,
		"-vn",      // drop video
		"-ac", "1", // mono
		"-ar", "16000", // whisper requires 16 kHz
		"-c:a", "pcm_s16le",
		outPath,
	}
	_, stderr, err := f.runner()(ctx, f.Exe, args)
	if err != nil {
		return fmt.Errorf("demux failed: %w (%s)", err, lastLine(stderr))
	}
	info, statErr := os.Stat(outPath)
	if statErr != nil || info.Size() == 0 {
		return fmt.Errorf("demux produced no output at %s", filepath.Base(outPath))
	}
	return nil
}

// ToSeparationWAV converts any input into 44.1 kHz 16-bit stereo PCM WAV, the
// format the UVR5 models were trained on.
//
// Keeping this separate from ToWhisperWAV matters: the separator needs the full
// band and both channels, while whisper needs neither. One file cannot serve
// both without throwing away information before separation.
func (f *FFmpeg) ToSeparationWAV(ctx context.Context, inPath, outPath string) error {
	if _, err := os.Stat(inPath); err != nil {
		return fmt.Errorf("separation input: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("separation output dir: %w", err)
	}

	args := []string{
		"-hide_banner", "-nostdin", "-y",
		"-i", inPath,
		"-vn",
		"-ac", "2",
		"-ar", "44100",
		"-c:a", "pcm_s16le",
		outPath,
	}
	_, stderr, err := f.runner()(ctx, f.Exe, args)
	if err != nil {
		return fmt.Errorf("extract audio for separation: %w (%s)", err, lastLine(stderr))
	}
	info, statErr := os.Stat(outPath)
	if statErr != nil || info.Size() == 0 {
		return fmt.Errorf("separation input produced no audio at %s", filepath.Base(outPath))
	}
	return nil
}

// ToWAVPair decodes the source once into the two files the pipeline needs: a
// 44.1 kHz stereo track for UVR5 and a 16 kHz mono track for whisper-cli.
//
// Decoding twice would cost a second pass over every source file; a single ffmpeg
// invocation can write both outputs from one demux.
func (f *FFmpeg) ToWAVPair(ctx context.Context, inPath, separationPath, whisperPath string) error {
	if _, err := os.Stat(inPath); err != nil {
		return fmt.Errorf("demux input: %w", err)
	}
	for _, out := range []string{separationPath, whisperPath} {
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return fmt.Errorf("demux output dir: %w", err)
		}
	}

	args := []string{
		"-hide_banner", "-nostdin", "-y",
		"-i", inPath,
		"-vn",
		"-map", "0:a:0", "-ac", "2", "-ar", "44100", "-c:a", "pcm_s16le", separationPath,
		"-map", "0:a:0", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", whisperPath,
	}
	_, stderr, err := f.runner()(ctx, f.Exe, args)
	if err != nil {
		return fmt.Errorf("demux failed: %w (%s)", err, lastLine(stderr))
	}
	for _, out := range []string{separationPath, whisperPath} {
		info, statErr := os.Stat(out)
		if statErr != nil || info.Size() == 0 {
			return fmt.Errorf("demux produced no output at %s", filepath.Base(out))
		}
	}
	return nil
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if t := strings.TrimSpace(lines[i]); t != "" {
			return t
		}
	}
	return ""
}
