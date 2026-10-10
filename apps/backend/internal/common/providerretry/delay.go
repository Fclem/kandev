// Package providerretry extracts allowlisted retry timing without retaining provider data.
package providerretry

import (
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const maxMilliseconds = math.MaxInt64 / int64(time.Millisecond)
const maxTextBytes = 4096

var millisecondText = regexp.MustCompile(`(?i)\bretry-after-ms[=:\s]+([0-9]{1,12})(?:\s|[,;]|$)`)
var secondsText = regexp.MustCompile(`(?i)(?:\bPlease retry in\s+|\bretryDelay["\s:=]+)([0-9]+(?:\.[0-9]+)?)s(?:["\s,;.]|$)`)

func scalar(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case json.Number:
		return string(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(v, 10)
	case int:
		return strconv.Itoa(v)
	default:
		return ""
	}
}

// Milliseconds rejects fractional, nonpositive, and unrepresentable delays.
func Milliseconds(value any) *int64 {
	text := scalar(value)
	for _, ch := range text {
		if ch < '0' || ch > '9' {
			return nil
		}
	}
	number, err := strconv.ParseInt(text, 10, 64)
	if err != nil || number <= 0 || number > maxMilliseconds {
		return nil
	}
	return &number
}

func seconds(value any, suffix bool) *int64 {
	text := scalar(value)
	if suffix {
		if !strings.HasSuffix(text, "s") {
			return nil
		}
		text = strings.TrimSuffix(text, "s")
	}
	if text == "" {
		return nil
	}
	for _, ch := range text {
		if (ch < '0' || ch > '9') && ch != '.' {
			return nil
		}
	}
	duration, err := time.ParseDuration(text + "s")
	if err != nil || duration <= 0 || duration%time.Millisecond != 0 {
		return nil
	}
	number := int64(duration / time.Millisecond)
	return &number
}

// FromData gives all millisecond fields precedence over all seconds fields.
func FromData(data any) *int64 {
	fields, ok := data.(map[string]any)
	if !ok {
		return nil
	}
	headers, _ := fields["headers"].(map[string]any)
	for _, m := range []map[string]any{fields, headers} {
		for _, key := range []string{"retry_after_ms", "retryAfterMs", "retry-after-ms"} {
			if delay := Milliseconds(m[key]); delay != nil {
				return delay
			}
		}
	}
	for _, m := range []map[string]any{fields, headers} {
		if delay := seconds(m["retry-after"], false); delay != nil {
			return delay
		}
		if delay := seconds(m["retryDelay"], true); delay != nil {
			return delay
		}
	}
	return nil
}

// FromText recognizes only bounded provider delay notices.
func FromText(text string) *int64 {
	if len(text) > maxTextBytes {
		return nil
	}
	if match := millisecondText.FindStringSubmatch(text); match != nil {
		if delay := Milliseconds(match[1]); delay != nil {
			return delay
		}
	}
	if match := secondsText.FindStringSubmatch(text); match != nil {
		return seconds(match[1], false)
	}
	return nil
}

// Duration validates again at the stream consumption boundary.
func Duration(milliseconds *int64) time.Duration {
	if milliseconds == nil || *milliseconds <= 0 || *milliseconds > maxMilliseconds {
		return 0
	}
	return time.Duration(*milliseconds) * time.Millisecond
}
