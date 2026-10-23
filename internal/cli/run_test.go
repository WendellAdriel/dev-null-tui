package cli

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestInformationalCommandsDoNotStartDemo(t *testing.T) {
	for _, args := range [][]string{nil, {"--help"}, {"-h"}, {"--version"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := run(args, &stdout, &stderr, func(io.Writer, string) error {
				t.Fatal("informational command started the TUI")
				return nil
			})
			if err != nil || stdout.Len() == 0 || stderr.Len() != 0 {
				t.Fatalf("run(%q): error=%v stdout=%q stderr=%q", args, err, &stdout, &stderr)
			}
		})
	}
}

func TestInvalidArgumentsDoNotStartDemo(t *testing.T) {
	for _, args := range [][]string{{"--unknown"}, {"login"}, {"--demo", "extra"}, {"--demo", "--version"}, {"--demo", "--theme=purple"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := run(args, &stdout, &stderr, func(io.Writer, string) error {
				t.Fatal("invalid arguments started the TUI")
				return nil
			})
			if err == nil {
				t.Fatalf("run(%q) accepted invalid arguments", args)
			}
		})
	}
}

func TestDemoRequiresExplicitFlagAndPassesTheme(t *testing.T) {
	var stdout, stderr bytes.Buffer
	called := false
	err := run([]string{"--demo", "--theme=light"}, &stdout, &stderr, func(output io.Writer, theme string) error {
		called = true
		if output != &stdout || theme != "light" {
			t.Fatalf("unexpected demo output or theme: %v, %q", output, theme)
		}
		return nil
	})
	if err != nil || !called {
		t.Fatalf("demo did not start: called=%v error=%v", called, err)
	}
}

func TestDemoFailureIsReturned(t *testing.T) {
	want := errors.New("terminal unavailable")
	err := run([]string{"--demo"}, io.Discard, io.Discard, func(io.Writer, string) error {
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("got %v, want %v", err, want)
	}
}

func TestAccountStartupAndCallbackAreExplicitAndSeparateFromDemo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	called := false
	err := runCommands([]string{"--connect", "--theme=light", "--server=http://127.0.0.1:8080"}, &stdout, &stderr, commands{account: func(output io.Writer, theme, server string) error {
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
	for _, args := range [][]string{{"--connect", "--demo"}, {"--connect", "--version"}, {"callback"}, {"callback", callback, "extra"}, {"--server=https://other.example"}} {
		if err := runCommands(args, io.Discard, io.Discard, commands{}); err == nil {
			t.Fatalf("invalid account arguments accepted: %q", args)
		}
	}
}
