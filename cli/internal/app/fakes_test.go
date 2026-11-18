package app_test

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/neelanjan-mukherji/dev-environment/cli/internal/app"
	"github.com/neelanjan-mukherji/dev-environment/cli/internal/setup"
	"github.com/neelanjan-mukherji/dev-environment/cli/internal/sys"
)

// fakeRunner stands in for the machine: every command succeeds with empty
// output unless the first rule whose substring occurs in the command line says
// otherwise. A command line is its extra env, then the command: "A=1 chezmoi init".
type fakeRunner struct {
	rules []rule
	mu    sync.Mutex
	calls []string
}

type rule struct {
	match string
	out   string
	fail  bool
	do    func() // side effect, e.g. a script writing to a log
}

func (f *fakeRunner) Run(_ context.Context, c sys.Cmd) (string, error) {
	line := strings.Join(append(append(slices.Clone(c.Env), c.Name), c.Args...), " ")
	f.mu.Lock()
	f.calls = append(f.calls, line)
	f.mu.Unlock()
	for _, r := range f.rules {
		if strings.Contains(line, r.match) {
			if r.do != nil {
				r.do()
			}
			if r.fail {
				return r.out, errors.New("exit status 1")
			}
			return r.out, nil
		}
	}
	return "", nil
}

// called returns the index of the first call containing all of parts, or -1.
func (f *fakeRunner) called(parts ...string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, c := range f.calls {
		if all(c, parts) {
			return i
		}
	}
	return -1
}

func all(s string, parts []string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}

func (f *fakeRunner) log() string { return strings.Join(f.calls, "\n") }

// fakePrompter is a person at the terminal. Confirm answers come from answers by
// question substring (unlisted questions are answered no); edit changes the
// settings form. A nil *fakePrompter is nobody at the terminal.
type fakePrompter struct {
	answers map[string]bool
	// explainFirst: questions (by substring) first answered with "explain", once.
	explainFirst map[string]bool
	edit         func(setup.Settings) setup.Settings
	asked        []setup.Question
	edited       bool
}

func (p *fakePrompter) Interactive() bool { return p != nil }

func (p *fakePrompter) Ask(q setup.Question) (setup.Answer, error) {
	p.asked = append(p.asked, q)
	for k := range p.explainFirst {
		if strings.Contains(q.Text, k) && q.Explain != "" {
			delete(p.explainFirst, k)
			return setup.Explain, nil
		}
	}
	for k, v := range p.answers {
		if strings.Contains(q.Text, k) {
			if v {
				return setup.Yes, nil
			}
			return setup.No, nil
		}
	}
	return setup.No, nil
}

func (p *fakePrompter) EditSettings(s setup.Settings) (setup.Settings, error) {
	p.edited = true
	if p.edit != nil {
		s = p.edit(s)
	}
	return s, nil
}

func run(t *testing.T, r *fakeRunner, p *fakePrompter, args ...string) (string, int) {
	t.Helper()
	var out bytes.Buffer
	deps := app.Deps{Runner: r, Prompter: p}
	if p == nil {
		deps.Prompter = nobody{}
	}
	code := app.Main(context.Background(), args, &out, &out, deps)
	return out.String(), code
}

type nobody struct{}

func (nobody) Interactive() bool                        { return false }
func (nobody) Ask(setup.Question) (setup.Answer, error) { return setup.No, errors.New("nobody to ask") }
func (nobody) EditSettings(s setup.Settings) (setup.Settings, error) {
	return s, errors.New("nobody to ask")
}
