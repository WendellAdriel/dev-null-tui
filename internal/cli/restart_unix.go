//go:build !windows

package cli

import (
	"os"

	"golang.org/x/sys/unix"
)

// restartApp replaces this process with the updated binary, keeping its arguments and environment.
func restartApp(executable string) error {
	return unix.Exec(executable, os.Args, os.Environ()) // #nosec G204 -- The path is this program's own resolved binary.
}
