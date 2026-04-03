package mpris

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/darkliquid/musicon/internal/ui"
	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"
)

// This file contains the concrete D-Bus bridge that mirrors the playback
// service onto the standard MPRIS object model expected by desktop clients.

const (
	busName         = "org.mpris.MediaPlayer2.musicon"
	objectPath      = dbus.ObjectPath("/org/mpris/MediaPlayer2")
	rootInterface   = "org.mpris.MediaPlayer2"
	playerInterface = "org.mpris.MediaPlayer2.Player"
	trackPrefix     = "/org/mpris/MediaPlayer2/track/"
)

// Bridge projects Musicon playback state onto the MPRIS D-Bus interfaces.
type Bridge struct {
	mu sync.Mutex

	playback ui.PlaybackService

	conn  *dbus.Conn
	props *prop.Properties
	quit  chan struct{}
	wg    sync.WaitGroup
}

// NewBridge constructs an MPRIS bridge around the supplied playback service.
func NewBridge(playback ui.PlaybackService) *Bridge {
	return &Bridge{playback: playback}
}

// Start claims the Musicon MPRIS bus name and begins exporting properties.
func (b *Bridge) Start() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.conn != nil {
		return nil
	}

	conn, err := dbus.SessionBus()
	if err != nil {
		return err
	}

	reply, err := conn.RequestName(busName, dbus.NameFlagDoNotQueue)
	if err != nil {
		_ = conn.Close()
		return err
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		_ = conn.Close()
		return fmt.Errorf("mpris bus name unavailable: %s", reply.String())
	}

	if err := conn.Export(b, objectPath, rootInterface); err != nil {
		_ = conn.Close()
		return err
	}
	if err := conn.ExportMethodTable(b.playerMethods(), objectPath, playerInterface); err != nil {
		_ = conn.Close()
		return err
	}

	props, err := prop.Export(conn, objectPath, prop.Map{
		rootInterface:   b.rootProperties(),
		playerInterface: b.playerProperties(),
	})
	if err != nil {
		_ = conn.Close()
		return err
	}

	node := &introspect.Node{
		Name: string(objectPath),
		Interfaces: []introspect.Interface{
			introspect.IntrospectData,
			prop.IntrospectData,
			{
				Name:       rootInterface,
				Methods:    introspect.Methods(b),
				Properties: props.Introspection(rootInterface),
			},
			{
				Name:       playerInterface,
				Methods:    b.playerIntrospection(),
				Properties: props.Introspection(playerInterface),
			},
		},
	}
	if err := conn.Export(introspect.NewIntrospectable(node), objectPath, "org.freedesktop.DBus.Introspectable"); err != nil {
		_ = conn.Close()
		return err
	}

	b.conn = conn
	b.props = props
	b.quit = make(chan struct{})
	b.wg.Add(1)
	go b.refreshLoop()
	go b.refreshProperties()
	return nil
}

// Close stops background refresh work and releases the D-Bus connection.
func (b *Bridge) Close() error {
	b.mu.Lock()
	if b.conn == nil {
		b.mu.Unlock()
		return nil
	}
	conn := b.conn
	quit := b.quit
	b.conn = nil
	b.props = nil
	b.quit = nil
	b.mu.Unlock()

	close(quit)
	b.wg.Wait()
	return conn.Close()
}

// Raise implements the MPRIS root Raise method and currently no-ops because Musicon has no GUI window.
func (b *Bridge) Raise() *dbus.Error { return nil }

// Quit implements the MPRIS root Quit method and currently no-ops because process shutdown stays CLI-owned.
func (b *Bridge) Quit() *dbus.Error { return nil }

// Next implements the MPRIS Next transport method.
func (b *Bridge) Next() *dbus.Error { return b.call(b.playback.Next) }

// Previous implements the MPRIS Previous transport method.
func (b *Bridge) Previous() *dbus.Error { return b.call(b.playback.Previous) }

// PlayPause implements the MPRIS PlayPause transport method.
func (b *Bridge) playPause() *dbus.Error { return b.call(b.playback.TogglePause) }

// Pause implements the MPRIS Pause transport method.
func (b *Bridge) pause() *dbus.Error {
	snapshot := b.snapshot()
	if snapshot.Track == nil || snapshot.Paused {
		return nil
	}
	return b.call(b.playback.TogglePause)
}

// Play implements the MPRIS Play transport method.
func (b *Bridge) play() *dbus.Error {
	snapshot := b.snapshot()
	if snapshot.Track != nil && !snapshot.Paused {
		return nil
	}
	return b.call(b.playback.TogglePause)
}

// Stop implements the MPRIS Stop transport method.
func (b *Bridge) stop() *dbus.Error {
	snapshot := b.snapshot()
	if snapshot.Track == nil {
		return nil
	}
	if !snapshot.Paused {
		if err := b.playback.TogglePause(); err != nil {
			return dbus.MakeFailedError(err)
		}
	}
	if snapshot.Position == 0 {
		return nil
	}
	return nil
}

// OpenUri implements the MPRIS OpenUri method and currently no-ops because Musicon does not open external URIs directly.
func (b *Bridge) openURI(uri string) *dbus.Error {
	_ = uri
	return nil
}

func (b *Bridge) next() *dbus.Error { return b.call(b.playback.Next) }

func (b *Bridge) previous() *dbus.Error { return b.call(b.playback.Previous) }

func (b *Bridge) seek(offset int64) *dbus.Error {
	_ = offset
	return dbus.MakeFailedError(fmt.Errorf("seek is not supported"))
}

func (b *Bridge) setPosition(trackID dbus.ObjectPath, position int64) *dbus.Error {
	_, _ = trackID, position
	return dbus.MakeFailedError(fmt.Errorf("seek is not supported"))
}

func (b *Bridge) playerMethods() map[string]any {
	return map[string]any{
		"Next":        b.next,
		"Previous":    b.previous,
		"PlayPause":   b.playPause,
		"Pause":       b.pause,
		"Play":        b.play,
		"Stop":        b.stop,
		"Seek":        b.seek,
		"SetPosition": b.setPosition,
		"OpenUri":     b.openURI,
	}
}

func (b *Bridge) playerIntrospection() []introspect.Method {
	return []introspect.Method{
		{Name: "Next"},
		{Name: "Previous"},
		{Name: "PlayPause"},
		{
			Name: "Seek",
			Args: []introspect.Arg{
				{Name: "Offset", Type: "x", Direction: "in"},
			},
		},
		{Name: "Pause"},
		{Name: "Play"},
		{Name: "Stop"},
		{
			Name: "SetPosition",
			Args: []introspect.Arg{
				{Name: "TrackId", Type: "o", Direction: "in"},
				{Name: "Position", Type: "x", Direction: "in"},
			},
		},
		{
			Name: "OpenUri",
			Args: []introspect.Arg{
				{Name: "Uri", Type: "s", Direction: "in"},
			},
		},
	}
}

func (b *Bridge) rootProperties() map[string]*prop.Prop {
	return map[string]*prop.Prop{
		"CanQuit":             {Value: false, Writable: false, Emit: prop.EmitConst},
		"CanRaise":            {Value: false, Writable: false, Emit: prop.EmitConst},
		"HasTrackList":        {Value: false, Writable: false, Emit: prop.EmitConst},
		"Identity":            {Value: "Musicon", Writable: false, Emit: prop.EmitConst},
		"DesktopEntry":        {Value: "musicon", Writable: false, Emit: prop.EmitConst},
		"SupportedUriSchemes": {Value: []string{}, Writable: false, Emit: prop.EmitConst},
		"SupportedMimeTypes":  {Value: []string{}, Writable: false, Emit: prop.EmitConst},
	}
}

func (b *Bridge) playerProperties() map[string]*prop.Prop {
	return map[string]*prop.Prop{
		"PlaybackStatus": {Value: "Stopped", Writable: false, Emit: prop.EmitTrue},
		"LoopStatus": {
			Value:    "None",
			Writable: true,
			Emit:     prop.EmitTrue,
			Callback: func(c *prop.Change) *dbus.Error {
				status, _ := c.Value.(string)
				return b.call(func() error { return b.playback.SetRepeat(status == "Playlist") })
			},
		},
		"Rate":     {Value: 1.0, Writable: false, Emit: prop.EmitConst},
		"Shuffle":  {Value: false, Writable: false, Emit: prop.EmitConst},
		"Metadata": {Value: map[string]dbus.Variant{}, Writable: false, Emit: prop.EmitTrue},
		"Volume": {
			Value:    0.7,
			Writable: true,
			Emit:     prop.EmitTrue,
			Callback: func(c *prop.Change) *dbus.Error {
				value, _ := c.Value.(float64)
				target := int(value * 100)
				current := b.snapshot().Volume
				return b.call(func() error { return b.playback.AdjustVolume(target - current) })
			},
		},
		"Position":      {Value: int64(0), Writable: false, Emit: prop.EmitFalse},
		"MinimumRate":   {Value: 1.0, Writable: false, Emit: prop.EmitConst},
		"MaximumRate":   {Value: 1.0, Writable: false, Emit: prop.EmitConst},
		"CanGoNext":     {Value: false, Writable: false, Emit: prop.EmitTrue},
		"CanGoPrevious": {Value: false, Writable: false, Emit: prop.EmitTrue},
		"CanPlay":       {Value: true, Writable: false, Emit: prop.EmitTrue},
		"CanPause":      {Value: true, Writable: false, Emit: prop.EmitTrue},
		"CanSeek":       {Value: false, Writable: false, Emit: prop.EmitTrue},
		"CanControl":    {Value: true, Writable: false, Emit: prop.EmitConst},
		"CanStop":       {Value: true, Writable: false, Emit: prop.EmitConst},
	}
}

func (b *Bridge) refreshLoop() {
	defer b.wg.Done()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			b.refreshProperties()
		case <-b.quit:
			return
		}
	}
}

func (b *Bridge) refreshProperties() {
	b.mu.Lock()
	props := b.props
	b.mu.Unlock()
	if props == nil {
		return
	}

	snapshot := b.snapshot()
	props.SetMust(playerInterface, "PlaybackStatus", playbackStatus(snapshot))
	props.SetMust(playerInterface, "LoopStatus", loopStatus(snapshot))
	props.SetMust(playerInterface, "Metadata", metadata(snapshot))
	props.SetMust(playerInterface, "Volume", float64(snapshot.Volume)/100.0)
	props.SetMust(playerInterface, "Position", snapshot.Position.Microseconds())
	props.SetMust(playerInterface, "CanGoNext", snapshot.QueueLength > 0 && snapshot.QueueIndex < snapshot.QueueLength-1)
	props.SetMust(playerInterface, "CanGoPrevious", snapshot.QueueLength > 0 && snapshot.QueueIndex > 0)
	props.SetMust(playerInterface, "CanPlay", snapshot.QueueLength > 0)
	props.SetMust(playerInterface, "CanPause", snapshot.Track != nil)
	props.SetMust(playerInterface, "CanSeek", snapshot.Track != nil && snapshot.Duration > 0)
}

func (b *Bridge) snapshot() ui.PlaybackSnapshot {
	if b.playback == nil {
		return ui.PlaybackSnapshot{}
	}
	return b.playback.Snapshot()
}

func (b *Bridge) call(fn func() error) *dbus.Error {
	if b.playback == nil {
		return nil
	}
	if err := fn(); err != nil {
		return dbus.MakeFailedError(err)
	}
	go b.refreshProperties()
	return nil
}

func playbackStatus(snapshot ui.PlaybackSnapshot) string {
	if snapshot.Track == nil {
		return "Stopped"
	}
	if snapshot.Paused {
		return "Paused"
	}
	return "Playing"
}

func loopStatus(snapshot ui.PlaybackSnapshot) string {
	if snapshot.Repeat {
		return "Playlist"
	}
	return "None"
}

func metadata(snapshot ui.PlaybackSnapshot) map[string]dbus.Variant {
	if snapshot.Track == nil {
		return map[string]dbus.Variant{
			"mpris:trackid": dbus.MakeVariant(trackObjectPath("idle")),
		}
	}

	track := snapshot.Track
	data := map[string]dbus.Variant{
		"mpris:trackid": dbus.MakeVariant(trackObjectPath(track.ID)),
		"xesam:title":   dbus.MakeVariant(track.Title),
	}
	if track.Artist != "" {
		data["xesam:artist"] = dbus.MakeVariant([]string{track.Artist})
	}
	if track.Album != "" {
		data["xesam:album"] = dbus.MakeVariant(track.Album)
	}
	if track.Source != "" {
		data["xesam:comment"] = dbus.MakeVariant([]string{"Source: " + track.Source})
	}
	if snapshot.Duration > 0 {
		data["mpris:length"] = dbus.MakeVariant(snapshot.Duration.Microseconds())
	}
	return data
}

func trackObjectPath(id string) dbus.ObjectPath {
	clean := sanitizeObjectID(id)
	if clean == "" {
		clean = "unknown"
	}
	return dbus.ObjectPath(trackPrefix + clean)
}

func sanitizeObjectID(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return strings.Trim(b.String(), "_")
}
