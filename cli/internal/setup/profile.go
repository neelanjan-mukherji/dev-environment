package setup

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/neelanjan-mukherji/dev-environment/cli/internal/sys"
)

// KnownComponents can be added to or removed from any profile
// (the same list as $known in home/.chezmoi.toml.tmpl).
var KnownComponents = []string{"docker", "devtools", "node", "python", "go", "rust", "kubernetes", "java", "cloud", "desktop"}

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
		return selection{}, fmt.Errorf("%w (%s)", errNotSetUp, strings.TrimSpace(out))
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
	if co.Profile != "" && !slices.Contains(ProfileNames(), co.Profile) {
		return fmt.Errorf("unknown profile %q (known: %s)", co.Profile, strings.Join(ProfileNames(), ", "))
	}
	s, err := readSelection(ctx, d.Runner)
	if err != nil {
		return err
	}
	r, err := newRun(d, Options{Yes: co.Yes})
	if err != nil {
		return err
	}

	addComponents, addTools := splitComponents(co.Add)
	removeComponents, removeTools := splitComponents(co.Remove)
	if r.OS == "windows" && len(addTools)+len(removeTools) > 0 {
		return errors.New("the Windows host has no mise: add tools on the Linux side (in WSL: dev add " +
			strings.Join(append(addTools, removeTools...), " ") + ")")
	}

	local := filepath.Join(r.home, ".config/mise/conf.d/local.toml")
	if len(addTools) > 0 {
		r.ui.step("Adding " + strings.Join(addTools, ", ") + " to this machine (" + local + ")")
		if err := r.attached(ctx, nil, "mise", append([]string{"use", "--path", local}, addTools...)...); err != nil {
			return err
		}
	}
	if len(removeTools) > 0 {
		r.ui.step("Removing " + strings.Join(removeTools, ", ") + " from this machine")
		if err := r.attached(ctx, nil, "mise", append([]string{"unuse", "--path", local}, removeTools...)...); err != nil {
			return err
		}
	}
	if co.Profile == "" && len(addComponents)+len(removeComponents) == 0 {
		return nil
	}
	if hasV1Links(r.home) {
		return errV1
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
	for _, c := range addComponents {
		remove = without(remove, c)
		if !slices.Contains(add, c) && !slices.Contains(base, c) {
			add = append(add, c)
		}
	}
	for _, c := range removeComponents {
		add = without(add, c)
		if !slices.Contains(remove, c) && (base == nil || slices.Contains(base, c)) {
			remove = append(remove, c)
		}
	}
	r.opts = Options{Source: s.repo, Profile: profile, Add: add, Remove: remove, Yes: co.Yes, setComponents: true}
	r.reconfigure = true
	if err := r.steps(ctx, r.configure, r.review, r.applyAndSummarise); err != nil {
		return err
	}
	if slices.Contains(removeComponents, "desktop") {
		// Removing desktop only stops managing it: dev never uninstalls an app or reverts a preference.
		r.ui.info("desktop is no longer managed here; nothing was uninstalled or reverted " +
			"(VS Code, its extensions, settings and the OS tweaks stay as they are).")
	}
	return nil
}

// splitComponents separates known components from mise tool names.
func splitComponents(names []string) (components, tools []string) {
	for _, x := range names {
		if slices.Contains(KnownComponents, x) {
			components = append(components, x)
		} else {
			tools = append(tools, x)
		}
	}
	return components, tools
}

func without(list []string, x string) []string {
	return slices.DeleteFunc(slices.Clone(list), func(s string) bool { return s == x })
}
