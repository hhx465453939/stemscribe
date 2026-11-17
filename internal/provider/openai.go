package provider

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Message is one chat turn.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatResult is a completed (possibly streamed) completion.
type ChatResult struct {
	Text       string
	Usage      Usage
	Streamed   bool
	Duration   time.Duration
	StatusCode int
}

// Usage reports token accounting when the endpoint provides it.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// ErrKind classifies failures so the pipeline can decide whether polishing is
// recoverable (PIPELINE.md §6: polish must never fail the whole episode).
type ErrKind string

const (
	ErrAuth     ErrKind = "LLM_AUTH"
	ErrNotFound ErrKind = "LLM_NOT_FOUND"
	ErrRateLim  ErrKind = "LLM_RATE_LIMIT"
	ErrTimeout  ErrKind = "LLM_TIMEOUT"
	ErrNetwork  ErrKind = "LLM_NETWORK"
	ErrFormat   ErrKind = "LLM_BAD_FORMAT"
	ErrContext  ErrKind = "LLM_CONTEXT_EXCEEDED"
	ErrRefused  ErrKind = "LLM_REFUSED"
)

// Error carries a stable kind plus the provider's own message.
type Error struct {
	Kind    ErrKind
	Message string
	Status  int
	Err     error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Kind, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Kind, e.Message)
}

func (e *Error) Unwrap() error { return e.Err }

// Retryable reports whether an automatic retry is worthwhile.
func (e *Error) Retryable() bool {
	switch e.Kind {
	case ErrRateLim, ErrTimeout, ErrNetwork:
		return true
	default:
		return false
	}
}

// Client talks to one endpoint.
type Client struct {
	cfg  Config
	http *http.Client
}

// New validates the configuration and builds a client.
func New(cfg Config) (*Client, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("base_url is required")
	}
	if !strings.HasPrefix(cfg.BaseURL, "http://") && !strings.HasPrefix(cfg.BaseURL, "https://") {
		return nil, fmt.Errorf("base_url must be http(s), got %q", cfg.BaseURL)
	}
	if cfg.Kind == "" {
		cfg.Kind = KindOpenAICompat
	}
	if cfg.Kind != KindOpenAICompat && cfg.Kind != KindAnthropic {
		return nil, fmt.Errorf("unsupported provider kind %q", cfg.Kind)
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.InsecureTLS {
		// Explicitly opted in by the user; the UI must show a red warning.
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // #nosec G402
	}

	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: timeout, Transport: transport},
	}, nil
}

// ID returns the configured provider id.
func (c *Client) ID() string { return c.cfg.ID }

func (c *Client) url(path string) string {
	base := strings.TrimRight(c.cfg.BaseURL, "/")
	// Accept both ".../v1" and ".../v1/" style base URLs.
	return base + "/" + strings.TrimLeft(path, "/")
}

func (c *Client) applyHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.APIKey != "" {
		if c.cfg.Kind == KindAnthropic {
			req.Header.Set("x-api-key", c.cfg.APIKey)
			req.Header.Set("anthropic-version", "2023-06-01")
		} else {
			req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
		}
	}
	for k, v := range c.cfg.ExtraHeaders {
		// Never let an extra header shadow the credential we just set.
		if strings.EqualFold(k, "authorization") || strings.EqualFold(k, "x-api-key") {
			continue
		}
		req.Header.Set(k, v)
	}
}

// ListModels refreshes the model list. This is the "刷新" button.
func (c *Client) ListModels(ctx context.Context) ([]string, error) {
	if c.cfg.Kind == KindAnthropic {
		// The Messages API has no public model listing; return nothing so the UI
		// falls back to manual entry instead of pretending it knows.
		return nil, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url("models"), nil)
	if err != nil {
		return nil, &Error{Kind: ErrNetwork, Message: "build request", Err: err}
	}
	c.applyHeaders(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, wrapTransportError(err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, &Error{Kind: ErrNetwork, Message: "read body", Err: err}
	}
	if err = statusError(resp.StatusCode, body); err != nil {
		return nil, err
	}

	// Accept both OpenAI's {"data":[{"id":...}]} and bare arrays / {"models":[...]}.
	var envelope struct {
		Data   []struct{ ID string `json:"id"` } `json:"data"`
		Models []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"models"`
	}
	if err = json.Unmarshal(body, &envelope); err == nil {
		// An empty list is a valid answer (a freshly started endpoint has no
		// models loaded yet). Returning an error here made the UI claim the
		// endpoint was broken — a real bug caught by TestExtraHeadersCannotShadowAuth.
		ids := make([]string, 0, len(envelope.Data)+len(envelope.Models))
		for _, m := range envelope.Data {
			if m.ID != "" {
				ids = append(ids, m.ID)
			}
		}
		for _, m := range envelope.Models {
			id := m.ID
			if id == "" {
				id = m.Name
			}
			if id != "" {
				ids = append(ids, id)
			}
		}
		return dedupe(ids), nil
	}

	var bare []struct{ ID string `json:"id"` }
	if err = json.Unmarshal(body, &bare); err == nil && len(bare) > 0 {
		ids := make([]string, 0, len(bare))
		for _, m := range bare {
			if m.ID != "" {
				ids = append(ids, m.ID)
			}
		}
		if len(ids) > 0 {
			return dedupe(ids), nil
		}
	}

	return nil, &Error{Kind: ErrFormat, Message: "could not parse model list", Status: resp.StatusCode}
}

func dedupe(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := in[:0]
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// Chat sends a completion request and returns the accumulated text.
// When p.Stream is true it consumes SSE and reports deltas through onDelta.
func (c *Client) Chat(ctx context.Context, model string, msgs []Message, p Params, onDelta func(string)) (ChatResult, error) {
	if model == "" {
		return ChatResult{}, fmt.Errorf("model is required")
	}
	started := time.Now()

	if c.cfg.Kind == KindAnthropic {
		return c.chatAnthropic(ctx, model, msgs, p, onDelta, started)
	}
	return c.chatOpenAI(ctx, model, msgs, p, onDelta, started)
}

type openAIRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	TopP        float64   `json:"top_p,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Stop        []string  `json:"stop,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
	Seed        *int      `json:"seed,omitempty"`
}

func (c *Client) chatOpenAI(ctx context.Context, model string, msgs []Message, p Params, onDelta func(string), started time.Time) (ChatResult, error) {
	payload := openAIRequest{
		Model:       model,
		Messages:    msgs,
		Temperature: p.Temperature,
		TopP:        p.TopP,
		MaxTokens:   p.MaxTokens,
		Stop:        p.Stop,
		Stream:      p.Stream,
		Seed:        p.Seed,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return ChatResult{}, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url("chat/completions"), bytes.NewReader(raw))
	if err != nil {
		return ChatResult{}, &Error{Kind: ErrNetwork, Message: "build request", Err: err}
	}
	c.applyHeaders(req)
	if p.Stream {
		req.Header.Set("Accept", "text/event-stream")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return ChatResult{}, wrapTransportError(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		return ChatResult{}, statusError(resp.StatusCode, body)
	}

	if !p.Stream {
		var out struct {
			Choices []struct {
				Message Message `json:"message"`
			} `json:"choices"`
			Usage Usage `json:"usage"`
		}
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return ChatResult{}, &Error{Kind: ErrNetwork, Message: "read body", Err: readErr}
		}
		if err = json.Unmarshal(body, &out); err != nil {
			return ChatResult{}, &Error{Kind: ErrFormat, Message: "parse completion", Err: err}
		}
		text := ""
		if len(out.Choices) > 0 {
			text = out.Choices[0].Message.Content
		}
		if onDelta != nil && text != "" {
			onDelta(text)
		}
		return ChatResult{Text: text, Usage: out.Usage, Duration: time.Since(started), StatusCode: resp.StatusCode}, nil
	}

	return parseSSE(resp.Body, onDelta, started, resp.StatusCode)
}

// parseSSE consumes server-sent events, tolerating junk lines rather than
// aborting a long polish run on one malformed frame.
func parseSSE(r io.Reader, onDelta func(string), started time.Time, status int) (ChatResult, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 4<<20)

	var b strings.Builder
	var usage Usage
	var badLines int

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var frame struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Usage Usage `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &frame); err != nil {
			badLines++
			if badLines > 10 {
				return ChatResult{}, &Error{Kind: ErrFormat, Message: "too many malformed SSE frames"}
			}
			continue
		}
		if frame.Usage.CompletionTokens > 0 {
			usage = frame.Usage
		}
		if len(frame.Choices) > 0 {
			chunk := frame.Choices[0].Delta.Content
			if chunk != "" {
				b.WriteString(chunk)
				if onDelta != nil {
					onDelta(chunk)
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return ChatResult{}, wrapTransportError(err)
	}

	return ChatResult{Text: b.String(), Usage: usage, Streamed: true, Duration: time.Since(started), StatusCode: status}, nil
}

type anthropicRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	System      string    `json:"system,omitempty"`
	MaxTokens   int       `json:"max_tokens"`
	Temperature float64   `json:"temperature,omitempty"`
	TopP        float64   `json:"top_p,omitempty"`
	Stop        []string  `json:"stop_sequences,omitempty"`
}

func (c *Client) chatAnthropic(ctx context.Context, model string, msgs []Message, p Params, onDelta func(string), started time.Time) (ChatResult, error) {
	var system []string
	systemMsgs := make([]Message, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == "system" {
			system = append(system, m.Content)
			continue
		}
		systemMsgs = append(systemMsgs, m)
	}

	maxTok := p.MaxTokens
	if maxTok <= 0 {
		maxTok = 4096
	}
	payload := anthropicRequest{
		Model:       model,
		Messages:    systemMsgs,
		System:      strings.Join(system, "\n\n"),
		MaxTokens:   maxTok,
		Temperature: p.Temperature,
		TopP:        p.TopP,
		Stop:        p.Stop,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return ChatResult{}, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url("messages"), bytes.NewReader(raw))
	if err != nil {
		return ChatResult{}, &Error{Kind: ErrNetwork, Message: "build request", Err: err}
	}
	c.applyHeaders(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return ChatResult{}, wrapTransportError(err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ChatResult{}, &Error{Kind: ErrNetwork, Message: "read body", Err: err}
	}
	if err = statusError(resp.StatusCode, body); err != nil {
		return ChatResult{}, err
	}

	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err = json.Unmarshal(body, &out); err != nil {
		return ChatResult{}, &Error{Kind: ErrFormat, Message: "parse message", Err: err}
	}
	var b strings.Builder
	for _, block := range out.Content {
		if block.Type == "text" {
			b.WriteString(block.Text)
		}
	}
	text := b.String()
	if onDelta != nil && text != "" {
		onDelta(text)
	}
	return ChatResult{
		Text:       text,
		Usage:      Usage{PromptTokens: out.Usage.InputTokens, CompletionTokens: out.Usage.OutputTokens},
		Duration:   time.Since(started),
		StatusCode: resp.StatusCode,
	}, nil
}

func statusError(status int, body []byte) error {
	if status >= 200 && status < 300 {
		return nil
	}
	snippet := strings.TrimSpace(string(body))
	if len(snippet) > 400 {
		snippet = snippet[:400]
	}

	var kind ErrKind
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		kind = ErrAuth
	case status == http.StatusNotFound:
		kind = ErrNotFound
	case status == http.StatusTooManyRequests:
		kind = ErrRateLim
	case status == http.StatusRequestTimeout || status == http.StatusGatewayTimeout:
		kind = ErrTimeout
	case status == http.StatusBadRequest && looksLikeContextOverflow(snippet):
		kind = ErrContext
	default:
		kind = ErrNetwork
	}
	return &Error{Kind: kind, Message: snippet, Status: status}
}

func looksLikeContextOverflow(s string) bool {
	l := strings.ToLower(s)
	return strings.Contains(l, "context length") ||
		strings.Contains(l, "context window") ||
		strings.Contains(l, "too many tokens") ||
		strings.Contains(l, "maximum context")
}

func wrapTransportError(err error) error {
	if err == nil {
		return nil
	}
	if errorsIsDeadline(err) {
		return &Error{Kind: ErrTimeout, Message: "request timed out", Err: err}
	}
	return &Error{Kind: ErrNetwork, Message: "transport failure", Err: err}
}

func errorsIsDeadline(err error) bool {
	type timeout interface{ Timeout() bool }
	if t, ok := err.(timeout); ok && t.Timeout() {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "deadline exceeded")
}
