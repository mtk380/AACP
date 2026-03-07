package abci

import "aacp/internal/types"

type GasContext struct {
	Limit uint64
	Used  uint64
}

func NewGasContext(limit uint64) *GasContext {
	return &GasContext{Limit: limit}
}

func (g *GasContext) Consume(amount uint64, reason string) error {
	g.Used += amount
	if g.Used > g.Limit {
		return types.NewAppError(types.CodeOutOfGas, "out of gas during %s", reason)
	}
	return nil
}
