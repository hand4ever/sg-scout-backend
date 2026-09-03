package crawler

import (
	"errors"
	"fmt"
	"os"
	"time"

	"gorm.io/gorm"
	"sg.scout/model"
)

// RunViewOf exports runView for the handler layer.
func RunViewOf(r *model.CrawlerRun) *RunView { return runView(r) }

// PageView is the page header payload inside page detail.
type PageView struct {
	ID            uint64    `json:"id"`
	TaskID        uint64    `json:"task_id"`
	URL           string    `json:"url"`
	Title         string    `json:"title"`
	Depth         int       `json:"depth"`
	LatestVersion int       `json:"latest_version"`
	FirstSeenAt   time.Time `json:"first_seen_at"`
	LastSeenAt    time.Time `json:"last_seen_at"`
}

// PageVersionMeta mirrors page_version rows (version timeline).
type PageVersionMeta struct {
	Version     int       `json:"version"`
	Kind        string    `json:"kind"`
	CrawledAt   time.Time `json:"crawled_at"`
	Fingerprint string    `json:"fingerprint"`
}

// PageDetailView is GET /crawler/pages/{id} payload (content = latest body
// markdown, front-matter stripped; versions = full timeline).
type PageDetailView struct {
	Page     *PageView         `json:"page"`
	Content  string            `json:"content"`
	Versions []PageVersionMeta `json:"versions"`
	Backup   BackupInfo        `json:"backup"`
}

// BackupInfo describes the html backup file presence.
type BackupInfo struct {
	Exists   bool   `json:"exists"`
	Filename string `json:"filename"`
}

// GetPageDetail assembles page + latest content + version timeline.
func GetPageDetail(pageID uint64) (*PageDetailView, error) {
	var page model.CrawlerPage
	if err := model.DB.First(&page, pageID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: page %d", ErrNotFound, pageID)
		}
		return nil, err
	}
	dv := &PageDetailView{
		Page: &PageView{
			ID: page.ID, TaskID: page.TaskID, URL: page.URL, Title: page.Title,
			Depth: page.Depth, LatestVersion: page.LatestVersion,
			FirstSeenAt: page.FirstSeenAt, LastSeenAt: page.LastSeenAt,
		},
		Versions: []PageVersionMeta{},
	}
	var versions []model.PageVersion
	if err := model.DB.Where("page_id = ?", pageID).Order("version ASC").Find(&versions).Error; err != nil {
		return nil, err
	}
	for _, v := range versions {
		dv.Versions = append(dv.Versions, PageVersionMeta{
			Version: v.Version, Kind: v.Kind, CrawledAt: v.CrawledAt, Fingerprint: v.Fingerprint,
		})
	}
	if content, err := ReadLatestContent(page.TaskID, page.ID); err == nil {
		dv.Content = content
	}
	if _, err := os.Stat(backupHTMLPath(pageDir(page.TaskID, page.ID))); err == nil {
		dv.Backup = BackupInfo{Exists: true, Filename: "backup.html"}
	}
	return dv, nil
}
