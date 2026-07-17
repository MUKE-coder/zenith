package audit

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// linkTimeout caps checking one link.
const linkTimeout = 10 * time.Second

// linkConcurrency is how many links are checked at once.
//
// Modest on purpose: these requests hit the client's own server, and an audit
// that reads as a burst of traffic -- or trips their rate limiter -- has made
// the site worse to measure it.
const linkConcurrency = 5

// maxLinksPerPage caps how many links one page's audit checks.
const maxLinksPerPage = 100

// BrokenLink is a link that did not resolve.
type BrokenLink struct {
	URL    string `json:"url"`
	Reason string `json:"reason"`
}

// LinkChecker checks links, remembering what it has already seen.
//
// The cache is the point: a nav bar's links appear on every page, and checking
// them once per page would multiply an audit's outbound requests by the number
// of pages for no new information.
type LinkChecker struct {
	client *http.Client

	mu   sync.Mutex
	seen map[string]*BrokenLink // nil value means the link is fine
}

// NewLinkChecker returns a LinkChecker.
func NewLinkChecker() *LinkChecker {
	return &LinkChecker{
		client: &http.Client{
			Timeout: linkTimeout,
			// Follow redirects, but not forever: a redirect loop is itself a
			// broken link.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
		seen: map[string]*BrokenLink{},
	}
}

// Check tests a page's links and returns the broken ones.
func (c *LinkChecker) Check(ctx context.Context, links []string) []BrokenLink {
	targets := c.pending(links)

	var (
		wg     sync.WaitGroup
		guard  = make(chan struct{}, linkConcurrency)
		mu     sync.Mutex
		broken []BrokenLink
	)

	for _, link := range targets {
		wg.Add(1)

		go func(link string) {
			defer wg.Done()

			select {
			case guard <- struct{}{}:
				defer func() { <-guard }()
			case <-ctx.Done():
				return
			}

			result := c.test(ctx, link)

			c.mu.Lock()
			c.seen[link] = result
			c.mu.Unlock()

			if result != nil {
				mu.Lock()
				broken = append(broken, *result)
				mu.Unlock()
			}
		}(link)
	}

	wg.Wait()

	// Links already known bad from an earlier page still belong in this
	// page's findings: they are broken here too.
	c.mu.Lock()
	for _, link := range links {
		if result, known := c.seen[link]; known && result != nil && !contains(broken, link) {
			broken = append(broken, *result)
		}
	}
	c.mu.Unlock()

	return broken
}

// pending returns the links not already checked, capped.
func (c *LinkChecker) pending(links []string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	out := make([]string, 0, len(links))
	for _, link := range links {
		if len(out) >= maxLinksPerPage {
			break
		}
		if _, known := c.seen[link]; known {
			continue
		}
		if !checkable(link) {
			continue
		}
		out = append(out, link)
	}
	return out
}

// test reports whether a link is broken, and why.
func (c *LinkChecker) test(ctx context.Context, link string) *BrokenLink {
	ctx, cancel := context.WithTimeout(ctx, linkTimeout)
	defer cancel()

	// HEAD first: it is the cheap question, and most servers answer it.
	status, err := c.request(ctx, http.MethodHead, link)

	// Plenty of servers reject HEAD outright (405) or mishandle it. A GET
	// settles whether the link is actually broken or merely HEAD-averse.
	if err != nil || status == http.StatusMethodNotAllowed || status >= 500 {
		status, err = c.request(ctx, http.MethodGet, link)
	}

	if err != nil {
		if ctx.Err() != nil {
			return &BrokenLink{URL: link, Reason: "timed out"}
		}
		return &BrokenLink{URL: link, Reason: "could not be reached"}
	}

	if status >= 400 {
		return &BrokenLink{URL: link, Reason: fmt.Sprintf("returned %d", status)}
	}
	return nil
}

func (c *LinkChecker) request(ctx context.Context, method, link string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, method, link, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", UserAgent)

	resp, err := c.client.Do(req)
	if err != nil {
		return 0, err
	}
	// The body is never read: only the status matters, and downloading a page
	// per link would make the audit heavier than the site.
	resp.Body.Close()

	return resp.StatusCode, nil
}

// checkable filters out links that are not fetchable HTTP.
func checkable(link string) bool {
	parsed, err := url.Parse(link)
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	// A fragment-only link points within the page; there is nothing to fetch.
	if parsed.Host == "" {
		return false
	}
	// mailto:, tel: and friends are already excluded by the scheme check.
	return !strings.HasPrefix(link, "#")
}

func contains(links []BrokenLink, url string) bool {
	for _, link := range links {
		if link.URL == url {
			return true
		}
	}
	return false
}
