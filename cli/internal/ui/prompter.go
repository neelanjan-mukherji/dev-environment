// Package ui is the person at the terminal: huh forms behind setup.Prompter.
package ui

import (
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/mattn/go-isatty"

	"github.com/neelanjan-mukherji/dev-environment/cli/internal/setup"
)

// Terminal asks on the controlling terminal.
type Terminal struct{}

// Interactive is true when stdin and stdout are terminals.
func (Terminal) Interactive() bool {
	return isatty.IsTerminal(os.Stdin.Fd()) && isatty.IsTerminal(os.Stdout.Fd())
}

// accessible turns off the full-screen widgets on dumb terminals.
func accessible() bool { return os.Getenv("TERM") == "dumb" || os.Getenv("ACCESSIBLE") != "" }

func (Terminal) Ask(q setup.Question) (setup.Answer, error) {
	if q.Explain == "" {
		var ok bool
		err := huh.NewConfirm().Title(q.Text).Affirmative("Yes").Negative("No").Value(&ok).
			WithTheme(huh.ThemeCatppuccin()).WithAccessible(accessible()).Run()
		if ok {
			return setup.Yes, err
		}
		return setup.No, err
	}
	a := setup.Yes
	err := huh.NewSelect[setup.Answer]().Title(q.Text).
		Options(huh.NewOption("Yes", setup.Yes), huh.NewOption("No", setup.No), huh.NewOption(q.Explain, setup.Explain)).
		Value(&a).WithTheme(huh.ThemeCatppuccin()).WithAccessible(accessible()).Run()
	return a, err
}

func (Terminal) EditSettings(s setup.Settings) (setup.Settings, error) {
	var opts []huh.Option[string]
	for _, p := range setup.Profiles {
		opts = append(opts, huh.NewOption(p.Name+"  "+p.Summary, p.Name))
	}
	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title("Profile").Description("What to install. Components can be adjusted later with `dev profile`.").
			Options(opts...).Value(&s.Profile),
		huh.NewMultiSelect[string]().Title("Optional components").Description("In no profile; add them here or later with `dev add`.").
			Options(huh.NewOptions(setup.OptionalComponents...)...).Value(&s.Optional),
		huh.NewInput().Title("Git author name").Description("Blank to skip.").Value(&s.Name),
		huh.NewInput().Title("Git author email").
			Description("Your GitHub noreply address keeps your real one out of public commits. Blank to skip.").
			Value(&s.Email).
			Validate(func(v string) error {
				if v != "" && !strings.Contains(v, "@") {
					return errNotEmail
				}
				return nil
			}),
	)).WithTheme(huh.ThemeCatppuccin()).WithAccessible(accessible())
	if err := form.Run(); err != nil {
		return s, err
	}
	s.Name, s.Email = strings.TrimSpace(s.Name), strings.TrimSpace(s.Email)
	return s, nil
}

type notEmail struct{}

func (notEmail) Error() string { return "that doesn't look like an email address" }

var errNotEmail = notEmail{}
