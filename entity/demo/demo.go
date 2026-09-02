package demo

// Filter binds multi-value query parameters for /demo/search.
type Filter struct {
	Tags []string `query:"tag"`
}

// EchoReq binds a path parameter for /demo/echo/:str.
type EchoReq struct {
	Str string `param:"str"`
}
