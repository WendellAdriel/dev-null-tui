package release

import (
	"fmt"
	"regexp"
	"strings"
)

const Version = "v0.1.1"
const Repository = "WendellAdriel/dev-null-tui"

var stableVersion = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

func ValidVersion(version string) bool {
	return stableVersion.MatchString(version)
}

func Compare(a, b string) int {
	left, right := strings.Split(strings.TrimPrefix(a, "v"), "."), strings.Split(strings.TrimPrefix(b, "v"), ".")
	for i := range left {
		if len(left[i]) < len(right[i]) {
			return -1
		}
		if len(left[i]) > len(right[i]) {
			return 1
		}
		if result := strings.Compare(left[i], right[i]); result != 0 {
			return result
		}
	}
	return 0
}

func AssetName(goos, arch string) (string, error) {
	if (goos != "linux" && goos != "darwin" && goos != "windows") || (arch != "amd64" && arch != "arm64") {
		return "", fmt.Errorf("no release binary for %s/%s", goos, arch)
	}
	name := "devnull_" + goos + "_" + arch
	if goos == "windows" {
		name += ".exe"
	}
	return name, nil
}
