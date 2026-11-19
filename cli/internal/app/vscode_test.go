package app_test

import (
	"strings"
	"testing"
)

// VS Code extensions (desktop component): `dev update` updates them on every OS,
// only when desktop is on and code is there.

func TestUpdateUpdatesVSCodeExtensionsWhenDesktopIsOn(t *testing.T) {
	for _, goos := range []string{"linux", "darwin", "windows"} {
		t.Run(goos, func(t *testing.T) {
			b := newBox(t)
			b.configured(t)
			// current first: the selection template also mentions workingTree.
			r := &fakeRunner{rules: []rule{current("minimal", "", "desktop", "", "desktop", b.repo), sourceAt(b.repo)}}
			if out, code := runOn(t, goos, r, nil, "update", "--yes"); code != 0 {
				t.Fatalf("exit %d\n%s\ncalls:\n%s", code, out, r.log())
			}
			if r.called("code --update-extensions") < 0 {
				t.Fatalf("extensions not updated; calls:\n%s", r.log())
			}
		})
	}
}

func TestUpdateLeavesVSCodeAloneWithoutDesktop(t *testing.T) {
	b := newBox(t)
	b.configured(t)
	r := &fakeRunner{rules: []rule{current("dev", devBase, "", "", devBase, b.repo), sourceAt(b.repo)}}
	if out, code := run(t, r, nil, "update", "--yes"); code != 0 {
		t.Fatalf("exit %d\n%s\ncalls:\n%s", code, out, r.log())
	}
	if r.called("code --update-extensions") >= 0 {
		t.Fatalf("ran code without desktop; calls:\n%s", r.log())
	}
}

func TestUpdateWarnsWhenItCannotTellWhetherDesktopIsOn(t *testing.T) {
	b := newBox(t)
	b.configured(t)
	r := &fakeRunner{rules: []rule{
		{match: "execute-template {{ .profile }}", fail: true, out: "template: bad config"}, sourceAt(b.repo),
	}}
	out, code := run(t, r, nil, "update", "--yes")
	if code != 0 {
		t.Fatalf("an unreadable selection failed the update: exit %d\n%s", code, out)
	}
	if !strings.Contains(out, "VS Code extensions") || !strings.Contains(out, "template: bad config") {
		t.Fatalf("want a warning that names VS Code extensions and the cause\n%s", out)
	}
}

func TestUpdateSkipsVSCodeExtensionsWhenCodeIsMissing(t *testing.T) {
	for goos, lookup := range map[string]string{"linux": "command -v code", "windows": "where.exe code"} {
		t.Run(goos, func(t *testing.T) {
			b := newBox(t)
			b.configured(t)
			r := &fakeRunner{rules: []rule{
				current("minimal", "", "desktop", "", "desktop", b.repo), sourceAt(b.repo),
				{match: lookup, fail: true},
			}}
			if out, code := runOn(t, goos, r, nil, "update", "--yes"); code != 0 {
				t.Fatalf("a missing code failed the update: exit %d\n%s", code, out)
			}
			if r.called("code --update-extensions") >= 0 {
				t.Fatalf("ran code that is not there; calls:\n%s", r.log())
			}
		})
	}
}
