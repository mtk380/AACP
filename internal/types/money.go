package types

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
)

// Money uses a decimal string and exact rational arithmetic to avoid float drift.
type Money struct {
	Currency string `json:"currency"`
	Amount   string `json:"amount"`
}

func (m Money) Validate() error {
	if m.Currency == "" {
		return errors.New("currency is required")
	}
	if _, err := ParseDecimal(m.Amount); err != nil {
		return fmt.Errorf("invalid amount: %w", err)
	}
	return nil
}

func ParseDecimal(s string) (*big.Rat, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, errors.New("empty decimal")
	}
	r := new(big.Rat)
	if _, ok := r.SetString(s); !ok {
		return nil, fmt.Errorf("cannot parse %q", s)
	}
	return r, nil
}

func FormatDecimal(r *big.Rat, scale int) string {
	if r == nil {
		return "0"
	}
	if scale < 0 {
		scale = 0
	}
	return r.FloatString(scale)
}

func AddMoney(a, b Money, scale int) (Money, error) {
	if a.Currency != b.Currency {
		return Money{}, errors.New("currency mismatch")
	}
	ra, err := ParseDecimal(a.Amount)
	if err != nil {
		return Money{}, err
	}
	rb, err := ParseDecimal(b.Amount)
	if err != nil {
		return Money{}, err
	}
	r := new(big.Rat).Add(ra, rb)
	return Money{Currency: a.Currency, Amount: FormatDecimal(r, scale)}, nil
}

func SubMoney(a, b Money, scale int) (Money, error) {
	if a.Currency != b.Currency {
		return Money{}, errors.New("currency mismatch")
	}
	ra, err := ParseDecimal(a.Amount)
	if err != nil {
		return Money{}, err
	}
	rb, err := ParseDecimal(b.Amount)
	if err != nil {
		return Money{}, err
	}
	r := new(big.Rat).Sub(ra, rb)
	if r.Sign() < 0 {
		return Money{}, errors.New("negative result")
	}
	return Money{Currency: a.Currency, Amount: FormatDecimal(r, scale)}, nil
}

func ApplyBPS(m Money, bps uint32, scale int) (Money, error) {
	r, err := ParseDecimal(m.Amount)
	if err != nil {
		return Money{}, err
	}
	factor := new(big.Rat).SetFrac64(int64(bps), 10000)
	res := new(big.Rat).Mul(r, factor)
	return Money{Currency: m.Currency, Amount: FormatDecimal(res, scale)}, nil
}
