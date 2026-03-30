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

func TestClampSquareWithCellWidthRatioUsesMoreColumnsForTallCells(t *testing.T) {
	viewport := ClampSquareWithCellWidthRatio(120, 40, 0.5)

	if viewport.Width != 80 || viewport.Height != 40 {
		t.Fatalf("expected 80x40 viewport, got %#v", viewport)
	}
	if viewport.Size != 40 {
		t.Fatalf("expected visual square size 40, got %d", viewport.Size)
	}

	innerWidth, innerHeight := viewport.Inner(1)
	if innerWidth != 78 || innerHeight != 38 {
		t.Fatalf("expected inner 78x38, got %dx%d", innerWidth, innerHeight)
	}
}

func TestSizeRequirementsCheckWithCellWidthRatioFailsWhenWidthCannotSupportVisualSquare(t *testing.T) {
	reqs := SizeRequirements{MinWidth: 20, MinHeight: 20, MinSquare: 20}

	check := reqs.CheckWithCellWidthRatio(30, 40, 0.5)
	if check.Fits() {
		t.Fatalf("expected width-limited visual square to fail, got %#v", check)
	}
	if got := check.MissingSquare(); got != 5 {
		t.Fatalf("expected missing square 5, got %d", got)
	}
}
