package debug

import (
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof" // Register pprof handlers on DefaultServeMux
)

// StartPprof starts an HTTP server with pprof handlers on an ephemeral port.
// It returns the actual port the server is listening on.
// The server runs in a background goroutine and serves until the process exits.
func StartPprof() (int, error) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, fmt.Errorf("failed to listen for pprof: %w", err)
	}

	port := listener.Addr().(*net.TCPAddr).Port
	fmt.Printf("pprof server listening on http://localhost:%d/debug/pprof/\n", port)

	go func() {
		_ = http.Serve(listener, nil)
	}()

	return port, nil
}
