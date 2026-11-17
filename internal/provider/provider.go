// Package provider adapts chat-completion endpoints and resolves what each
// model can actually do (ADR-005).
//
// The hard part of this package is not HTTP: it is admitting that no endpoint
// tells you its context window. Capabilities therefore come from three sources,
// and the user's manual override always wins.
package provider

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Kind identifies the wire protocol.
type Kind string

const (
	KindOpenAICompat Kind = "openai_compat"
	KindAnthropic    Kind = "anthropic"
)

// CapsSource records where capability facts came from. Surfaced in the UI as a
// badge so the user knows whether a number is trustworthy.
type CapsSource string

const (
	CapsRegistry CapsSource = "registry"
	CapsProbe    CapsSource = "probe"
	CapsManual   CapsSource = "manual"
)

// Caps is the capability matrix the UI must expose (PRD §5).
type Caps struct {
	ContextWindow    int    `json:"context_window"`
	MaxOutputTokens  int    `json:"max_output_tokens"`
	Vision           bool   `json:"vision"`
	Tools            bool   `json:"tools"`
	Streaming        bool   `json:"streaming"`
	ReasoningSupport bool   `json:"reasoning_supported"`
	// ReasoningMode names the vendor dialect: "effort", "budget_tokens", "switch".
	ReasoningMode string `json:"reasoning_mode,omitempty"`
}

// Params is the per-model sampling configuration.
type Params struct {
	Temperature    float64  `json:"temperature"`
	TopP           float64  `json:"top_p"`
	MaxTokens      int      `json:"max_tokens"`
	ReasoningEffort string  `json:"reasoning_effort,omitempty"`
	ThinkingBudget int      `json:"thinking_budget_tokens,omitempty"`
	Stop           []string `json:"stop,omitempty"`
	Stream         bool     `json:"stream"`
	Seed           *int     `json:"seed,omitempty"`
}

// Model is one selectable model plus its resolved capabilities.
type Model struct {
	ID         string     `json:"id"`
	Display    string     `json:"display,omitempty"`
	Caps       Caps       `json:"caps"`
	CapsSource CapsSource `json:"caps_source"`
	Params     Params     `json:"params"`
	Stale      bool       `json:"stale,omitempty"`
}

// Config describes one endpoint.
type Config struct {
	ID           string
	Label        string
	Kind         Kind
	BaseURL      string
	APIKey       string
	Timeout      time.Duration
	MaxRetries   int
	ExtraHeaders map[string]string
	InsecureTLS  bool
}

// ConservativeDefaults are used whenever nothing else is known. Deliberately
// pessimistic: over-estimating the context window causes silent truncation,
// which is worse than sending an extra chunk.
func ConservativeDefaults() Caps {
	return Caps{
		ContextWindow:    131072,
		MaxOutputTokens:  4096,
		Vision:           false,
		Tools:            false,
		Streaming:        true,
		ReasoningSupport: false,
	}
}

// DefaultParams is the shipped sampling configuration: low temperature because
// polishing must not invent facts.
func DefaultParams(c Caps) Params {
	maxTok := 4096
	if c.MaxOutputTokens > 0 && c.MaxOutputTokens < maxTok {
		maxTok = c.MaxOutputTokens
	}
	return Params{
		Temperature: 0.3,
		TopP:        0.9,
		MaxTokens:   maxTok,
		Stream:      true,
	}
}

// MergeCaps resolves capabilities from the three tiers.
//
// Fields are layered additively (defaults -> probe -> registry -> manual) so a
// partial override never erases knowledge gained from a better source. The
// returned source names the highest tier that contributed.
//
// Manual wins because a user who paid for a 1M-context model must be able to say
// so when the endpoint refuses to advertise it.
func MergeCaps(registry, probe *Caps, manual *Caps) (Caps, CapsSource) {
	merged := ConservativeDefaults()
	src := CapsProbe
	if probe != nil {
		apply(&merged, *probe)
	}
	if registry != nil {
		apply(&merged, *registry)
		src = CapsRegistry
	}
	if manual != nil {
		apply(&merged, *manual)
		src = CapsManual
	}
	return sanitize(merged), src
}

// apply copies non-zero fields so a partial override does not erase defaults.
func apply(dst *Caps, src Caps) {
	if src.ContextWindow != 0 {
		dst.ContextWindow = src.ContextWindow
	}
	if src.MaxOutputTokens != 0 {
		dst.MaxOutputTokens = src.MaxOutputTokens
	}
	if src.Vision {
		dst.Vision = true
	}
	if src.Tools {
		dst.Tools = true
	}
	if src.Streaming {
		dst.Streaming = true
	}
	if src.ReasoningSupport {
		dst.ReasoningSupport = true
	}
	if src.ReasoningMode != "" {
		dst.ReasoningMode = src.ReasoningMode
	}
}

func sanitize(c Caps) Caps {
	if c.ContextWindow < 512 {
		c.ContextWindow = ConservativeDefaults().ContextWindow
	}
	if c.MaxOutputTokens < 16 {
		c.MaxOutputTokens = 16
	}
	return c
}

// Segment is one transcript line with a timestamp anchor.
type Segment struct {
	StartMS int    `json:"start_ms"`
	EndMS   int    `json:"end_ms"`
	Text    string `json:"text"`
}

// Anchor renders the segment's start as the [HH:MM:SS] tag used in prompts.
func (s Segment) Anchor() string { return FormatAnchor(s.StartMS) }

// FormatAnchor converts milliseconds to HH:MM:SS.
func FormatAnchor(ms int) string {
	if ms < 0 {
		ms = 0
	}
	total := ms / 1000
	h := total / 3600
	m := (total % 3600) / 60
	sec := total % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, sec)
}

// EstimateTokens is a deliberately conservative estimate used only for chunking.
//
// CJK text costs roughly one token per 1.8 characters while Latin text costs
// about one per 3.5 characters; taking the worse of the two avoids building a
// chunk that the endpoint then rejects.
func EstimateTokens(text string) int {
	cjk, other := 0, 0
	for _, r := range text {
		if r >= 0x2E80 && r <= 0x9FFF || r >= 0xF900 && r <= 0xFAFF || r >= 0xFF00 && r <= 0xFFEF {
			cjk++
			continue
		}
		other++
	}
	est := float64(cjk)/1.8 + float64(other)/3.5
	return int(math.Ceil(est))
}

// Chunk is a run of segments that fits a token budget.
type Chunk struct {
	Segments []Segment
	Tokens   int
}

// SplitByAnchors groups segments into chunks of at most budgetTokens.
//
// Splitting is done on segment boundaries only: cutting mid-segment would drop
// the anchor, and an anchor-less polish block cannot be traced back to the video
// (ADR-005). A single oversized segment is emitted alone rather than truncated.
func SplitByAnchors(segs []Segment, budgetTokens int) ([]Chunk, error) {
	if budgetTokens <= 0 {
		return nil, fmt.Errorf("budgetTokens must be positive, got %d", budgetTokens)
	}
	var chunks []Chunk
	var cur Chunk
	for _, s := range segs {
		cost := EstimateTokens(s.Text) + 8 // anchor + punctuation overhead
		if cost > budgetTokens {
			// Emit what we have, then the oversized segment on its own.
			if len(cur.Segments) > 0 {
				chunks = append(chunks, cur)
				cur = Chunk{}
			}
			chunks = append(chunks, Chunk{Segments: []Segment{s}, Tokens: cost})
			continue
		}
		if cur.Tokens+cost > budgetTokens && len(cur.Segments) > 0 {
			chunks = append(chunks, cur)
			cur = Chunk{}
		}
		cur.Segments = append(cur.Segments, s)
		cur.Tokens += cost
	}
	if len(cur.Segments) > 0 {
		chunks = append(chunks, cur)
	}
	return chunks, nil
}

// RenderSegments formats segments as anchored lines for the prompt body.
func RenderSegments(segs []Segment) string {
	var b strings.Builder
	for _, s := range segs {
		b.WriteString("[")
		b.WriteString(s.Anchor())
		b.WriteString("] ")
		b.WriteString(strings.TrimSpace(s.Text))
		b.WriteString("\n")
	}
	return b.String()
}

// ExtractAnchors returns the ordered list of anchors present in text.
func ExtractAnchors(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if len(line) < 11 || line[0] != '[' || line[9] != ']' {
			continue
		}
		candidate := line[1:9]
		if isClock(candidate) {
			out = append(out, candidate)
		}
	}
	return out
}

func isClock(s string) bool {
	if len(s) != 8 || s[2] != ':' || s[5] != ':' {
		return false
	}
	for i, r := range s {
		if i == 2 || i == 5 {
			continue
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	// Range-check the components: "99:99:99" is digit-shaped but not a clock.
	h, _ := strconv.Atoi(s[0:2])
	m, _ := strconv.Atoi(s[3:5])
	sec, _ := strconv.Atoi(s[6:8])
	return m < 60 && sec < 60 && h < 24
}

// VerifyAnchors checks that every input anchor survives into the output.
//
// This is the guard against a model "cleaning up" the transcript into something
// that can no longer be traced to a timestamp. Missing anchors mean the block
// must be replaced by the original text and marked degraded.
func VerifyAnchors(in, out []string) (missing []string) {
	have := make(map[string]struct{}, len(out))
	for _, a := range out {
		have[a] = struct{}{}
	}
	for _, a := range in {
		if _, ok := have[a]; !ok {
			missing = append(missing, a)
		}
	}
	return missing
}
