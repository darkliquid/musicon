package main

import (
	"fmt"
	"os"

	"github.com/darkliquid/musicon/internal/audio"
	"github.com/darkliquid/musicon/internal/mpris"
	"github.com/darkliquid/musicon/internal/ui"
)

func main() {
	engine := audio.NewEngine(audio.Options{})
	defer engine.Close()

	playback := engine.PlaybackService()
	bridge := mpris.NewBridge(playback)
	if err := bridge.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "musicon: mpris unavailable: %v\n", err)
	} else {
		defer bridge.Close()
	}

	app := ui.NewApp(ui.Services{
		Queue:    engine.QueueService(),
		Playback: playback,
	})
	if err := ui.Run(app); err != nil {
		fmt.Fprintf(os.Stderr, "musicon: %v\n", err)
		os.Exit(1)
	}
}
