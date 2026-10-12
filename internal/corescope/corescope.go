// Package corescope is a client for a CoreScope instance's public API:
// listing repeater nodes and fetching each one's observed reach data (used
// by internal/calibration).
package corescope

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"sync"
)

// pageLimit is the page size used when paginating GET /api/nodes.
const pageLimit = 500

// Node mirrors one entry of CoreScope's GET /api/nodes response.
type Node struct {
	PublicKey     string   `json:"public_key"`
	Name          *string  `json:"name"`
	Role          string   `json:"role"`
	Lat           *float64 `json:"lat"`
	Lon           *float64 `json:"lon"`
	LastHeard     *string  `json:"last_heard"`
	FirstSeen     *string  `json:"first_seen"`
	AdvertCount   *int     `json:"advert_count"`
	RelayCount1h  *int     `json:"relay_count_1h"`
	RelayCount24h *int     `json:"relay_count_24h"`
	HashSize      *int     `json:"hash_size"`
	DefaultScope  *string  `json:"default_scope"`
}

type nodesResponse struct {
	Nodes   []Node `json:"nodes"`
	Total   int    `json:"total"`
	HasMore *bool  `json:"has_more"`
}

// ReachLink mirrors one entry of CoreScope's GET /api/nodes/:pubkey/reach
// "links" array — real observed relay-path evidence between two repeaters,
// not a prediction. Bottleneck is the weaker direction's observation
// count, which CoreScope itself already frames as "how sure are we this
// link is real and mutually usable" — used directly as a confidence weight
// by internal/calibration.
type ReachLink struct {
	Pubkey     string   `json:"pubkey"`
	Name       string   `json:"name"`
	Lat        *float64 `json:"lat"`
	Lon        *float64 `json:"lon"`
	Bottleneck int      `json:"bottleneck"`
	Bidir      bool     `json:"bidir"`
	WeHear     int      `json:"we_hear"`
	TheyHear   int      `json:"they_hear"`
}

type reachResponse struct {
	Links []ReachLink `json:"links"`
}

// Client talks to one CoreScope instance.
type Client struct {
	BaseURL string
	HTTP    *http.Client
	Region  string
}

// NewClient returns a Client for baseURL. A nil httpClient uses
// http.DefaultClient.
func NewClient(baseURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{BaseURL: baseURL, HTTP: httpClient}
}

// FetchRepeaters fetches every role=repeater node, paginating as needed.
func (c *Client) FetchRepeaters(ctx context.Context) ([]Node, error) {
	return c.fetchNodes(ctx, "repeater")
}

// CoreScope can filter a page after pagination. has_more describes the scanned
// page; advancing by returned rows would repeat or silently drop memberships.
func (c *Client) fetchNodes(ctx context.Context, role string) ([]Node, error) {
	var nodes []Node
	const maxNodes = 100000
	for offset := 0; offset < maxNodes; {
		q := url.Values{"limit": {strconv.Itoa(pageLimit)}, "offset": {strconv.Itoa(offset)}}
		if role != "" {
			q.Set("role", role)
		}
		var page nodesResponse
		if err := c.getJSON(ctx, c.scopedURL(c.BaseURL+"/api/nodes?"+q.Encode()), &page); err != nil {
			return nil, err
		}
		nodes = append(nodes, page.Nodes...)
		if page.HasMore != nil {
			if !*page.HasMore {
				return nodes, nil
			}
			offset += pageLimit
		} else {
			offset += len(page.Nodes)
			if len(page.Nodes) == 0 || offset >= page.Total {
				return nodes, nil
			}
		}
	}
	return nil, fmt.Errorf("corescope: node paging exceeded %d scanned rows", maxNodes)
}

func (c *Client) scopedURL(raw string) string {
	if c.Region == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	q.Set("region", c.Region)
	u.RawQuery = q.Encode()
	return u.String()
}

// FetchReach fetches one node's observed reach data for the given lookback
// window (days).
func (c *Client) FetchReach(ctx context.Context, pubkey string, days int) ([]ReachLink, error) {
	url := c.scopedURL(fmt.Sprintf("%s/api/nodes/%s/reach?days=%d", c.BaseURL, url.PathEscape(pubkey), days))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("corescope: building request for %s: %w", url, err)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var parsed reachResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	return parsed.Links, nil
}

// FetchAllReach fetches reach data for every node concurrently, keyed by
// public key. A node whose fetch fails is simply omitted — treated as "no
// evidence available" for calibration — rather than aborting the whole
// batch, consistent with this tool's graceful-degradation approach
// elsewhere. progress is called with (done, total) as each node resolves.
func FetchAllReach(ctx context.Context, c *Client, nodes []Node, days int, progress func(done, total int)) map[string][]ReachLink {
	results := make(map[string][]ReachLink, len(nodes))
	var mu sync.Mutex

	jobs := make(chan string, len(nodes))
	for _, n := range nodes {
		jobs <- n.PublicKey
	}
	close(jobs)

	var wg sync.WaitGroup
	const workers = 12
	done := 0
	total := len(nodes)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for pubkey := range jobs {
				links, err := c.FetchReach(ctx, pubkey, days)
				mu.Lock()
				if err != nil {
					log.Printf("corescope: reach fetch failed for %s: %v", pubkey, err)
				} else {
					results[pubkey] = links
				}
				done++
				if progress != nil {
					progress(done, total)
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return results
}
