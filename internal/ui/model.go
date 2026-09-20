package ui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/scotthellings/ssh-manager/internal/config"
	"github.com/scotthellings/ssh-manager/internal/sshconfig"
)

type screen int

const (
	screenList screen = iota
	screenHost
	screenGroup
	screenSettings
	screenConfirm
	screenMove
	screenHelp
)

type rowKind int

const (
	rowGroup rowKind = iota
	rowHost
)

type row struct {
	kind  rowKind
	group string
	node  *sshconfig.Node
}

// Model is the root bubbletea model.
type Model struct {
	cfg *config.Config
	doc *sshconfig.Doc

	screen   screen
	prev     screen
	rows     []row
	cursor   int
	collapse map[string]bool

	filter    textinput.Model
	filtering bool

	host     hostForm
	group    groupForm
	move     moveForm
	settings settingsForm
	confirm  confirmPrompt

	status string
	err    string
	width  int
	height int
}

// New builds the model. firstRun forces the settings screen.
func New(cfg *config.Config, firstRun bool) (*Model, error) {
	applyColor(cfg.Color)
	m := &Model{cfg: cfg, collapse: map[string]bool{}, screen: screenList}
	f := textinput.New()
	f.Prompt = "/"
	f.CharLimit = 64
	m.filter = f

	if firstRun {
		m.screen = screenSettings
		m.settings = newSettingsForm(cfg, true, m.inputWidth())
		m.doc = &sshconfig.Doc{Path: cfg.ConfigFile}
		return m, nil
	}
	if err := m.reload(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Model) reload() error {
	doc, err := sshconfig.Load(m.cfg.ConfigFile)
	if err != nil {
		return err
	}
	m.doc = doc
	m.rebuild()
	return nil
}

// rebuild flattens the document into the visible rows, honouring the filter
// and collapsed groups.
func (m *Model) rebuild() {
	q := strings.ToLower(strings.TrimSpace(m.filter.Value()))
	m.rows = nil
	for _, g := range m.doc.Groups() {
		hosts := m.doc.Hosts(g)
		var keep []*sshconfig.Node
		for _, n := range hosts {
			if q == "" || matches(n, q) {
				keep = append(keep, n)
			}
		}
		if len(keep) == 0 {
			continue
		}
		m.rows = append(m.rows, row{kind: rowGroup, group: g})
		if m.collapse[g] && q == "" {
			continue
		}
		for _, n := range keep {
			m.rows = append(m.rows, row{kind: rowHost, group: g, node: n})
		}
	}
	if m.cursor >= len(m.rows) {
		m.cursor = max(0, len(m.rows)-1)
	}
}

func matches(n *sshconfig.Node, q string) bool {
	if strings.Contains(strings.ToLower(n.Alias), q) || strings.Contains(strings.ToLower(n.Comment), q) {
		return true
	}
	for _, p := range n.Params {
		if strings.Contains(strings.ToLower(p.Value), q) || strings.Contains(strings.ToLower(p.Key), q) {
			return true
		}
	}
	return false
}

func (m *Model) Init() tea.Cmd { return textinput.Blink }

func (m *Model) inputWidth() int {
	w := m.width - 8
	if w < 20 {
		w = 40
	}
	if w > 70 {
		w = 70
	}
	return w
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.host.setWidth(m.inputWidth())
		m.settings.input.Width = m.inputWidth()
		m.group.input.Width = m.inputWidth()
		return m, nil
	case connectedMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
		}
		return m, nil
	case tea.KeyMsg:
		switch m.screen {
		case screenList:
			return m.updateList(msg)
		case screenHost:
			return m.updateHost(msg)
		case screenGroup:
			return m.updateGroup(msg)
		case screenSettings:
			return m.updateSettings(msg)
		case screenConfirm:
			return m.updateConfirm(msg)
		case screenMove:
			return m.updateMove(msg)
		case screenHelp:
			m.screen = m.prev
			return m, nil
		}
	}
	return m, nil
}

// current returns the host under the cursor, or nil.
func (m *Model) current() *sshconfig.Node {
	if m.cursor < len(m.rows) && m.rows[m.cursor].kind == rowHost {
		return m.rows[m.cursor].node
	}
	return nil
}

func (m *Model) currentGroup() string {
	if m.cursor < len(m.rows) {
		return m.rows[m.cursor].group
	}
	return sshconfig.Ungrouped
}

func (m *Model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.filtering {
		switch msg.String() {
		case "esc":
			m.filtering = false
			m.filter.SetValue("")
			m.filter.Blur()
			m.rebuild()
			return m, nil
		case "enter":
			m.filtering = false
			m.filter.Blur()
			return m, nil
		}
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		m.rebuild()
		return m, cmd
	}

	m.err = ""
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.rows)-1 {
			m.cursor++
		}
	case "g":
		m.cursor = 0
	case "G":
		m.cursor = max(0, len(m.rows)-1)
	case "/":
		m.filtering = true
		m.filter.Focus()
		return m, textinput.Blink
	case "enter":
		if m.cursor < len(m.rows) && m.rows[m.cursor].kind == rowGroup {
			g := m.rows[m.cursor].group
			m.collapse[g] = !m.collapse[g]
			m.rebuild()
			return m, nil
		}
		if n := m.current(); n != nil {
			m.openHost(n)
		}
	case "n":
		m.openHost(nil)
	case "c":
		if n := m.current(); n != nil {
			return m, connect(n.Alias)
		}
	case "d":
		if n := m.current(); n != nil {
			alias := n.Alias
			m.ask(fmt.Sprintf("delete host %q?", alias), func() error {
				m.doc.Delete(alias)
				return m.save("deleted " + alias)
			})
		} else if m.cursor < len(m.rows) {
			g := m.rows[m.cursor].group
			if g == sshconfig.Ungrouped {
				m.err = "cannot delete the Ungrouped bucket"
				return m, nil
			}
			m.ask(fmt.Sprintf("delete group %q? (hosts move to Ungrouped)", g), func() error {
				m.doc.DeleteGroup(g, false)
				return m.save("deleted group " + g)
			})
		}
	case " ":
		if n := m.current(); n != nil {
			n.Enabled = !n.Enabled
			state := "enabled"
			if !n.Enabled {
				state = "disabled"
			}
			if err := m.save(state + " " + n.Alias); err != nil {
				m.err = err.Error()
			}
		}
	case "a":
		m.screen, m.group = screenGroup, newGroupForm("", m.inputWidth())
		return m, textinput.Blink
	case "r":
		if m.cursor < len(m.rows) && m.rows[m.cursor].kind == rowGroup {
			m.screen, m.group = screenGroup, newGroupForm(m.rows[m.cursor].group, m.inputWidth())
			return m, textinput.Blink
		}
	case "m":
		if n := m.current(); n != nil {
			groups := append([]string{sshconfig.Ungrouped}, m.doc.Groups()...)
			m.move = moveForm{groups: dedupe(groups), alias: n.Alias}
			m.screen = screenMove
		}
	case "s":
		m.screen = screenSettings
		m.settings = newSettingsForm(m.cfg, false, m.inputWidth())
		return m, textinput.Blink
	case "?":
		m.prev, m.screen = screenList, screenHelp
	}
	return m, nil
}

func (m *Model) openHost(n *sshconfig.Node) {
	group := m.currentGroup()
	if n != nil {
		group = n.Group
	}
	m.host = newHostForm(n, m.doc.Groups(), group, m.inputWidth())
	m.screen = screenHost
	m.err = ""
}

func (m *Model) updateHost(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.screen = screenList
		return m, nil
	case "ctrl+s":
		return m, m.saveHost()
	case "tab", "down":
		if m.host.focus == extraIdx && msg.String() == "down" {
			break // let the textarea move its own cursor
		}
		m.host.focusOn(m.host.focus + 1)
		return m, textinput.Blink
	case "shift+tab", "up":
		if m.host.focus == extraIdx && msg.String() == "up" {
			break
		}
		m.host.focusOn(m.host.focus - 1)
		return m, textinput.Blink
	case "enter":
		if m.host.focus == extraIdx {
			break // newline in the keyword box
		}
		if m.host.focus == groupIdx || m.host.focus == formInputs-1 {
			return m, m.saveHost()
		}
		m.host.focusOn(m.host.focus + 1)
		return m, textinput.Blink
	case "left", "right":
		if m.host.focus == groupIdx {
			groups := append([]string{sshconfig.Ungrouped}, m.host.groups...)
			groups = dedupe(groups)
			i := indexOf(groups, m.host.group)
			if msg.String() == "left" {
				i--
			} else {
				i++
			}
			m.host.group = groups[(i+len(groups))%len(groups)]
			return m, nil
		}
	}
	var cmd tea.Cmd
	if m.host.focus == extraIdx {
		m.host.extra, cmd = m.host.extra.Update(msg)
	} else if m.host.focus < len(m.host.inputs) {
		m.host.inputs[m.host.focus], cmd = m.host.inputs[m.host.focus].Update(msg)
	}
	return m, cmd
}

func (m *Model) saveHost() tea.Cmd {
	n, err := m.host.node()
	if err != nil {
		m.err = err.Error()
		return nil
	}
	if err := m.doc.Upsert(m.host.oldName, n); err != nil {
		m.err = err.Error()
		return nil
	}
	if err := m.save("saved " + n.Alias); err != nil {
		m.err = err.Error()
		return nil
	}
	m.screen = screenList
	return nil
}

func (m *Model) updateGroup(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.screen = screenList
		return m, nil
	case "enter":
		name := strings.TrimSpace(m.group.input.Value())
		var err error
		if m.group.old == "" {
			err = m.doc.AddGroup(name)
		} else {
			err = m.doc.RenameGroup(m.group.old, name)
		}
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		if err := m.save("saved group " + name); err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.screen = screenList
		return m, nil
	}
	var cmd tea.Cmd
	m.group.input, cmd = m.group.input.Update(msg)
	return m, cmd
}

func (m *Model) updateMove(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.screen = screenList
	case "up", "k":
		if m.move.cursor > 0 {
			m.move.cursor--
		}
	case "down", "j":
		if m.move.cursor < len(m.move.groups)-1 {
			m.move.cursor++
		}
	case "enter":
		g := m.move.groups[m.move.cursor]
		if err := m.doc.MoveToGroup([]string{m.move.alias}, g); err != nil {
			m.err = err.Error()
			return m, nil
		}
		if err := m.save(fmt.Sprintf("moved %s to %s", m.move.alias, g)); err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.screen = screenList
	}
	return m, nil
}

func (m *Model) updateSettings(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.settings.firstRun {
			return m, tea.Quit
		}
		applyColor(m.cfg.Color)
		m.screen = screenList
		return m, nil
	case "tab", "shift+tab":
		m.settings.focus = 1 - m.settings.focus
		if m.settings.focus == 0 {
			m.settings.input.Focus()
		} else {
			m.settings.input.Blur()
		}
		return m, textinput.Blink
	case "ctrl+n":
		if len(m.settings.candidates) > 0 {
			m.settings.input.SetValue(m.settings.candidates[m.settings.candidate%len(m.settings.candidates)])
			m.settings.input.CursorEnd()
			m.settings.candidate++
		}
		return m, nil
	case "left", "right":
		if m.settings.focus == 1 {
			i := m.settings.color
			if msg.String() == "left" {
				i--
			} else {
				i++
			}
			m.settings.color = (i + len(Colors)) % len(Colors)
			applyColor(Colors[m.settings.color]) // preview as you go
			return m, nil
		}
	case "enter":
		path := config.ExpandPath(m.settings.input.Value())
		if path == "" {
			m.err = "config file cannot be empty"
			return m, nil
		}
		m.cfg.ConfigFile = path
		m.cfg.Color = Colors[m.settings.color]
		if err := m.cfg.Save(); err != nil {
			m.err = err.Error()
			return m, nil
		}
		if err := m.reload(); err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.settings.firstRun = false
		m.status, m.err = "settings saved", ""
		m.screen = screenList
		return m, nil
	}
	var cmd tea.Cmd
	if m.settings.focus == 0 {
		m.settings.input, cmd = m.settings.input.Update(msg)
	}
	return m, cmd
}

func (m *Model) ask(question string, action func() error) {
	m.confirm = confirmPrompt{question: question, action: action}
	m.screen = screenConfirm
}

func (m *Model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		if err := m.confirm.action(); err != nil {
			m.err = err.Error()
		}
		m.screen = screenList
	case "n", "N", "esc", "q":
		m.screen = screenList
	}
	return m, nil
}

// save writes the document and refreshes the rows.
func (m *Model) save(status string) error {
	if err := m.doc.Save(); err != nil {
		return err
	}
	m.status, m.err = status, ""
	m.rebuild()
	return nil
}

type connectedMsg struct{ err error }

// connect suspends the TUI and hands the terminal to ssh.
func connect(alias string) tea.Cmd {
	path, err := exec.LookPath("ssh")
	if err != nil {
		return func() tea.Msg { return connectedMsg{err: fmt.Errorf("ssh not found on PATH")} }
	}
	c := exec.Command(path, alias)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return tea.ExecProcess(c, func(err error) tea.Msg { return connectedMsg{err: err} })
}

func dedupe(xs []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
