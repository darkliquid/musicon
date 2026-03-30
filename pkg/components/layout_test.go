package components

import "testing"

func TestSizeRequirementsCheckFits(t *testing.T) {
	reqs := SizeRequirements{MinWidth: 20, MinHeight: 20, MinSquare: 20}

	check := reqs.Check(24, 30)
	if !check.Fits() {
		t.Fatalf("expected size check to fit, got %#v", check)
	}
}

func TestSizeRequirementsCheckMissingValues(t *testing.T) {
	reqs := SizeRequirements{MinWidth: 20, MinHeight: 20, MinSquare: 20}

	check := reqs.Check(18, 16)
	if check.Fits() {
		t.Fatalf("expected size check to fail, got %#v", check)
	}
	if got := check.MissingWidth(); got != 2 {
		t.Fatalf("expected missing width 2, got %d", got)
	}
	if got := check.MissingHeight(); got != 4 {
		t.Fatalf("expected missing height 4, got %d", got)
	}
	if got := check.MissingSquare(); got != 4 {
		t.Fatalf("expected missing square 4, got %d", got)
	}
}
