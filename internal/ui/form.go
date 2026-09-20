package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"

	"github.com/scotthellings/ssh-manager/internal/config"
	"github.com/scotthellings/ssh-manager/internal/sshconfig"
)

// fields are the keywords the host form exposes directly; everything else
// lives in the free-form "Other" box.
var fields = []struct {
	key, label, placeholder string
}{
	{"", "Host", "short name you type: ssh <name>"},
	{"HostName", "HostName", "real host or IP"},
	{"User", "User", "login user"},
	{"Port", "Port", "22"},
	{"IdentityFile", "IdentityFile", "~/.ssh/id_ed25519"},
	{"ProxyJump", "ProxyJump", "bastion.example.com"},
	{"LocalForward", "LocalForward", "8000 localhost:3306"},
	{"", "Comment", "note shown beside the host"},
}

var (
	fieldAlias = 0
	fieldPort  = 3
	fieldIdent = 4
	fieldFwd   = 6
	fieldNote  = 7
	// extraIdx is the index of the free-form keyword box, after the fields.
	extraIdx = len(fields)
	// groupIdx is the group picker, last of all.
	groupIdx   = extraIdx + 1
	formInputs = groupIdx + 1
)

// hostForm edits one Host block.
type hostForm struct {
	inputs  []textinput.Model
	extra   textarea.Model
	focus   int
	oldName string
	group   string
	groups  []string
	enabled bool
}

func newHostForm(n *sshconfig.Node, groups []string, group string, width int) hostForm {
	f := hostForm{group: group, groups: groups, enabled: true}
	for _, fd := range fields {
		in := textinput.New()
		in.Placeholder = fd.placeholder
		in.CharLimit = 256
		in.Width = width
		f.inputs = append(f.inputs, in)
	}
	ta := textarea.New()
	ta.Placeholder = "ForwardAgent yes\nServerAliveInterval 60"
	ta.SetWidth(width)
	ta.SetHeight(4)
	ta.ShowLineNumbers = false
	f.extra = ta

	if n != nil {
		f.oldName, f.group, f.enabled = n.Alias, n.Group, n.Enabled
		f.inputs[fieldAlias].SetValue(n.Alias)
		f.inputs[fieldNote].SetValue(n.Comment)
		var rest []sshconfig.Param
		seen := map[string]bool{}
		for _, p := range n.Params {
			if i := fieldIndex(p.Key); i >= 0 && !seen[strings.ToLower(p.Key)] {
				seen[strings.ToLower(p.Key)] = true
				f.inputs[i].SetValue(p.Value)
				continue
			}
			rest = append(rest, p)
		}
		f.extra.SetValue(sshconfig.RenderParams(rest))
	}
	if keys := config.Keys(); len(keys) > 0 {
		f.inputs[fieldIdent].Placeholder = keys[0]
	}
	f.inputs[0].Focus()
	return f
}

// fieldIndex maps a keyword to its form field, or -1 when it has none.
func fieldIndex(key string) int {
	for i, fd := range fields {
		if fd.key != "" && strings.EqualFold(fd.key, key) {
			return i
		}
	}
	return -1
}

func (f *hostForm) setWidth(w int) {
	// the zero-value form has no textarea yet; nothing to resize.
	if f.inputs == nil {
		return
	}
	for i := range f.inputs {
		f.inputs[i].Width = w
	}
	f.extra.SetWidth(w)
}

func (f *hostForm) blurAll() {
	for i := range f.inputs {
		f.inputs[i].Blur()
	}
	f.extra.Blur()
}

func (f *hostForm) focusOn(i int) {
	f.blurAll()
	f.focus = (i + formInputs) % formInputs
	switch f.focus {
	case extraIdx:
		f.extra.Focus()
	case groupIdx:
	default:
		f.inputs[f.focus].Focus()
	}
}

// node builds the host block the form describes.
func (f *hostForm) node() (sshconfig.Node, error) {
	n := sshconfig.Node{
		Kind:    sshconfig.KindHost,
		Alias:   strings.TrimSpace(f.inputs[fieldAlias].Value()),
		Comment: strings.TrimSpace(f.inputs[fieldNote].Value()),
		Group:   f.group,
		Enabled: f.enabled,
	}
	for i, fd := range fields {
		if fd.key == "" {
			continue
		}
		if v := strings.TrimSpace(f.inputs[i].Value()); v != "" {
			n.Params = append(n.Params, sshconfig.Param{Key: fd.key, Value: v})
		}
	}
	extra, err := sshconfig.ParseParams(f.extra.Value())
	if err != nil {
		return n, err
	}
	n.Params = append(n.Params, extra...)
	return n, nil
}

// groupForm creates or renames a group.
type groupForm struct {
	input textinput.Model
	old   string
}

func newGroupForm(old string, width int) groupForm {
	in := textinput.New()
	in.Placeholder = "group name"
	in.CharLimit = 64
	in.Width = width
	in.SetValue(old)
	in.CursorEnd()
	in.Focus()
	return groupForm{input: in, old: old}
}

// moveForm picks a destination group for the host under the cursor.
type moveForm struct {
	groups []string
	cursor int
	alias  string
}

// settingsForm edits the config file path and accent colour.
type settingsForm struct {
	input      textinput.Model
	candidates []string
	candidate  int
	color      int
	focus      int // 0 path, 1 colour
	firstRun   bool
}

func newSettingsForm(cfg *config.Config, firstRun bool, width int) settingsForm {
	in := textinput.New()
	in.Placeholder = config.Default()
	in.CharLimit = 256
	in.Width = width
	in.SetValue(cfg.ConfigFile)
	in.CursorEnd()
	in.Focus()
	return settingsForm{
		input:      in,
		candidates: config.Candidates(),
		color:      indexOf(Colors, colorOrDefault(cfg.Color)),
		firstRun:   firstRun,
	}
}

func colorOrDefault(c string) string {
	if ValidColor(c) {
		return c
	}
	return DefaultColor
}

// confirmPrompt is a yes/no question with an action to run on yes.
type confirmPrompt struct {
	question string
	action   func() error
}
