// Package urlutil holds URL normalization & dedup helpers shared by the
// crawler service and the local engine driver (feature 002 US3). Moved out
// of service/crawler so the engine package can import it without a cycle.
package urlutil

import (
	"errors"
	"net/url"
	"sort"
	"strings"
)

// trackingParams are advertising/share source parameters treated as noise when
// deciding whether two URLs address the same page (research.md §8). Content
// parameters like ?id=1&page=2 are preserved.
var trackingParams = map[string]bool{
	"utm_source": true, "utm_medium": true, "utm_campaign": true,
	"utm_term": true, "utm_content": true,
	"from": true, "source": true, "spm": true,
	"share_token": true, "scene": true, "isappinstalled": true,
}

// errUnsupportedScheme marks URLs that are not http/https.
var errUnsupportedScheme = errors.New("unsupported scheme: only http/https")

// nonWebExtensions are file links that are recorded but never fetched (FR-022).
var nonWebExtensions = map[string]bool{
	".pdf": true, ".jpg": true, ".jpeg": true, ".png": true, ".gif": true,
	".webp": true, ".svg": true, ".mp4": true, ".avi": true, ".mov": true,
	".zip": true, ".rar": true, ".7z": true, ".gz": true,
	".doc": true, ".docx": true, ".xls": true, ".xlsx": true,
	".ppt": true, ".pptx": true,
}

// normalizeHost lowers the host and strips the www. prefix for equivalence
// (A2/FR-011: www and bare host are the same site).
func normalizeHost(host string) string {
	h := strings.ToLower(host)
	return strings.TrimPrefix(h, "www.")
}

// SplitTokens splits a comma/space/; list into trimmed lowercased tokens
// (no host semantics — used for include_url URL-substring lists).
func SplitTokens(raw string) []string {
	var out []string
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' || r == ';' || r == '\n' || r == '\t' }) {
		p := strings.ToLower(strings.TrimSpace(part))
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// NormalizeHostName exports host normalization for the engine driver.
func NormalizeHostName(host string) string { return normalizeHost(host) }

// SplitHosts parses a comma/space-separated list of tokens into normalized
// host names (drop empties, strip scheme/port for host-like entries). Also
// reused for include_url substring lists where normalization is a no-op apart
// from lowercasing.
func SplitHosts(raw string) []string {
	var out []string
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' || r == ';' || r == '\n' || r == '	' }) {
		h := strings.ToLower(strings.TrimSpace(part))
		if h == "" {
			continue
		}
		if u, err := url.Parse("//" + h); err == nil && u.Host != "" {
			h = u.Host
		}
		out = append(out, normalizeHost(h))
	}
	return out
}

// normalizeURL canonicalizes a URL for dedup (FR-010): lowercase scheme/host,
// strips www prefix, drops fragment and trailing slash, drops tracking
// parameters, sorts remaining query keys. Only http/https is accepted.
func normalizeURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errUnsupportedScheme
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = normalizeHost(u.Host)

	// drop fragment; path: strip trailing slash except root
	u.Fragment = ""
	if u.Path != "/" && strings.HasSuffix(u.Path, "/") {
		u.Path = strings.TrimSuffix(u.Path, "/")
	}
	if u.Path == "" {
		u.Path = "/"
	}

	// filter tracking params, sort remaining
	q := u.Query()
	for k := range q {
		if trackingParams[strings.ToLower(k)] {
			q.Del(k)
		}
	}
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		for _, v := range q[k] {
			parts = append(parts, k+"="+v)
		}
	}
	u.RawQuery = strings.Join(parts, "&")

	return u.String(), nil
}

// URLKey returns the dedup key of a URL: sha256 of its canonical form.
func URLKey(raw string) (string, error) {
	norm, err := normalizeURL(raw)
	if err != nil {
		return "", err
	}
	return Fingerprint(norm), nil
}

// SameHost reports whether linkHost belongs to the same site as entryHost
// (www-equivalent). Used for default subpage scope (FR-011).
func SameHost(entryHost, linkHost string) bool {
	return normalizeHost(entryHost) == normalizeHost(linkHost)
}

// SubdomainAllowed reports whether linkHost is within entryHost's domain,
// including subdomains (used when include_subdomain is enabled, FR-016).
func SubdomainAllowed(entryHost, linkHost string) bool {
	e := normalizeHost(entryHost)
	l := normalizeHost(linkHost)
	return l == e || strings.HasSuffix(l, "."+e)
}

// IsNonWebURL reports whether the path ends with a non-web file extension
// that should be recorded but not fetched (FR-022).
func IsNonWebURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	lower := strings.ToLower(u.Path)
	idx := strings.LastIndexByte(lower, '.')
	if idx < 0 {
		return false
	}
	return nonWebExtensions[lower[idx:]]
}
