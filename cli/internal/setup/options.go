// Package setup is `dev setup`: it gets the repo and tools in place, settles the
// machine's settings, previews and backs up what will change, lets chezmoi apply
// the dotfiles and run the install scripts, and summarises what happened.
package setup

import (
	"fmt"
	"os"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Options are the answers setup would otherwise ask for. Every prompt has one.
type Options struct {
	Source  string   `yaml:"source"`
	Profile string   `yaml:"profile"`
	Name    string   `yaml:"name"`
	Email   string   `yaml:"email"`
	Add     []string `yaml:"add"`
	Remove  []string `yaml:"remove"`
	Theme   string   `yaml:"theme"`
	Yes     bool     `yaml:"yes"`
	// NoSudo: never use sudo on this machine (remembered; false turns it off).
	// Root-only steps are skipped and listed in skipped.log.
	NoSudo *bool `yaml:"no_sudo"`

	// Identity steps. A person is asked about each one; with --yes they run only
	// when chosen here.
	SSHKey       bool `yaml:"ssh_key"`       // create ~/.ssh/id_ed25519 if missing
	GitHub       bool `yaml:"github"`        // gh auth login, then add the key to the account
	SkipIdentity bool `yaml:"skip_identity"` // don't offer them at all

	// Windows: install WSL, or set up its Linux side. A person is asked; with
	// --yes it runs only when chosen here (installing WSL needs a restart).
	WSL bool `yaml:"wsl"`

	DryRun bool `yaml:"-"`
	Resume bool `yaml:"-"`

	// setComponents passes Add and Remove even when empty (they replace the
	// machine's overrides instead of keeping them).
	setComponents bool
}

// LoadConfig reads a machine.yaml (the same keys as the flags).
func LoadConfig(path string) (Options, error) {
	var o Options
	b, err := os.ReadFile(path)
	if err != nil {
		return o, err
	}
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	if err := dec.Decode(&o); err != nil {
		return o, fmt.Errorf("%s: %w", path, err)
	}
	return o, nil
}

// Settings are what a person reviews before anything is applied.
type Settings struct {
	Profile string
	Name    string
	Email   string
}

// Profiles in the order they are offered, with what each adds.
var Profiles = []struct{ Name, Summary string }{
	{"minimal", "shell, editor and CLI tools"},
	{"dev", "minimal + Docker, Node, Python, Go, Rust"},
	{"full", "dev + Kubernetes and Java"},
}

// Question is a yes/no question. When Explain is set it names a third answer
// ("Show the diff", "Show the command") that shows more and asks again.
type Question struct {
	Text    string
	Explain string
}

// Answer to a Question.
type Answer int

const (
	No Answer = iota
	Yes
	Explain
)

// ProfileNames lists the profiles in order.
func ProfileNames() []string {
	var names []string
	for _, p := range Profiles {
		names = append(names, p.Name)
	}
	return names
}

// Prompter is the person at the terminal.
type Prompter interface {
	// Interactive reports whether there is anyone to ask.
	Interactive() bool
	Ask(Question) (Answer, error)
	EditSettings(Settings) (Settings, error)
}

// blank is how dev tells the config template "empty on purpose": a value that
// is set (so the template neither asks nor falls back to a default) and trims to "".
const blank = " "

// env turns the options into the DEVENV_* variables the chezmoi config template reads.
func (o Options) env() []string {
	var env []string
	add := func(k, v string) {
		if v != "" {
			env = append(env, k+"="+v)
		}
	}
	add("DEVENV_PROFILE", o.Profile)
	add("DEVENV_NAME", o.Name)
	add("DEVENV_EMAIL", o.Email)
	list := func(k string, v []string) {
		switch {
		case len(v) > 0:
			env = append(env, k+"="+strings.Join(v, ","))
		case o.setComponents:
			env = append(env, k+"="+blank)
		}
	}
	list("DEVENV_ADD", o.Add)
	list("DEVENV_REMOVE", o.Remove)
	add("DEVENV_THEME", o.Theme)
	if o.NoSudo != nil {
		v := "0"
		if *o.NoSudo {
			v = "1"
		}
		env = append(env, "DEVENV_NO_SUDO="+v)
	}
	return env
}
