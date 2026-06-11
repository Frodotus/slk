package mcp

// The socket protocol is newline-delimited JSON: one Request per line, one
// Response per line.

// Request is a single socket request.
type Request struct {
	Method string `json:"method"`         // "get_context" | "set_draft"
	Text   string `json:"text,omitempty"` // set_draft: the draft text
}

// DraftResult is the outcome of a set_draft.
type DraftResult struct {
	OK      bool   `json:"ok"`
	Target  string `json:"target,omitempty"`  // "thread" | "channel"
	Channel string `json:"channel,omitempty"` // channel name the draft landed in
	Reason  string `json:"reason,omitempty"`  // why it failed, when !OK
}

// Response is a single socket response.
type Response struct {
	OK       bool         `json:"ok"`
	Error    string       `json:"error,omitempty"`
	Snapshot *Snapshot    `json:"snapshot,omitempty"` // get_context
	Draft    *DraftResult `json:"draft,omitempty"`    // set_draft
}
