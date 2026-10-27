package tui

import (
	"net/http"
	"strconv"
	"strings"
)

func retryAfterText(header, fallback string) string {
	header = strings.TrimSpace(header)
	if seconds, err := strconv.ParseUint(header, 10, 64); err == nil {
		unit := "seconds"
		if seconds == 1 {
			unit = "second"
		}
		return "Rate limited. Try again in " + strconv.FormatUint(seconds, 10) + " " + unit + "."
	}
	if at, err := http.ParseTime(header); err == nil {
		return "Rate limited. Try again after " + at.UTC().Format("2 Jan 2006 15:04 MST") + "."
	}
	return fallback
}
