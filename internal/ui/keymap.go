package ui

import (
	"strings"

	bubblekey "github.com/charmbracelet/bubbles/key"
	"github.com/darkliquid/musicon/pkg/components"
)

type KeybindOptions struct {
	Global   GlobalKeybindOptions
	Queue    QueueKeybindOptions
	Playback PlaybackKeybindOptions
}

type GlobalKeybindOptions struct {
	Quit       []string
	ToggleMode []string
	ToggleHelp []string
}

type QueueKeybindOptions struct {
	ToggleSearchFocus []string
	SourcePrev        []string
	SourceNext        []string
	FilterTracks      []string
	FilterStreams     []string
	FilterPlaylists   []string
	ActivateSelected  []string
	MoveSelectedUp    []string
	MoveSelectedDown  []string
	ClearQueue        []string
	RemoveSelected    []string
	BrowserUp         []string
	BrowserDown       []string
	BrowserHome       []string
	BrowserEnd        []string
	BrowserPageUp     []string
	BrowserPageDown   []string
}

type PlaybackKeybindOptions struct {
	CyclePane     []string
	ToggleInfo    []string
	ToggleRepeat  []string
	ToggleStream  []string
	TogglePause   []string
	PreviousTrack []string
	NextTrack     []string
	SeekBackward  []string
	SeekForward   []string
	VolumeDown    []string
	VolumeUp      []string
}

type KeyMap struct {
	Global   GlobalKeyMap
	Queue    QueueKeyMap
	Playback PlaybackKeyMap
}

type GlobalKeyMap struct {
	Quit       bubblekey.Binding
	ToggleMode bubblekey.Binding
	ToggleHelp bubblekey.Binding
}

type QueueKeyMap struct {
	ToggleSearchFocus bubblekey.Binding
	SourcePrev        bubblekey.Binding
	SourceNext        bubblekey.Binding
	FilterTracks      bubblekey.Binding
	FilterStreams     bubblekey.Binding
	FilterPlaylists   bubblekey.Binding
	ActivateSelected  bubblekey.Binding
	MoveSelectedUp    bubblekey.Binding
	MoveSelectedDown  bubblekey.Binding
	ClearQueue        bubblekey.Binding
	RemoveSelected    bubblekey.Binding
	Browser           components.ListKeyMap
}

type PlaybackKeyMap struct {
	CyclePane     bubblekey.Binding
	ToggleInfo    bubblekey.Binding
	ToggleRepeat  bubblekey.Binding
	ToggleStream  bubblekey.Binding
	TogglePause   bubblekey.Binding
	PreviousTrack bubblekey.Binding
	NextTrack     bubblekey.Binding
	SeekBackward  bubblekey.Binding
	SeekForward   bubblekey.Binding
	VolumeDown    bubblekey.Binding
	VolumeUp      bubblekey.Binding
}

func defaultKeybindOptions() KeybindOptions {
	return KeybindOptions{
		Global: GlobalKeybindOptions{
			Quit:       []string{"ctrl+c"},
			ToggleMode: []string{"tab"},
			ToggleHelp: []string{"?"},
		},
		Queue: QueueKeybindOptions{
			ToggleSearchFocus: []string{"ctrl+f"},
			SourcePrev:        []string{"["},
			SourceNext:        []string{"]"},
			FilterTracks:      []string{"1"},
			FilterStreams:     []string{"2"},
			FilterPlaylists:   []string{"3"},
			ActivateSelected:  []string{"enter"},
			MoveSelectedUp:    []string{"ctrl+k"},
			MoveSelectedDown:  []string{"ctrl+j"},
			ClearQueue:        []string{"ctrl+x"},
			RemoveSelected:    []string{"x"},
			BrowserUp:         []string{"up", "k"},
			BrowserDown:       []string{"down", "j"},
			BrowserHome:       []string{"home"},
			BrowserEnd:        []string{"end"},
			BrowserPageUp:     []string{"pgup"},
			BrowserPageDown:   []string{"pgdown"},
		},
		Playback: PlaybackKeybindOptions{
			CyclePane:     []string{"v"},
			ToggleInfo:    []string{"i"},
			ToggleRepeat:  []string{"r"},
			ToggleStream:  []string{"s"},
			TogglePause:   []string{"space"},
			PreviousTrack: []string{"["},
			NextTrack:     []string{"]"},
			SeekBackward:  []string{"left"},
			SeekForward:   []string{"right"},
			VolumeDown:    []string{"-"},
			VolumeUp:      []string{"=", "+"},
		},
	}
}

func normalizedKeyMap(options KeybindOptions) KeyMap {
	defaults := defaultKeybindOptions()

	global := GlobalKeyMap{
		Quit:       newBinding(options.Global.Quit, defaults.Global.Quit, "quit"),
		ToggleMode: newBinding(options.Global.ToggleMode, defaults.Global.ToggleMode, "switch mode"),
		ToggleHelp: newBinding(options.Global.ToggleHelp, defaults.Global.ToggleHelp, "toggle help"),
	}

	queue := QueueKeyMap{
		ToggleSearchFocus: newBinding(options.Queue.ToggleSearchFocus, defaults.Queue.ToggleSearchFocus, "focus or unfocus search"),
		SourcePrev:        newBinding(options.Queue.SourcePrev, defaults.Queue.SourcePrev, "previous source"),
		SourceNext:        newBinding(options.Queue.SourceNext, defaults.Queue.SourceNext, "next source"),
		FilterTracks:      newBinding(options.Queue.FilterTracks, defaults.Queue.FilterTracks, "toggle track filter"),
		FilterStreams:     newBinding(options.Queue.FilterStreams, defaults.Queue.FilterStreams, "toggle stream filter"),
		FilterPlaylists:   newBinding(options.Queue.FilterPlaylists, defaults.Queue.FilterPlaylists, "toggle playlist filter"),
		ActivateSelected:  newBinding(options.Queue.ActivateSelected, defaults.Queue.ActivateSelected, "toggle selected row"),
		MoveSelectedUp:    newBinding(options.Queue.MoveSelectedUp, defaults.Queue.MoveSelectedUp, "move queued item up"),
		MoveSelectedDown:  newBinding(options.Queue.MoveSelectedDown, defaults.Queue.MoveSelectedDown, "move queued item down"),
		ClearQueue:        newBinding(options.Queue.ClearQueue, defaults.Queue.ClearQueue, "clear queue"),
		RemoveSelected:    newBinding(options.Queue.RemoveSelected, defaults.Queue.RemoveSelected, "remove selected queued item"),
		Browser: components.ListKeyMap{
			Up:       newBinding(options.Queue.BrowserUp, defaults.Queue.BrowserUp, "move up"),
			Down:     newBinding(options.Queue.BrowserDown, defaults.Queue.BrowserDown, "move down"),
			Home:     newBinding(options.Queue.BrowserHome, defaults.Queue.BrowserHome, "jump to top"),
			End:      newBinding(options.Queue.BrowserEnd, defaults.Queue.BrowserEnd, "jump to bottom"),
			PageUp:   newBinding(options.Queue.BrowserPageUp, defaults.Queue.BrowserPageUp, "page up"),
			PageDown: newBinding(options.Queue.BrowserPageDown, defaults.Queue.BrowserPageDown, "page down"),
		},
	}

	playback := PlaybackKeyMap{
		CyclePane:     newBinding(options.Playback.CyclePane, defaults.Playback.CyclePane, "cycle pane"),
		ToggleInfo:    newBinding(options.Playback.ToggleInfo, defaults.Playback.ToggleInfo, "toggle track info"),
		ToggleRepeat:  newBinding(options.Playback.ToggleRepeat, defaults.Playback.ToggleRepeat, "toggle repeat"),
		ToggleStream:  newBinding(options.Playback.ToggleStream, defaults.Playback.ToggleStream, "toggle stream continuation"),
		TogglePause:   newBinding(options.Playback.TogglePause, defaults.Playback.TogglePause, "toggle play/pause"),
		PreviousTrack: newBinding(options.Playback.PreviousTrack, defaults.Playback.PreviousTrack, "previous track"),
		NextTrack:     newBinding(options.Playback.NextTrack, defaults.Playback.NextTrack, "next track"),
		SeekBackward:  newBinding(options.Playback.SeekBackward, defaults.Playback.SeekBackward, "seek backward"),
		SeekForward:   newBinding(options.Playback.SeekForward, defaults.Playback.SeekForward, "seek forward"),
		VolumeDown:    newBinding(options.Playback.VolumeDown, defaults.Playback.VolumeDown, "lower volume"),
		VolumeUp:      newBinding(options.Playback.VolumeUp, defaults.Playback.VolumeUp, "raise volume"),
	}

	return KeyMap{
		Global:   global,
		Queue:    queue,
		Playback: playback,
	}
}

func newBinding(keys, fallback []string, desc string) bubblekey.Binding {
	keys = normalizeBindingKeys(keys)
	if len(keys) == 0 {
		keys = normalizeBindingKeys(fallback)
	}
	return bubblekey.NewBinding(
		bubblekey.WithKeys(keys...),
		bubblekey.WithHelp(strings.Join(keys, " / "), desc),
	)
}

func normalizeBindingKeys(keys []string) []string {
	normalized := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		key = strings.ToLower(strings.TrimSpace(key))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, key)
	}
	return normalized
}

func bindingLabel(binding bubblekey.Binding) string {
	help := binding.Help()
	if help.Key != "" {
		return help.Key
	}
	return strings.Join(binding.Keys(), " / ")
}

func helpLine(binding bubblekey.Binding, desc string) string {
	return padKeyLabel(bindingLabel(binding), 18) + desc
}

func helpLinePair(left, right bubblekey.Binding, desc string) string {
	return padKeyLabel(bindingLabel(left)+" / "+bindingLabel(right), 18) + desc
}

func padKeyLabel(label string, width int) string {
	if width < 1 {
		return label
	}
	if len(label) >= width {
		return label + " "
	}
	return label + strings.Repeat(" ", width-len(label))
}
