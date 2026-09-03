package crawler

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	crawlerentity "sg.scout/entity/crawler"
	"sg.scout/model"
)

// Sentinel business errors mapped to HTTP statuses by the handler layer.
var (
	ErrBadRequest   = errors.New("参数错误")
	ErrNotFound     = errors.New("资源不存在")
	ErrConflict     = errors.New("状态冲突")
	ErrEngineConfig = errors.New("引擎未配置")
)

// CreateTask validates and creates a task (FR-001/FR-025: config locked after).
func CreateTask(req *crawlerentity.CreateTaskReq) (*TaskView, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: 请求体不能为空", ErrBadRequest)
	}
	if req.SourceType != "" && req.SourceType != "web" {
		return nil, fmt.Errorf("%w: v1 仅支持来源类型 web", ErrBadRequest)
	}
	if req.EntryURL == "" {
		return nil, fmt.Errorf("%w: entry_url 必填", ErrBadRequest)
	}
	key, err := URLKey(req.EntryURL)
	if err != nil {
		return nil, fmt.Errorf("%w: entry_url 必须是可解析的 http(s) 地址", ErrBadRequest)
	}

	t := &model.CrawlerTask{
		SourceType:      "web",
		EntryURL:        req.EntryURL,
		EntryURLKey:     key,
		Depth:           intOr(req.Depth, crawlerentity.DefaultDepth),
		IncludeSubdomain: boolOr(req.IncludeSubdomain, false),
		PageLimit:       intOr(req.PageLimit, crawlerentity.DefaultPageLimit),
		RetryTimes:      intOr(req.RetryTimes, crawlerentity.DefaultRetryTimes),
		RetryIntervalS:  intOr(req.RetryIntervalS, crawlerentity.DefaultRetryIntervalS),
		ThrottlePages:   intOr(req.ThrottlePages, crawlerentity.DefaultThrottlePages),
		ThrottleSeconds: intOr(req.ThrottleSeconds, crawlerentity.DefaultThrottleSecs),
		TimeoutS:        intOr(req.TimeoutS, crawlerentity.DefaultTimeoutS),
		Status:          "pending",
	}
	if t.Depth < 0 {
		return nil, fmt.Errorf("%w: depth 必须 ≥0", ErrBadRequest)
	}
	if t.PageLimit < 1 {
		return nil, fmt.Errorf("%w: page_limit 必须 ≥1", ErrBadRequest)
	}
	if t.RetryTimes < 0 || t.RetryTimes > 10 {
		return nil, fmt.Errorf("%w: retry_times 范围 0-10", ErrBadRequest)
	}
	if t.ThrottlePages < 1 || t.ThrottleSeconds < 1 {
		return nil, fmt.Errorf("%w: 节流参数必须 ≥1", ErrBadRequest)
	}
	if t.TimeoutS < 1 {
		return nil, fmt.Errorf("%w: timeout_s 必须 ≥1", ErrBadRequest)
	}
	if err := model.DB.Create(t).Error; err != nil {
		return nil, fmt.Errorf("create task: %w", err)
	}
	return taskView(t), nil
}

// ListTasks returns tasks (optionally filtered by status) with latest run summary.
func ListTasks(status string) ([]*TaskView, int64, error) {
	q := model.DB.Model(&model.CrawlerTask{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var tasks []model.CrawlerTask
	if err := q.Order("id DESC").Find(&tasks).Error; err != nil {
		return nil, 0, err
	}
	views := make([]*TaskView, 0, len(tasks))
	for i := range tasks {
		v := taskView(&tasks[i])
		last, err := latestRun(tasks[i].ID)
		if err == nil && last != nil {
			v.LastRun = runView(last)
		}
		views = append(views, v)
	}
	return views, total, nil
}

// GetTask returns one task plus its recent runs.
func GetTask(id uint64) (*TaskView, []*RunView, error) {
	var t model.CrawlerTask
	if err := model.DB.First(&t, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, fmt.Errorf("%w: task %d", ErrNotFound, id)
		}
		return nil, nil, err
	}
	tv := taskView(&t)
	var runs []model.CrawlerRun
	if err := model.DB.Where("task_id = ?", id).Order("id DESC").Limit(20).Find(&runs).Error; err != nil {
		return nil, nil, err
	}
	views := make([]*RunView, 0, len(runs))
	for i := range runs {
		views = append(views, runView(&runs[i]))
	}
	return tv, views, nil
}

// TaskActive reports whether a task currently has a queued/running run.
func TaskActive(taskID uint64) (bool, error) {
	var n int64
	err := model.DB.Model(&model.CrawlerRun{}).
		Where("task_id = ? AND status IN ?", taskID, []string{"queued", "running"}).
		Count(&n).Error
	return n > 0, err
}

// SetTaskPageCount refreshes the task page_count from its pages.
func SetTaskPageCount(taskID uint64) error {
	var n int64
	if err := model.DB.Model(&model.CrawlerPage{}).Where("task_id = ?", taskID).Count(&n).Error; err != nil {
		return err
	}
	now := time.Now()
	return model.DB.Model(&model.CrawlerTask{}).Where("id = ?", taskID).
		Updates(map[string]any{"page_count": n, "last_run_at": &now}).Error
}

func latestRun(taskID uint64) (*model.CrawlerRun, error) {
	var r model.CrawlerRun
	err := model.DB.Where("task_id = ?", taskID).Order("id DESC").First(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &r, err
}

func intOr(p *int, def int) int {
	if p != nil {
		return *p
	}
	return def
}

func boolOr(p *bool, def bool) bool {
	if p != nil {
		return *p
	}
	return def
}
