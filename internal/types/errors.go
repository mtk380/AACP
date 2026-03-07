package types

import "fmt"

const (
	CodeOK = uint32(0)

	CodeDecodeFail   = uint32(1)
	CodeBadSignature = uint32(2)
	CodeUnknownMod   = uint32(3)
	CodeBadNonce     = uint32(4)
	CodeOutOfGas     = uint32(5)

	CodeAMXBase     = uint32(10)
	CodeAAPBase     = uint32(20)
	CodeWEAVEBase   = uint32(30)
	CodeCapUTXOBase = uint32(40)
	CodeREPBase     = uint32(50)
	CodeARBBase     = uint32(60)
	CodeFIATBase    = uint32(70)
	CodeNodeBase    = uint32(80)
	CodeGovBase     = uint32(90)
	CodeAFDBase     = uint32(100)
)

type AppError struct {
	Code uint32
	Msg  string
}

func (e AppError) Error() string {
	return fmt.Sprintf("code=%d msg=%s", e.Code, e.Msg)
}

func NewAppError(code uint32, format string, args ...any) error {
	return AppError{Code: code, Msg: fmt.Sprintf(format, args...)}
}

func CodeFromError(err error, fallback uint32) uint32 {
	if err == nil {
		return CodeOK
	}
	if ae, ok := err.(AppError); ok {
		return ae.Code
	}
	return fallback
}
