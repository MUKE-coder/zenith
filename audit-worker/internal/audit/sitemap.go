package audit

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// MaxPages caps how many pages one audit renders.
//
// Every page costs a real browser render, so an unbounded crawl of a large
// site would run for hours and pin the worker against one client. The cap is
// stated in the report rather than hidden, so a truncated audit does not read
// as a complete one.
const MaxPages = 50

// maxSitemapBytes caps a sitemap download. A malformed or hostile sitemap must
// not be able to exhaust memory.
const maxSitemapBytes = 10 << 20 // 10MB

// maxSitemapIndexes caps how many child sitemaps an index is followed into.
const maxSitemapIndexes = 20

// sitemapTimeout caps fetching one sitemap.
const sitemapTimeout = 20 * time.Second

// urlset is a sitemap.
type urlset struct {
	XMLName xml.Name `xml:"urlset"`
	URLs    []struct {
		Loc string `xml:"loc"`
	} `xml:"url"`
}

// sitemapIndex is a sitemap of sitemaps.
type sitemapIndex struct {
	XMLName  xml.Name `xml:"sitemapindex"`
	Sitemaps []struct {
		Loc string `xml:"loc"`
	} `xml:"sitemap"`
}

// ErrNoSitemap means the site has no readable sitemap.
type ErrNoSitemap struct {
	URL    string
	Reason string
}

func (e *ErrNoSitemap) Error() string {
	// Written for the developer reading it in the console, not for a log.
	return fmt.Sprintf("Couldn't read the sitemap at %s. %s", e.URL, e.Reason)
}

// FetchSitemap returns the pages listed in a site's sitemap.
//
// It follows a sitemap index one level, which is how large sites are usually
// structured, and caps the result at MaxPages.
//
// https is tried first and http second. Not every site a developer manages has
// a certificate -- an internal tool, a legacy client, a local test -- and
// refusing to audit those would be a purity the product cannot afford. The
// https attempt going first means a site that has it is never audited over
// plaintext.
func FetchSitemap(ctx context.Context, client *http.Client, domain string) ([]string, error) {
	var firstErr error

	for _, scheme := range []string{"https", "http"} {
		root := fmt.Sprintf("%s://%s/sitemap.xml", scheme, domain)

		pages, err := fetchSitemapAt(ctx, client, root, 0)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if len(pages) == 0 {
			if firstErr == nil {
				firstErr = &ErrNoSitemap{URL: root, Reason: "It lists no pages."}
			}
			continue
		}
		return pages, nil
	}

	return nil, firstErr
}

func fetchSitemapAt(ctx context.Context, client *http.Client, sitemapURL string, depth int) ([]string, error) {
	// One level of indirection: an index of indexes is either a mistake or a
	// loop, and following it forever is not a feature.
	if depth > 1 {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(ctx, sitemapTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sitemapURL, nil)
	if err != nil {
		return nil, &ErrNoSitemap{URL: sitemapURL, Reason: "That is not a valid URL."}
	}
	req.Header.Set("User-Agent", UserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, &ErrNoSitemap{URL: sitemapURL, Reason: "The site could not be reached."}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, &ErrNoSitemap{
			URL:    sitemapURL,
			Reason: "There is no sitemap there. Add one, or check the domain is right.",
		}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ErrNoSitemap{
			URL:    sitemapURL,
			Reason: fmt.Sprintf("The site returned %d.", resp.StatusCode),
		}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSitemapBytes))
	if err != nil {
		return nil, &ErrNoSitemap{URL: sitemapURL, Reason: "The sitemap could not be read."}
	}

	// An index first: its <sitemap> elements would otherwise parse as an empty
	// urlset and look like a sitemap listing no pages.
	var index sitemapIndex
	if err := xml.Unmarshal(body, &index); err == nil && len(index.Sitemaps) > 0 {
		var pages []string

		for i, child := range index.Sitemaps {
			if i >= maxSitemapIndexes || len(pages) >= MaxPages {
				break
			}
			childPages, err := fetchSitemapAt(ctx, client, strings.TrimSpace(child.Loc), depth+1)
			if err != nil {
				// One bad child sitemap must not sink the whole audit.
				continue
			}
			pages = append(pages, childPages...)
		}
		return capPages(pages), nil
	}

	var set urlset
	if err := xml.Unmarshal(body, &set); err != nil {
		return nil, &ErrNoSitemap{URL: sitemapURL, Reason: "It is not valid XML."}
	}

	pages := make([]string, 0, len(set.URLs))
	for _, entry := range set.URLs {
		loc := strings.TrimSpace(entry.Loc)
		if isPageURL(loc) {
			pages = append(pages, loc)
		}
	}

	return capPages(pages), nil
}

func capPages(pages []string) []string {
	// Deduplicate: a sitemap index whose children overlap would otherwise
	// spend the page budget rendering the same URL twice.
	seen := make(map[string]bool, len(pages))
	out := make([]string, 0, len(pages))

	for _, page := range pages {
		if seen[page] {
			continue
		}
		seen[page] = true
		out = append(out, page)

		if len(out) >= MaxPages {
			break
		}
	}
	return out
}

func isPageURL(raw string) bool {
	if raw == "" {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}
