package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"

	tea "charm.land/bubbletea/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/tui"
)

var Version = "dev"

func Run(args []string, stdout, stderr io.Writer) error {
	return run(args, stdout, stderr, func(output io.Writer, theme string) error {
		_, err := tea.NewProgram(tui.New(theme), tea.WithOutput(output)).Run()
		return err
	})
}

func run(args []string, stdout, stderr io.Writer, start func(io.Writer, string) error) error {
	flags := flag.NewFlagSet("devnull", flag.ContinueOnError)
	flags.SetOutput(stderr)
	demo := flags.Bool("demo", false, "open the offline visual demo")
	version := flags.Bool("version", false, "print the version")
	theme := flags.String("theme", "auto", "demo appearance: auto, light, or dark")
	flags.Usage = func() {
		_, _ = fmt.Fprint(stdout, "Usage: devnull [--demo] [--theme auto|light|dark]\n\n/dev/null terminal client. Account connection is not available yet.\n\n")
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
	if *version && *demo {
		return errors.New("choose either --version or --demo")
	}
	if *version {
		_, err := fmt.Fprintln(stdout, "devnull", Version)
		return err
	}
	if !*demo {
		flags.Usage()
		return nil
	}
	return start(stdout, *theme)
}
