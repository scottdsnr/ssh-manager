package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/scotthellings/ssh-manager/internal/sshconfig"
)

func (m *Model) View() string {
	switch m.screen {
	case screenHost:
		return m.hostView()
	case screenGroup:
		return m.groupView()
	case screenSettings:
		return m.settingsView()
	case screenConfirm:
		return m.confirmView()
	case screenMove:
		return m.moveView()
	case screenHelp:
		return m.helpView()
	}
	return m.listView()
}

func (m *Model) header(sub string) string {
	line := titleStyle.Render(" ssh-manager ")
	if sub != "" {
		line += "  " + helpStyle.Render(sub)
	}
	return line + "\n\n"
}

func (m *Model) footer(keys string) string {
	var b strings.Builder
	b.WriteString("\n" + helpStyle.Render(keys) + "\n")
	if m.err != "" {
		b.WriteString(errStyle.Render(m.err) + "\n")
	} else if m.status != "" {
		b.WriteString(okStyle.Render(m.status) + "\n")
	}
	return b.String()
}

func (m *Model) listView() string {
	var b strings.Builder
	b.WriteString(m.header(m.cfg.ConfigFile))

	if len(m.rows) == 0 {
		b.WriteString(helpStyle.Render("no hosts yet — press n to add one") + "\n")
	}
	for i, r := range m.rows {
		cursor := "  "
		if i == m.cursor {
			cursor = cursorStyle.Render("> ")
		}
		if r.kind == rowGroup {
			mark := " "
			if m.collapse[r.group] {
				mark = "▸"
			}
			b.WriteString(cursor + groupStyle.Render(mark+" "+r.group) + "\n")
			continue
		}
		b.WriteString(cursor + "  " + m.hostLine(r.node) + "\n")
	}

	if m.filtering || m.filter.Value() != "" {
		b.WriteString("\n" + m.filter.View() + "\n")
	}
	return b.String() + m.footer("n new  enter edit  c connect  space on/off  d delete  m move  a group  r rename  / filter  s settings  ? help  q quit")
}

// hostLine renders one host as "alias  user@target  # comment".
func (m *Model) hostLine(n *sshconfig.Node) string {
	name := nameStyle.Render(pad(n.Alias, 18))
	detail := n.Target()
	if f := n.Get("LocalForward"); f != "" {
		detail += "  ⇢ " + f
	} else if j := n.Get("ProxyJump"); j != "" {
		detail += "  via " + j
	}
	line := name + " " + cmdStyle.Render(detail)
	if n.Comment != "" {
		line += cmdStyle.Render("  # " + n.Comment)
	}
	if !n.Enabled {
		return disabledStyle.Render(stripANSI(line))
	}
	return line
}

func (m *Model) hostView() string {
	title := "new host"
	if m.host.oldName != "" {
		title = "edit " + m.host.oldName
	}
	var b strings.Builder
	b.WriteString(m.header(title))
	for i, fd := range fields {
		label := labelStyle.Render(fd.label)
		if i == m.host.focus {
			label = cursorStyle.Render(pad(fd.label, 13))
		}
		b.WriteString(label + " " + m.host.inputs[i].View() + "\n")
	}
	other := labelStyle.Render("Other")
	if m.host.focus == extraIdx {
		other = cursorStyle.Render(pad("Other", 13))
	}
	b.WriteString("\n" + other + "\n" + m.host.extra.View() + "\n")

	group := labelStyle.Render("Group")
	if m.host.focus == groupIdx {
		group = cursorStyle.Render(pad("Group", 13))
	}
	b.WriteString("\n" + group + " " + boxStyle.Render(m.host.group) + "\n")
	return b.String() + m.footer("tab next  ←/→ group  ctrl+s save  esc cancel")
}

func (m *Model) groupView() string {
	title := "new group"
	if m.group.old != "" {
		title = "rename " + m.group.old
	}
	return m.header(title) + m.group.input.View() + "\n" + m.footer("enter save  esc cancel")
}

func (m *Model) moveView() string {
	var b strings.Builder
	b.WriteString(m.header("move " + m.move.alias + " to group"))
	for i, g := range m.move.groups {
		cursor := "  "
		if i == m.move.cursor {
			cursor = cursorStyle.Render("> ")
		}
		b.WriteString(cursor + g + "\n")
	}
	return b.String() + m.footer("enter move  esc cancel")
}

func (m *Model) settingsView() string {
	var b strings.Builder
	b.WriteString(m.header("settings"))
	b.WriteString(labelStyle.Render("Config") + " " + m.settings.input.View() + "\n")
	if len(m.settings.candidates) > 0 {
		b.WriteString(helpStyle.Render("   found: "+strings.Join(m.settings.candidates, ", ")) + "\n")
	}
	swatch := lipgloss.NewStyle().Foreground(accents[Colors[m.settings.color]]).Render("████ " + Colors[m.settings.color])
	label := labelStyle.Render("Colour")
	if m.settings.focus == 1 {
		label = cursorStyle.Render(pad("Colour", 10))
	}
	b.WriteString("\n" + label + " " + swatch + "\n")
	return b.String() + m.footer("tab switch  ctrl+n cycle found files  ←/→ colour  enter save  esc back")
}

func (m *Model) confirmView() string {
	return m.header("") + boxStyle.Render(m.confirm.question) + "\n" + m.footer("y yes  n no")
}

func (m *Model) helpView() string {
	rows := [][2]string{
		{"↑/↓, k/j", "move"},
		{"enter", "fold a group / edit the host under the cursor"},
		{"n", "new host"},
		{"c", "ssh to the host under the cursor"},
		{"space", "comment the host block out, or back in"},
		{"d", "delete the host or group under the cursor"},
		{"m", "move the host to another group"},
		{"a / r", "add a group / rename the group under the cursor"},
		{"/", "filter by name, keyword or value"},
		{"s", "settings"},
		{"q", "quit"},
	}
	var b strings.Builder
	b.WriteString(m.header("keys"))
	for _, r := range rows {
		b.WriteString(labelStyle.Render(pad(r[0], 10)) + " " + r[1] + "\n")
	}
	return b.String() + m.footer("any key to go back")
}

func pad(s string, w int) string {
	if len(s) >= w {
		return s
	}
	return s + strings.Repeat(" ", w-len(s))
}

// stripANSI removes styling so a disabled row can be struck through as one
// piece rather than in coloured fragments.
func stripANSI(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		switch {
		case r == '\x1b':
			in = true
		case in && r == 'm':
			in = false
		case !in:
			b.WriteRune(r)
		}
	}
	return b.String()
}
