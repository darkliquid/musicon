package components

import "math"

// SquareViewport describes the largest centered square that fits inside the
// current terminal dimensions. Width and Height are terminal-cell dimensions
// chosen to approximate a visual 1:1 square when terminal cells are not
// physically square.
type SquareViewport struct {
	TerminalWidth  int
	TerminalHeight int
	Width          int
	Height         int
	Size           int
	CellWidthRatio float64
}

// SizeRequirements describes explicit terminal and viewport minimums.
type SizeRequirements struct {
	MinWidth  int
	MinHeight int
	MinSquare int
}

// SizeCheck reports how the current terminal dimensions compare to the
// requested minimums.
type SizeCheck struct {
	Requirements SizeRequirements
	Viewport     SquareViewport
}

// Fits reports whether the terminal satisfies all declared minimums.
func (c SizeCheck) Fits() bool {
	return c.Viewport.TerminalWidth >= c.Requirements.MinWidth &&
		c.Viewport.TerminalHeight >= c.Requirements.MinHeight &&
		c.Viewport.Size >= c.Requirements.MinSquare
}

// MissingWidth reports how many columns are still required to satisfy the
// configured minimum width.
func (c SizeCheck) MissingWidth() int {
	missing := c.Requirements.MinWidth - c.Viewport.TerminalWidth
	if missing < 0 {
		return 0
	}
	return missing
}

// MissingHeight reports how many rows are still required to satisfy the
// configured minimum height.
func (c SizeCheck) MissingHeight() int {
	missing := c.Requirements.MinHeight - c.Viewport.TerminalHeight
	if missing < 0 {
		return 0
	}
	return missing
}

// MissingSquare reports how many cells are still required to satisfy the
// configured square-viewport minimum.
func (c SizeCheck) MissingSquare() int {
	missing := c.Requirements.MinSquare - c.Viewport.Size
	if missing < 0 {
		return 0
	}
	return missing
}

// ClampSquare computes the usable square viewport from raw terminal dimensions.
func ClampSquare(width, height int) SquareViewport {
	return ClampSquareWithCellWidthRatio(width, height, 1)
}

// ClampSquareWithCellWidthRatio computes the usable visually square viewport
// from raw terminal dimensions and a terminal cell width-to-height ratio.
// A ratio below 1 means cells are taller than they are wide, so the viewport
// uses more columns than rows to remain visually square.
func ClampSquareWithCellWidthRatio(width, height int, cellWidthRatio float64) SquareViewport {
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}
	if cellWidthRatio <= 0 {
		cellWidthRatio = 1
	}

	size := height
	widthLimitedSize := int(math.Floor(float64(width) * cellWidthRatio))
	if widthLimitedSize < size {
		size = widthLimitedSize
	}
	if size < 0 {
		size = 0
	}

	viewWidth := 0
	if size > 0 {
		viewWidth = int(math.Round(float64(size) / cellWidthRatio))
	}
	if viewWidth > width {
		viewWidth = width
		size = int(math.Round(float64(viewWidth) * cellWidthRatio))
	}
	if size > height {
		size = height
	}
	viewHeight := size

	return SquareViewport{
		TerminalWidth:  width,
		TerminalHeight: height,
		Width:          viewWidth,
		Height:         viewHeight,
		Size:           size,
		CellWidthRatio: cellWidthRatio,
	}
}

// Check computes the current viewport and evaluates it against explicit
// minimum-size requirements.
func (r SizeRequirements) Check(width, height int) SizeCheck {
	return r.CheckWithCellWidthRatio(width, height, 1)
}

// CheckWithCellWidthRatio computes the current viewport and evaluates it
// against explicit minimum-size requirements while accounting for non-square
// terminal cells.
func (r SizeRequirements) CheckWithCellWidthRatio(width, height int, cellWidthRatio float64) SizeCheck {
	return SizeCheck{
		Requirements: r,
		Viewport:     ClampSquareWithCellWidthRatio(width, height, cellWidthRatio),
	}
}

// Inner returns the content area inside a border of the given thickness.
func (v SquareViewport) Inner(border int) (int, int) {
	innerWidth := max(v.Width-border*2, 0)
	innerHeight := max(v.Height-border*2, 0)
	return innerWidth, innerHeight
}
