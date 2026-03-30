package main

import (
	"bytes"
	"errors"
	"testing"
)

func TestPrintUsableBackendsWritesOnePerLine(t *testing.T) {
	var out bytes.Buffer
	err := printUsableBackends(&out, "", func() ([]string, error) {
		return []string{"auto", "alsa", "jack"}, nil
	})
	if err != nil {
		t.Fatalf("print usable backends failed: %v", err)
	}

	if got := out.String(); got != "auto\nalsa\njack\n" {
		t.Fatalf("unexpected backend listing: %q", got)
	}
}

func TestPrintUsableBackendsMarksSelectedBackend(t *testing.T) {
	var out bytes.Buffer
	err := printUsableBackends(&out, "alsa", func() ([]string, error) {
		return []string{"auto", "alsa", "jack"}, nil
	})
	if err != nil {
		t.Fatalf("print usable backends failed: %v", err)
	}

	if got := out.String(); got != "auto\nalsa [selected]\njack\n" {
		t.Fatalf("unexpected backend listing: %q", got)
	}
}

func TestPrintUsableBackendsReturnsListingError(t *testing.T) {
	want := errors.New("boom")
	err := printUsableBackends(&bytes.Buffer{}, "", func() ([]string, error) {
		return nil, want
	})
	if !errors.Is(err, want) {
		t.Fatalf("expected %v, got %v", want, err)
	}
}
