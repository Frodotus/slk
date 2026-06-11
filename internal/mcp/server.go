package mcp

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
)

// Bridge is implemented by the running TUI to answer socket requests:
// return the current focus, and set a draft into the active composer.
type Bridge interface {
	Context() Snapshot
	SetDraft(text string) DraftResult
}

// Serve listens on a unix socket at path and answers requests using b. A
// stale socket file at path is removed first, and the socket is chmod'd to
// 0600 (user-only — the trust boundary). Returns the listener; Close it to
// stop serving. Accept runs in a background goroutine.
func Serve(path string, b Bridge) (net.Listener, error) {
	_ = os.Remove(path) // clear a stale socket from a previous run
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	_ = os.Chmod(path, 0o600)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener closed
			}
			go handleConn(conn, b)
		}
	}()
	return ln, nil
}

func handleConn(conn net.Conn, b Bridge) {
	defer conn.Close()
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	w := bufio.NewWriter(conn)
	for sc.Scan() {
		resp := dispatch(sc.Bytes(), b)
		out, _ := json.Marshal(resp)
		_, _ = w.Write(out)
		_ = w.WriteByte('\n')
		_ = w.Flush()
	}
}

// dispatch parses a request line and produces a response. Split out so it
// can be unit-tested without a socket.
func dispatch(line []byte, b Bridge) Response {
	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		return Response{OK: false, Error: "bad request: " + err.Error()}
	}
	switch req.Method {
	case "get_context":
		snap := b.Context()
		return Response{OK: true, Snapshot: &snap}
	case "set_draft":
		dr := b.SetDraft(req.Text)
		return Response{OK: true, Draft: &dr}
	default:
		return Response{OK: false, Error: "unknown method: " + req.Method}
	}
}
