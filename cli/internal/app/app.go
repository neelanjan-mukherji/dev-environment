// Package app wires the dev commands together. Main is the whole CLI behind its
// Deps, so tests drive it exactly as a user would, against a fake machine.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/neelanjan-mukherji/dev-environment/cli/internal/doctor"
	"github.com/neelanjan-mukherji/dev-environment/cli/internal/setup"
	"github.com/neelanjan-mukherji/dev-environment/cli/internal/sys"
)

// Deps are the CLI's connections to the machine and the person.
type Deps struct {
	Runner   sys.Runner
	Prompter setup.Prompter
	Version  string
	OS       string // runtime.GOOS; tests set it to play another OS
}

// errFailed means the command already reported what went wrong; exit non-zero quietly.
var errFailed = errors.New("failed")

// Main runs dev with args and returns the process exit code.
func Main(ctx context.Context, args []string, stdout, stderr io.Writer, d Deps) int {
	root := &cobra.Command{
		Use:           "dev",
		Short:         "Set up and maintain this machine from the dev-environment repo",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       d.Version,
	}
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.AddCommand(doctorCmd(d), setupCmd(d), updateCmd(d), profileCmd(d), addCmd(d, true), addCmd(d, false), migrateCmd(d))

	err := root.ExecuteContext(ctx)
	switch {
	case err == nil:
		return 0
	case errors.Is(err, errFailed), errors.Is(err, setup.ErrReported):
		return 1
	default:
		fmt.Fprintln(stderr, "dev:", err)
		return 1
	}
}

func doctorCmd(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check that this machine matches its profile",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			m, err := doctor.ReadMachine(cmd.Context(), d.Runner)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "== profile: %s\n", m.Profile)
			if doctor.Run(cmd.Context(), d.Runner, doctor.Checks(m), cmd.OutOrStdout()) > 0 {
				return errFailed
			}
			return nil
		},
	}
}

func setupCmd(d Deps) *cobra.Command { return setupLike(d, "setup", false) }

func migrateCmd(d Deps) *cobra.Command { return setupLike(d, "migrate-v1", true) }

// setupLike is `dev setup`, or with migrate `dev migrate-v1`: the same steps,
// which on a v1 machine remove the Stow links first, but only when there is a
// v1 setup to migrate.
func setupLike(d Deps, use string, migrate bool) *cobra.Command {
	var o setup.Options
	var config string
	var noSudo bool
	short := "Install and configure this machine (safe to re-run)"
	if migrate {
		short = "Move a v1 (GNU Stow) machine to this setup; suggests the full profile"
	}
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Long: short + `.

Shows what it will do and asks before risky steps. Every question has a flag,
and --config machine.yaml takes the same keys (flags win over the file).
Answers are remembered; later runs only confirm them.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Flags().Changed("no-sudo") {
				o.NoSudo = &noSudo
			}
			if config != "" {
				file, err := setup.LoadConfig(config)
				if err != nil {
					return err
				}
				o = mergeFlags(cmd, file, o)
			}
			if migrate && !setup.HasV1() {
				fmt.Fprintln(cmd.OutOrStdout(), "Nothing to migrate: no v1 (Stow) links into a dotfiles/ tree found.")
				return nil
			}
			return setup.Run(cmd.Context(), o, d.setupDeps(cmd))
		},
	}
	f := cmd.Flags()
	f.StringVar(&config, "config", "", "read answers from a machine.yaml")
	f.StringVar(&o.Source, "source", "", "use this dev-environment checkout (default ~/dev-environment, cloned if missing)")
	f.StringVar(&o.Profile, "profile", "", strings.Join(setup.ProfileNames(), ", "))
	f.StringVar(&o.Name, "name", "", "git author name")
	f.StringVar(&o.Email, "email", "", "git author email (GitHub noreply recommended)")
	f.StringSliceVar(&o.Add, "add", nil, "extra components ("+strings.Join(setup.KnownComponents, ", ")+")")
	f.StringSliceVar(&o.Remove, "remove", nil, "components to leave out")
	f.StringVar(&o.Theme, "theme", "", "catppuccin flavor: latte, frappe, macchiato, mocha")
	f.BoolVarP(&o.Yes, "yes", "y", false, "no questions: accept defaults and confirmations")
	f.BoolVar(&noSudo, "no-sudo", false, "never use sudo here (remembered; --no-sudo=false undoes it): root-only steps are skipped")
	f.BoolVar(&o.DryRun, "dry-run", false, "show what would change, change nothing")
	f.BoolVar(&o.Resume, "resume", false, "only retry the install steps that failed last time")
	f.BoolVar(&o.SSHKey, "ssh-key", false, "with --yes: create ~/.ssh/id_ed25519 if missing")
	f.BoolVar(&o.GitHub, "github", false, "with --yes: gh auth login and add the SSH key (needs a person for the browser/code)")
	f.BoolVar(&o.SkipIdentity, "skip-identity", false, "don't offer the SSH key and GitHub steps")
	return cmd
}

// mergeFlags starts from the config file and lays the flags that were given on top.
func mergeFlags(cmd *cobra.Command, file, flags setup.Options) setup.Options {
	o := file
	set := func(name string, apply func()) {
		if cmd.Flags().Changed(name) {
			apply()
		}
	}
	set("source", func() { o.Source = flags.Source })
	set("profile", func() { o.Profile = flags.Profile })
	set("name", func() { o.Name = flags.Name })
	set("email", func() { o.Email = flags.Email })
	set("add", func() { o.Add = flags.Add })
	set("remove", func() { o.Remove = flags.Remove })
	set("theme", func() { o.Theme = flags.Theme })
	set("yes", func() { o.Yes = flags.Yes })
	set("no-sudo", func() { o.NoSudo = flags.NoSudo })
	set("ssh-key", func() { o.SSHKey = flags.SSHKey })
	set("github", func() { o.GitHub = flags.GitHub })
	set("skip-identity", func() { o.SkipIdentity = flags.SkipIdentity })
	o.DryRun, o.Resume = flags.DryRun, flags.Resume
	return o
}

func updateCmd(d Deps) *cobra.Command {
	var uo setup.UpdateOptions
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Pull the repo, re-apply, upgrade tools and plugins (--system: OS packages too)",
		Long: `Pull the repo, re-apply the dotfiles and install steps, upgrade mise tools
within their pins, and update zsh/tmux plugins. Neovim plugins are restored to
the repo's lazy-lock.json. OS packages are upgraded only with --system.

Nothing updates by itself: shells show a notice at most once a day when the
repo is behind (that check is ` + "`dev update --check`" + `).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return setup.Update(cmd.Context(), uo, d.setupDeps(cmd))
		},
	}
	f := cmd.Flags()
	f.BoolVar(&uo.System, "system", false, "also upgrade OS packages (apt)")
	f.BoolVarP(&uo.Yes, "yes", "y", false, "no questions")
	f.BoolVar(&uo.Check, "check", false, "only record how far behind the repo is (used by the shell notice)")
	return cmd
}

func (d Deps) setupDeps(cmd *cobra.Command) setup.Deps {
	return setup.Deps{Runner: d.Runner, Prompter: d.Prompter, Out: cmd.OutOrStdout(), OS: d.OS}
}

func profileCmd(d Deps) *cobra.Command {
	var co setup.ChangeOptions
	cmd := &cobra.Command{
		Use:   "profile [" + strings.Join(setup.ProfileNames(), "|") + "]",
		Short: "Show or switch this machine's profile (keeps component overrides)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && len(co.Add)+len(co.Remove) == 0 {
				return setup.ShowProfile(cmd.Context(), d.setupDeps(cmd))
			}
			if len(args) == 1 {
				co.Profile = args[0]
			}
			return setup.Change(cmd.Context(), co, d.setupDeps(cmd))
		},
	}
	f := cmd.Flags()
	f.StringSliceVar(&co.Add, "add", nil, "components to add")
	f.StringSliceVar(&co.Remove, "remove", nil, "components to leave out")
	f.BoolVarP(&co.Yes, "yes", "y", false, "no questions")
	return cmd
}

func addCmd(d Deps, add bool) *cobra.Command {
	var co setup.ChangeOptions
	use, short := "add <component|tool>...", "Add components ("+strings.Join(setup.KnownComponents, ", ")+") or any mise tool to this machine"
	if !add {
		use, short = "remove <component|tool>...", "Remove components or machine-local tools from this machine"
	}
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Long: short + `.

A component changes this machine's selection and re-applies. Anything else is a
mise tool (e.g. terraform@1.9) kept only on this machine, in
~/.config/mise/conf.d/local.toml.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if add {
				co.Add = args
			} else {
				co.Remove = args
			}
			return setup.Change(cmd.Context(), co, d.setupDeps(cmd))
		},
	}
	cmd.Flags().BoolVarP(&co.Yes, "yes", "y", false, "no questions")
	return cmd
}
