package main

import (
	"fmt"
	"os"

	"github.com/darkliquid/musicon/internal/audio"
	"github.com/darkliquid/musicon/internal/ui"
)

func main() {
	engine := audio.NewEngine(audio.Options{})
	defer engine.Close()

	app := ui.NewApp(ui.Services{
		Queue:    engine.QueueService(),
		Playback: engine.PlaybackService(),
	})
	if err := ui.Run(app); err != nil {
		fmt.Fprintf(os.Stderr, "musicon: %v\n", err)
		os.Exit(1)
	}
}
