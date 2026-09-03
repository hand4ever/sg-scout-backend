package engine

import (
	"strings"
	"testing"
)

// noisyArticle mimics a legacy hospital detail page: nav/header/footer noise
// around a real article body.
const noisyArticle = `<!DOCTYPE html>
<html><head><title>泾县医院新闻正文</title></head>
<body>
<header><nav><a href="/index.asp">首页</a><a href="/ksjs.asp">科室介绍</a><a href="/yygk.asp">医院概况</a><a href="/ywbd.asp">预约就诊</a></nav></header>
<main>
<h1>我院开展护理技能竞赛活动</h1>
<p>为提升护理队伍整体素质，我院于近期举办了护理技能竞赛。全院各科室护理人员踊跃报名参加。</p>
<p>竞赛分为理论知识考核与操作技能展示两个环节，全面考察参赛人员的专业水平。</p>
<table><tr><td>联系电话：0563-5999000</td><td>地址：泾县桃花潭路</td></tr></table>
</main>
<footer>版权所有 泾县医院 · 备案号皖ICP备xxxx</footer>
</body></html>`

func TestExtractMainMarkdown_StripsNav(t *testing.T) {
	title, md, ok := ExtractMainMarkdown(noisyArticle, "http://www.ahjxyy.com/yydtxs.asp?newsid=1")
	if !ok {
		t.Fatal("extraction should succeed on an article page")
	}
	if title != "泾县医院新闻正文" {
		t.Errorf("title = %q", title)
	}
	for _, want := range []string{"护理技能竞赛", "理论知识考核"} {
		if !strings.Contains(md, want) {
			t.Errorf("main markdown missing %q: %s", want, md)
		}
	}
	// Body-embedded contact tables (inside <main>) are conservatively kept by
	// readability as content; only peripheral nav/footer noise must vanish.
	for _, noise := range []string{"科室介绍", "医院概况", "预约就诊", "版权所有", "备案号"} {
		if strings.Contains(md, noise) {
			t.Errorf("main markdown still contains noise %q: %s", noise, md)
		}
	}
}
