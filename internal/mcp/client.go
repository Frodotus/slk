package mcp

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"time"
)

// Client talks to a running slk's MCP socket. Each call opens a short-lived
// connection (the protocol is one request/response per connection-or-line).
type Client struct {
	path string
}

// NewClient returns a Client for the socket at path.
func NewClient(path string) *Client { return &Client{path: path} }

func (c *Client) call(req Request) (Response, error) {
	conn, err := net.DialTimeout("unix", c.path, 2*time.Second)
	if err != nil {
		return Response{}, errors.New("slk is not running, or its MCP server is disabled (set [mcp] enabled = true and restart slk)")
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	out, _ := json.Marshal(req)
	out = append(out, '\n')
	if _, err := conn.Write(out); err != nil {
		return Response{}, err
	}
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	if !sc.Scan() {
		return Response{}, errors.New("no response from slk")
	}
	var resp Response
	if err := json.Unmarshal(sc.Bytes(), &resp); err != nil {
		return Response{}, err
	}
	return resp, nil
}

// Context fetches the current focus from the running slk.
func (c *Client) Context() (Snapshot, error) {
	resp, err := c.call(Request{Method: "get_context"})
	if err != nil {
		return Snapshot{}, err
	}
	if !resp.OK {
		return Snapshot{}, errors.New(resp.Error)
	}
	if resp.Snapshot == nil {
		return Snapshot{}, nil
	}
	return *resp.Snapshot, nil
}

// SetDraft asks the running slk to put text into its active composer.
func (c *Client) SetDraft(text string) (DraftResult, error) {
	resp, err := c.call(Request{Method: "set_draft", Text: text})
	if err != nil {
		return DraftResult{}, err
	}
	if !resp.OK {
		return DraftResult{}, errors.New(resp.Error)
	}
	if resp.Draft == nil {
		return DraftResult{}, nil
	}
	return *resp.Draft, nil
}
