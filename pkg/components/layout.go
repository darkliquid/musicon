package components

// SquareViewport describes the largest centered square that fits inside the
// current terminal dimensions.
type SquareViewport struct {
	TerminalWidth  int
	TerminalHeight int
	Size           int
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
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}

	size := width
	if height < size {
		size = height
	}

	return SquareViewport{
		TerminalWidth:  width,
		TerminalHeight: height,
		Size:           size,
	}
}

// Check computes the current viewport and evaluates it against explicit
// minimum-size requirements.
func (r SizeRequirements) Check(width, height int) SizeCheck {
	return SizeCheck{
		Requirements: r,
		Viewport:     ClampSquare(width, height),
	}
}

// Inner returns the content area inside a border of the given thickness.
func (v SquareViewport) Inner(border int) (int, int) {
	inner := v.Size - border*2
	if inner < 0 {
		inner = 0
	}
	return inner, inner
}
