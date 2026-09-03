package proofread

import (
	"fmt"
	"strings"
	"testing"
)

func logOK(t *testing.T, format string, args ...any) {
	t.Helper()
	msg := fmt.Sprintf(format, args...)
	if testing.Verbose() {
		t.Log(msg)
	} else {
		fmt.Println(msg)
	}
}

// TestMDToPlain_Table covers the md→plain conversion rules (research D1):
// heading/quote/bullet markers stripped, ordered numbers kept, links→text,
// images→alt placeholder, emphasis markers removed, fences/blank kept 1:1.
func TestMDToPlain_Table(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"heading", "# 医院新闻\n\n正文段落", []string{"医院新闻", "", "正文段落"}},
		{"bullet", "- 项目甲\n- 项目乙", []string{"项目甲", "项目乙"}},
		{"ordered kept", "1. 第一项\n2. 第二项", []string{"1. 第一项", "2. 第二项"}},
		{"quote", "> 引用文字", []string{"引用文字"}},
		{"link to text", "详见[官网](https://example.com)。", []string{"详见官网。"}},
		{"image alt", "![院徽](https://example.com/logo.png)", []string{"[图片:院徽]"}},
		{"emphasis", "这是**重要**内容与`代码`。", []string{"这是重要内容与代码。"}},
		{"fence raw inside", "```\n- keep me\n```", []string{"", "- keep me", ""}},
		{"blank preserved", "a\n\nb", []string{"a", "", "b"}},
		{"inline tag", "正文<strong>加粗</strong>文本", []string{"正文加粗文本"}},
		{"plain unchanged", "普通一行文字。", []string{"普通一行文字。"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MDToPlain(tc.in)
			if strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
				t.Errorf("MDToPlain(%q) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
	logOK(t, "mdplain table cases passed: %d", len(cases))
}

// TestMDToPlain_LineCount1to1 asserts the line stream preserves source lines.
func TestMDToPlain_LineCount1to1(t *testing.T) {
	body := "# 标题\n\n第一段文字。\n\n> 引文\n- 列表项\n1. 有序项\n"
	got := MDToPlain(body)
	src := strings.Split(body, "\n")
	if len(got) != len(src) {
		t.Fatalf("line count = %d, want %d (1:1)", len(got), len(src))
	}
	logOK(t, "1:1 line mapping ok (%d lines)", len(got))
}
