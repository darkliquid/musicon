package components

import (
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// ListKeyMap defines the navigation bindings used by List.
type ListKeyMap struct {
	Up       key.Binding
	Down     key.Binding
	Home     key.Binding
	End      key.Binding
	PageUp   key.Binding
	PageDown key.Binding
}

// DefaultListKeyMap returns the built-in navigation bindings for List.
func DefaultListKeyMap() ListKeyMap {
	return ListKeyMap{
		Up:       key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("up / k", "move up")),
		Down:     key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("down / j", "move down")),
		Home:     key.NewBinding(key.WithKeys("home"), key.WithHelp("home", "jump to top")),
		End:      key.NewBinding(key.WithKeys("end"), key.WithHelp("end", "jump to bottom")),
		PageUp:   key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup", "page up")),
		PageDown: key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("pgdown", "page down")),
	}
}

// ListItem is a generic single-row list entry.
type ListItem struct {
	Leading  string
	Title    string
	Subtitle string
	Meta     string
}

// listEntry wraps a ListItem to satisfy the bubbles list.Item interface.
type listEntry struct {
	item ListItem
}

func (e listEntry) FilterValue() string { return e.item.Title }

// listDelegate renders list rows with prefix marker, leading indicator,
// title+subtitle, right-aligned meta, and focus/selection styling.
type listDelegate struct {
	focused bool
}

func (d *listDelegate) Height() int                             { return 1 }
func (d *listDelegate) Spacing() int                            { return 0 }
func (d *listDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d *listDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	entry, ok := item.(listEntry)
	if !ok {
		return
	}
	it := entry.item
	width := m.Width()

	anchoredPrefix := ""
	if it.Leading != "" {
		anchoredPrefix = it.Leading + " "
	}
	scrollLabel := it.Title
	if it.Subtitle != "" {
		scrollLabel += " — " + it.Subtitle
	}
	label := anchoredPrefix + scrollLabel

	const prefixWidth = 2
	metaWidth := lipgloss.Width(it.Meta)
	leftWidth := width - prefixWidth
	if metaWidth > 0 {
		leftWidth = width - prefixWidth - metaWidth - 1
	}
	if leftWidth < 1 {
		leftWidth = 1
	}

	selected := index == m.Index()
	leftLabel := truncate(label, leftWidth)
	if selected && d.focused && lipgloss.Width(label) > leftWidth {
		leftLabel = anchoredMarquee(anchoredPrefix, scrollLabel, leftWidth, marqueeStep())
	}
	left := lipgloss.NewStyle().Width(leftWidth).Render(leftLabel)
	row := left
	if metaWidth > 0 {
		meta := lipgloss.NewStyle().Width(metaWidth).Align(lipgloss.Right).Foreground(lipgloss.Color("246")).Render(it.Meta)
		row = lipgloss.JoinHorizontal(lipgloss.Left, left, " ", meta)
	}

	prefix := "  "
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	if selected {
		prefix = "▸ "
		style = style.Bold(true)
		if d.focused {
			style = style.Foreground(lipgloss.Color("230")).Background(lipgloss.Color("63"))
		} else {
			style = style.Foreground(lipgloss.Color("255")).Background(lipgloss.Color("238"))
		}
	} else if !d.focused {
		style = style.Faint(true)
	}

	fmt.Fprint(w, style.Width(width).Render(truncate(prefix+row, width)))
}

// List is a reusable selectable list widget backed by charm.land/bubbles/v2/list.
type List struct {
	inner      list.Model
	delegate   *listDelegate
	keymap     ListKeyMap
	itemCount  int
	emptyTitle string
	emptyBody  string
	width      int
	height     int
}

// NewList constructs a selectable list with default sizing and empty-state messaging.
func NewList() List {
	d := &listDelegate{}
	km := DefaultListKeyMap()
	inner := list.New(nil, d, 20, 5)
	applyKeyMap(&inner, km)

	return List{
		inner:      inner,
		delegate:   d,
		keymap:     km,
		width:      20,
		height:     5,
		emptyTitle: "Nothing here",
		emptyBody:  "No items are available in this panel yet.",
	}
}

// applyKeyMap maps ListKeyMap bindings to the inner model and disables all
// built-in chrome and unwanted key bindings.
func applyKeyMap(m *list.Model, km ListKeyMap) {
	m.SetShowTitle(false)
	m.SetShowFilter(false)
	m.SetShowStatusBar(false)
	m.SetShowHelp(false)
	m.SetShowPagination(false)
	m.SetFilteringEnabled(false)
	m.DisableQuitKeybindings()
	m.InfiniteScrolling = false
	m.KeyMap = list.KeyMap{
		CursorUp:             km.Up,
		CursorDown:           km.Down,
		GoToStart:            km.Home,
		GoToEnd:              km.End,
		PrevPage:             km.PageUp,
		NextPage:             km.PageDown,
		Filter:               key.NewBinding(),
		ClearFilter:          key.NewBinding(),
		CancelWhileFiltering: key.NewBinding(),
		AcceptWhileFiltering: key.NewBinding(),
		ShowFullHelp:         key.NewBinding(),
		CloseFullHelp:        key.NewBinding(),
		Quit:                 key.NewBinding(),
		ForceQuit:            key.NewBinding(),
	}
}

// SetItems replaces the list contents while preserving a valid selection index.
func (l *List) SetItems(items []ListItem) {
	entries := make([]list.Item, len(items))
	for i, it := range items {
		entries[i] = listEntry{item: it}
	}
	l.inner.SetItems(entries)
	l.itemCount = len(items)
}

// SetSize updates the list's render bounds.
func (l *List) SetSize(width, height int) {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	l.width = width
	l.height = height
	l.inner.SetSize(width, height)
}

// SetFocused toggles focused-row styling.
func (l *List) SetFocused(focused bool) {
	l.delegate.focused = focused
}

// SetKeyMap replaces the navigation bindings used by Update.
func (l *List) SetKeyMap(keymap ListKeyMap) {
	l.keymap = keymap
	applyKeyMap(&l.inner, keymap)
}

// SetEmptyState configures the placeholder shown when the list has no items.
func (l *List) SetEmptyState(title, body string) {
	l.emptyTitle = title
	l.emptyBody = body
}

// Update applies navigation input and reports whether selection changed.
func (l *List) Update(msg tea.Msg) bool {
	if _, ok := msg.(tea.KeyPressMsg); !ok {
		return false
	}
	if l.itemCount == 0 {
		return false
	}
	prev := l.inner.Index()
	m, _ := l.inner.Update(msg)
	l.inner = m
	return l.inner.Index() != prev
}

// SelectedIndex reports the currently selected row index.
func (l List) SelectedIndex() int {
	return l.inner.Index()
}

// SetSelectedIndex moves selection to index after clamping it into range.
func (l *List) SetSelectedIndex(index int) {
	l.inner.Select(index)
}

// View renders the visible portion of the list or its empty state.
func (l List) View() string {
	if l.width <= 0 || l.height <= 0 {
		return ""
	}
	if l.itemCount == 0 {
		return RenderEmptyState(l.width, l.height, l.emptyTitle, l.emptyBody)
	}
	return lipgloss.NewStyle().Width(l.width).Height(l.height).Render(l.inner.View())
}

func truncate(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(value) <= width {
		return value
	}
	if width == 1 {
		return "…"
	}
	return ansi.Truncate(value, width-1, "…")
}

func marqueeStep() int {
	return int(time.Now().UnixMilli() / 250)
}

func marquee(value string, width, step int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(value) <= width {
		return value
	}

	base := []rune(value)
	gap := []rune("   ")
	loop := append(append(append([]rune(nil), base...), gap...), base...)
	if len(loop) == 0 {
		return ""
	}

	offset := step % (len(base) + len(gap))
	var b strings.Builder
	remaining := width
	for i := offset; i < len(loop) && remaining > 0; i++ {
		r := loop[i]
		rw := runeWidth(r)
		if rw > remaining {
			break
		}
		b.WriteRune(r)
		remaining -= rw
	}
	return lipgloss.NewStyle().Width(width).Render(b.String())
}

func anchoredMarquee(prefix, value string, width, step int) string {
	prefixWidth := lipgloss.Width(prefix)
	if prefixWidth >= width {
		return truncate(prefix, width)
	}
	if lipgloss.Width(prefix)+lipgloss.Width(value) <= width {
		return prefix + value
	}
	return prefix + marquee(value, width-prefixWidth, step)
}

func runeWidth(r rune) int {
	if r == 0 || r == '\n' {
		return 0
	}
	if r < utf8.RuneSelf {
		return 1
	}
	return lipgloss.Width(string(r))
}
