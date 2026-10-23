package auth

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
)

func portString(port int) string { return strconv.Itoa(port) }

func parseBrowserURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || strings.ContainsAny(raw, "\r\n\x00") {
		return "", errors.New("invalid browser URL")
	}
	origin := *u
	origin.Path, origin.RawPath, origin.RawQuery, origin.Fragment = "", "", "", ""
	if _, err := parseOrigin(origin.String()); err != nil {
		return "", err
	}
	return u.String(), nil
}
