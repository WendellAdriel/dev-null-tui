package release

import (
	"embed"
	"io/fs"
	"strings"
)

const summarySuffix = "-summary.md"

//go:embed notes/*-summary.md
var notes embed.FS

// Summary returns the release notes shown by What's new for a version.
func Summary(version string) (string, bool) {
	data, err := notes.ReadFile("notes/" + version + summarySuffix)
	return string(data), err == nil
}

func SummaryVersions() []string {
	entries, _ := fs.ReadDir(notes, "notes")
	versions := make([]string, 0, len(entries))
	for _, entry := range entries {
		versions = append(versions, strings.TrimSuffix(entry.Name(), summarySuffix))
	}
	return versions
}
