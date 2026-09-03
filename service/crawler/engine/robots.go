package engine

import (
	"bufio"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// robotsEntry is the parsed robots.txt of one host (User-agent: * section).
type robotsEntry struct {
	disallow []string // path prefixes (longest match wins; empty entry = allow all)
	allow    []string
	fetched  time.Time
}

// robotsClient is the shared short-timeout HTTP client for robots.txt fetches.
var robotsClient = newRobotsHTTPClient()

// newRobotsHTTPClient builds the client used for robots.txt lookups.
func newRobotsHTTPClient() *http.Client {
	return &http.Client{Timeout: 4 * time.Second}
}

// robotsCache memoizes per-host robots.txt for a short TTL. Fail-open on
// robots fetch errors (infra outage must not block crawling; per-page errors
// still surface through page status).
type robotsCache struct {
	mu  sync.Mutex
	m   map[string]*robotsEntry
	ttl time.Duration
}

func newRobotsCache() *robotsCache {
	return &robotsCache{m: map[string]*robotsEntry{}, ttl: time.Hour}
}

// allowed reports whether fetching path on host is permitted by robots.txt.
// A nil/empty entry means "allow" (fail-open).
func (c *robotsCache) allowed(client *http.Client, u *url.URL) bool {
	c.mu.Lock()
	e, ok := c.m[u.Host]
	c.mu.Unlock()
	if !ok || time.Since(e.fetched) > c.ttl {
		e = fetchRobots(client, u)
		c.mu.Lock()
		c.m[u.Host] = e
		c.mu.Unlock()
	}
	if e == nil {
		return true // robots.txt absent/unreachable: fail-open
	}
	r := e.rule(u.Path)
	return r.prefix == "" || r.allow // no matching rule = allowed
}

// ruleResult is the longest matching robots rule (RFC 9309 approximation).
type ruleResult struct {
	prefix string
	allow  bool
}

// fetchRobots downloads and parses {scheme}://{host}/robots.txt.
// Absent/404/unreachable robots.txt => nil entry (allow).
func fetchRobots(client *http.Client, u *url.URL) *robotsEntry {
	ru := *u
	ru.Path = "/robots.txt"
	ru.RawQuery = ""
	req, err := http.NewRequest(http.MethodGet, ru.String(), nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	resp, err := robotsClient.Do(req)
	if err != nil {
		return &robotsEntry{fetched: time.Now()} // unreachable: allow + cache briefly
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &robotsEntry{fetched: time.Now()} // no robots.txt: allow + cache
	}
	e := &robotsEntry{fetched: time.Now()}
	inStar := false
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		field, value := strings.ToLower(strings.TrimSpace(parts[0])), strings.TrimSpace(parts[1])
		if field == "user-agent" {
			inStar = value == "*"
			continue
		}
		if field == "useragent" {
			inStar = value == "*"
			continue
		}
		if !inStar {
			continue
		}
		switch field {
		case "disallow":
			if value == "" {
				e.allow = append(e.allow, "/") // empty disallow = allow all
			} else {
				e.disallow = append(e.disallow, value)
			}
		case "allow":
			e.allow = append(e.allow, value)
		}
	}
	if len(e.disallow) == 0 && len(e.allow) == 0 {
		return nil
	}
	return e
}

// rule applies longest-match robots semantics (per RFC 9309 approximation):
// no matching rule = allowed; longest match wins; ties prefer Allow.
func (e *robotsEntry) rule(path string) ruleResult {
	if path == "" {
		path = "/"
	}
	best := ruleResult{prefix: ""}
	consider := func(r string, isAllow bool) {
		if !strings.HasPrefix(path, r) {
			return
		}
		if len(r) > len(best.prefix) || (len(r) == len(best.prefix) && isAllow && !best.allow) {
			best = ruleResult{prefix: r, allow: isAllow}
		}
	}
	for _, r := range e.disallow {
		consider(r, false)
	}
	for _, r := range e.allow {
		consider(r, true)
	}
	return best
}

// IsRobotsDisallowed is a test helper mirroring the public check.
func (c *robotsCache) IsRobotsDisallowed(client *http.Client, rawURL string) (bool, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false, fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	return !c.allowed(client, u), nil
}

// DefaultUserAgent exposes the fetch UA for tests.
const DefaultUserAgent = defaultUserAgent

const defaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36 SGScoutBot/0.1"
