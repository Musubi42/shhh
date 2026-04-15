package cmdaudit

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestServeReportBasic(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "index.html"), "hello audit")

	url, stop, err := ServeReport(dir)
	if err != nil {
		t.Fatalf("ServeReport: %v", err)
	}
	if stop == nil {
		t.Fatal("stop func is nil")
	}
	if !strings.HasPrefix(url, "http://127.0.0.1:") {
		t.Errorf("url = %q, want http://127.0.0.1: prefix", url)
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		stop()
		t.Fatalf("GET %s: %v", url, err)
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		stop()
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "hello audit") {
		t.Errorf("body = %q, want contains 'hello audit'", string(body))
	}

	stop()

	// After stop, a subsequent request should fail or not return 200
	// within the short timeout window.
	shortClient := &http.Client{Timeout: 500 * time.Millisecond}
	resp2, err2 := shortClient.Get(url)
	if err2 == nil {
		resp2.Body.Close()
		if resp2.StatusCode == 200 {
			t.Error("expected server to be stopped, but request succeeded with 200")
		}
	}
}

func TestServeReportSetsSecurityHeaders(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "index.html"), "x")

	url, stop, err := ServeReport(dir)
	if err != nil {
		t.Fatalf("ServeReport: %v", err)
	}
	defer stop()

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	checks := map[string]string{
		"Cache-Control":         "no-store",
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":       "no-referrer",
	}
	for h, want := range checks {
		if got := resp.Header.Get(h); got != want {
			t.Errorf("header %s = %q, want %q", h, got, want)
		}
	}
}

func TestServeReportLoopbackOnly(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "index.html"), "x")

	url, stop, err := ServeReport(dir)
	if err != nil {
		t.Fatalf("ServeReport: %v", err)
	}
	defer stop()

	// URL must be loopback.
	if !strings.HasPrefix(url, "http://127.0.0.1:") {
		t.Errorf("url = %q, want http://127.0.0.1: prefix (loopback only)", url)
	}
	if strings.Contains(url, "0.0.0.0") {
		t.Errorf("url = %q must not bind 0.0.0.0", url)
	}

	// Confirm it's actually reachable on loopback.
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("loopback GET: %v", err)
	}
	resp.Body.Close()
}

func TestServeReportNonexistentDir(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	url, stop, err := ServeReport(missing)
	if err == nil {
		if stop != nil {
			stop()
		}
		t.Fatalf("expected error for nonexistent dir, got url=%q", url)
	}
	if stop != nil {
		t.Error("stop should be nil on error")
	}

	// Also test non-directory (a regular file).
	f := filepath.Join(t.TempDir(), "a.txt")
	writeFile(t, f, "hi")
	_, stop2, err2 := ServeReport(f)
	if err2 == nil {
		if stop2 != nil {
			stop2()
		}
		t.Error("expected error when path is a file, not a directory")
	}
}

func TestServeReportStopIdempotent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "index.html"), "x")

	_, stop, err := ServeReport(dir)
	if err != nil {
		t.Fatalf("ServeReport: %v", err)
	}

	stop()
	// Second call must not panic or block.
	done := make(chan struct{})
	go func() {
		stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("second stop() blocked")
	}
}

func TestServeReportServesSubpaths(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "projects", "sub.html"), "sub content")

	url, stop, err := ServeReport(dir)
	if err != nil {
		t.Fatalf("ServeReport: %v", err)
	}
	defer stop()

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url + "projects/sub.html")
	if err != nil {
		t.Fatalf("GET sub: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "sub content") {
		t.Errorf("body = %q, want contains 'sub content'", string(body))
	}
}
