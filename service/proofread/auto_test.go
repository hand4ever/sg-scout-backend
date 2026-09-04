package proofread

import (
	"strings"
	"testing"
)

// TestLocate covers the candidate localization contract (research D4): the
// engine returns orig_text only and the server finds it in the draft line
// stream, first match wins.
func TestLocate(t *testing.T) {
	draft := "第一行有错词付合。\n第二行也付合一次。\n第三行逻缉错误。"
	cases := []struct {
		name         string
		orig         string
		wantLine     int
		wantSpan     string
		wantNotFound bool
	}{
		{"single line hit", "付合", 1, "付合", false},
		{"multi-byte around hit", "错词", 1, "错词", false},
		{"hit later line", "逻缉", 3, "逻缉", false},
		{"second occurrence first-match", "付合", 1, "付合", false}, // first line wins
		{"multiline orig", "行有错词付合。\n第二", 1, "行有错词付合。\n第二", false},
		{"absent", "不存在文本xyz", 0, "", true},
		{"empty orig", "", 0, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, ok := Locate(draft, tc.orig)
			if tc.wantNotFound {
				if ok {
					t.Errorf("Locate(%q) = %+v, want not-found", tc.orig, a)
				}
				return
			}
			if !ok {
				t.Fatalf("Locate(%q) not found, want hit", tc.orig)
			}
			if a.StartLine != tc.wantLine {
				t.Errorf("start line = %d, want %d", a.StartLine, tc.wantLine)
			}
			got, ok := ExtractOrigByAnchor(draft, a)
			if !ok || got != tc.wantSpan {
				t.Errorf("extracted orig = %q (ok=%v), want %q", got, ok, tc.wantSpan)
			}
		})
	}
	logOK(t, "locate cases passed: %d", len(cases))
}

// TestLocate_RuneOffset asserts offsets are rune-based (emoji safe).
func TestLocate_RuneOffset(t *testing.T) {
	draft := "正文😀然后有错词。"
	a, ok := Locate(draft, "错词")
	if !ok {
		t.Fatal("Locate failed")
	}
	got, _ := ExtractOrigByAnchor(draft, a)
	if got != "错词" {
		t.Errorf("extracted = %q, want 错词", got)
	}
	if a.StartOff != 6 { // 正文(2)+😀(1)+然后(2)+有(1)=6 runes before 错词 (0-based)
		t.Errorf("start off = %d, want 6 (rune-based)", a.StartOff)
	}
	logOK(t, "rune offset ok: start_off=%d", a.StartOff)
}

// TestValidateCandidate covers the shared engine candidate type rules.
func TestValidateCandidate(t *testing.T) {
	ok := func(c Candidate) bool {
		return ValidateCandidate(c) == nil
	}
	cases := []struct {
		name string
		c    Candidate
		want bool
	}{
		{"fix ok", Candidate{OpType: "fix", OrigText: "付合", Replacement: "符合", Reason: "错词"}, true},
		{"replace ok", Candidate{OpType: "replace", OrigText: "整句错误", Replacement: "修正句", Reason: ""}, true},
		{"delete ok no replacement", Candidate{OpType: "delete", OrigText: "多余的字", Replacement: "", Reason: ""}, true},
		{"delete with replacement ok (ignored)", Candidate{OpType: "delete", OrigText: "多余", Replacement: "x", Reason: ""}, true},
		{"insert rejected v1", Candidate{OpType: "insert", OrigText: "ctx", Replacement: "新增"}, false},
		{"bad op type", Candidate{OpType: "fixx", OrigText: "a", Replacement: "b"}, false},
		{"fix missing replacement", Candidate{OpType: "fix", OrigText: "a", Replacement: ""}, false},
		{"fix missing orig", Candidate{OpType: "fix", OrigText: "", Replacement: "b"}, false},
		{"replacement too long", Candidate{OpType: "fix", OrigText: "a", Replacement: strings.Repeat("长", 2001)}, false},
		{"replacement newline", Candidate{OpType: "fix", OrigText: "a", Replacement: "b\nc"}, false},
	}
	for _, tc := range cases {
		if got := ok(tc.c); got != tc.want {
			t.Errorf("%s: ValidateCandidate(%+v) ok=%v, want %v", tc.name, tc.c, got, tc.want)
		}
	}
	logOK(t, "candidate validation cases passed: %d", len(cases))
}

// TestParseLexicon covers dictionary file parsing (research D7).
func TestParseLexicon(t *testing.T) {
	content := "# 注释行\n\n付合→符合\n逻缉=>逻辑\n错别\t正词\n"
	entries, err := ParseLexicon(content)
	if err != nil {
		t.Fatalf("ParseLexicon err: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("entries = %d, want 3", len(entries))
	}
	if entries[0].Word != "付合" || entries[0].Replacement != "符合" {
		t.Errorf("entry0 = %+v", entries[0])
	}
	if entries[1].Word != "逻缉" || entries[1].Replacement != "逻辑" {
		t.Errorf("entry1 = %+v", entries[1])
	}
	if entries[2].Word != "错别" || entries[2].Replacement != "正词" {
		t.Errorf("entry2 = %+v", entries[2])
	}
	// Empty / comment-only files error.
	if _, err := ParseLexicon("# only comment\n"); err == nil {
		t.Error("comment-only lexicon should error")
	}
	if _, err := ParseLexicon("没有分隔符的行"); err == nil {
		t.Error("unparsable line should error")
	}
	logOK(t, "lexicon parse ok: %d entries", len(entries))
}

// TestMatchLexicon covers first-occurrence matching per word.
func TestMatchLexicon(t *testing.T) {
	entries, _ := ParseLexicon("付合→符合\n逻缉→逻辑\n")
	lines := draftLines("文中付合出现两次，付合again。\n还有逻缉。")
	matches := MatchLexicon(lines, entries)
	if len(matches) != 2 {
		t.Fatalf("matches = %d, want 2 (one per word)", len(matches))
	}
	if matches[0].Entry.Word != "付合" || matches[0].StartLine != 1 {
		t.Errorf("match0 = %+v", matches[0])
	}
	if matches[1].Entry.Word != "逻缉" || matches[1].StartLine != 2 {
		t.Errorf("match1 = %+v", matches[1])
	}
	logOK(t, "lexicon match ok: %d hits", len(matches))
}

// TestParseLLMResponse covers the model output JSON contract.
func TestParseLLMResponse(t *testing.T) {
	okJSON := `{"items":[
		{"op_type":"fix","orig_text":"付合","replacement":"符合","reason":"错别字"},
		{"op_type":"replace","orig_text":"句子","replacement":"修正句","reason":""}
	]}`
	cands, err := ParseLLMResponse(okJSON)
	if err != nil {
		t.Fatalf("ParseLLMResponse err: %v", err)
	}
	if len(cands) != 2 {
		t.Fatalf("cands = %d, want 2", len(cands))
	}
	if cands[0].OpType != "fix" || cands[0].OrigText != "付合" || cands[0].Replacement != "符合" {
		t.Errorf("cand0 = %+v", cands[0])
	}
	// Fenced JSON tolerated.
	if _, err := ParseLLMResponse("```json\n" + okJSON + "\n```"); err != nil {
		t.Errorf("fenced json should parse: %v", err)
	}
	// Empty items = empty candidates, no error.
	cands, err = ParseLLMResponse(`{"items":[]}`)
	if err != nil || len(cands) != 0 {
		t.Errorf("empty items: cands=%v err=%v", cands, err)
	}
	// Garbage fails.
	if _, err := ParseLLMResponse("我不是JSON"); err == nil {
		t.Error("garbage should fail parsing")
	}
	logOK(t, "llm response parse cases passed")
}

// TestRunSummary covers the summary rendering used by the run row + log entry.
func TestRunSummary(t *testing.T) {
	states := []runEngineState{
		{Name: "词库", Status: engineOK, Cards: 3},
		{Name: "大模型", Status: engineFailed, Error: "超时"},
	}
	s := runSummary(states)
	if !strings.Contains(s, "词库 3 条") || !strings.Contains(s, "大模型 失败：超时") {
		t.Errorf("summary = %q", s)
	}
	logOK(t, "run summary ok: %s", s)
}

// TestDedupeKey covers candidate/card identity for dedupe (op+orig+replacement).
func TestDedupeKey(t *testing.T) {
	if dedupeKeyOf(Candidate{OpType: "fix", OrigText: "a", Replacement: "b"}) !=
		dedupeKeyOf(Candidate{OpType: "fix", OrigText: "a", Replacement: "b"}) {
		t.Error("same candidate should share key")
	}
	if dedupeKeyOf(Candidate{OpType: "fix", OrigText: "a", Replacement: "b"}) ==
		dedupeKeyOf(Candidate{OpType: "replace", OrigText: "a", Replacement: "b"}) {
		t.Error("different op should differ")
	}
	logOK(t, "dedupe key identity ok")
}
