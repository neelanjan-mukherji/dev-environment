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

// Prompter is the person at the terminal.
type Prompter interface {
	// Interactive reports whether there is anyone to ask.
	Interactive() bool
	Confirm(question string) (bool, error)
	EditSettings(Settings) (Settings, error)
}

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
			env = append(env, k+"= ") // a single space: "none" to the template
		}
	}
	list("DEVENV_ADD", o.Add)
	list("DEVENV_REMOVE", o.Remove)
	add("DEVENV_THEME", o.Theme)
	return env
}
