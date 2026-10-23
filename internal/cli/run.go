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
	return runCommands(args, stdout, stderr, commands{demo: func(output io.Writer, theme string) error {
		_, err := tea.NewProgram(tui.New(theme), tea.WithOutput(output)).Run()
		return err
	}, account: startAccount, callback: func(raw string) error {
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
	demo     func(io.Writer, string) error
	account  func(io.Writer, string, string) error
	callback func(string) error
}

func run(args []string, stdout, stderr io.Writer, start func(io.Writer, string) error) error {
	return runCommands(args, stdout, stderr, commands{demo: start})
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
	demo := flags.Bool("demo", false, "open the offline visual demo")
	connect := flags.Bool("connect", false, "open the account connection screen")
	server := flags.String("server", auth.DefaultOrigin, "account server origin (with --connect)")
	version := flags.Bool("version", false, "print the version")
	theme := flags.String("theme", "auto", "appearance: auto, light, or dark")
	flags.Usage = func() {
		_, _ = fmt.Fprint(stdout, "Usage: devnull [--demo | --connect] [--theme auto|light|dark]\n\n/dev/null terminal client. Connect an account or open the offline demo.\n\n")
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
	if (*version && (*demo || *connect)) || (*demo && *connect) {
		return errors.New("choose one of --version, --demo, or --connect")
	}
	if !*connect && *server != auth.DefaultOrigin {
		return errors.New("--server requires --connect")
	}
	if *version {
		_, err := fmt.Fprintln(stdout, "devnull", Version)
		return err
	}
	if *connect {
		return actions.account(stdout, *theme, *server)
	}
	if !*demo {
		flags.Usage()
		return nil
	}
	return actions.demo(stdout, *theme)
}

func startAccount(output io.Writer, theme, origin string) (err error) {
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
	model := tui.NewAccount(theme, store, origin, context.Background())
	defer func() { err = errors.Join(err, model.Close()) }()
	_, err = tea.NewProgram(model, tea.WithOutput(output)).Run()
	return err
}
