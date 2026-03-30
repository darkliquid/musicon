package main

import (
	"bytes"
	"errors"
	"testing"
)

func TestPrintSelectedOptionsWritesOnePerLine(t *testing.T) {
	var out bytes.Buffer
	err := printSelectedOptions(&out, "", func() ([]string, error) {
		return []string{"auto", "alsa", "jack"}, nil
	})
	if err != nil {
		t.Fatalf("print selected options failed: %v", err)
	}

	if got := out.String(); got != "auto\nalsa\njack\n" {
		t.Fatalf("unexpected backend listing: %q", got)
	}
}

func TestPrintSelectedOptionsMarksSelectedOption(t *testing.T) {
	var out bytes.Buffer
	err := printSelectedOptions(&out, "alsa", func() ([]string, error) {
		return []string{"auto", "alsa", "jack"}, nil
	})
	if err != nil {
		t.Fatalf("print selected options failed: %v", err)
	}

	if got := out.String(); got != "auto\nalsa [selected]\njack\n" {
		t.Fatalf("unexpected backend listing: %q", got)
	}
}

func TestPrintSelectedOptionsReturnsListingError(t *testing.T) {
	want := errors.New("boom")
	err := printSelectedOptions(&bytes.Buffer{}, "", func() ([]string, error) {
		return nil, want
	})
	if !errors.Is(err, want) {
		t.Fatalf("expected %v, got %v", want, err)
	}
}
