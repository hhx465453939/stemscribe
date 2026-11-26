// Package downloader fetches registry artifacts with resume, checksum
// verification and atomic installation.
//
// ADR-003 mandates four properties, and each is enforced here rather than at the
// call site:
//  1. resume        — a partial .part file continues with a Range request
//  2. verify        — sha256 is computed over the whole file, streaming
//  3. atomic        — the final path only appears after verification passes
//  4. containment   — the destination must be an absolute path with no traversal
package downloader

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hhx465453939/stemscribe/internal/fsutil"
)

// Error codes are stable identifiers consumed by the UI (ARCHITECTURE.md §7).
const (
	CodeHTTP         = "DL_HTTP"
	CodeNetwork      = "DL_NET"
	CodeSizeMismatch = "DL_SIZE_MISMATCH"
	CodeSHAMismatch  = "DL_SHA_MISMATCH"
	CodeDiskFull     = "DL_DISK_FULL"
	CodeBadRequest   = "DL_BAD_REQUEST"
	CodeCanceled     = "DL_CANCELED"
)

// Error is a download failure with a stable code and a human hint.
type Error struct {
	Code    string
	Message string
	Hint    string
	Err     error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.Err }

// Retryable reports whether the UI should offer an automatic retry.
func (e *Error) Retryable() bool {
	switch e.Code {
	case CodeNetwork, CodeHTTP:
		return true
	default:
		return false
	}
}

// Progress is called at most every progressIntervalBytes to keep the event
// channel from flooding the UI (ARCHITECTURE.md §4: 5 Hz cap at the UI layer,
// this is the inner throttle).
type Progress func(done, total int64, bytesPerSec int64)

const (
	progressIntervalBytes = 512 * 1024
	partSuffix            = ".part"
)

// Artifact describes one file to fetch.
type Artifact struct {
	// URL is absolute and already mirror-resolved.
	URL string
	// DestPath is an absolute path inside the work directory.
	DestPath string
	// Size is the expected byte length; 0 disables the length check.
	Size int64
	// SHA256 is the expected lowercase hex digest. Empty means "unverified",
	// which is only acceptable when the registry marks the entry TBD.
	SHA256 string
}

// Result reports what actually happened, for the provenance record.
type Result struct {
	Path     string
	Bytes    int64
	SHA256   string
	Resumed  bool
	Verified bool
	TookTime time.Duration
}

// Client performs downloads. The zero value is usable.
type Client struct {
	HTTP      *http.Client
	UserAgent string
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 0} // per-request context governs cancellation
}

func (c *Client) userAgent() string {
	if c.UserAgent != "" {
		return c.UserAgent
	}
	return "Stemscribe/0.0 (+https://github.com/hhx465453939/stemscribe)"
}

// Fetch downloads a.URL to a.DestPath, resuming from a.DestPath+".part" if present.
//
// Network interruptions are retried with the backoff PIPELINE.md §6 promises.
// This is not cosmetic: the CUDA runtime is a 574 MB asset on a link that
// periodically drops an HTTP/2 stream, and a single PROTOCOL_ERROR used to end
// the install at 50% with a perfectly resumable .part sitting on disk.
func (c *Client) Fetch(ctx context.Context, a Artifact, prog Progress) (Result, error) {
	if err := validateArtifact(a); err != nil {
		return Result{}, err
	}
	var last error
	for attempt := 0; attempt <= len(fetchBackoff); attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return Result{}, &Error{Code: CodeCanceled, Message: "download canceled", Err: ctx.Err()}
			case <-time.After(fetchBackoff[attempt-1]):
			}
		}
		res, err := c.fetchOnce(ctx, a, prog)
		if err == nil {
			return res, nil
		}
		last = err
		if !retryableDownload(err) || ctx.Err() != nil {
			return Result{}, err
		}
	}
	return Result{}, last
}

// fetchBackoff is the delay before each retry: 1s, 4s, 12s (PIPELINE.md §6).
var fetchBackoff = []time.Duration{time.Second, 4 * time.Second, 12 * time.Second}

// retryableDownload decides whether another attempt can plausibly succeed.
// A 404/403 will not, and a sha mismatch means the bytes were wrong, not slow.
func retryableDownload(err error) bool {
	var de *Error
	if !errors.As(err, &de) {
		return false
	}
	switch de.Code {
	case CodeNetwork, CodeSizeMismatch:
		// Both keep the .part, so the next attempt resumes where this one died.
		return true
	}
	if strings.HasPrefix(de.Code, CodeHTTP+"_") {
		code, convErr := strconv.Atoi(strings.TrimPrefix(de.Code, CodeHTTP+"_"))
		if convErr != nil {
			return false
		}
		return code >= 500 || code == http.StatusRequestTimeout || code == http.StatusTooManyRequests
	}
	return false
}

func (c *Client) fetchOnce(ctx context.Context, a Artifact, prog Progress) (Result, error) {
	started := time.Now()

	partPath := a.DestPath + partSuffix
	if err := os.MkdirAll(filepath.Dir(a.DestPath), 0o755); err != nil {
		return Result{}, &Error{Code: CodeBadRequest, Message: "cannot create destination directory", Err: err}
	}

	var offset int64
	if info, statErr := os.Stat(partPath); statErr == nil {
		offset = info.Size()
		if a.Size > 0 && offset > a.Size {
			// Stale/corrupt partial: start clean rather than trusting it.
			offset = 0
			if rmErr := os.Remove(partPath); rmErr != nil {
				return Result{}, &Error{Code: CodeBadRequest, Message: "cannot reset partial file", Err: rmErr}
			}
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	if err != nil {
		return Result{}, &Error{Code: CodeBadRequest, Message: "build request", Err: err}
	}
	req.Header.Set("User-Agent", c.userAgent())
	req.Header.Set("Accept-Encoding", "identity") // checksums are over raw bytes
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}

	resp, err := c.httpClient().Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			return Result{}, &Error{Code: CodeCanceled, Message: "download canceled", Err: err}
		}
		return Result{}, &Error{Code: CodeNetwork, Message: "request failed", Hint: "检查网络或切换镜像源", Err: err}
	}
	defer func() { _ = resp.Body.Close() }()

	resumed := false
	switch resp.StatusCode {
	case http.StatusOK:
		// Server ignored Range: restart from zero to avoid appending a full body
		// onto an existing prefix, which would silently corrupt the file.
		offset = 0
	case http.StatusPartialContent:
		resumed = offset > 0
	case http.StatusRequestedRangeNotSatisfiable:
		// Our partial is at or past EOF; drop it and retry once from scratch.
		if rmErr := os.Remove(partPath); rmErr != nil {
			return Result{}, &Error{Code: CodeBadRequest, Message: "cannot clear partial", Err: rmErr}
		}
		return c.fetchOnce(ctx, a, prog)
	default:
		return Result{}, &Error{
			Code:    fmt.Sprintf("%s_%d", CodeHTTP, resp.StatusCode),
			Message: fmt.Sprintf("unexpected status %s", resp.Status),
			Hint:    hintForStatus(resp.StatusCode),
		}
	}

	flags := os.O_CREATE | os.O_WRONLY
	if resumed || offset > 0 {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(partPath, flags, 0o644)
	if err != nil {
		return Result{}, &Error{Code: CodeDiskFull, Message: "cannot open partial file for writing", Hint: "检查磁盘空间与目录权限", Err: err}
	}

	total := a.Size
	if total <= 0 {
		total = offset + resp.ContentLength
	}

	written, copyErr := copyWithProgress(f, resp.Body, offset, total, prog)
	closeErr := f.Close()
	if copyErr != nil {
		if isDiskFull(copyErr) {
			return Result{}, &Error{Code: CodeDiskFull, Message: "disk full while writing", Hint: "清理工作目录后重试", Err: copyErr}
		}
		if errors.Is(copyErr, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			return Result{}, &Error{Code: CodeCanceled, Message: "download canceled", Err: copyErr}
		}
		return Result{}, &Error{Code: CodeNetwork, Message: "transfer interrupted", Hint: "已保留 .part，可续传", Err: copyErr}
	}
	if closeErr != nil {
		return Result{}, &Error{Code: CodeBadRequest, Message: "flush partial file", Err: closeErr}
	}

	if a.Size > 0 && written != a.Size {
		// Keep the partial: a truncated transfer is resumable, a deleted one is not.
		return Result{}, &Error{
			Code:    CodeSizeMismatch,
			Message: fmt.Sprintf("size mismatch: got %d bytes, want %d", written, a.Size),
			Hint:    "已保留 .part，重试将续传",
		}
	}

	sum, err := hashFile(partPath)
	if err != nil {
		return Result{}, &Error{Code: CodeBadRequest, Message: "hash partial file", Err: err}
	}
	verified := false
	if a.SHA256 != "" {
		if !strings.EqualFold(sum, a.SHA256) {
			// Corrupt payload: the partial is worthless, remove it so a retry
			// cannot resume into the same bad bytes.
			_ = os.Remove(partPath)
			return Result{}, &Error{
				Code:    CodeSHAMismatch,
				Message: fmt.Sprintf("sha256 mismatch: got %s", sum),
				Hint:    "下载内容损坏或被篡改，已丢弃",
			}
		}
		verified = true
	}

	if err = install(partPath, a.DestPath); err != nil {
		return Result{}, &Error{Code: CodeBadRequest, Message: "install artifact", Err: err}
	}

	return Result{
		Path:     a.DestPath,
		Bytes:    written,
		SHA256:   sum,
		Resumed:  resumed,
		Verified: verified,
		TookTime: time.Since(started),
	}, nil
}

func validateArtifact(a Artifact) error {
	if a.URL == "" {
		return &Error{Code: CodeBadRequest, Message: "empty url"}
	}
	if !strings.HasPrefix(a.URL, "http://") && !strings.HasPrefix(a.URL, "https://") {
		return &Error{Code: CodeBadRequest, Message: "url must be http(s)"}
	}
	if a.DestPath == "" || !filepath.IsAbs(a.DestPath) {
		return &Error{Code: CodeBadRequest, Message: "destination must be absolute"}
	}
	// Check the raw path: filepath.Clean would resolve the ".." away and the
	// escape attempt would become undetectable (this was a real bug caught by
	// TestFetchRejectsUnsafeDests).
	if fsutil.HasDotDotSegment(a.DestPath) {
		return &Error{Code: CodeBadRequest, Message: "destination escapes work directory"}
	}
	if a.Size < 0 {
		return &Error{Code: CodeBadRequest, Message: "negative size"}
	}
	if a.SHA256 != "" {
		if len(a.SHA256) != 64 {
			return &Error{Code: CodeBadRequest, Message: "sha256 must be 64 hex chars"}
		}
		if _, err := hex.DecodeString(a.SHA256); err != nil {
			return &Error{Code: CodeBadRequest, Message: "sha256 is not valid hex"}
		}
	}
	return nil
}

func copyWithProgress(dst io.Writer, src io.Reader, offset, total int64, prog Progress) (int64, error) {
	buf := make([]byte, 256*1024)
	var written int64
	lastReport := offset
	startedAt := time.Now()

	for {
		n, readErr := src.Read(buf)
		if n > 0 {
			if _, writeErr := dst.Write(buf[:n]); writeErr != nil {
				return offset + written, writeErr
			}
			written += int64(n)
			if prog != nil && offset+written-lastReport >= progressIntervalBytes {
				lastReport = offset + written
				var bps int64
				if elapsed := time.Since(startedAt).Seconds(); elapsed > 0 {
					bps = int64(float64(written) / elapsed)
				}
				prog(offset+written, total, bps)
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				if prog != nil {
					var bps int64
					if elapsed := time.Since(startedAt).Seconds(); elapsed > 0 {
						bps = int64(float64(written) / elapsed)
					}
					prog(offset+written, total, bps)
				}
				return offset + written, nil
			}
			return offset + written, readErr
		}
	}
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	// Streaming: a model file can be >1 GB, never load it into memory.
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// install moves the verified partial into place. Same-directory rename keeps it
// atomic on NTFS.
func install(partPath, destPath string) error {
	if err := os.Rename(partPath, destPath); err == nil {
		return nil
	}
	// Fallback for the "target exists" case: remove and retry once.
	if _, statErr := os.Stat(destPath); statErr == nil {
		if rmErr := os.Remove(destPath); rmErr != nil {
			return rmErr
		}
		if err := os.Rename(partPath, destPath); err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("rename %s -> %s failed", partPath, destPath)
}

func isDiskFull(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no space") ||
		strings.Contains(msg, "disk full") ||
		strings.Contains(msg, "not enough space")
}

func hintForStatus(code int) string {
	switch code {
	case http.StatusNotFound:
		return "文件不存在：可能尚未发布或镜像未同步"
	case http.StatusForbidden:
		return "访问被拒绝：可能需要登录或链接已失效"
	case http.StatusTooManyRequests:
		return "请求过于频繁：稍后重试或切换镜像"
	default:
		return "服务端返回异常，请重试或切换镜像"
	}
}
