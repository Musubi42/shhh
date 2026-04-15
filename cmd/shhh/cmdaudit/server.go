// Package cmdaudit implements the `shhh audit` subcommand, including
// the local HTTP server that serves generated audit reports.
package cmdaudit

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"time"
)

// ServeReport starts an HTTP file server serving the given directory
// on 127.0.0.1 with an OS-assigned port. Returns the URL the report
// is reachable at, plus a Stop function the caller must call to
// shut the server down cleanly.
//
// The server is bound to loopback only (127.0.0.1), never 0.0.0.0,
// so it is not reachable from the network.
//
// Port selection uses net.Listen("tcp", "127.0.0.1:0") which asks
// the OS for any free ephemeral port. This makes collisions with
// existing services impossible.
//
// The server respects a graceful shutdown timeout: Stop() will give
// in-flight requests up to 3 seconds to finish before forcibly
// closing them.
//
// Usage pattern:
//
//	url, stop, err := ServeReport(dir)
//	if err != nil { return err }
//	defer stop()
//	fmt.Println("Report at", url)
//	waitForInterrupt()  // caller blocks here
func ServeReport(dir string) (url string, stop func(), err error) {
	info, statErr := os.Stat(dir)
	if statErr != nil {
		return "", nil, fmt.Errorf("serve report: %w", statErr)
	}
	if !info.IsDir() {
		return "", nil, fmt.Errorf("serve report: %q is not a directory", dir)
	}

	listener, lerr := net.Listen("tcp", "127.0.0.1:0")
	if lerr != nil {
		return "", nil, fmt.Errorf("serve report: bind loopback: %w", lerr)
	}

	tcpAddr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		_ = listener.Close()
		return "", nil, fmt.Errorf("serve report: unexpected listener address type %T", listener.Addr())
	}
	port := tcpAddr.Port
	url = fmt.Sprintf("http://127.0.0.1:%d/", port)

	srv := &http.Server{
		Handler:           secureFileServer(dir),
		ReadHeaderTimeout: 5 * time.Second,
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = srv.Serve(listener)
	}()

	var once sync.Once
	stop = func() {
		once.Do(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = srv.Shutdown(ctx)
			<-done
		})
	}

	return url, stop, nil
}

// secureFileServer wraps http.FileServer with a small middleware that
// sets conservative security headers on every response. The audit
// report contains placeholders rather than raw secrets, but still
// includes metadata (project names, paths) that should not be cached
// by shared proxies or sniffed as a different content type.
func secureFileServer(dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		fs.ServeHTTP(w, r)
	})
}
