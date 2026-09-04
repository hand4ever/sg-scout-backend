package proofread

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	entityproofread "sg.scout/entity/proofread"
	"sg.scout/model"
)

// Auto-check run orchestration (research D8): StartAutoCheck creates a run row
// and spawns one goroutine that executes every enabled engine sequentially,
// landing candidates as cards. The run row is the pollable status source
// (FR-011/012); per-engine results are stored in the run's engines JSON.

// logActionAutoRun marks one finished auto-check in the proofread log (D14).
const logActionAutoRun = "auto_run"

// Engine result statuses inside a run.
const (
	engineOK     = "ok"
	engineFailed = "failed"
)

// MaxCandidatesPerEngine caps one engine's landed candidates per run
// (research D11): guards against runaway LLM/lexicon output.
const MaxCandidatesPerEngine = 500

// runEngineState is one engine's execution record inside run.engines.
type runEngineState struct {
	EngineID      uint64 `json:"engine_id"`
	Name          string `json:"name"`
	Type          string `json:"type"`
	ConfigSummary string `json:"config_summary"`
	Status        string `json:"status"` // ok | failed
	Cards         int    `json:"cards"`
	Dropped       int    `json:"dropped"`
	CostMS        int64  `json:"cost_ms"`
	Error         string `json:"error"`
}

// RunView is one run row for list/detail APIs.
type RunView struct {
	ID         uint64           `json:"id"`
	DocID      uint64           `json:"doc_id"`
	Status     string           `json:"status"`
	Summary    string           `json:"summary"`
	Engines    []runEngineState `json:"engines,omitempty"` // detail only
	StartedAt  time.Time        `json:"started_at"`
	FinishedAt *time.Time       `json:"finished_at"`
}

// runScheduler guards concurrent execution per document (FR-016/A7): a global
// map docID → running flag. Single-user deployment; map kept small.
var (
	runMu       sync.Mutex
	runningDocs = map[uint64]bool{}
)

// StartAutoCheck begins an auto-check run (contracts §6): validates the doc,
// requires ≥1 enabled engine, rejects a concurrent run on the same doc, then
// spawns the executor goroutine and returns the run view immediately.
func StartAutoCheck(docID uint64) (*RunView, error) {
	doc, err := docByID(docID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(doc.DraftText) == "" {
		return nil, fmt.Errorf("%w: 无可校对内容", ErrBadRequest)
	}
	snaps, err := EnabledSnapshots()
	if err != nil {
		return nil, err
	}
	if len(snaps) == 0 {
		return nil, fmt.Errorf("%w: 暂无启用的校对引擎，请先在设置中启用", ErrBadRequest)
	}
	runMu.Lock()
	if runningDocs[docID] {
		runMu.Unlock()
		return nil, fmt.Errorf("%w: 已有校对任务进行中", ErrConflict)
	}
	runningDocs[docID] = true
	runMu.Unlock()

	enginesJSON, err := marshalRunEngines(snaps)
	if err != nil {
		releaseDoc(docID)
		return nil, err
	}
	now := time.Now()
	run := model.ProofreadAutoRun{
		DocID: docID, Status: entityproofread.RunStatusRunning,
		Engines: enginesJSON, StartedAt: now,
	}
	if err := model.DB.Create(&run).Error; err != nil {
		releaseDoc(docID)
		return nil, err
	}
	go executeRun(doc, run.ID, snaps)
	return runViewOf(&run, false), nil
}

// releaseDoc clears the per-doc running flag.
func releaseDoc(docID uint64) {
	runMu.Lock()
	delete(runningDocs, docID)
	runMu.Unlock()
}

// executeRun runs every enabled engine sequentially and finalizes the run row
// (research D8). Runs in its own goroutine; each engine's state is persisted
// as it finishes so polling sees progress.
func executeRun(doc *model.ProofreadDocument, runID uint64, snaps []EngineSnapshot) {
	defer func() {
		releaseDoc(doc.ID)
		if r := recover(); r != nil {
			// Constitution VI: never swallow — mark the whole run failed.
			finalizeRun(runID, entityproofread.RunStatusFailed, fmt.Sprintf("自动校对内部错误：%v", r))
		}
	}()

	lines := draftLines(doc.DraftText)
	states := make([]runEngineState, 0, len(snaps))
	anyOK, anyFailed := false, false
	for _, snap := range snaps {
		st := runEngineState{
			EngineID: snap.EngineID, Name: snap.Name, Type: snap.Type,
			ConfigSummary: snap.ConfigSummary, Status: engineOK,
		}
		start := time.Now()
		landed, dropped, err := runOneEngine(doc, runID, snap, lines)
		st.CostMS = time.Since(start).Milliseconds()
		st.Cards = landed
		st.Dropped = dropped
		if err != nil {
			st.Status = engineFailed
			st.Error = err.Error()
			anyFailed = true
		} else {
			anyOK = true
		}
		// Persist progress after each engine (visible while running).
		states = append(states, st)
		persistRunEngines(runID, states)
	}

	status := entityproofread.RunStatusDone
	switch {
	case anyOK && anyFailed:
		status = entityproofread.RunStatusPartialFailed
	case !anyOK && anyFailed:
		status = entityproofread.RunStatusFailed
	}
	summary := runSummary(states)
	finalizeRun(runID, status, summary)
	// Audit one auto_run log entry (D14); failure must not lose the run state.
	if err := writeLog(doc.ID, logActionAutoRun, nil, summary, map[string]any{
		"run_id": runID, "status": status,
	}); err != nil {
		markRunAuditError(runID, err)
	}
}

// runOneEngine executes a single engine against the draft and lands its
// candidates as cards.
//   - lexicon: load dict file → parse → match all → candidates
//   - llm:     cap input length (D13) → call provider → parse candidates
//   - httpapi: v1 not wired to a real third party (Q2) → engine-level failure
func runOneEngine(doc *model.ProofreadDocument, runID uint64, snap EngineSnapshot, lines []string) (landed, dropped int, err error) {
	cfg, _, err := EngineConfigByID(snap.EngineID)
	if err != nil {
		return 0, 0, fmt.Errorf("引擎配置读取失败：%v", err)
	}
	var cands []Candidate
	switch snap.Type {
	case entityproofread.EngineTypeLexicon:
		path, _ := strField(cfg, "dict_path")
		if path == "" {
			return 0, 0, fmt.Errorf("词库引擎缺少词库文件路径")
		}
		var entries []lexiconEntry
		entries, err = LoadLexiconFile(path)
		if err != nil {
			return 0, 0, err
		}
		matches := MatchLexicon(lines, entries)
		cands = make([]Candidate, 0, len(matches))
		for _, m := range matches {
			cands = append(cands, Candidate{
				OpType: entityproofread.OpTypeFix, OrigText: m.Entry.Word,
				Replacement: m.Entry.Replacement, Reason: "词库比对",
			})
		}
	case entityproofread.EngineTypeLLM:
		provider, _ := strField(cfg, "provider")
		modelName, _ := strField(cfg, "model")
		if provider == "" || modelName == "" {
			return 0, 0, fmt.Errorf("大模型引擎缺少 provider/model 配置")
		}
		maxChars := intField(cfg, "max_chars", defaultMaxChars)
		if runeLen(doc.DraftText) > maxChars {
			return 0, 0, fmt.Errorf("底稿过长（%d 字符，上限 %d），请分段校对或缩小校对范围", runeLen(doc.DraftText), maxChars)
		}
		effort, _ := strField(cfg, "effort")
		cands, err = RunLLMEngine(provider, modelName, doc.DraftText, effort)
		if err != nil {
			return 0, 0, err
		}
	case entityproofread.EngineTypeHTTPAPI:
		return 0, 0, fmt.Errorf("第三方校对 API 引擎 v1 未接入真实服务（预留适配器，接入见 contracts 附录 A）")
	default:
		return 0, 0, fmt.Errorf("未知引擎类型 %q", snap.Type)
	}
	return landCandidates(doc, runID, snap, cands)
}

// intField reads an int config value with a default.
func intField(m map[string]any, key string, def int) int {
	v, ok := m[key]
	if !ok {
		return def
	}
	switch n := v.(type) {
	case float64:
		if n > 0 {
			return int(n)
		}
	case int:
		if n > 0 {
			return n
		}
	}
	return def
}

// landCandidates runs the card-landing pipeline (research D5/D11):
//  1. ValidateCandidate type rules → dropped
//  2. Locate orig_text in the draft → dropped if not found
//  3. Skip overlaps with already-accepted cards → dropped
//  4. Dedupe by (op_type, orig_text, replacement) vs existing pending + this run → dropped
//  5. Cap per-engine count → engine failure when exceeded
//  6. Insert card (source=engine, engine_name snapshot, run_id, status=pending)
func landCandidates(doc *model.ProofreadDocument, runID uint64, snap EngineSnapshot, cands []Candidate) (landed, dropped int, err error) {
	if len(cands) == 0 {
		return 0, 0, nil
	}
	var existing []model.ProofreadCard
	if err := model.DB.Where("doc_id = ?", doc.ID).Find(&existing).Error; err != nil {
		return 0, 0, err
	}
	now := time.Now()
	seen := map[string]bool{}
	for i := range existing {
		if existing[i].Status == entityproofread.StatusPending {
			seen[dedupeKey(&existing[i])] = true
		}
	}
	insertCards := make([]model.ProofreadCard, 0, len(cands))
	for _, c := range cands {
		if verr := ValidateCandidate(c); verr != nil {
			dropped++
			continue
		}
		anchor, ok := Locate(doc.DraftText, c.OrigText)
		if !ok {
			dropped++ // not found in draft (engine hallucination, D4)
			continue
		}
		// Accepted-overlap skip (spec US5-2 / research D5).
		conflict := false
		for i := range existing {
			e := &existing[i]
			if e.Status == entityproofread.StatusAccepted && anchorsOverlap(anchorOf(e), anchor) {
				conflict = true
				break
			}
		}
		if conflict {
			dropped++
			continue
		}
		key := dedupeKeyOf(c)
		if seen[key] {
			dropped++ // duplicate: existing pending or earlier in this run
			continue
		}
		seen[key] = true
		if len(insertCards) >= MaxCandidatesPerEngine {
			return landed, dropped, fmt.Errorf("产出超限（>%d 条），请调优词库/提示词或缩小底稿", MaxCandidatesPerEngine)
		}
		anchorVersion := doc.DraftVersion
		insertCards = append(insertCards, model.ProofreadCard{
			DocID: doc.ID, OpType: c.OpType,
			StartLine: anchor.StartLine, StartOff: anchor.StartOff,
			EndLine: anchor.EndLine, EndOff: anchor.EndOff,
			OrigText: c.OrigText, Replacement: c.Replacement,
			Reason: strings.TrimSpace(c.Reason), Status: entityproofread.StatusPending,
			AnchorVersion: anchorVersion,
			Source:        entityproofread.SourceEngine,
			EngineName:    snap.Name,
			RunID:         &runID,
			CreatedAt:     now, UpdatedAt: now,
		})
	}
	if len(insertCards) == 0 {
		return 0, dropped, nil
	}
	// Bulk insert (single query; card rows are independent of each other).
	if err := model.DB.Create(&insertCards).Error; err != nil {
		return 0, dropped, err
	}
	return len(insertCards), dropped, nil
}

// dedupeKey identifies a candidate/card for dedupe (op_type+orig+replacement).
func dedupeKey(c *model.ProofreadCard) string {
	return c.OpType + "\x00" + c.OrigText + "\x00" + c.Replacement
}

func dedupeKeyOf(c Candidate) string {
	return c.OpType + "\x00" + c.OrigText + "\x00" + c.Replacement
}

// marshalRunEngines serializes the initial engine snapshot array.
func marshalRunEngines(snaps []EngineSnapshot) (string, error) {
	b, err := json.Marshal(snaps)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// persistRunEngines writes back the current engine states array (progress).
func persistRunEngines(runID uint64, states []runEngineState) {
	b, err := json.Marshal(states)
	if err != nil {
		return
	}
	model.DB.Model(&model.ProofreadAutoRun{}).Where("id = ?", runID).
		Update("engines", string(b))
}

// finalizeRun sets the terminal status/summary/finished_at.
func finalizeRun(runID uint64, status, summary string) {
	now := time.Now()
	model.DB.Model(&model.ProofreadAutoRun{}).Where("id = ?", runID).Updates(map[string]any{
		"status": status, "summary": summary, "finished_at": now,
	})
}

// markRunAuditError records that the auto_run log entry could not be written
// (best-effort; keeps the run record honest).
func markRunAuditError(runID uint64, cause error) {
	model.DB.Model(&model.ProofreadAutoRun{}).Where("id = ?", runID).
		Update("summary", fmt.Sprintf("（校对日志写入失败：%v）", cause))
}

// runSummary renders "自动校对：词库 3 条，大模型 5 条，大模型失败：超时".
func runSummary(states []runEngineState) string {
	parts := make([]string, 0, len(states))
	for _, s := range states {
		if s.Status == engineOK {
			parts = append(parts, fmt.Sprintf("%s %d 条", s.Name, s.Cards))
		} else {
			parts = append(parts, fmt.Sprintf("%s 失败：%s", s.Name, s.Error))
		}
	}
	return "自动校对：" + strings.Join(parts, "，")
}

// runViewOf converts a model row into the API view. detail=false omits the
// engines array (list endpoint, contracts §7).
func runViewOf(r *model.ProofreadAutoRun, detail bool) *RunView {
	v := &RunView{
		ID: r.ID, DocID: r.DocID, Status: r.Status, Summary: r.Summary,
		StartedAt: r.StartedAt, FinishedAt: r.FinishedAt,
	}
	if detail {
		var states []runEngineState
		if err := json.Unmarshal([]byte(r.Engines), &states); err != nil || states == nil {
			states = []runEngineState{}
		}
		v.Engines = states
	}
	return v
}

// ListRuns returns a document's runs, newest first (limit 50, contracts §7).
func ListRuns(docID uint64) ([]RunView, error) {
	if _, err := docByID(docID); err != nil {
		return nil, err
	}
	var rows []model.ProofreadAutoRun
	if err := model.DB.Where("doc_id = ?", docID).Order("id DESC").Limit(50).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]RunView, 0, len(rows))
	for i := range rows {
		out = append(out, *runViewOf(&rows[i], false))
	}
	return out, nil
}

// RunDetail returns one run with engine-level results (contracts §8).
func RunDetail(docID, runID uint64) (*RunView, error) {
	var r model.ProofreadAutoRun
	if err := model.DB.Where("id = ? AND doc_id = ?", runID, docID).First(&r).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: 运行记录 %d 不存在", ErrNotFound, runID)
		}
		return nil, err
	}
	return runViewOf(&r, true), nil
}

// DeleteRunsByDoc removes a document's runs (cascade on doc delete, D12).
func DeleteRunsByDoc(docID uint64) error {
	return model.DB.Where("doc_id = ?", docID).Delete(&model.ProofreadAutoRun{}).Error
}

// MarkInterruptedRuns flags runs left running at startup as failed
// (research D8 restart 兜底; called once from main).
func MarkInterruptedRuns() {
	now := time.Now()
	model.DB.Model(&model.ProofreadAutoRun{}).Where("status = ?", entityproofread.RunStatusRunning).
		Updates(map[string]any{"status": entityproofread.RunStatusFailed, "finished_at": now})
}
