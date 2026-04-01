package components

import (
	"fmt"
	"strconv"
	"strings"
)

// Theme describes the semantic color roles shared by Musicon UI renderers.
type Theme struct {
	Background     string `toml:"background"`
	Surface        string `toml:"surface"`
	SurfaceVariant string `toml:"surface_variant"`
	Primary        string `toml:"primary"`
	OnPrimary      string `toml:"on_primary"`
	Text           string `toml:"text"`
	TextMuted      string `toml:"text_muted"`
	TextSubtle     string `toml:"text_subtle"`
	Border         string `toml:"border"`
	Warning        string `toml:"warning"`
	OnWarning      string `toml:"on_warning"`
}

// DefaultTheme returns Musicon's built-in semantic palette.
func DefaultTheme() Theme {
	return Theme{
		Background:     "235",
		Surface:        "236",
		SurfaceVariant: "238",
		Primary:        "63",
		OnPrimary:      "230",
		Text:           "252",
		TextMuted:      "246",
		TextSubtle:     "244",
		Border:         "240",
		Warning:        "52",
		OnWarning:      "230",
	}
}

// Normalize trims theme values and fills any missing roles from the built-in palette.
func (t Theme) Normalize() Theme {
	normalized := DefaultTheme()

	if value := normalizeThemeColor(t.Background); value != "" {
		normalized.Background = value
	}
	if value := normalizeThemeColor(t.Surface); value != "" {
		normalized.Surface = value
	}
	if value := normalizeThemeColor(t.SurfaceVariant); value != "" {
		normalized.SurfaceVariant = value
	}
	if value := normalizeThemeColor(t.Primary); value != "" {
		normalized.Primary = value
	}
	if value := normalizeThemeColor(t.OnPrimary); value != "" {
		normalized.OnPrimary = value
	}
	if value := normalizeThemeColor(t.Text); value != "" {
		normalized.Text = value
	}
	if value := normalizeThemeColor(t.TextMuted); value != "" {
		normalized.TextMuted = value
	}
	if value := normalizeThemeColor(t.TextSubtle); value != "" {
		normalized.TextSubtle = value
	}
	if value := normalizeThemeColor(t.Border); value != "" {
		normalized.Border = value
	}
	if value := normalizeThemeColor(t.Warning); value != "" {
		normalized.Warning = value
	}
	if value := normalizeThemeColor(t.OnWarning); value != "" {
		normalized.OnWarning = value
	}

	return normalized
}

// Validate reports the first invalid theme role.
func (t Theme) Validate() error {
	normalized := t.Normalize()

	if err := validateThemeColor("background", normalized.Background); err != nil {
		return err
	}
	if err := validateThemeColor("surface", normalized.Surface); err != nil {
		return err
	}
	if err := validateThemeColor("surface_variant", normalized.SurfaceVariant); err != nil {
		return err
	}
	if err := validateThemeColor("primary", normalized.Primary); err != nil {
		return err
	}
	if err := validateThemeColor("on_primary", normalized.OnPrimary); err != nil {
		return err
	}
	if err := validateThemeColor("text", normalized.Text); err != nil {
		return err
	}
	if err := validateThemeColor("text_muted", normalized.TextMuted); err != nil {
		return err
	}
	if err := validateThemeColor("text_subtle", normalized.TextSubtle); err != nil {
		return err
	}
	if err := validateThemeColor("border", normalized.Border); err != nil {
		return err
	}
	if err := validateThemeColor("warning", normalized.Warning); err != nil {
		return err
	}
	if err := validateThemeColor("on_warning", normalized.OnWarning); err != nil {
		return err
	}

	return nil
}

func normalizeThemeColor(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "#") {
		return strings.ToLower(value)
	}
	return value
}

func validateThemeColor(role, value string) error {
	if value == "" {
		return fmt.Errorf("theme color %q is empty", role)
	}
	if after, ok := strings.CutPrefix(value, "#"); ok {
		hex := after
		if len(hex) != 3 && len(hex) != 6 {
			return fmt.Errorf("theme color %q must use #rgb or #rrggbb syntax, got %q", role, value)
		}
		for _, r := range hex {
			switch {
			case r >= '0' && r <= '9':
			case r >= 'a' && r <= 'f':
			case r >= 'A' && r <= 'F':
			default:
				return fmt.Errorf("theme color %q contains a non-hex digit: %q", role, value)
			}
		}
		return nil
	}

	index, err := strconv.Atoi(value)
	if err != nil || index < 0 || index > 255 {
		return fmt.Errorf("theme color %q must be an xterm index (0-255) or hex value, got %q", role, value)
	}
	return nil
}
