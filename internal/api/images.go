package api

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg" // Registers the JPEG header reader for DecodeConfig.
	_ "image/png"  // Registers the PNG header reader for DecodeConfig.
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"

	_ "golang.org/x/image/webp" // Registers the WebP header reader for DecodeConfig.

	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

const (
	MaxImages = 3
	// MaxImageBytes is the server's "10mb": Laravel reads it as 10,000 KB of 1,024 bytes.
	MaxImageBytes        = 10_000 * 1024
	MaxImageSide         = 6000
	MaxDescriptionLength = 1000
)

// ImageFile is a local image that passed the server's checks, ready to upload.
type ImageFile struct {
	auth.Upload
	Path          string
	Width, Height int
	Description   string
}

// ReadImage applies the server's image rules locally, so a refused file never costs an upload.
func ReadImage(path string) (ImageFile, error) {
	name := filepath.Base(path)
	info, err := os.Stat(path)
	if err != nil {
		return ImageFile{}, fmt.Errorf("%s cannot be read", name)
	}
	if !info.Mode().IsRegular() {
		return ImageFile{}, fmt.Errorf("%s is not a file", name)
	}
	if info.Size() > MaxImageBytes {
		return ImageFile{}, fmt.Errorf("%s is larger than 10 MB", name)
	}
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return ImageFile{}, fmt.Errorf("%s cannot be read", name)
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, MaxImageBytes+1))
	if err != nil {
		return ImageFile{}, fmt.Errorf("%s cannot be read", name)
	}
	if len(data) > MaxImageBytes {
		return ImageFile{}, fmt.Errorf("%s is larger than 10 MB", name)
	}
	kind := imageType(data)
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if kind == "" || err != nil || "image/"+format != kind {
		return ImageFile{}, fmt.Errorf("%s is not a JPEG, PNG, or WebP image", name)
	}
	if config.Width > MaxImageSide || config.Height > MaxImageSide {
		return ImageFile{}, fmt.Errorf("%s is larger than %d pixels on a side", name, MaxImageSide)
	}
	return ImageFile{Upload: auth.Upload{Name: name, Type: kind, Data: data}, Path: path, Width: config.Width, Height: config.Height}, nil
}

// imageType reads the file signature, as the server does, rather than trusting the extension.
func imageType(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}):
		return "image/jpeg"
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return "image/png"
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return "image/webp"
	}
	return ""
}

func validDescription(description string) error {
	if utf8.RuneCountInString(trimInput(description)) > MaxDescriptionLength {
		return fmt.Errorf("image descriptions are limited to %d characters", MaxDescriptionLength)
	}
	return nil
}

// DroppedPaths reads a paste as file paths when every entry names an existing file. Terminals paste dropped files as
// quoted, backslash-escaped, or file:// paths, separated by spaces or newlines.
func DroppedPaths(pasted string) ([]string, bool) {
	entries := splitPaths(strings.TrimSpace(pasted))
	if len(entries) == 0 {
		return nil, false
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if strings.HasPrefix(entry, "file://") {
			u, err := url.Parse(entry)
			if err != nil || (u.Host != "" && u.Host != "localhost") {
				return nil, false
			}
			entry = u.Path
		}
		if strings.HasPrefix(entry, "~/") {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, false
			}
			entry = filepath.Join(home, entry[2:])
		}
		if !filepath.IsAbs(entry) {
			return nil, false
		}
		info, err := os.Stat(entry)
		if err != nil || !info.Mode().IsRegular() {
			return nil, false
		}
		paths = append(paths, entry)
	}
	return paths, true
}

func splitPaths(text string) []string {
	var entries []string
	var current strings.Builder
	quote, escaped, started := rune(0), false, false
	flush := func() {
		if started {
			entries = append(entries, current.String())
		}
		current.Reset()
		started = false
	}
	for _, r := range text {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
		case r == '\\' && quote != '\'' && runtime.GOOS != "windows":
			escaped, started = true, true
		case quote != 0 && r == quote:
			quote = 0
		case quote == 0 && (r == '\'' || r == '"'):
			quote, started = r, true
		case quote == 0 && (r == ' ' || r == '\t' || r == '\n' || r == '\r'):
			flush()
		default:
			current.WriteRune(r)
			started = true
		}
	}
	if quote != 0 || escaped {
		return nil
	}
	flush()
	return entries
}
