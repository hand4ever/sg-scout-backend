package response

type ErrMsg struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
	TraceID string `json:"trace_id"`
	Cost    string `json:"cost"`
	Extra   string `json:"extra,omitempty"`
}

type Code int

const (
	ErrCodeOk      Code = 0
	ErrCodeCustom  Code = 100001
	ErrCodeNetwork Code = 100002 // 网络
	ErrCodeDBDown  Code = 100003 // database unreachable
)
