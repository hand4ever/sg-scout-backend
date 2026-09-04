package proofread

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"sg.scout/config"
)

// LLM engine support (research D6): OpenAI-compatible chat/completions over
// stdlib net/http (0 new dependencies). The provider's model is asked to
// return a JSON object {"items":[...]} of candidates; the response is parsed
// strictly — a malformed payload fails THIS engine only (FR-013 isolation).

// llmItem is one candidate parsed from an LLM response.
type llmItem struct {
	OpType      string `json:"op_type"`
	OrigText    string `json:"orig_text"`
	Replacement string `json:"replacement"`
	Reason      string `json:"reason"`
}

// systemPrompt instructs the model to output the candidate JSON contract.
const systemPrompt = "你是一名专业的中文文字校对助手。检查用户提供的全文，找出错别字、标点误用、用词不当、语法问题。输出 JSON 对象：{\"items\": [{\"op_type\": \"fix\", \"orig_text\": \"原文中的问题文本（必须与原文完全一致的子串）\", \"replacement\": \"修正后文本\", \"reason\": \"问题说明\"}]}。规则：1) 只输出 JSON，不要任何其他文字；2) op_type 只能是 fix（改正）或 replace（整句/整段替换）或 delete（删除）；3) orig_text 必须是原文中真实存在的连续文本；4) 每处问题一条，最多 20 条；5) 不要改写无问题部分。"

// llmResponse mirrors the chat/completions success payload subset.
type llmResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// ParseLLMResponse extracts candidates from the model's content JSON.
func ParseLLMResponse(content string) ([]Candidate, error) {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)
	var resp struct {
		Items []llmItem `json:"items"`
	}
	if err := json.Unmarshal([]byte(content), &resp); err != nil {
		return nil, fmt.Errorf("模型输出不是合法 JSON：%v", err)
	}
	out := make([]Candidate, 0, len(resp.Items))
	for _, it := range resp.Items {
		out = append(out, Candidate{
			OpType: strings.TrimSpace(it.OpType), OrigText: it.OrigText,
			Replacement: it.Replacement, Reason: strings.TrimSpace(it.Reason),
		})
	}
	return out, nil
}

// RunLLMEngine calls the configured provider once for the whole draft text and
// returns candidates. Errors are engine-fatal (returned to the run record).
// Text above maxChars is rejected before the call (research D13).
//
// V4-era calling convention (deepseek-v4-pro/flash, 2026-04+): thinking mode is
// controlled by the `thinking` + `reasoning_effort` params, NOT temperature
// (temperature is ignored while thinking is enabled). effort:
//   - none (default) → thinking disabled (fast; temperature=0.1)
//   - low/high/max   → thinking enabled with graded effort
func RunLLMEngine(provider, model, draft, effort string) ([]Candidate, error) {
	p, ok := config.Cfg.Proofread.Providers[provider]
	if !ok {
		return nil, fmt.Errorf("校对服务 %q 未在 config.toml 配置", provider)
	}
	timeout := p.TimeoutS
	if timeout <= 0 {
		timeout = 120
	}
	effort = strings.ToLower(strings.TrimSpace(effort))
	thinking := map[string]any{"type": "enabled"}
	if effort == "" || effort == "none" || effort == "off" {
		// Default: thinking OFF (user preference 2026-09-04) — fast + stable.
		thinking = map[string]any{"type": "disabled"}
		effort = "" // temperature applies only outside thinking mode
	}
	baseURL := strings.TrimRight(p.BaseURL, "/")
	payload := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": draft},
		},
		"thinking":   thinking,
		"max_tokens": 4000, // thinking consumes budget; cap high enough (v4 probe)
	}
	if effort != "" {
		payload["reasoning_effort"] = effort
	} else {
		payload["temperature"] = 0.1
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	client := &http.Client{Timeout: time.Duration(timeout) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用校对服务失败：%v", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取校对服务响应失败：%v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("校对服务返回 %d：%s", resp.StatusCode, truncateCN(string(raw), 200))
	}
	var lr llmResponse
	if err := json.Unmarshal(raw, &lr); err != nil {
		return nil, fmt.Errorf("校对服务响应解析失败：%v", err)
	}
	if len(lr.Choices) == 0 {
		return nil, fmt.Errorf("校对服务返回空结果（无 choices）")
	}
	return ParseLLMResponse(lr.Choices[0].Message.Content)
}

// truncateCN limits a string for error messages.
func truncateCN(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
