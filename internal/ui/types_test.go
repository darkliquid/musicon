package ui

import (
	"testing"

	"github.com/darkliquid/musicon/pkg/coverart"
)

func TestTrackInfoCoverArtMetadataMergesDisplayFields(t *testing.T) {
	embedded := &coverart.Image{Data: []byte("img"), MIMEType: "image/jpeg"}
	track := TrackInfo{
		Title:  "Song",
		Artist: "Artist",
		Album:  "Album",
		Artwork: coverart.Metadata{
			IDs: coverart.IDs{
				SpotifyTrackID: "track-id",
			},
			Local: &coverart.LocalMetadata{
				AudioPath: "/music/song.mp3",
				Embedded:  embedded,
			},
		},
	}

	got := track.CoverArtMetadata()
	if got.Title != "Song" || got.Artist != "Artist" || got.Album != "Album" {
		t.Fatalf("expected display fields merged into artwork metadata, got %#v", got)
	}
	if got.IDs.SpotifyTrackID != "track-id" {
		t.Fatalf("expected spotify track id preserved, got %#v", got.IDs)
	}
	if got.Local == nil || got.Local.AudioPath != "/music/song.mp3" || got.Local.Embedded != embedded {
		t.Fatalf("expected local metadata preserved, got %#v", got.Local)
	}
}
