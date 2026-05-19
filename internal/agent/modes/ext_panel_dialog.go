package modes

import (
	"fmt"
	"strings"

	"github.com/mattn/go-runewidth"

	"github.com/patriceckhart/zot/internal/tui"
)

type extPanelDialog struct {
	active bool
	ext    string
	id     string
	title  string
	lines  []string
	footer string
}

func newExtPanelDialog() *extPanelDialog { return &extPanelDialog{} }

func (d *extPanelDialog) Active() bool { return d != nil && d.active }

func (d *extPanelDialog) Open(ext, id, title string, lines []string, footer string) {
	d.active = true
	d.ext = ext
	d.id = id
	d.title = title
	d.lines = append([]string(nil), lines...)
	d.footer = footer
}

func (d *extPanelDialog) Update(title string, lines []string, footer string) {
	if !d.active {
		return
	}
	if title != "" {
		d.title = title
	}
	d.lines = append(d.lines[:0], lines...)
	d.footer = footer
}

func (d *extPanelDialog) Close() {
	d.active = false
	d.ext = ""
	d.id = ""
	d.title = ""
	d.lines = nil
	d.footer = ""
}

func (d *extPanelDialog) Render(th tui.Theme, width int) []string {
	if !d.Active() {
		return nil
	}
	title := d.title
	if title == "" {
		title = d.ext
	}
	out := []string{frameHeaderColor(th, title, width, th.Accent)}
	for _, l := range d.lines {
		plain := stripANSIBytes(l)
		trimmed := strings.TrimLeft(plain, " ")
		selected := strings.HasPrefix(trimmed, "▸ ") || strings.HasPrefix(trimmed, "● ")
		out = append(out, styleExtPanelLine(th, l, plain, width, selected))
	}
	if strings.TrimSpace(d.footer) != "" {
		out = append(out, "")
		out = append(out, th.FG256(th.Muted, d.footer))
	}
	out = append(out, frameRuleColor(th, width, th.Accent))
	return out
}

func styleExtPanelLine(th tui.Theme, raw string, plain string, width int, selected bool) string {
	if selected {
		if visible := runewidth.StringWidth(plain); visible < width {
			plain += strings.Repeat(" ", width-visible)
		}
		base := fmt.Sprintf("\x1b[38;5;%dm\x1b[48;5;%dm", th.SelectionFG, th.SelectionBG)
		green := fmt.Sprintf("\x1b[38;5;%dm\x1b[48;5;%dm", th.Tool, th.SelectionBG)
		return base + strings.ReplaceAll(plain, "✓", green+"✓"+base) + "\x1b[0m"
	}
	if raw != plain {
		return raw + "\x1b[0m"
	}
	styled := th.FG256(th.Muted, plain)
	styled = strings.ReplaceAll(styled, "✓", th.FG256(th.Tool, "✓"))
	return styled
}
