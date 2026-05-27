package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	accent = lipgloss.Color("#a78bfa")
	good   = lipgloss.Color("#34d399")
	bad    = lipgloss.Color("#f87171")
	warn   = lipgloss.Color("#fbbf24")
	subtle = lipgloss.Color("#6b7280")
	muted  = lipgloss.Color("#9ca3af")
	surface = lipgloss.Color("#1f2937")
	base   = lipgloss.Color("#111827")

	accentStyle = lipgloss.NewStyle().Foreground(accent)
	goodStyle   = lipgloss.NewStyle().Foreground(good)
	badStyle    = lipgloss.NewStyle().Foreground(bad)
	warnStyle   = lipgloss.NewStyle().Foreground(warn)
	subtleStyle = lipgloss.NewStyle().Foreground(subtle)
	mutedStyle  = lipgloss.NewStyle().Foreground(muted)
	boldStyle   = lipgloss.NewStyle().Bold(true)

	selectedStyle = lipgloss.NewStyle().Background(surface).Bold(true)
	normalStyle   = lipgloss.NewStyle()

	tabActiveStyle   = lipgloss.NewStyle().Foreground(accent).Bold(true).Padding(0, 3).PaddingBottom(0).Border(lipgloss.NormalBorder(), false, false, true, false).BorderForeground(accent)
	tabInactiveStyle = lipgloss.NewStyle().Foreground(muted).Padding(0, 3).PaddingBottom(0)
	headerBarStyle   = lipgloss.NewStyle().Background(surface).Padding(0, 2)
	statusBarStyle   = lipgloss.NewStyle().Foreground(subtle).Padding(0, 2)
	helpBarStyle     = lipgloss.NewStyle().Foreground(muted).Padding(0, 2)
	inputStyle       = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent).Padding(0, 1)
	loadingStyle     = lipgloss.NewStyle().Foreground(warn).Bold(true)
	errorStyle       = lipgloss.NewStyle().Foreground(bad)

	metaTitleStyle   = lipgloss.NewStyle().Foreground(accent).Bold(true)
	metaLabelStyle   = lipgloss.NewStyle().Foreground(subtle)
	metaValueStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#e5e7eb"))
	metaScoreStyle   = lipgloss.NewStyle().Foreground(warn).Bold(true)
	metaGenreStyle   = lipgloss.NewStyle().Foreground(muted)
	metaSynopsisStyle = lipgloss.NewStyle().Foreground(muted).Width(40)
	metaDivider      = lipgloss.NewStyle().Foreground(surface).Render("│")
	metaPanelStyle   = lipgloss.NewStyle().Padding(0, 1).Border(lipgloss.NormalBorder(), false, false, false, true).BorderForeground(surface)
)

func Navbar(tabs []string, active, width int) string {
	var parts []string
	for i, t := range tabs {
		if i == active {
			parts = append(parts, tabActiveStyle.Render(t))
		} else {
			parts = append(parts, tabInactiveStyle.Render(t))
		}
	}
	joined := lipgloss.JoinHorizontal(lipgloss.Top, parts...)
	return headerBarStyle.Width(width - 4).Render(joined)
}

func FormatPeers(s, l int) string {
	if s == 0 && l == 0 {
		return mutedStyle.Render("S:-  L:-")
	}
	return fmt.Sprintf("%s %s",
		goodStyle.Render(fmt.Sprintf("S:%-3d", s)),
		badStyle.Render(fmt.Sprintf("L:%-3d", l)),
	)
}

func PadRight(s string, w int) string {
	pad := w - lipgloss.Width(s)
	if pad < 0 {
		pad = 0
	}
	return s + strings.Repeat(" ", pad)
}

func Truncate(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	if w <= 1 {
		return ""
	}
	runes := []rune(s)
	result := ""
	width := 0
	for _, r := range runes {
		rw := lipgloss.Width(string(r))
		if width+rw+1 > w {
			result += "…"
			break
		}
		result += string(r)
		width += rw
	}
	return result
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func wrapText(text string, lineWidth int) []string {
	var lines []string
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	current := words[0]
	for _, w := range words[1:] {
		if len(current)+1+len(w) <= lineWidth {
			current += " " + w
		} else {
			lines = append(lines, current)
			current = w
		}
	}
	lines = append(lines, current)
	return lines
}

type StatusMsg string
type ErrMsg string

func (e ErrMsg) Error() string { return string(e) }
