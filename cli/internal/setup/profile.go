package setup

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/neelanjan-mukherji/dev-environment/cli/internal/sys"
)

// KnownComponents can be added to or removed from any profile
// (the same list as $known in home/.chezmoi.toml.tmpl).
var KnownComponents = []string{"docker", "devtools", "node", "python", "go", "rust", "kubernetes", "java", "cloud"}

// selection is this machine's profile and component overrides.
type selection struct {
	profile           string
	base, add, remove []string // base: the profile's own components
	effective         []string
	repo              string
}

const selectionTemplate = `{{ .profile }}
{{ index .profiles .profile | join "," }}
{{ .components.add | join "," }}
{{ .components.remove | join "," }}
{{ includeTemplate "components" . }}
{{ .chezmoi.workingTree }}
`

func readSelection(ctx context.Context, r sys.Runner) (selection, error) {
	out, err := r.Run(ctx, sys.Command("chezmoi", "execute-template", selectionTemplate))
	if err != nil {
		return selection{}, fmt.Errorf("this machine was never set up; run `dev setup` first (%s)", strings.TrimSpace(out))
	}
	l := strings.Split(strings.TrimRight(out, "\n"), "\n")
	for len(l) < 6 {
		l = append(l, "")
	}
	list := func(s string) []string {
		if s == "" {
			return nil
		}
		return strings.Split(s, ",")
	}
	return selection{profile: l[0], base: list(l[1]), add: list(l[2]), remove: list(l[3]), effective: list(l[4]), repo: l[5]}, nil
}

// ShowProfile prints the machine's profile and components.
func ShowProfile(ctx context.Context, d Deps) error {
	s, err := readSelection(ctx, d.Runner)
	if err != nil {
		return err
	}
	u := newUI(d.Out)
	u.info("profile:    " + s.profile)
	for _, c := range s.add {
		u.info("            + " + c)
	}
	for _, c := range s.remove {
		u.info("            - " + c)
	}
	u.info("components: " + orNone(strings.Join(s.effective, ",")))
	u.info("Change with `dev profile <minimal|dev|full>`, `dev add <component|tool>`, `dev remove ...`.")
	return nil
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

// ChangeOptions change the machine's selection, then re-apply.
type ChangeOptions struct {
	Profile string   // "" keeps the current one
	Add     []string // components or tools
	Remove  []string
	Yes     bool
}

// Change switches the profile and/or adds and removes components (merged with
// the machine's existing overrides) and re-applies. Anything that is not a
// component is a mise tool for this machine only, kept in
// ~/.config/mise/conf.d/local.toml where chezmoi does not touch it.
func Change(ctx context.Context, co ChangeOptions, d Deps) error {
	if co.Profile != "" && !slices.ContainsFunc(Profiles, func(p struct{ Name, Summary string }) bool { return p.Name == co.Profile }) {
		return fmt.Errorf("unknown profile %q (known: minimal, dev, full)", co.Profile)
	}
	s, err := readSelection(ctx, d.Runner)
	if err != nil {
		return err
	}
	r, err := newRun(d, Options{Yes: co.Yes})
	if err != nil {
		return err
	}
	prependPath(r.bin, filepath.Join(r.home, ".local/share/mise/shims"))

	var addC, removeC, addT, removeT []string
	for _, x := range co.Add {
		if slices.Contains(KnownComponents, x) {
			addC = append(addC, x)
		} else {
			addT = append(addT, x)
		}
	}
	for _, x := range co.Remove {
		if slices.Contains(KnownComponents, x) {
			removeC = append(removeC, x)
		} else {
			removeT = append(removeT, x)
		}
	}

	local := filepath.Join(r.home, ".config/mise/conf.d/local.toml")
	if len(addT) > 0 {
		r.ui.step("Adding " + strings.Join(addT, ", ") + " to this machine (" + local + ")")
		if err := r.attached(ctx, nil, "mise", append([]string{"use", "--path", local}, addT...)...); err != nil {
			return err
		}
	}
	if len(removeT) > 0 {
		r.ui.step("Removing " + strings.Join(removeT, ", ") + " from this machine")
		if err := r.attached(ctx, nil, "mise", append([]string{"unuse", "--path", local}, removeT...)...); err != nil {
			return err
		}
	}
	if co.Profile == "" && len(addC)+len(removeC) == 0 {
		return nil
	}

	profile := s.profile
	if co.Profile != "" {
		profile = co.Profile
	}
	// An override is recorded only where it differs from the profile's own components.
	base := s.base
	if co.Profile != "" && co.Profile != s.profile {
		base = nil // unknown here; keep explicit overrides for the new profile
	}
	add, remove := slices.Clone(s.add), slices.Clone(s.remove)
	for _, c := range addC {
		remove = without(remove, c)
		if !slices.Contains(add, c) && !slices.Contains(base, c) {
			add = append(add, c)
		}
	}
	for _, c := range removeC {
		add = without(add, c)
		if !slices.Contains(remove, c) && (base == nil || slices.Contains(base, c)) {
			remove = append(remove, c)
		}
	}
	r.o = Options{Source: s.repo, Profile: profile, Add: add, Remove: remove, Yes: co.Yes, setComponents: true}
	r.reconfigure = true
	return r.steps(ctx, r.configure, r.review, r.applyAndSummarise)
}

func without(list []string, x string) []string {
	return slices.DeleteFunc(slices.Clone(list), func(s string) bool { return s == x })
}
