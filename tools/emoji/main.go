package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "Usage: go run ./tools/emoji <web emoji-shortcodes.json>")
		os.Exit(2)
	}
	if err := syncCatalog(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func syncCatalog(source string) error {
	data, err := os.ReadFile(source) // #nosec G304 G703 -- The developer explicitly selects this local catalog, not remote input.
	if err != nil {
		return err
	}
	var catalog map[string]string
	if err := json.Unmarshal(data, &catalog); err != nil {
		return err
	}
	if len(catalog) == 0 {
		return fmt.Errorf("emoji catalog is empty")
	}
	for name, value := range catalog {
		if name == "" || value == "" {
			return fmt.Errorf("emoji catalog contains an empty name or value")
		}
	}
	data, err = json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile("internal/tui/emoji.json", append(data, '\n'), 0600)
}
