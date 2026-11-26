package downloader

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// wrongSHA returns a syntactically valid digest that never matches payload.
func wrongSHA(payload []byte) string {
	return sha256Hex(append(payload, []byte("tampered")...))
}

// rangeServer serves payload with Range support and records the last Range header.
func rangeServer(t *testing.T, payload []byte, lastRange *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if lastRange != nil {
			*lastRange = r.Header.Get("Range")
		}
		start := int64(0)
		if rg := r.Header.Get("Range"); rg != "" {
			if _, err := fmt.Sscanf(rg, "bytes=%d-", &start); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if start >= int64(len(payload)) {
				w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
				return
			}
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(payload)-1, len(payload)))
			w.Header().Set("Content-Length", fmt.Sprint(int64(len(payload))-start))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(payload[start:])
			return
		}
		w.Header().Set("Content-Length", fmt.Sprint(len(payload)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
}

func TestFetchFullDownloadVerifies(t *testing.T) {
	payload := bytes.Repeat([]byte("stemscribe-payload-"), 5000)
	srv := rangeServer(t, payload, nil)
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "model.bin")
	var progressCalls int
	res, err := (&Client{}).Fetch(context.Background(), Artifact{
		URL:      srv.URL + "/model.bin",
		DestPath: dest,
		Size:     int64(len(payload)),
		SHA256:   sha256Hex(payload),
	}, func(done, total, bps int64) { progressCalls++ })
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if !res.Verified {
		t.Error("verified = false, want true when sha256 matches")
	}
	if res.Bytes != int64(len(payload)) {
		t.Errorf("bytes = %d, want %d", res.Bytes, len(payload))
	}
	if res.Resumed {
		t.Error("resumed = true on a fresh download")
	}
	if progressCalls == 0 {
		t.Error("progress callback never invoked; UI would show a frozen bar")
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("downloaded content mismatch")
	}
	if _, statErr := os.Stat(dest + partSuffix); statErr == nil {
		t.Error(".part file survived a successful install")
	}
}

func TestFetchResumesFromPartial(t *testing.T) {
	payload := bytes.Repeat([]byte("0123456789abcdef"), 10000)
	half := len(payload) / 2
	var seenRange string
	srv := rangeServer(t, payload, &seenRange)
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "model.bin")
	if err := os.WriteFile(dest+partSuffix, payload[:half], 0o644); err != nil {
		t.Fatalf("seed partial: %v", err)
	}

	res, err := (&Client{}).Fetch(context.Background(), Artifact{
		URL:      srv.URL + "/model.bin",
		DestPath: dest,
		Size:     int64(len(payload)),
		SHA256:   sha256Hex(payload),
	}, nil)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if !res.Resumed {
		t.Error("resumed = false, want true when a .part prefix exists")
	}
	if want := fmt.Sprintf("bytes=%d-", half); seenRange != want {
		t.Errorf("Range header = %q, want %q", seenRange, want)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("resumed content mismatch")
	}
}

func TestFetchRestartsOnIgnoredRange(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), 4096)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "model.bin")
	if err := os.WriteFile(dest+partSuffix, bytes.Repeat([]byte("y"), 1024), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	res, err := (&Client{}).Fetch(context.Background(), Artifact{
		URL:      srv.URL,
		DestPath: dest,
		SHA256:   sha256Hex(payload),
	}, nil)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if res.Resumed {
		t.Error("resumed = true, but server sent a full body; appending would corrupt the file")
	}
	got, _ := os.ReadFile(dest)
	if !bytes.Equal(got, payload) {
		t.Fatalf("content corrupted: got %d bytes, want %d", len(got), len(payload))
	}
}

func TestSHAMismatchDropsPartial(t *testing.T) {
	payload := []byte("real-bytes")
	srv := rangeServer(t, payload, nil)
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "model.bin")
	_, err := (&Client{}).Fetch(context.Background(), Artifact{
		URL:      srv.URL,
		DestPath: dest,
		Size:     int64(len(payload)),
		SHA256:   wrongSHA(payload),
	}, nil)
	if err == nil {
		t.Fatal("expected sha mismatch error")
	}
	var derr *Error
	if !asError(err, &derr) || derr.Code != CodeSHAMismatch {
		t.Fatalf("error code = %v, want %s", err, CodeSHAMismatch)
	}
	if _, statErr := os.Stat(dest); statErr == nil {
		t.Error("destination must not exist after a failed verification")
	}
	if _, statErr := os.Stat(dest + partSuffix); statErr == nil {
		t.Error(".part must be removed on sha mismatch so a retry cannot resume bad bytes")
	}
}

func TestSizeMismatchKeepsPartial(t *testing.T) {
	noBackoff(t) // 3 retries with real backoff would add 17 s to the suite
	payload := []byte("short")
	srv := rangeServer(t, payload, nil)
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "model.bin")
	_, err := (&Client{}).Fetch(context.Background(), Artifact{
		URL:      srv.URL,
		DestPath: dest,
		Size:     9999,
	}, nil)
	if err == nil {
		t.Fatal("expected size mismatch error")
	}
	var derr *Error
	if !asError(err, &derr) || derr.Code != CodeSizeMismatch {
		t.Fatalf("error code = %v, want %s", err, CodeSizeMismatch)
	}
	if _, statErr := os.Stat(dest + partSuffix); statErr != nil {
		t.Error(".part must be kept on size mismatch: the transfer is resumable")
	}
}

func TestFetchMapsHTTP404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "model.bin")
	_, err := (&Client{}).Fetch(context.Background(), Artifact{URL: srv.URL, DestPath: dest}, nil)
	if err == nil {
		t.Fatal("expected 404 error")
	}
	var derr *Error
	if !asError(err, &derr) {
		t.Fatalf("not a *Error: %v", err)
	}
	if !strings.HasPrefix(derr.Code, CodeHTTP) {
		t.Errorf("code = %q, want prefix %q", derr.Code, CodeHTTP)
	}
	if derr.Hint == "" {
		t.Error("hint must not be empty: users need an actionable next step")
	}
}

func TestFetchRejectsUnsafeDests(t *testing.T) {
	srv := rangeServer(t, []byte("x"), nil)
	defer srv.Close()

	cases := []string{
		"relative/model.bin",
		// Build the traversal by hand: filepath.Join cleans ".." away, which would
		// mean the function under test never sees an escape attempt at all.
		t.TempDir() + string(filepath.Separator) + ".." + string(filepath.Separator) + "escape.bin",
		"",
	}
	for _, dest := range cases {
		_, err := (&Client{}).Fetch(context.Background(), Artifact{URL: srv.URL, DestPath: dest}, nil)
		if err == nil {
			t.Errorf("dest %q accepted, want rejection", dest)
		}
	}
}

func TestFetchRejectsNonHTTPScheme(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "x.bin")
	if _, err := (&Client{}).Fetch(context.Background(), Artifact{URL: "ftp://example.org/x", DestPath: dest}, nil); err == nil {
		t.Fatal("ftp scheme must be rejected")
	}
}

func TestFetchRejectsMalformedSHA(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "x.bin")
	for _, sum := range []string{"deadbeef", "not-a-digest"} {
		_, err := (&Client{}).Fetch(context.Background(), Artifact{
			URL: "https://example.org/x", DestPath: dest, SHA256: sum,
		}, nil)
		if err == nil {
			t.Errorf("sha %q accepted, want rejection", sum)
		}
	}
}

func TestFetchCanceledContext(t *testing.T) {
	payload := bytes.Repeat([]byte("z"), 1<<20)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	dest := filepath.Join(t.TempDir(), "x.bin")
	_, err := (&Client{}).Fetch(ctx, Artifact{URL: srv.URL, DestPath: dest}, nil)
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	var derr *Error
	if !asError(err, &derr) || derr.Code != CodeCanceled {
		t.Fatalf("code = %v, want %s", err, CodeCanceled)
	}
}

func TestErrorRetryable(t *testing.T) {
	for _, e := range []*Error{{Code: CodeNetwork}, {Code: CodeHTTP}} {
		if !e.Retryable() {
			t.Errorf("%s should be retryable", e.Code)
		}
	}
	for _, e := range []*Error{{Code: CodeSHAMismatch}, {Code: CodeDiskFull}, {Code: CodeBadRequest}, {Code: CodeCanceled}} {
		if e.Retryable() {
			t.Errorf("%s should not be retryable", e.Code)
		}
	}
}

func TestErrorUnwrapAndMessage(t *testing.T) {
	base := fmt.Errorf("boom")
	e := &Error{Code: CodeNetwork, Message: "request failed", Err: base}
	if e.Unwrap() != base {
		t.Error("Unwrap must expose the underlying error")
	}
	if !strings.Contains(e.Error(), CodeNetwork) || !strings.Contains(e.Error(), "boom") {
		t.Errorf("Error() = %q, want code and cause", e.Error())
	}
	plain := &Error{Code: CodeCanceled, Message: "canceled"}
	if strings.Contains(plain.Error(), "<nil>") {
		t.Errorf("Error() with nil cause = %q", plain.Error())
	}
}

func TestInstallOverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "target.bin")
	if err := os.WriteFile(dest, []byte("old"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	part := dest + partSuffix
	if err := os.WriteFile(part, []byte("new"), 0o644); err != nil {
		t.Fatalf("seed part: %v", err)
	}
	if err := install(part, dest); err != nil {
		t.Fatalf("install: %v", err)
	}
	got, _ := os.ReadFile(dest)
	if string(got) != "new" {
		t.Errorf("content = %q, want %q", got, "new")
	}
}

func TestIsDiskFull(t *testing.T) {
	if isDiskFull(nil) {
		t.Error("nil is not disk-full")
	}
	if !isDiskFull(fmt.Errorf("write: no space left on device")) {
		t.Error("ENOSPC should be detected")
	}
	if isDiskFull(fmt.Errorf("permission denied")) {
		t.Error("permission error is not disk-full")
	}
}

func TestStalePartialOversizeResets(t *testing.T) {
	payload := []byte("current")
	srv := rangeServer(t, payload, nil)
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "model.bin")
	if err := os.WriteFile(dest+partSuffix, bytes.Repeat([]byte("stale"), 100), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	res, err := (&Client{}).Fetch(context.Background(), Artifact{
		URL: srv.URL, DestPath: dest, Size: int64(len(payload)), SHA256: sha256Hex(payload),
	}, nil)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if res.Resumed {
		t.Error("oversized stale partial must be discarded, not resumed")
	}
	got, _ := os.ReadFile(dest)
	if !bytes.Equal(got, payload) {
		t.Errorf("content = %q, want %q", got, payload)
	}
}

func TestUserAgentDefault(t *testing.T) {
	c := &Client{}
	if strings.Contains(c.userAgent(), "Go-http-client") {
		t.Error("default user agent should identify the app, not the stdlib")
	}
	if c.httpClient() == nil {
		t.Error("httpClient must never be nil")
	}
}

func asError(err error, target **Error) bool {
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

// noBackoff keeps the retry tests fast; the production delays are 1s/4s/12s.
func noBackoff(t *testing.T) {
	t.Helper()
	old := fetchBackoff
	fetchBackoff = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	t.Cleanup(func() { fetchBackoff = old })
}

// A dropped HTTP/2 stream (the real failure: PROTOCOL_ERROR from GitHub at 50%)
// must resume, not report "transfer interrupted" and abandon a valid .part.
func TestFetchRetriesInterruptedTransfer(t *testing.T) {
	noBackoff(t)
	payload := bytes.Repeat([]byte("retry-payload-"), 20000)
	half := len(payload) / 2
	attempts := 0
	var secondRange string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		start := 0
		if rg := r.Header.Get("Range"); rg != "" {
			if _, err := fmt.Sscanf(rg, "bytes=%d-", &start); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			secondRange = rg
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(payload)-1, len(payload)))
			w.Header().Set("Content-Length", fmt.Sprint(len(payload)-start))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(payload[start:])
			return
		}
		w.Header().Set("Content-Length", fmt.Sprint(len(payload)))
		w.WriteHeader(http.StatusOK)
		if attempts == 1 {
			// Write less than Content-Length and return: the client sees an
			// unexpected EOF, exactly like a cut stream.
			_, _ = w.Write(payload[:half])
			return
		}
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "model.bin")
	res, err := (&Client{}).Fetch(context.Background(), Artifact{
		URL:      srv.URL + "/model.bin",
		DestPath: dest,
		Size:     int64(len(payload)),
		SHA256:   sha256Hex(payload),
	}, nil)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2 (one interrupted + one resumed)", attempts)
	}
	if secondRange != fmt.Sprintf("bytes=%d-", half) {
		t.Errorf("resume Range = %q, want bytes=%d-", secondRange, half)
	}
	if !res.Resumed || !res.Verified {
		t.Errorf("resumed=%v verified=%v, want true/true", res.Resumed, res.Verified)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Error("final file does not match the payload")
	}
}

// Retrying a 404 would only waste the user's time; the classification must hold.
func TestFetchDoesNotRetryNotFound(t *testing.T) {
	noBackoff(t)
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		http.NotFound(w, r)
	}))
	defer srv.Close()

	_, err := (&Client{}).Fetch(context.Background(), Artifact{
		URL:      srv.URL + "/missing.zip",
		DestPath: filepath.Join(t.TempDir(), "missing.zip"),
	}, nil)
	if err == nil {
		t.Fatal("expected an error for 404")
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1: a 404 is not retryable", attempts)
	}
}
