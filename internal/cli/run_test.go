package cli

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestInformationalCommandsDoNotStartApplication(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}, {"--version"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := runCommands(args, &stdout, &stderr, commands{app: func(io.Writer, string, string, string) error {
				t.Fatal("informational command started the TUI")
				return nil
			}})
			if err != nil || stdout.Len() == 0 || stderr.Len() != 0 {
				t.Fatalf("run(%q): error=%v stdout=%q stderr=%q", args, err, &stdout, &stderr)
			}
		})
	}
}

func TestInvalidArgumentsDoNotStartApplication(t *testing.T) {
	for _, args := range [][]string{{"--unknown"}, {"login"}, {"--version", "extra"}, {"--theme=purple"}, {"--icons=emoji"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := runCommands(args, &stdout, &stderr, commands{app: func(io.Writer, string, string, string) error {
				t.Fatal("invalid arguments started the TUI")
				return nil
			}})
			if err == nil {
				t.Fatalf("run(%q) accepted invalid arguments", args)
			}
		})
	}
}

func TestUpdateCommandDoesNotStartApplication(t *testing.T) {
	called := false
	err := runCommands([]string{"update"}, io.Discard, io.Discard, commands{update: func(output io.Writer) error {
		called = true
		return nil
	}})
	if err != nil || !called {
		t.Fatalf("update not dispatched: %v", err)
	}
	if err := runCommands([]string{"update", "extra"}, io.Discard, io.Discard, commands{}); err == nil {
		t.Fatal("update accepted arguments")
	}
}

func TestFirstReleaseVersion(t *testing.T) {
	var output bytes.Buffer
	if err := runCommands([]string{"--version"}, &output, io.Discard, commands{}); err != nil {
		t.Fatal(err)
	}
	if output.String() != "devnull v0.1.1\n" {
		t.Fatalf("unexpected version %q", &output)
	}
}

func TestDefaultStartupPassesAppearanceAndServer(t *testing.T) {
	var stdout, stderr bytes.Buffer
	called := false
	err := runCommands([]string{"--theme=light", "--server=http://127.0.0.1:8080"}, &stdout, &stderr, commands{app: func(output io.Writer, theme, server, icons string) error {
		called = true
		if output != &stdout || theme != "light" || server != "http://127.0.0.1:8080" || icons != "nerd" {
			t.Fatalf("unexpected application output, theme, or icons: %v, %q, %q", output, theme, icons)
		}
		return nil
	}})
	if err != nil || !called {
		t.Fatalf("application did not start: called=%v error=%v", called, err)
	}
}

func TestIconsComeFromTheFlagOrTheEnvironment(t *testing.T) {
	t.Setenv("DEVNULL_ICONS", "plain")
	for args, want := range map[string]string{"": "plain", "--icons=nerd": "nerd"} {
		var got string
		err := runCommands(strings.Fields(args), io.Discard, io.Discard, commands{app: func(_ io.Writer, _, _, icons string) error {
			got = icons
			return nil
		}})
		if err != nil || got != want {
			t.Fatalf("run(%q): icons=%q error=%v, want %q", args, got, err, want)
		}
	}
}

func TestDefaultStartupFailureIsReturned(t *testing.T) {
	want := errors.New("terminal unavailable")
	err := runCommands(nil, io.Discard, io.Discard, commands{app: func(io.Writer, string, string, string) error {
		return want
	}})
	if !errors.Is(err, want) {
		t.Fatalf("got %v, want %v", err, want)
	}
}

func TestStartupOptionsAndPrivateCallbackDelivery(t *testing.T) {
	var stdout, stderr bytes.Buffer
	called := false
	err := runCommands([]string{"--theme=light", "--server=http://127.0.0.1:8080"}, &stdout, &stderr, commands{app: func(output io.Writer, theme, server, _ string) error {
		called = true
		if output != &stdout || theme != "light" || server != "http://127.0.0.1:8080" {
			t.Fatal("account options not forwarded")
		}
		return nil
	}})
	if err != nil || !called {
		t.Fatalf("account startup: %v", err)
	}
	callback := "devnull://auth/callback?code=synthetic&state=example"
	err = runCommands([]string{"callback", callback}, &stdout, &stderr, commands{callback: func(raw string) error {
		if raw != callback {
			t.Fatal("callback changed")
		}
		return errors.New("safe callback error")
	}})
	if err == nil || strings.Contains(stdout.String(), "synthetic") || strings.Contains(stderr.String(), "synthetic") {
		t.Fatal("callback leaked or failure hidden")
	}
	for _, args := range [][]string{{"--connect"}, {"--demo"}, {"--log"}, {"callback"}, {"callback", callback, "extra"}} {
		if err := runCommands(args, io.Discard, io.Discard, commands{}); err == nil {
			t.Fatalf("invalid account arguments accepted: %q", args)
		}
	}
}
