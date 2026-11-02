package cli

import "errors"

func restartApp(string) error {
	return errors.New("restarting in place is not supported on Windows")
}
