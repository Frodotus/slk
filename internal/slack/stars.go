package slackclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// starsListResponse is the subset of the stars.list response slk needs:
// the starred items and classic page-based paging.
type starsListResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
	Items []struct {
		Type    string `json:"type"`    // channel | im | group | message | file | ...
		Channel string `json:"channel"` // set for channel/im/group stars
	} `json:"items"`
	Paging struct {
		Page  int `json:"page"`
		Pages int `json:"pages"`
	} `json:"paging"`
}

// StarsList returns the conversation IDs the user has starred. Slack
// stores starred conversations in the legacy stars.list API, NOT in the
// channelSections "stars" section (whose membership arrives empty), so
// this is the canonical source for the sidebar's Starred section.
//
// Only channel/im/group stars are returned; starred messages and files
// are ignored. Paginates over the classic page/pages paging. This
// endpoint is undocumented for xoxc sessions; it may break if Slack
// changes the API.
func (c *Client) StarsList(ctx context.Context) ([]string, error) {
	var ids []string
	seen := map[string]bool{}
	for page := 1; ; page++ {
		body, err := c.callStarsList(ctx, page)
		if err != nil {
			return nil, err
		}
		var resp starsListResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("decoding stars.list: %w", err)
		}
		if !resp.OK {
			return nil, fmt.Errorf("stars.list: %s", resp.Error)
		}
		for _, it := range resp.Items {
			switch it.Type {
			case "channel", "im", "group":
				if it.Channel != "" && !seen[it.Channel] {
					seen[it.Channel] = true
					ids = append(ids, it.Channel)
				}
			}
		}
		// Terminate on the locally-incremented page, not resp.Paging.Page:
		// if Slack fails to echo the requested page (e.g. returns 0) while
		// reporting Pages > 0, keying off the response could loop forever.
		if resp.Paging.Pages == 0 || page >= resp.Paging.Pages {
			break
		}
	}
	return ids, nil
}

func (c *Client) callStarsList(ctx context.Context, page int) ([]byte, error) {
	endpoint := c.apiBaseURL + "stars.list"
	form := url.Values{
		"token": {c.token},
		"count": {"200"},
		"page":  {strconv.Itoa(page)},
	}
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating stars.list request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := newCookieHTTPClient(c.cookie).Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling stars.list API: %w", err)
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// wsStarEvent is the star_added / star_removed WS payload. slk only
// surfaces starred conversations in the sidebar, so channelID() returns
// "" for starred messages or files.
type wsStarEvent struct {
	Item struct {
		Type    string `json:"type"`
		Channel string `json:"channel"`
	} `json:"item"`
}

// channelID returns the starred conversation ID, or "" if the starred
// item is not a channel/im/group.
func (e wsStarEvent) channelID() string {
	switch e.Item.Type {
	case "channel", "im", "group":
		return e.Item.Channel
	}
	return ""
}
