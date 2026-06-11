package slackclient

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// TestStarsList_FiltersAndPaginates covers the headline behaviors of the
// stars.list integration: only channel/im/group stars are surfaced
// (messages/files dropped), duplicates are collapsed, and pagination
// follows the classic page/pages paging across multiple pages.
func TestStarsList_FiltersAndPaginates(t *testing.T) {
	page1 := `{"ok":true,"items":[
		{"type":"channel","channel":"C1"},
		{"type":"message","channel":"C9","ts":"1.2"},
		{"type":"im","channel":"D1"},
		{"type":"file","file":"F1"}
	],"paging":{"page":1,"pages":2}}`
	page2 := `{"ok":true,"items":[
		{"type":"group","channel":"G1"},
		{"type":"channel","channel":"C1"}
	],"paging":{"page":2,"pages":2}}`

	var pagesSeen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		pagesSeen = append(pagesSeen, r.FormValue("page"))
		w.Header().Set("Content-Type", "application/json")
		if r.FormValue("page") == "2" {
			fmt.Fprint(w, page2)
		} else {
			fmt.Fprint(w, page1)
		}
	}))
	defer srv.Close()

	c := &Client{apiBaseURL: srv.URL + "/", token: "xoxc-test", cookie: "d=x"}
	ids, err := c.StarsList(context.Background())
	if err != nil {
		t.Fatalf("StarsList: %v", err)
	}

	// channel C1, im D1, group G1; message + file excluded; C1 deduped.
	want := []string{"C1", "D1", "G1"}
	if !reflect.DeepEqual(ids, want) {
		t.Errorf("StarsList = %v, want %v", ids, want)
	}
	if !reflect.DeepEqual(pagesSeen, []string{"1", "2"}) {
		t.Errorf("requested pages = %v, want [1 2]", pagesSeen)
	}
}

// TestStarsList_TerminatesWhenPageNotEchoed guards the pagination fix: a
// server that reports a single page but never echoes the page number must
// not loop forever — termination keys off the local page counter.
func TestStarsList_TerminatesWhenPageNotEchoed(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		// page omitted (decodes to 0) but pages == 1.
		fmt.Fprint(w, `{"ok":true,"items":[{"type":"channel","channel":"C1"}],"paging":{"pages":1}}`)
	}))
	defer srv.Close()

	c := &Client{apiBaseURL: srv.URL + "/", token: "xoxc-test", cookie: "d=x"}
	ids, err := c.StarsList(context.Background())
	if err != nil {
		t.Fatalf("StarsList: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected exactly 1 page fetch, got %d (pagination did not terminate)", calls)
	}
	if !reflect.DeepEqual(ids, []string{"C1"}) {
		t.Errorf("ids = %v, want [C1]", ids)
	}
}
