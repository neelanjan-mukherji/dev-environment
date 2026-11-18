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
)

// fakeRunner stands in for the machine: every command succeeds with empty
// output unless a rule whose substring occurs in the command line says otherwise.
type fakeRunner struct {
	rules []rule
	mu    sync.Mutex
	calls []string
}

type rule struct {
	match string
	out   string
	fail  bool
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	line := strings.Join(append([]string{name}, args...), " ")
	f.mu.Lock()
	f.calls = append(f.calls, line)
	f.mu.Unlock()
	for _, r := range f.rules {
		if strings.Contains(line, r.match) {
			if r.fail {
				return r.out, errors.New("exit status 1")
			}
			return r.out, nil
		}
	}
	return "", nil
}

// machine answers the doctor's question "which profile, components and email?"
// the way `chezmoi execute-template` does on a configured machine.
func machine(profile, components, email string) rule {
	return rule{match: "execute-template", out: profile + "\n" + components + "\n" + email + "\n"}
}

const fullComponents = "docker,devtools,node,python,go,rust,kubernetes,java"

func doctor(t *testing.T, r *fakeRunner) (string, int) {
	t.Helper()
	var out bytes.Buffer
	code := app.Main(context.Background(), []string{"doctor"}, &out, &out, r)
	return out.String(), code
}

func TestDoctorChecksFullProfileTools(t *testing.T) {
	r := &fakeRunner{rules: []rule{machine("full", fullComponents, "ci@example.com")}}
	out, code := doctor(t, r)
	if code != 0 {
		t.Fatalf("exit %d, want 0\n%s", code, out)
	}
	// The full profile per docs/v2-design.md: Kubernetes and Java tooling.
	for _, tool := range []string{"java", "mvn", "gradle", "spring", "kubectl", "helm", "k9s", "node", "docker"} {
		if !strings.Contains(out, "ok   zsh has "+tool+"\n") {
			t.Errorf("no passing check for %s\n%s", tool, out)
		}
	}
}

func TestDoctorOnMinimalChecksNoLanguagesAreInstalled(t *testing.T) {
	r := &fakeRunner{rules: []rule{machine("minimal", "", "ci@example.com")}}
	out, code := doctor(t, r)
	if code != 0 {
		t.Fatalf("exit %d, want 0\n%s", code, out)
	}
	// minimal = shell + editor + CLI tools, no languages (docs/v2-design.md).
	if strings.Contains(out, "zsh has node") || strings.Contains(out, "zsh has java") {
		t.Errorf("minimal machine checked for language tools\n%s", out)
	}
	if !strings.Contains(out, "ok   mise does not manage node\n") {
		t.Errorf("minimal machine did not check that node is absent\n%s", out)
	}
	if !strings.Contains(out, "ok   zsh has lazygit\n") {
		t.Errorf("core tools not checked\n%s", out)
	}
}

func TestDoctorReportsAFailedCheckAndExitsNonZero(t *testing.T) {
	r := &fakeRunner{rules: []rule{
		machine("full", fullComponents, "ci@example.com"),
		{match: "command -v k9s", out: "k9s: not found\n", fail: true},
	}}
	out, code := doctor(t, r)
	if code != 1 {
		t.Fatalf("exit %d, want 1\n%s", code, out)
	}
	if !strings.Contains(out, "FAIL zsh has k9s\n    k9s: not found\n") {
		t.Errorf("failure or its output not reported\n%s", out)
	}
	if !strings.Contains(out, " 1 failed") {
		t.Errorf("summary does not count the failure\n%s", out)
	}
}

func TestDoctorExplainsAnUnconfiguredMachine(t *testing.T) {
	r := &fakeRunner{rules: []rule{{match: "execute-template", out: "chezmoi: no config\n", fail: true}}}
	out, code := doctor(t, r)
	if code != 1 || !strings.Contains(out, "has setup run?") {
		t.Fatalf("exit %d, want 1 with a hint to run setup\n%s", code, out)
	}
}

func TestDoctorChecksGitIdentityOnlyWhenOneWasConfigured(t *testing.T) {
	out, _ := doctor(t, &fakeRunner{rules: []rule{machine("minimal", "", "")}})
	if strings.Contains(out, "git identity") {
		t.Errorf("checked git identity although none was configured\n%s", out)
	}
	r := &fakeRunner{rules: []rule{machine("minimal", "", "me@example.com")}}
	doctor(t, r)
	if !slices.ContainsFunc(r.calls, func(c string) bool { return strings.Contains(c, `"me@example.com"`) }) {
		t.Errorf("git identity not compared with the configured email; calls:\n%s", strings.Join(r.calls, "\n"))
	}
}
