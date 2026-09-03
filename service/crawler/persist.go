package crawler

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sg.scout/config"
	"sg.scout/model"
	"sg.scout/service/crawler/engine"
	"sg.scout/service/crawler/urlutil"
)

// storageRoot returns the artifact file root from config (default ./data).
func storageRoot() string {
	root := config.Cfg.Crawler.StorageRoot
	if root == "" {
		root = "./data"
	}
	return root
}

// pageDir returns <root>/crawler/{taskID}/{pageID}/ (markdown-file-contract.md).
func pageDir(taskID, pageID uint64) string {
	return filepath.Join(storageRoot(), "crawler", fmt.Sprintf("%d", taskID), fmt.Sprintf("%d", pageID))
}

// currentMDPath is the always-latest body markdown file (downstream entry).
func currentMDPath(dir string) string { return filepath.Join(dir, filepath.Base(dir)+".md") }

func historyDir(dir string) string     { return filepath.Join(dir, "history") }
func backupHTMLPath(dir string) string { return filepath.Join(dir, "backup.html") }

func versionPath(dir string, version int) string {
	return filepath.Join(historyDir(dir), fmt.Sprintf("%04d.md", version))
}

// frontMatter renders the stable YAML header (8 fields, FR-020).
func frontMatter(t *model.CrawlerTask, p *model.CrawlerPage, version int, fp, crawledAt string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "---\n")
	fmt.Fprintf(&b, "source_type: %s\n", t.SourceType)
	fmt.Fprintf(&b, "page_id: %d\n", p.ID)
	fmt.Fprintf(&b, "title: %s\n", yamlEscape(p.Title))
	fmt.Fprintf(&b, "url: %s\n", yamlEscape(p.URL))
	fmt.Fprintf(&b, "task_id: %d\n", t.ID)
	fmt.Fprintf(&b, "version: %d\n", version)
	fmt.Fprintf(&b, "fingerprint: %s\n", fp)
	fmt.Fprintf(&b, "crawled_at: %s\n", crawledAt)
	fmt.Fprintf(&b, "---\n\n")
	return b.String()
}

func yamlEscape(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	return strings.ReplaceAll(s, "\n", " ")
}

// stripFrontMatter removes the YAML header block from a markdown body file,
// returning the pure engine markdown (used for API content delivery).
func stripFrontMatter(content string) string {
	if !strings.HasPrefix(content, "---\n") {
		return content
	}
	if idx := strings.Index(content[4:], "\n---\n"); idx >= 0 {
		return content[4+idx+5:]
	}
	return content
}

// writeArtifacts persists md (current + history) and html backup for a page
// version. Enforced by spec: files are written at fetch time, never converted
// on export (FR-015/A8).
func writeArtifacts(dir string, md string, version int, rawHTML string) error {
	if err := os.MkdirAll(historyDir(dir), 0o755); err != nil {
		return err
	}
	base := filepath.Base(dir)
	// history/{version:04d}.md
	if err := os.WriteFile(versionPath(dir, version), []byte(md), 0o644); err != nil {
		return err
	}
	// current {page_id}.md (overwrite to latest)
	if err := os.WriteFile(filepath.Join(dir, base+".md"), []byte(md), 0o644); err != nil {
		return err
	}
	// backup.html — latest snapshot only (A8)
	if rawHTML != "" {
		if err := os.WriteFile(backupHTMLPath(dir), []byte(rawHTML), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// SavePageResult persists one engine page result into DB + files, returning
// its outcome status (new|unchanged|changed|offline|failed) per FR-008/FR-028.
func SavePageResult(t *model.CrawlerTask, runID uint64, pr *engine.PageResult, depth int) (string, error) {
	// content_mode="main": replace full-page markdown with go-readability
	// extracted article body before archiving (feature 002). Scoped to the
	// goquery engine (local fetch pipeline); cloud/render engines keep their
	// own full markdown until a unified extractor lands.
	if t.ContentMode == "main" && t.Engine == "goquery" && pr.RawHTML != "" && !pr.Failed {
		if title, mainMD, ok := engine.ExtractMainMarkdown(pr.RawHTML, pr.URL); ok {
			pr.Title = title
			pr.Markdown = mainMD
		}
	}
	status, err := classifyPage(pr)
	if err != nil {
		return "", err
	}
	key, err := urlutil.URLKey(pr.URL)
	if err != nil {
		return "", err
	}

	var page model.CrawlerPage
	findErr := model.DB.Where("task_id = ? AND url_key = ?", t.ID, key).First(&page).Error
	pageNotFound := findErr != nil

	if status == "offline" || status == "failed" {
		// No content: record outcome only, never touch files/versions (FR-028).
		if pageNotFound {
			page = model.CrawlerPage{
				TaskID: t.ID, URL: pr.URL, URLKey: key, Depth: depth,
				Title: pr.Title, FirstSeenAt: time.Now(), LastSeenAt: time.Now(),
			}
			if err := model.DB.Create(&page).Error; err != nil {
				return "", err
			}
		}
		return status, upsertRunPage(runID, page.ID, status, pr.Err)
	}

	md := strings.TrimSpace(pr.Markdown)
	if md == "" {
		md = pr.URL // never write an empty body file (contract: title empty -> url)
	}
	fp := urlutil.Fingerprint(md)
	now := time.Now()

	if pageNotFound {
		// First fetch -> version 1 (kind=first).
		page = model.CrawlerPage{
			TaskID: t.ID, URL: pr.URL, URLKey: key, Depth: depth,
			Title: pr.Title, LatestVersion: 1, LatestFingerprint: fp,
			FirstSeenAt: now, LastSeenAt: now,
		}
		if err := model.DB.Create(&page).Error; err != nil {
			return "", err
		}
		dir := pageDir(t.ID, page.ID)
		body := frontMatter(t, &page, 1, fp, now.Format(time.RFC3339)) + md + "\n"
		if err := writeArtifacts(dir, body, 1, pr.RawHTML); err != nil {
			return "", fmt.Errorf("write artifacts: %w", err)
		}
		if err := createVersion(t.ID, page.ID, runID, 1, "first", fp, len(md), now); err != nil {
			return "", err
		}
		return "new", upsertRunPage(runID, page.ID, "new", "")
	}

	// Existing page: compare fingerprint (FR-028: same -> reuse, no writes).
	if fp == page.LatestFingerprint {
		return "unchanged", upsertRunPage(runID, page.ID, "unchanged", "")
	}
	next := page.LatestVersion + 1
	kind := "change"
	if page.LatestVersion == 0 {
		kind = "first" // retried page that never had content yet (feature 002 retry-failed)
	}
	// Refresh in-memory state first so the front-matter uses the new values.
	page.Title = pr.Title
	page.URL = pr.URL
	page.Depth = depth
	page.LatestVersion = next
	page.LatestFingerprint = fp
	page.LastSeenAt = now
	if err := model.DB.Model(&model.CrawlerPage{}).Where("id = ?", page.ID).
		Updates(map[string]any{
			"title": page.Title, "url": page.URL, "depth": page.Depth,
			"latest_version": next, "latest_fingerprint": fp, "last_seen_at": now,
		}).Error; err != nil {
		return "", err
	}
	dir := pageDir(t.ID, page.ID)
	body := frontMatter(t, &page, next, fp, now.Format(time.RFC3339)) + md + "\n"
	if err := writeArtifacts(dir, body, next, pr.RawHTML); err != nil {
		return "", fmt.Errorf("write artifacts: %w", err)
	}
	if err := createVersion(t.ID, page.ID, runID, next, kind, fp, len(md), now); err != nil {
		return "", err
	}
	return "changed", upsertRunPage(runID, page.ID, "changed", "")
}

// classifyPage maps an engine result to our outcome status (research §5):
// engine-failed -> failed; target 404 -> offline; 2xx/304 -> content path.
func classifyPage(pr *engine.PageResult) (string, error) {
	if pr == nil {
		return "", fmt.Errorf("engine returned nil result")
	}
	if pr.Failed {
		return "failed", nil
	}
	switch {
	case pr.StatusCode == 404:
		return "offline", nil
	case pr.StatusCode >= 200 && pr.StatusCode < 300 || pr.StatusCode == 304:
		return "content", nil
	default:
		return "failed", nil
	}
}

func upsertRunPage(runID, pageID uint64, status, errMsg string) error {
	now := time.Now()
	var existing model.RunPage
	err := model.DB.Where("run_id = ? AND page_id = ?", runID, pageID).First(&existing).Error
	if err == nil {
		return model.DB.Model(&model.RunPage{}).Where("id = ?", existing.ID).
			Updates(map[string]any{"status": status, "error": errMsg, "crawled_at": &now}).Error
	}
	return model.DB.Create(&model.RunPage{
		RunID: runID, PageID: pageID, Status: status,
		Error: errMsg, CrawledAt: &now, CreatedAt: now,
	}).Error
}

func createVersion(taskID, pageID, runID uint64, version int, kind, fp string, charCount int, at time.Time) error {
	return model.DB.Create(&model.PageVersion{
		PageID: pageID, Version: version, RunID: runID, Kind: kind,
		Fingerprint: fp, CharCount: charCount, CrawledAt: at, CreatedAt: at,
	}).Error
}

// ReadLatestContent returns the current body markdown (front-matter stripped).
func ReadLatestContent(taskID, pageID uint64) (string, error) {
	dir := pageDir(taskID, pageID)
	data, err := os.ReadFile(currentMDPath(dir))
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%w: 页面内容文件不存在", ErrNotFound)
		}
		return "", err
	}
	return stripFrontMatter(string(data)), nil
}

// ReadVersionContent returns a specific version body markdown.
func ReadVersionContent(taskID, pageID uint64, version int) (string, error) {
	data, err := os.ReadFile(versionPath(pageDir(taskID, pageID), version))
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%w: 版本 %d 文件不存在", ErrNotFound, version)
		}
		return "", err
	}
	return stripFrontMatter(string(data)), nil
}

// RemoveTaskDir deletes the artifact directory of a whole task (FR-021).
func RemoveTaskDir(taskID uint64) error {
	return os.RemoveAll(filepath.Join(storageRoot(), "crawler", fmt.Sprintf("%d", taskID)))
}

// RemovePageDir deletes the artifact directory of one page (FR-021).
func RemovePageDir(taskID, pageID uint64) error {
	return os.RemoveAll(pageDir(taskID, pageID))
}
