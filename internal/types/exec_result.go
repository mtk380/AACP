package types

// ExecTxResult mirrors ABCI execution output at app level.
type ExecTxResult struct {
	Code     uint32            `json:"code"`
	Log      string            `json:"log,omitempty"`
	GasUsed  uint64            `json:"gas_used,omitempty"`
	Events   []Event           `json:"events,omitempty"`
	Data     []byte            `json:"data,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type Event struct {
	Type       string            `json:"type"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

func SuccessResult(gasUsed uint64, events ...Event) *ExecTxResult {
	return &ExecTxResult{Code: CodeOK, GasUsed: gasUsed, Events: events}
}

func FailResult(code uint32, msg string) *ExecTxResult {
	return &ExecTxResult{Code: code, Log: msg}
}
