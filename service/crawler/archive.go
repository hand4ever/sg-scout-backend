package crawler

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"sg.scout/model"
)

// zipDir packs a directory tree into a zip written to w (stdlib only).
// Export semantics: pack the existing artifact files as-is, no conversion
// (FR-015 — files are produced at fetch time).
func zipDir(w io.Writer, root string) error {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: 文件目录不存在", ErrNotFound)
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("export source is not a directory: %s", root)
	}
	zw := zip.NewWriter(w)
	defer zw.Close()
	return filepath.Walk(root, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		fh, err := zw.Create(filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(fh, f)
		return err
	})
}

// ExportTaskZip writes the whole task artifact directory as zip.
func ExportTaskZip(w io.Writer, taskID uint64) error {
	var task model.CrawlerTask
	if err := model.DB.First(&task, taskID).Error; err != nil {
		return fmt.Errorf("%w: task %d", ErrNotFound, taskID)
	}
	root := filepath.Join(storageRoot(), "crawler", fmt.Sprintf("%d", taskID))
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return fmt.Errorf("%w: 任务尚无落盘文件", ErrNotFound)
	}
	return zipDir(w, root)
}

// ExportPageZip writes a single page artifact directory as zip.
func ExportPageZip(w io.Writer, pageID uint64) error {
	var page model.CrawlerPage
	if err := model.DB.First(&page, pageID).Error; err != nil {
		return fmt.Errorf("%w: page %d", ErrNotFound, pageID)
	}
	root := pageDir(page.TaskID, page.ID)
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return fmt.Errorf("%w: 页面尚无落盘文件", ErrNotFound)
	}
	return zipDir(w, root)
}
