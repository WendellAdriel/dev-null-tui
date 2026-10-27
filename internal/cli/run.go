package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/auth"
	"github.com/WendellAdriel/dev-null-cli/internal/release"
	"github.com/WendellAdriel/dev-null-cli/internal/tui"
	"github.com/WendellAdriel/dev-null-cli/internal/update"
)

var Version = release.Version

func Run(args []string, stdout, stderr io.Writer) error {
	return runCommands(args, stdout, stderr, commands{app: startApp, update: func(output io.Writer) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		return update.Run(ctx, Version, output)
	}, callback: func(raw string) error {
		store, err := auth.DefaultStore()
		if err != nil {
			return err
		}
		if err := store.Prepare(); err != nil {
			return err
		}
		return auth.DeliverCallback(context.Background(), store, raw)
	}})
}

type commands struct {
	app      func(io.Writer, string, string, string) error
	callback func(string) error
	update   func(io.Writer) error
}

func runCommands(args []string, stdout, stderr io.Writer, actions commands) error {
	if len(args) > 0 && args[0] == "update" {
		if len(args) != 1 {
			return errors.New("usage: devnull update")
		}
		return actions.update(stdout)
	}
	if len(args) > 0 && args[0] == "callback" {
		if len(args) != 2 {
			return errors.New("usage: devnull callback <complete callback link>")
		}
		return actions.callback(args[1])
	}
	flags := flag.NewFlagSet("devnull", flag.ContinueOnError)
	flags.SetOutput(stderr)
	server := flags.String("server", auth.DefaultOrigin, "server origin")
	version := flags.Bool("version", false, "print the version")
	theme := flags.String("theme", "auto", "appearance: auto, light, or dark")
	defaultIcons := os.Getenv("DEVNULL_ICONS")
	if defaultIcons == "" {
		defaultIcons = "nerd"
	}
	icons := flags.String("icons", defaultIcons, "icon set: nerd (needs a Nerd Font) or plain; defaults to $DEVNULL_ICONS")
	flags.Usage = func() {
		_, _ = fmt.Fprint(stdout, "Usage: devnull [--theme auto|light|dark] [--icons nerd|plain] [--server origin]\n       devnull update\n\n/dev/null terminal client. Opens your account and application.\n\n")
		flags.SetOutput(stdout)
		flags.PrintDefaults()
		flags.SetOutput(stderr)
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	if *theme != "auto" && *theme != "light" && *theme != "dark" {
		return fmt.Errorf("unknown theme %q: choose auto, light, or dark", *theme)
	}
	if *icons != "nerd" && *icons != "plain" {
		return fmt.Errorf("unknown icon set %q: choose nerd or plain", *icons)
	}
	if *version {
		_, err := fmt.Fprintln(stdout, "devnull", Version)
		return err
	}
	return actions.app(stdout, *theme, *server, *icons)
}

func startApp(output io.Writer, theme, origin, icons string) (err error) {
	if _, err := auth.NewClient(origin, nil); err != nil {
		return err
	}
	store, err := auth.DefaultStore()
	if err != nil {
		return err
	}
	if err := store.Prepare(); err != nil {
		return err
	}
	lock, err := store.Lock()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, lock.Close()) }()
	model := tui.NewApp(theme, store, origin, context.Background()).WithIcons(icons)
	defer func() { err = errors.Join(err, model.Close()) }()
	defer func() {
		_, cleanupErr := io.WriteString(output, model.GraphicsCleanup())
		err = errors.Join(err, cleanupErr)
	}()
	_, err = tea.NewProgram(model, tea.WithOutput(output)).Run()
	return err
}
