package app_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// machine answers the doctor's question "which profile, components and email?"
// the way `chezmoi execute-template` does on a configured machine.
func machine(profile, components, email string) rule {
	return machineWith(profile, components, email, "mocha")
}

func machineWith(profile, components, email, flavor string) rule {
	return machineOn("linux", profile, components, email, flavor)
}

func machineOn(goos, profile, components, email, flavor string) rule {
	return rule{match: "execute-template", out: profile + "\n" + components + "\n" + email + "\n" + flavor + "\n" + goos + "\n"}
}

const fullComponents = "docker,devtools,node,python,go,rust,kubernetes,java"

func doctor(t *testing.T, r *fakeRunner) (string, int) {
	t.Helper()
	return run(t, r, nil, "doctor")
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
	if !slices.ContainsFunc(r.calls, func(c string) bool { return strings.Contains(c, `'me@example.com'`) }) {
		t.Errorf("git identity not compared with the configured email; calls:\n%s", strings.Join(r.calls, "\n"))
	}
}

func TestDoctorChecksTheBatThemeOfTheConfiguredFlavor(t *testing.T) {
	r := &fakeRunner{rules: []rule{machineWith("minimal", "", "", "latte")}}
	doctor(t, r)
	if r.called("Catppuccin Latte") < 0 || r.called("Catppuccin Mocha") >= 0 {
		t.Fatalf("bat theme check ignores the flavor; calls:\n%s", r.log())
	}
}

func TestDoctorReportsARecordedSkipAsSkippedNotFailed(t *testing.T) {
	b := newBox(t)
	write(t, filepath.Join(b.state, "skipped.log"), "2026-11-18 10:00:00\tLogin shell: no sudo\n")
	r := &fakeRunner{rules: []rule{machine("minimal", "", ""), {match: "login shell", fail: true}, {match: "cut -d: -f7", fail: true}}}
	out, code := doctor(t, r)
	if code != 0 || !strings.Contains(out, "skip login shell is zsh") {
		t.Fatalf("exit %d, want the login shell reported as skipped\n%s", code, out)
	}
}

func TestDoctorReportsASkippedDockerInstallAsSkipped(t *testing.T) {
	// No sudo, or Alpine: the docker step records a skip instead of installing.
	b := newBox(t)
	write(t, filepath.Join(b.state, "skipped.log"), "2026-11-18 10:00:00\tdocker (needs sudo)\n")
	r := &fakeRunner{rules: []rule{machine("dev", "docker", ""), {match: "docker", fail: true}}}
	out, code := doctor(t, r)
	if code != 0 || !strings.Contains(out, "skip zsh has docker") || !strings.Contains(out, "skip docker compose plugin") {
		t.Fatalf("exit %d, want the docker checks reported as skipped\n%s", code, out)
	}
}

func TestDoctorQuotesTheEmailForTheShell(t *testing.T) {
	r := &fakeRunner{rules: []rule{machine("minimal", "", "a$(touch x)@example.com")}}
	doctor(t, r)
	if r.called(`'a$(touch x)@example.com'`) < 0 {
		t.Fatalf("email not single-quoted; calls:\n%s", r.log())
	}
}
