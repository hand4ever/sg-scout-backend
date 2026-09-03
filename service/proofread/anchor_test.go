package proofread

import (
	"errors"
	"fmt"
	"testing"

	entityproofread "sg.scout/entity/proofread"
)

// TestAnchorsOverlap covers the FR-006 interval semantics: range-range
// intersection, point-in-range, identical points, adjacency allowed.
func TestAnchorsOverlap(t *testing.T) {
	type pos struct{ l, o int }
	cases := []struct {
		name string
		a, b cardAnchor
		want bool
	}{
		{"range intersect", cardAnchor{1, 2, 1, 5}, cardAnchor{1, 3, 1, 6}, true},
		{"touching boundary ok", cardAnchor{1, 2, 1, 5}, cardAnchor{1, 5, 1, 9}, false},
		{"disjoint lines ok", cardAnchor{1, 0, 1, 4}, cardAnchor{2, 0, 2, 4}, false},
		{"point inside range", cardAnchor{1, 3, 1, 3}, cardAnchor{1, 2, 1, 9}, true},
		{"point at range start ok", cardAnchor{1, 2, 1, 2}, cardAnchor{1, 2, 1, 9}, false},
		{"point at range end ok", cardAnchor{1, 9, 1, 9}, cardAnchor{1, 2, 1, 9}, false},
		{"identical points", cardAnchor{1, 4, 1, 4}, cardAnchor{1, 4, 1, 4}, true},
		{"points different lines ok", cardAnchor{1, 4, 1, 4}, cardAnchor{2, 1, 2, 1}, false},
		{"contained range", cardAnchor{1, 1, 1, 10}, cardAnchor{1, 3, 1, 6}, true},
		{"same range", cardAnchor{1, 2, 1, 6}, cardAnchor{1, 2, 1, 6}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := anchorsOverlap(tc.a, tc.b)
			if got != tc.want {
				t.Errorf("anchorsOverlap(%+v,%+v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
	logOK(t, "overlap cases passed: %d", len(cases))
}

// TestResolveAnchor validates range semantics against a real draft line stream
// (offsets are rune-based; multi-byte safe).
func TestResolveAnchor(t *testing.T) {
	lines := draftLines("今天天气很好，适合出游。\n第二行内容测试。\n第三行。")
	cases := []struct {
		name           string
		sl, so, el, eo int
		op             string
		wantOrig       string
		wantErr        bool
	}{
		{"fix single line", 1, 2, 1, 4, "fix", "天气", false},
		{"multi line join", 1, 2, 2, 2, "replace", "天气很好，适合出游。\n第二", false},
		{"insert point ok", 1, 2, 1, 2, "insert", "", false},
		{"insert with selection err", 1, 2, 1, 4, "insert", "", true},
		{"fix empty selection err", 1, 2, 1, 2, "fix", "", true},
		{"line out of range", 9, 0, 9, 1, "fix", "", true},
		{"offset overrun", 1, 0, 1, 99, "fix", "", true},
		{"end before start", 1, 5, 1, 2, "fix", "", true},
		{"emoji surrogate safe via runes", 2, 2, 2, 4, "fix", "行内", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, orig, err := resolveAnchor(lines, tc.sl, tc.so, tc.el, tc.eo, tc.op)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got orig %q", orig)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if orig != tc.wantOrig {
				t.Errorf("orig = %q, want %q", orig, tc.wantOrig)
			}
		})
	}
	logOK(t, "resolveAnchor cases passed: %d", len(cases))
}

// TestExtractOrig_RuneEdge asserts emoji/supplementary characters count as one
// rune for offsets and survive slicing.
func TestExtractOrig_RuneEdge(t *testing.T) {
	lines := []string{"正文😀表情文字"}
	a := cardAnchor{1, 2, 1, 4}
	orig, err := extractOrig(lines, a)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if orig != "😀表" {
		t.Errorf("orig = %q, want %q", orig, "😀表")
	}
	logOK(t, "rune-edge extract ok: %q", orig)
}

// TestValidOpType covers accepted op type vocabulary.
func TestValidOpType(t *testing.T) {
	for _, ok := range []string{entityproofread.OpTypeFix, entityproofread.OpTypeReplace,
		entityproofread.OpTypeDelete, entityproofread.OpTypeInsert} {
		if !validOpType(ok) {
			t.Errorf("validOpType(%s) = false, want true", ok)
		}
	}
	for _, bad := range []string{"", "fixx", "改", "DELETE"} {
		if validOpType(bad) {
			t.Errorf("validOpType(%q) = true, want false", bad)
		}
	}
	logOK(t, "op type vocabulary ok")
}

// TestErrWrapping asserts sentinel errors keep identity for handler mapping.
func TestErrWrapping(t *testing.T) {
	err := fmt.Errorf("%w: 内容为空", ErrBadRequest)
	if !errors.Is(err, ErrBadRequest) {
		t.Fatal("errors.Is lost sentinel identity")
	}
	logOK(t, "sentinel wrapping ok")
}
