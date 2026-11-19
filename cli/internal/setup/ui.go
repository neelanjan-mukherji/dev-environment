package setup

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

// ui writes setup's report. Colours switch off by themselves when w is not a terminal.
type ui struct {
	w                            io.Writer
	head, dim, okS, warnS, failS lipgloss.Style
}

func newUI(w io.Writer) ui {
	re := lipgloss.NewRenderer(w)
	return ui{
		w:     w,
		head:  re.NewStyle().Bold(true),
		dim:   re.NewStyle().Faint(true),
		okS:   re.NewStyle().Foreground(lipgloss.Color("2")),
		warnS: re.NewStyle().Foreground(lipgloss.Color("3")),
		failS: re.NewStyle().Foreground(lipgloss.Color("1")).Bold(true),
	}
}

func (u ui) step(s string) {
	fmt.Fprintf(u.w, "\n%s %s\n", u.head.Foreground(lipgloss.Color("5")).Render("==>"), u.head.Render(s))
}
func (u ui) info(s string) { fmt.Fprintf(u.w, "    %s\n", s) }
func (u ui) warn(s string) { fmt.Fprintf(u.w, "    %s\n", u.warnS.Render("! "+s)) }
func (u ui) raw(s string)  { fmt.Fprint(u.w, s) }

// command shows a command before it runs.
func (u ui) command(c string) { fmt.Fprintf(u.w, "    %s\n", u.dim.Render("$ "+c)) }

func (u ui) list(s string) {
	for _, l := range nonEmptyLines(s) {
		u.info(l)
	}
}

func (u ui) settings(s Settings) {
	orNoneDim := func(v string) string {
		if v == "" {
			return u.dim.Render("(none)")
		}
		return v
	}
	u.info("profile:    " + s.Profile)
	if len(s.Optional) > 0 {
		u.info("optional:   " + strings.Join(s.Optional, ", "))
	}
	u.info("git name:   " + orNoneDim(s.Name))
	u.info("git email:  " + orNoneDim(s.Email))
}

type status int

const (
	statusOK status = iota
	statusSkipped
	statusFailed
)

// row is one line of a summary table.
type row struct {
	status status
	step   string
}

// summary is the table at the end of a run.
func (u ui) summary(rows []row) {
	if len(rows) == 0 {
		u.info("Every step finished.")
		return
	}
	label := map[status]string{
		statusOK:      u.okS.Render("ok"),
		statusSkipped: u.warnS.Render("skipped"),
		statusFailed:  u.failS.Render("failed"),
	}
	t := table.New().Border(lipgloss.RoundedBorder()).Headers("", "step")
	for _, r := range rows {
		t.Row(label[r.status], r.step)
	}
	for _, l := range strings.Split(t.Render(), "\n") {
		u.info(l)
	}
}
