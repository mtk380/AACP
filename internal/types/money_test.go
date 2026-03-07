package types

import "testing"

func TestMoneyOps(t *testing.T) {
	a := Money{Currency: "CNY", Amount: "100.25"}
	b := Money{Currency: "CNY", Amount: "9.75"}
	sum, err := AddMoney(a, b, 2)
	if err != nil {
		t.Fatalf("add failed: %v", err)
	}
	if sum.Amount != "110.00" {
		t.Fatalf("unexpected sum %s", sum.Amount)
	}
	diff, err := SubMoney(a, b, 2)
	if err != nil {
		t.Fatalf("sub failed: %v", err)
	}
	if diff.Amount != "90.50" {
		t.Fatalf("unexpected diff %s", diff.Amount)
	}
	bps, err := ApplyBPS(a, 1200, 4)
	if err != nil {
		t.Fatalf("bps failed: %v", err)
	}
	if bps.Amount != "12.0300" {
		t.Fatalf("unexpected bps amount %s", bps.Amount)
	}
}
