//go:build !iavl

package state

import "fmt"

func newIAVLEngine() (engine, error) {
	return nil, fmt.Errorf("iavl backend requires build tag 'iavl'")
}
