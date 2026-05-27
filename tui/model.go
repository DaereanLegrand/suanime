package tui

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"suanime/providers"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Model struct {
	width  int
	height int

	tabs      []string
	activeTab int

	input    textinput.Model
	results  []*providers.AnimeTorrent
	cursor   int
	loading  bool
	status   string
	err      string
	searched bool

	dlManager  *DownloadManager
	downCursor int

	jikanMeta      *AnimeMeta
	jikanFetching  bool
	lastAnimeTitle string

	NoCover bool
}

func NewModel(cfg Config, noCover bool) *Model {
	ti := textinput.New()
	ti.Placeholder = "search anime..."
	ti.CharLimit = 200
	ti.Width = 60
	ti.Focus()

	dlm := NewDownloadManager(cfg)

	return &Model{
		tabs:      []string{"Search", "Downloads"},
		activeTab: 0,
		input:     ti,
		dlManager: dlm,
		NoCover:   noCover,
	}
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, tickCmd)
}

type searchDoneMsg struct {
	results []*providers.AnimeTorrent
}

type jikanResultMsg struct {
	meta *AnimeMeta
	err  error
}

type tickMsg time.Time

func tickCmd() tea.Msg { return tickMsg(time.Now()) }

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.input.Width = max(20, m.width-10)

	case tea.KeyMsg:
		if m.activeTab == 0 && m.input.Focused() {
			switch msg.String() {
			case "enter":
				q := strings.TrimSpace(m.input.Value())
				if q == "" {
					return m, nil
				}
				m.loading = true
				m.searched = true
				m.status = "searching..."
				m.err = ""
				m.input.Blur()
				return m, searchCmd(q)
			case "esc":
				m.input.Blur()
				return m, nil
			}
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
		return m.handleKey(msg)

	case searchDoneMsg:
		m.loading = false
		m.results = msg.results
		m.cursor = 0
		m.searched = true
		m.status = fmt.Sprintf("%d results", len(m.results))
		m.jikanMeta = nil
		m.jikanFetching = false
		m.lastAnimeTitle = ""
		if len(m.results) > 0 {
			title := extractAnimeTitle(m.results[0].Name)
			m.lastAnimeTitle = title
			m.jikanFetching = true
			return m, jikanFetchCmd(title)
		}

	case jikanResultMsg:
		m.jikanFetching = false
		if msg.err != nil {
			m.status = fmt.Sprintf("metadata: %v", msg.err)
		} else {
			m.jikanMeta = msg.meta
			if !m.NoCover && msg.meta.ImageURL != "" && m.width > 0 {
				rightW := m.width/2 - 2
				imgH := m.height / 3
				kittyShowImageFromURL(msg.meta.ImageURL, m.width/2+1, 3, rightW, imgH)
			}
		}

	case StatusMsg:
		m.status = string(msg)
		return m, nil

	case ErrMsg:
		m.err = string(msg)
		m.status = fmt.Sprintf("error: %s", string(msg))
		m.loading = false
		return m, nil

	case tickMsg:
		m.dlManager.Poll()
		if m.dlManager.RPCErr != "" {
			m.status = m.dlManager.RPCErr
		} else {
			items := m.dlManager.GetItems()
			dn := len(items)
			dr, dmet, dw, dp, dc, df := 0, 0, 0, 0, 0, 0
			for _, d := range items {
				switch d.Status {
				case StatusRunning: dr++
				case StatusMeta: dmet++
				case StatusWaiting: dw++
				case StatusPaused: dp++
				case StatusCompleted: dc++
				case StatusFailed: df++
				}
			}
			if dn > 0 {
				m.status = fmt.Sprintf("%d dls: run=%d meta=%d wait=%d comp=%d fail=%d", dn, dr, dmet, dw, dc, df)
			}
		}
		cmds = append(cmds, tea.Tick(2*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) }))
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()

	switch k {
	case "ctrl+c":
		return m, tea.Quit

	case "tab", "shift+tab":
		if m.activeTab == 0 {
			m.activeTab = 1
		} else {
			m.activeTab = 0
		}
		return m, nil

	case "1":
		m.activeTab = 0
		return m, nil

	case "2":
		m.activeTab = 1
		return m, nil

	case "q":
		return m, tea.Quit
	}

	if m.activeTab == 0 {
		return m.handleSearchKeys(msg)
	}
	m.handleDownloadKeys(msg)
	return m, nil
}

func (m *Model) handleSearchKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()

	switch k {
	case "/":
		m.input.Focus()
		m.input.SetValue("")
		m.cursor = 0
		return m, nil

	case "enter", "d":
		if m.cursor >= 0 && m.cursor < len(m.results) {
			t := m.results[m.cursor]
			if t.MagnetLink != "" {
				err := m.dlManager.AddDownload(t.Name, t.MagnetLink)
				if err != nil {
					m.status = fmt.Sprintf("download err: %s", err)
				} else {
					m.status = fmt.Sprintf("added: %s", Truncate(t.Name, 40))
				}
				return m, nil
			}
			m.status = fmt.Sprintf("no magnet link for: %s", Truncate(t.Name, 40))
			return m, nil
		}

	case "o":
		if m.cursor >= 0 && m.cursor < len(m.results) {
			t := m.results[m.cursor]
			if t.Link != "" {
				openLink(t.Link)
				return m, nil
			}
		}

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, m.jikanCmd()

	case "down", "j":
		if m.cursor < len(m.results)-1 {
			m.cursor++
		}
		return m, m.jikanCmd()
	}

	return m, nil
}

func (m *Model) handleDownloadKeys(msg tea.KeyMsg) {
	items := m.dlManager.GetItems()
	k := msg.String()

	switch k {
	case "up", "k":
		if m.downCursor > 0 {
			m.downCursor--
		}
	case "down", "j":
		if m.downCursor < len(items)-1 {
			m.downCursor++
		}
	case "p":
		if m.downCursor >= 0 && m.downCursor < len(items) {
			gid := items[m.downCursor].GID
			if err := m.dlManager.Pause(gid); err != nil {
				m.status = fmt.Sprintf("pause err: %s", err)
			} else {
				m.status = "paused"
			}
		}
	case "r":
		if m.downCursor >= 0 && m.downCursor < len(items) {
			gid := items[m.downCursor].GID
			if err := m.dlManager.Resume(gid); err != nil {
				m.status = fmt.Sprintf("resume err: %s", err)
			} else {
				m.status = "resumed"
			}
		}
	case "c":
		if m.downCursor >= 0 && m.downCursor < len(items) {
			gid := items[m.downCursor].GID
			if err := m.dlManager.Cancel(gid); err != nil {
				m.status = fmt.Sprintf("cancel err: %s", err)
			} else {
				m.status = "cancelled"
				items = m.dlManager.GetItems()
				if m.downCursor >= len(items) {
					m.downCursor = max(0, len(items)-1)
				}
			}
		}
	case "d":
		if m.downCursor >= 0 && m.downCursor < len(items) {
			gid := items[m.downCursor].GID
			if err := m.dlManager.Cancel(gid); err != nil {
				m.status = fmt.Sprintf("remove err: %s", err)
			} else {
				m.status = "removed"
				items = m.dlManager.GetItems()
				if m.downCursor >= len(items) {
					m.downCursor = max(0, len(items)-1)
				}
			}
		}
	}
}

func (m *Model) DownloadManager() *DownloadManager {
	return m.dlManager
}

func searchCmd(query string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		results := providers.SearchAll(ctx, query)
		return searchDoneMsg{results: results}
	}
}

func openLink(link string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", link)
	case "darwin":
		cmd = exec.Command("open", link)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", link)
	default:
		return
	}
	_ = cmd.Start()
}

func (m *Model) jikanCmd() tea.Cmd {
	if m.cursor < 0 || m.cursor >= len(m.results) {
		return nil
	}
	title := extractAnimeTitle(m.results[m.cursor].Name)
	if title == m.lastAnimeTitle || title == "" {
		return nil
	}
	m.lastAnimeTitle = title
	m.jikanFetching = true
	m.jikanMeta = nil
	return jikanFetchCmd(title)
}

func jikanFetchCmd(title string) tea.Cmd {
	return func() tea.Msg {
		meta, err := jikanSearch(title)
		return jikanResultMsg{meta: meta, err: err}
	}
}

func (m *Model) View() string {
	w := max(80, m.width)

	navbar := Navbar(m.tabs, m.activeTab, w)

	var content string
	if m.activeTab == 0 {
		content = m.searchView()
	} else {
		content = m.downloadsView()
	}

	status := statusBarStyle.Width(w).Render(m.status)
	help := helpBarStyle.Width(w).Render(strings.Join(m.helpKeys(), "  "))
	footer := lipgloss.JoinVertical(lipgloss.Left, status, help)

	main := lipgloss.JoinVertical(lipgloss.Left,
		navbar,
		content,
		footer,
	)

	return lipgloss.NewStyle().Width(w).Height(m.height).Render(main)
}

func (m *Model) helpKeys() []string {
	if m.activeTab == 0 {
		if m.input.Focused() {
			return []string{
				accentStyle.Render("enter") + " search",
				subtleStyle.Render("esc") + " back",
				subtleStyle.Render("tab") + " downloads",
				subtleStyle.Render("q") + " quit",
			}
		}
		if len(m.results) > 0 {
			return []string{
				accentStyle.Render("/") + " search",
				accentStyle.Render("enter") + " download",
				subtleStyle.Render("o") + " open",
				subtleStyle.Render("j/k") + " move",
				subtleStyle.Render("tab") + " downloads",
				subtleStyle.Render("q") + " quit",
			}
		}
		return []string{
			accentStyle.Render("/") + " search",
			subtleStyle.Render("tab") + " downloads",
			subtleStyle.Render("q") + " quit",
		}
	}
	return []string{
		accentStyle.Render("p") + " pause",
		accentStyle.Render("r") + " resume",
		accentStyle.Render("c") + " cancel",
		subtleStyle.Render("j/k") + " move",
		subtleStyle.Render("tab") + " search",
		subtleStyle.Render("q") + " quit",
	}
}

func (m *Model) searchView() string {
	if m.input.Focused() {
		return m.inputView()
	}
	if m.loading {
		return m.centeredView("searching...")
	}
	if m.err != "" && len(m.results) == 0 {
		return m.centeredView(m.err + "\n\npress / to search again")
	}
	if !m.searched {
		return m.centeredView("press / to search")
	}
	if len(m.results) == 0 {
		return m.centeredView("no results found\n\npress / to search again")
	}

	leftW := m.width / 2
	rightW := m.width - leftW - 1

	left := m.resultsList(leftW)
	right := m.metaPanel(rightW)

	divider := metaDivider
	return lipgloss.JoinHorizontal(lipgloss.Top, left, divider, right)
}

func (m *Model) inputView() string {
	return lipgloss.JoinVertical(lipgloss.Left,
		accentStyle.Bold(true).Render("Search"),
		"",
		inputStyle.Width(max(20, m.width-8)).Render(m.input.View()),
		"",
		subtleStyle.Render("type query and press enter"),
	)
}

func (m *Model) centeredView(msg string) string {
	return lipgloss.JoinVertical(lipgloss.Left,
		accentStyle.Bold(true).Render("Search"),
		"",
		msg,
	)
}

func (m *Model) resultsList(width int) string {
	var parts []string
	parts = append(parts, accentStyle.Bold(true).Render("Results"))
	parts = append(parts, mutedStyle.Render(fmt.Sprintf("  %-3s %-4s %-9s %s", "", "Ep", "Size", "Title")))

	visible := max(3, m.height-12)
	start := m.cursor - visible/2
	if start < 0 {
		start = 0
	}
	end := start + visible
	if end > len(m.results) {
		end = len(m.results)
		start = max(0, end-visible)
	}

	for i := start; i < end; i++ {
		t := m.results[i]
		marker := " "
		if i == m.cursor {
			marker = accentStyle.Render(">")
		}

		ep := "-"
		if t.IsBatch {
			ep = warnStyle.Render("BCH")
		} else if t.Episode > 0 {
			ep = fmt.Sprintf("%3d", t.Episode)
		}

		size := t.Size
		if t.SizeBytes > 0 {
			size = providers.FormatSize(t.SizeBytes)
		}

		peers := FormatPeers(t.Seeders, t.Leechers)
		titleWidth := max(20, width-30)
		title := Truncate(t.Name, titleWidth)

		line := fmt.Sprintf("%s %s  %-9s  %s  %s  %s",
			marker, ep, size, peers, title, subtleStyle.Render(fmt.Sprintf("[%s]", t.Provider)),
		)

		if i == m.cursor {
			parts = append(parts, selectedStyle.Width(width-2).Render(line))
		} else {
			parts = append(parts, mutedStyle.Render(line))
		}
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m *Model) metaPanel(width int) string {
	if m.jikanFetching {
		return metaPanelStyle.Width(width).Render(
			lipgloss.JoinVertical(lipgloss.Left,
				accentStyle.Bold(true).Render("Info"),
				"",
				loadingStyle.Render("fetching..."),
			),
		)
	}
	if m.jikanMeta == nil {
		if !m.NoCover {
			kittyClearImage()
		}
		return metaPanelStyle.Width(width).Render(
			lipgloss.JoinVertical(lipgloss.Left,
				accentStyle.Bold(true).Render("Info"),
				"",
				subtleStyle.Render("no metadata"),
			),
		)
	}

	meta := m.jikanMeta
	var parts []string

	parts = append(parts, accentStyle.Bold(true).Render("Info"))
	parts = append(parts, "")

	if meta.TitleEng != "" && meta.TitleEng != meta.Title {
		parts = append(parts, metaTitleStyle.Render(Truncate(meta.TitleEng, width-2)))
	}
	parts = append(parts, metaTitleStyle.Render(Truncate(meta.Title, width-2)))

	scoreStr := fmt.Sprintf("★ %.2f", meta.Score)
	parts = append(parts, fmt.Sprintf("%s  %s  %s",
		metaScoreStyle.Render(scoreStr),
		metaValueStyle.Render(fmt.Sprintf("%d", meta.Year)),
		metaValueStyle.Render(meta.Type),
	))

	if meta.Rank > 0 {
		parts = append(parts, fmt.Sprintf("Rank: %s  Popularity: %s",
			metaValueStyle.Render(fmt.Sprintf("#%d", meta.Rank)),
			metaValueStyle.Render(fmt.Sprintf("#%d", meta.Popularity)),
		))
	}
	if meta.Episodes > 0 {
		parts = append(parts, fmt.Sprintf("%s episodes  %s",
			metaValueStyle.Render(fmt.Sprintf("%d", meta.Episodes)),
			metaValueStyle.Render(meta.Status),
		))
	}

	if len(meta.Genres) > 0 {
		genres := make([]string, len(meta.Genres))
		for i, g := range meta.Genres {
			genres[i] = metaGenreStyle.Render(g)
		}
		parts = append(parts, strings.Join(genres, " · "))
	}

	if len(meta.Studios) > 0 {
		parts = append(parts, metaLabelStyle.Render(strings.Join(meta.Studios, ", ")))
	}

	if meta.Synopsis != "" {
		parts = append(parts, "")
		syn := Truncate(meta.Synopsis, width*6)
		parts = append(parts, metaSynopsisStyle.Width(width-2).Render(syn))
	}

	return metaPanelStyle.Width(width).Render(
		lipgloss.JoinVertical(lipgloss.Left, parts...),
	)
}
