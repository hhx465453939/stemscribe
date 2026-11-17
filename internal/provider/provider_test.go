package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMergeCapsManualWins(t *testing.T) {
	registry := &Caps{ContextWindow: 131072, Vision: true}
	manual := &Caps{ContextWindow: 1048576} // user says: this model is 1M

	got, src := MergeCaps(registry, nil, manual)
	if src != CapsManual {
		t.Errorf("source = %q, want manual", src)
	}
	if got.ContextWindow != 1048576 {
		t.Errorf("context = %d, want the manual 1M value", got.ContextWindow)
	}
	if !got.Vision {
		t.Error("vision from registry should survive a partial manual override")
	}
}

func TestMergeCapsRegistryBeatsProbe(t *testing.T) {
	got, src := MergeCaps(&Caps{ContextWindow: 200000}, &Caps{ContextWindow: 8192}, nil)
	if src != CapsRegistry {
		t.Errorf("source = %q, want registry", src)
	}
	if got.ContextWindow != 200000 {
		t.Errorf("context = %d, want 200000", got.ContextWindow)
	}
}

func TestMergeCapsProbeThenDefaults(t *testing.T) {
	got, src := MergeCaps(nil, &Caps{ContextWindow: 65536}, nil)
	if src != CapsProbe {
		t.Errorf("source = %q, want probe", src)
	}
	if got.ContextWindow != 65536 {
		t.Errorf("context = %d, want 65536", got.ContextWindow)
	}

	got, _ = MergeCaps(nil, nil, nil)
	if got.ContextWindow != ConservativeDefaults().ContextWindow {
		t.Errorf("context = %d, want conservative default", got.ContextWindow)
	}
}

func TestMergeCapsSanitizes(t *testing.T) {
	got, _ := MergeCaps(nil, nil, &Caps{ContextWindow: 1, MaxOutputTokens: 1})
	if got.ContextWindow < 512 {
		t.Errorf("context = %d, must be repaired from a nonsense value", got.ContextWindow)
	}
	if got.MaxOutputTokens < 16 {
		t.Errorf("max output = %d, must be repaired", got.MaxOutputTokens)
	}
}

func TestDefaultParams(t *testing.T) {
	p := DefaultParams(ConservativeDefaults())
	if p.Temperature > 0.5 {
		t.Errorf("temperature = %v, polish must not be creative", p.Temperature)
	}
	if !p.Stream {
		t.Error("streaming should default on")
	}
}

func TestFormatAnchor(t *testing.T) {
	cases := map[int]string{
		0:       "00:00:00",
		1000:    "00:00:01",
		61234:   "00:01:01",
		3600000: "01:00:00",
		7325000: "02:02:05",
		-5:      "00:00:00",
	}
	for ms, want := range cases {
		if got := FormatAnchor(ms); got != want {
			t.Errorf("FormatAnchor(%d) = %q, want %q", ms, got, want)
		}
	}
}

func TestEstimateTokensCJK(t *testing.T) {
	// Compare equal character counts: the production estimate charges ~1 token
	// per 1.8 CJK chars vs 1 per 3.5 Latin chars, so equal-length CJK text must
	// cost more. (The earlier version compared strings of different lengths.)
	latin := EstimateTokens("abcdefghijklmnopqrst")
	cjk := EstimateTokens("这是一个用于测试的中文文本内容示例")
	if cjk <= latin {
		t.Errorf("cjk estimate %d should exceed latin %d for equal length", cjk, latin)
	}
}

func TestSplitKeepsWholeSegments(t *testing.T) {
	segs := []Segment{
		{StartMS: 0, Text: strings.Repeat("a", 100)},
		{StartMS: 1000, Text: strings.Repeat("b", 100)},
		{StartMS: 2000, Text: strings.Repeat("c", 100)},
	}
	chunks, err := SplitByAnchors(segs, 40)
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	total := 0
	for _, c := range chunks {
		total += len(c.Segments)
		for _, s := range c.Segments {
			if s.Text == "" {
				t.Error("chunk contains an empty segment")
			}
		}
	}
	if total != len(segs) {
		t.Errorf("segments lost during chunking: %d of %d", total, len(segs))
	}
}

func TestSplitEmitsOversizedAlone(t *testing.T) {
	segs := []Segment{
		{StartMS: 0, Text: "short"},
		{StartMS: 1000, Text: strings.Repeat("x", 5000)},
		{StartMS: 2000, Text: "tail"},
	}
	chunks, err := SplitByAnchors(segs, 50)
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	found := false
	for _, c := range chunks {
		if len(c.Segments) == 1 && c.Segments[0].StartMS == 1000 {
			found = true
		}
	}
	if !found {
		t.Error("oversized segment must be emitted alone, never truncated")
	}
}

func TestSplitRejectsBadBudget(t *testing.T) {
	if _, err := SplitByAnchors(nil, 0); err == nil {
		t.Fatal("zero budget must be rejected")
	}
}

func TestAnchorsRoundTrip(t *testing.T) {
	segs := []Segment{
		{StartMS: 3000, Text: "他说这个方案的边界在这里"},
		{StartMS: 7000, Text: "然后我们再看反例"},
	}
	rendered := RenderSegments(segs)
	got := ExtractAnchors(rendered)
	if len(got) != 2 || got[0] != "00:00:03" || got[1] != "00:00:07" {
		t.Fatalf("anchors = %v", got)
	}
	if missing := VerifyAnchors(got, got); len(missing) != 0 {
		t.Errorf("identical anchor sets reported %d missing", len(missing))
	}
}

func TestVerifyAnchorsDetectsLoss(t *testing.T) {
	in := []string{"00:00:03", "00:00:07", "00:01:12"}
	out := []string{"00:00:03", "00:01:12"}
	missing := VerifyAnchors(in, out)
	if len(missing) != 1 || missing[0] != "00:00:07" {
		t.Fatalf("missing = %v, want the dropped anchor", missing)
	}
}

func TestExtractAnchorsIgnoresJunk(t *testing.T) {
	text := "[00:00:03] ok\n[bad] nope\n[brief] 没有时间码\n00:00:09 没有括号\n[99:99:99] 非法\n"
	got := ExtractAnchors(text)
	if len(got) != 1 || got[0] != "00:00:03" {
		t.Fatalf("anchors = %v, want only the valid one", got)
	}
}

// --- transport tests (AC-11) ---

func TestNewRejectsBadConfig(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Error("empty base_url must be rejected")
	}
	if _, err := New(Config{BaseURL: "ftp://x"}); err == nil {
		t.Error("non-http scheme must be rejected")
	}
	if _, err := New(Config{BaseURL: "https://x", Kind: "grpc"}); err == nil {
		t.Error("unknown kind must be rejected")
	}
}

func TestListModelsEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("auth header = %q", got)
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"qwen2.5:14b"},{"id":"qwen2.5:14b"},{"id":"llama3.1:8b"}]}`))
	}))
	defer srv.Close()

	c, err := New(Config{BaseURL: srv.URL + "/v1", APIKey: "test-key"})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	ids, err := c.ListModels(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("ids = %v, want 2 unique entries", ids)
	}
	if ids[0] != "qwen2.5:14b" || ids[1] != "llama3.1:8b" {
		t.Errorf("ids = %v", ids)
	}
}

func TestListModelsBareArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"id":"a"},{"id":"b"}]`))
	}))
	defer srv.Close()

	c, _ := New(Config{BaseURL: srv.URL})
	ids, err := c.ListModels(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("ids = %v", ids)
	}
}

func TestListModelsAuthFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid api key"}`))
	}))
	defer srv.Close()

	c, _ := New(Config{BaseURL: srv.URL})
	_, err := c.ListModels(context.Background())
	if err == nil {
		t.Fatal("expected auth error")
	}
	var perr *Error
	if !asProviderError(err, &perr) || perr.Kind != ErrAuth {
		t.Fatalf("kind = %v, want %s", err, ErrAuth)
	}
}

func TestChatStreamsAndForwards(t *testing.T) {
	var gotBody struct {
		Model       string  `json:"model"`
		Temperature float64 `json:"temperature"`
		MaxTokens   int     `json:"max_tokens"`
		Stream      bool    `json:"stream"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		frames := []string{
			`data: {"choices":[{"delta":{"content":"[00:00:03] 他说"}}]}`,
			`data: {"choices":[{"delta":{"content":"这个方案的边界"}}]}`,
			`data: {"choices":[{"delta":{"content":"在这里。\n[00:00:07] 然后看反例。"}}]}`,
			`data: [DONE]`,
		}
		for _, f := range frames {
			_, _ = fmt.Fprintf(w, "%s\n\n", f)
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer srv.Close()

	c, _ := New(Config{BaseURL: srv.URL + "/v1", APIKey: "k"})
	var deltas int
	res, err := c.Chat(context.Background(), "mock-model", []Message{{Role: "user", Content: "go"}},
		Params{Temperature: 0.3, MaxTokens: 4096, Stream: true},
		func(string) { deltas++ })
	if err != nil {
		t.Fatalf("chat: %v", err)
	}

	if gotBody.Model != "mock-model" {
		t.Errorf("model = %q", gotBody.Model)
	}
	if gotBody.Temperature != 0.3 {
		t.Errorf("temperature = %v, want 0.3 forwarded", gotBody.Temperature)
	}
	if gotBody.MaxTokens != 4096 {
		t.Errorf("max_tokens = %d, want 4096 forwarded", gotBody.MaxTokens)
	}
	if !gotBody.Stream {
		t.Error("stream flag not forwarded")
	}
	if !res.Streamed {
		t.Error("result should be marked streamed")
	}
	if deltas != 3 {
		t.Errorf("deltas = %d, want 3", deltas)
	}

	anchors := ExtractAnchors(res.Text)
	if len(anchors) != 2 {
		t.Fatalf("anchors = %v, want 2", anchors)
	}
	if missing := VerifyAnchors([]string{"00:00:03", "00:00:07"}, anchors); len(missing) != 0 {
		t.Errorf("anchor conservation failed: %v", missing)
	}
}

func TestChatNonStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"[00:00:01] done"}}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`))
	}))
	defer srv.Close()

	c, _ := New(Config{BaseURL: srv.URL + "/v1"})
	res, err := c.Chat(context.Background(), "m", []Message{{Role: "user", Content: "x"}}, Params{Stream: false}, nil)
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if res.Streamed {
		t.Error("non-stream request reported as streamed")
	}
	if res.Text != "[00:00:01] done" {
		t.Errorf("text = %q", res.Text)
	}
	if res.Usage.CompletionTokens != 5 {
		t.Errorf("usage = %+v", res.Usage)
	}
}

func TestChatMapsContextOverflow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"This model's maximum context length is 8192 tokens"}}`))
	}))
	defer srv.Close()

	c, _ := New(Config{BaseURL: srv.URL + "/v1"})
	_, err := c.Chat(context.Background(), "m", nil, Params{Stream: false}, nil)
	var perr *Error
	if !asProviderError(err, &perr) || perr.Kind != ErrContext {
		t.Fatalf("kind = %v, want %s", err, ErrContext)
	}
	if perr.Retryable() {
		t.Error("context overflow must not be auto-retried: the caller has to chunk instead")
	}
}

func TestChatRejectsEmptyModel(t *testing.T) {
	c, _ := New(Config{BaseURL: "https://example.org"})
	if _, err := c.Chat(context.Background(), "", nil, Params{}, nil); err == nil {
		t.Fatal("empty model must be rejected")
	}
}

func TestExtraHeadersCannotShadowAuth(t *testing.T) {
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	c, _ := New(Config{
		BaseURL: srv.URL + "/v1",
		APIKey:  "real-key",
		ExtraHeaders: map[string]string{
			"Authorization": "Bearer attacker-override",
			"X-Custom":      "ok",
		},
	})
	if _, err := c.ListModels(context.Background()); err != nil {
		t.Fatalf("list: %v", err)
	}
	if seen != "Bearer real-key" {
		t.Errorf("authorization header = %q, extra header must not shadow it", seen)
	}
}

func asProviderError(err error, target **Error) bool {
	for err != nil {
		if e, ok := err.(*Error); ok {
			*target = e
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
