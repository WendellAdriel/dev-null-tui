package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	tea "charm.land/bubbletea/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/auth"
	"github.com/WendellAdriel/dev-null-cli/internal/tui"
)

var Version = "dev"

func Run(args []string, stdout, stderr io.Writer) error {
	return runCommands(args, stdout, stderr, commands{app: startApp, callback: func(raw string) error {
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
	app      func(io.Writer, string, string) error
	callback func(string) error
}

func runCommands(args []string, stdout, stderr io.Writer, actions commands) error {
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
	flags.Usage = func() {
		_, _ = fmt.Fprint(stdout, "Usage: devnull [--theme auto|light|dark] [--server origin]\n\n/dev/null terminal client. Opens your account and application.\n\n")
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
	if *version {
		_, err := fmt.Fprintln(stdout, "devnull", Version)
		return err
	}
	return actions.app(stdout, *theme, *server)
}

func startApp(output io.Writer, theme, origin string) (err error) {
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
	model := tui.NewApp(theme, store, origin, context.Background())
	defer func() { err = errors.Join(err, model.Close()) }()
	defer func() {
		_, cleanupErr := io.WriteString(output, model.GraphicsCleanup())
		err = errors.Join(err, cleanupErr)
	}()
	_, err = tea.NewProgram(model, tea.WithOutput(output)).Run()
	return err
}
