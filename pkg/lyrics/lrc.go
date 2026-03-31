package lyrics

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var lrcTimestampPattern = regexp.MustCompile(`\[(\d+):(\d{2})(?:[.:](\d{1,3}))?\]`)

// ParseLRC parses raw LRC text into a reusable Document.
func ParseLRC(raw string) Document {
	raw = normalizeLineEndings(raw)
	lines := strings.Split(raw, "\n")
	document := Document{SyncedLyrics: raw}
	plain := make([]string, 0, len(lines))

	for _, rawLine := range lines {
		line := strings.TrimRight(rawLine, "\n")
		matches := lrcTimestampPattern.FindAllStringSubmatchIndex(line, -1)
		if len(matches) == 0 {
			parseLRCMeta(line, &document)
			if strings.HasPrefix(strings.TrimSpace(line), "[") && strings.Contains(line, ":") && strings.HasSuffix(strings.TrimSpace(line), "]") {
				continue
			}
			plain = append(plain, strings.TrimSpace(line))
			continue
		}

		textStart := matches[len(matches)-1][1]
		text := strings.TrimSpace(line[textStart:])
		for _, match := range matches {
			start := parseLRCTimestamp(line[match[2]:match[3]], line[match[4]:match[5]], matchComponent(line, match, 6, 7))
			document.TimedLines = append(document.TimedLines, TimedLine{Start: start, Text: text})
		}
	}

	sort.SliceStable(document.TimedLines, func(i, j int) bool {
		return document.TimedLines[i].Start < document.TimedLines[j].Start
	})
	if len(document.TimedLines) == 0 {
		document.SyncedLyrics = ""
	}
	document.PlainLyrics = strings.Join(trimTrailingEmptyLines(plain), "\n")
	return document
}

func parseLRCMeta(line string, document *Document) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "[") || !strings.HasSuffix(trimmed, "]") {
		return
	}
	body := strings.TrimSuffix(strings.TrimPrefix(trimmed, "["), "]")
	parts := strings.SplitN(body, ":", 2)
	if len(parts) != 2 {
		return
	}
	key := strings.ToLower(strings.TrimSpace(parts[0]))
	value := strings.TrimSpace(parts[1])
	switch key {
	case "ti":
		document.TrackName = firstNonEmpty(document.TrackName, value)
	case "ar":
		document.ArtistName = firstNonEmpty(document.ArtistName, value)
	case "al":
		document.AlbumName = firstNonEmpty(document.AlbumName, value)
	}
}

func parseLRCTimestamp(minutesPart, secondsPart, fractionalPart string) time.Duration {
	minutes, _ := strconv.Atoi(strings.TrimSpace(minutesPart))
	seconds, _ := strconv.Atoi(strings.TrimSpace(secondsPart))
	total := time.Duration(minutes)*time.Minute + time.Duration(seconds)*time.Second
	fractionalPart = strings.TrimSpace(fractionalPart)
	if fractionalPart == "" {
		return total
	}
	switch len(fractionalPart) {
	case 1:
		fractionalPart += "00"
	case 2:
		fractionalPart += "0"
	case 3:
	default:
		fractionalPart = fractionalPart[:3]
	}
	millis, _ := strconv.Atoi(fractionalPart)
	return total + time.Duration(millis)*time.Millisecond
}

func matchComponent(line string, match []int, startIdx, endIdx int) string {
	if len(match) <= endIdx || match[startIdx] < 0 || match[endIdx] < 0 {
		return ""
	}
	return line[match[startIdx]:match[endIdx]]
}
