// Package sshconfig parses and rewrites ~/.ssh/config as an ordered document,
// so unrelated content (Match blocks, Include lines, hand-written comments)
// survives a round-trip untouched.
package sshconfig

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	// groupPrefix marks a group header comment written by ssh-manager.
	groupPrefix = "# ===== "
	groupSuffix = " ====="
	// disabledPrefix marks a host block that is commented out but managed.
	disabledPrefix = "#!"
	// Ungrouped is the bucket for hosts that sit outside any group header.
	Ungrouped = "Ungrouped"
	// indent is the indentation used for keyword lines we write.
	indent = "    "
)

type Kind int

const (
	KindRaw Kind = iota
	KindGroup
	KindHost
)

// Param is one keyword line inside a Host block. Duplicates are legal and
// meaningful (IdentityFile, LocalForward), so params stay an ordered list.
type Param struct {
	Key   string
	Value string
}

// Node is one entry of the document: a host block, a group header, or a
// verbatim line we do not manage.
type Node struct {
	Kind Kind
	Raw  string // verbatim line, for KindRaw

	Group string // for KindGroup: its name. For KindHost: owning group.

	// Alias is the Host pattern list, e.g. "scott-1" or "gh *.github.com".
	Alias   string
	Params  []Param
	Enabled bool
	Comment string // trailing comment on the Host line, if any
}

// Doc is a parsed ssh config file.
type Doc struct {
	Path  string
	Nodes []*Node
}

var (
	groupRe = regexp.MustCompile(`^#\s*=====\s*(.*?)\s*=====\s*$`)
	hostRe  = regexp.MustCompile(`^\s*[Hh][Oo][Ss][Tt]\s+(.*)$`)
	// kwRe matches a keyword line, with "=" or whitespace as the separator.
	kwRe = regexp.MustCompile(`^\s*([A-Za-z][A-Za-z0-9-]*)(?:\s*=\s*|\s+)(.*)$`)
	// blockRe matches keywords that start a block we must not swallow.
	blockRe = regexp.MustCompile(`^\s*(?i:Host|Match)\b`)
)

// Load reads path into a Doc. A missing file parses as an empty document.
func Load(path string) (*Doc, error) {
	d := &Doc{Path: path}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return d, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		lines = append(lines, strings.TrimRight(sc.Text(), "\r"))
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	group := Ungrouped
	for i := 0; i < len(lines); i++ {
		if m := groupRe.FindStringSubmatch(lines[i]); m != nil {
			group = m[1]
			d.Nodes = append(d.Nodes, &Node{Kind: KindGroup, Group: group, Raw: lines[i]})
			continue
		}
		if n, next := parseHost(lines, i); n != nil {
			n.Group = group
			d.Nodes = append(d.Nodes, n)
			i = next
			continue
		}
		d.Nodes = append(d.Nodes, &Node{Kind: KindRaw, Raw: lines[i]})
	}
	return d, nil
}

// uncomment strips the disabled marker from a line, reporting whether it was
// there. A disabled block carries "#!" on every one of its lines.
func uncomment(line string) (string, bool) {
	t := strings.TrimSpace(line)
	if !strings.HasPrefix(t, disabledPrefix) {
		return line, false
	}
	idx := strings.Index(line, disabledPrefix)
	return line[:idx] + line[idx+len(disabledPrefix):], true
}

// parseHost reads a Host block starting at lines[i], returning the node and
// the index of its last line, or nil if this is not a Host line.
func parseHost(lines []string, i int) (*Node, int) {
	head, disabled := uncomment(lines[i])
	m := hostRe.FindStringSubmatch(head)
	if m == nil {
		return nil, i
	}
	alias, comment := splitComment(m[1])
	if strings.TrimSpace(alias) == "" {
		return nil, i
	}
	n := &Node{Kind: KindHost, Raw: lines[i], Alias: strings.TrimSpace(alias), Enabled: !disabled, Comment: comment}
	last := i
	for j := i + 1; j < len(lines); j++ {
		raw := lines[j]
		if disabled {
			var off bool
			raw, off = uncomment(raw)
			// A disabled block stops at the first line without the marker.
			if !off {
				break
			}
		}
		if strings.TrimSpace(raw) == "" || strings.HasPrefix(strings.TrimSpace(raw), "#") {
			break
		}
		if blockRe.MatchString(raw) {
			break
		}
		km := kwRe.FindStringSubmatch(raw)
		if km == nil {
			break
		}
		n.Params = append(n.Params, Param{Key: CanonicalKey(km[1]), Value: strings.TrimSpace(km[2])})
		last = j
	}
	return n, last
}

// splitComment separates a trailing comment from a Host line's pattern list.
func splitComment(s string) (value, comment string) {
	if i := strings.Index(s, "#"); i >= 0 {
		return strings.TrimSpace(s[:i]), strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s[i:]), "#"))
	}
	return strings.TrimSpace(s), ""
}

// Render turns the document back into file text.
func (d *Doc) Render() string {
	var b strings.Builder
	for _, n := range d.Nodes {
		switch n.Kind {
		case KindGroup:
			b.WriteString(groupPrefix + n.Group + groupSuffix)
		case KindHost:
			b.WriteString(n.Lines())
		default:
			b.WriteString(n.Raw)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// Lines renders a host block as its full multi-line definition.
func (n *Node) Lines() string {
	head := "Host " + n.Alias
	if n.Comment != "" {
		head += " # " + n.Comment
	}
	out := []string{head}
	for _, p := range n.Params {
		if strings.TrimSpace(p.Value) == "" {
			continue
		}
		out = append(out, indent+p.Key+" "+p.Value)
	}
	if !n.Enabled {
		for i, l := range out {
			out[i] = disabledPrefix + l
		}
	}
	return strings.Join(out, "\n")
}

// Get returns the first value for key, or "".
func (n *Node) Get(key string) string {
	for _, p := range n.Params {
		if strings.EqualFold(p.Key, key) {
			return p.Value
		}
	}
	return ""
}

// All returns every value for key, in order.
func (n *Node) All(key string) []string {
	var out []string
	for _, p := range n.Params {
		if strings.EqualFold(p.Key, key) {
			out = append(out, p.Value)
		}
	}
	return out
}

// Set replaces every value for key with one value, keeping the key's original
// position. An empty value removes the key.
func (n *Node) Set(key, value string) {
	value = strings.TrimSpace(value)
	var out []Param
	done := false
	for _, p := range n.Params {
		if !strings.EqualFold(p.Key, key) {
			out = append(out, p)
			continue
		}
		if !done && value != "" {
			out = append(out, Param{Key: CanonicalKey(key), Value: value})
			done = true
		}
	}
	if !done && value != "" {
		out = append(out, Param{Key: CanonicalKey(key), Value: value})
	}
	n.Params = out
}

// Target is the host:port a connection would reach, for display.
func (n *Node) Target() string {
	h := n.Get("HostName")
	if h == "" {
		h = n.Alias
	}
	if u := n.Get("User"); u != "" {
		h = u + "@" + h
	}
	if p := n.Get("Port"); p != "" && p != "22" {
		h += ":" + p
	}
	return h
}

// Save writes the document atomically, keeping a .bak of the previous content.
func (d *Doc) Save() error {
	if err := os.MkdirAll(filepath.Dir(d.Path), 0o700); err != nil {
		return err
	}
	mode := os.FileMode(0o600)
	if fi, err := os.Stat(d.Path); err == nil {
		mode = fi.Mode().Perm()
		if prev, err := os.ReadFile(d.Path); err == nil {
			_ = os.WriteFile(d.Path+".bak", prev, mode)
		}
	}
	tmp := fmt.Sprintf("%s.tmp-%d", d.Path, time.Now().UnixNano())
	if err := os.WriteFile(tmp, []byte(d.Render()), mode); err != nil {
		return err
	}
	return os.Rename(tmp, d.Path)
}

// Groups returns group names in file order, Ungrouped first when it holds
// anything.
func (d *Doc) Groups() []string {
	var out []string
	seen := map[string]bool{}
	add := func(g string) {
		if !seen[g] {
			seen[g] = true
			out = append(out, g)
		}
	}
	for _, n := range d.Nodes {
		if n.Kind == KindHost && n.Group == Ungrouped {
			add(Ungrouped)
		}
	}
	for _, n := range d.Nodes {
		if n.Kind == KindGroup {
			add(n.Group)
		}
	}
	return out
}

// Hosts returns the host blocks in a group, in file order.
func (d *Doc) Hosts(group string) []*Node {
	var out []*Node
	for _, n := range d.Nodes {
		if n.Kind == KindHost && n.Group == group {
			out = append(out, n)
		}
	}
	return out
}

// Find returns the node for a host alias, or nil.
func (d *Doc) Find(alias string) *Node {
	for _, n := range d.Nodes {
		if n.Kind == KindHost && n.Alias == alias {
			return n
		}
	}
	return nil
}

// AddGroup appends a group header if it does not already exist.
func (d *Doc) AddGroup(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("group name cannot be empty")
	}
	if strings.Contains(name, "=====") {
		return fmt.Errorf("group name cannot contain %q", "=====")
	}
	for _, g := range d.Groups() {
		if strings.EqualFold(g, name) {
			return fmt.Errorf("group %q already exists", name)
		}
	}
	if len(d.Nodes) > 0 && strings.TrimSpace(d.Nodes[len(d.Nodes)-1].Raw) != "" {
		d.Nodes = append(d.Nodes, &Node{Kind: KindRaw, Raw: ""})
	}
	d.Nodes = append(d.Nodes, &Node{Kind: KindGroup, Group: name, Raw: groupPrefix + name + groupSuffix})
	return nil
}

// RenameGroup renames a group header and its members.
func (d *Doc) RenameGroup(old, name string) error {
	name = strings.TrimSpace(name)
	if name == "" || old == Ungrouped {
		return fmt.Errorf("cannot rename this group")
	}
	for _, n := range d.Nodes {
		if n.Group == old {
			n.Group = name
		}
	}
	return nil
}

// DeleteGroup removes the header; its hosts fall back to Ungrouped unless
// withHosts is set, in which case they go too.
func (d *Doc) DeleteGroup(group string, withHosts bool) {
	var keep []*Node
	for _, n := range d.Nodes {
		switch {
		case n.Kind == KindGroup && n.Group == group:
			continue
		case n.Kind == KindHost && n.Group == group:
			if withHosts {
				continue
			}
			n.Group = Ungrouped
			keep = append(keep, n)
		default:
			keep = append(keep, n)
		}
	}
	d.Nodes = keep
	d.reflow()
}

// Upsert creates or updates a host block, placing new blocks at the end of
// their group.
func (d *Doc) Upsert(oldAlias string, n Node) error {
	if err := ValidateAlias(n.Alias); err != nil {
		return err
	}
	if err := ValidateParams(n.Params); err != nil {
		return err
	}
	if n.Group == "" {
		n.Group = Ungrouped
	}
	if ex := d.Find(n.Alias); ex != nil && n.Alias != oldAlias {
		return fmt.Errorf("host %q already exists", n.Alias)
	}
	if oldAlias != "" {
		if ex := d.Find(oldAlias); ex != nil {
			group := ex.Group
			ex.Alias, ex.Params, ex.Enabled, ex.Comment = n.Alias, n.Params, n.Enabled, n.Comment
			if group == n.Group {
				return nil
			}
			d.remove(oldAlias)
		}
	}
	node := n
	node.Kind, node.Raw = KindHost, ""
	d.insert(&node)
	return nil
}

// Delete removes a host block.
func (d *Doc) Delete(alias string) { d.remove(alias); d.reflow() }

func (d *Doc) remove(alias string) {
	for i, n := range d.Nodes {
		if n.Kind == KindHost && n.Alias == alias {
			d.Nodes = append(d.Nodes[:i], d.Nodes[i+1:]...)
			return
		}
	}
}

// insert places node after the last line belonging to its group.
func (d *Doc) insert(node *Node) {
	if node.Group == Ungrouped {
		// Before the first group header, so it stays out of every group.
		for i, n := range d.Nodes {
			if n.Kind == KindGroup {
				d.Nodes = append(d.Nodes[:i], append([]*Node{node}, d.Nodes[i:]...)...)
				return
			}
		}
		d.Nodes = append(d.Nodes, node)
		return
	}
	last := -1
	inGroup := false
	for i, n := range d.Nodes {
		switch {
		case n.Kind == KindGroup && n.Group == node.Group:
			inGroup, last = true, i
		case n.Kind == KindGroup:
			inGroup = false
		case inGroup && n.Kind == KindHost:
			last = i
		}
	}
	if last < 0 {
		_ = d.AddGroup(node.Group)
		d.Nodes = append(d.Nodes, node)
		return
	}
	d.Nodes = append(d.Nodes[:last+1], append([]*Node{node}, d.Nodes[last+1:]...)...)
}

// MoveToGroup moves the named hosts into group, keeping their relative order.
func (d *Doc) MoveToGroup(aliases []string, group string) error {
	if strings.TrimSpace(group) == "" {
		group = Ungrouped
	}
	if group != Ungrouped {
		known := false
		for _, g := range d.Groups() {
			if g == group {
				known = true
			}
		}
		if !known {
			if err := d.AddGroup(group); err != nil {
				return err
			}
		}
	}
	want := map[string]bool{}
	for _, a := range aliases {
		want[a] = true
	}
	var moving []Node
	for _, n := range d.Nodes {
		if n.Kind == KindHost && want[n.Alias] && n.Group != group {
			moving = append(moving, *n)
		}
	}
	for _, n := range moving {
		d.remove(n.Alias)
	}
	for _, n := range moving {
		node := n
		node.Group, node.Raw = group, ""
		d.insert(&node)
	}
	d.reflow()
	return nil
}

// reflow collapses runs of blank lines left behind by deletions.
func (d *Doc) reflow() {
	var keep []*Node
	blank := 0
	for _, n := range d.Nodes {
		if n.Kind == KindRaw && strings.TrimSpace(n.Raw) == "" {
			blank++
			if blank > 1 {
				continue
			}
		} else {
			blank = 0
		}
		keep = append(keep, n)
	}
	d.Nodes = keep
}

var aliasRe = regexp.MustCompile(`^[A-Za-z0-9_.*?\[\]!\-]+$`)

// ValidateAlias rejects Host patterns ssh would not accept. Several
// space-separated patterns on one Host line are allowed.
func ValidateAlias(alias string) error {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return fmt.Errorf("host name cannot be empty")
	}
	for _, p := range strings.Fields(alias) {
		if !aliasRe.MatchString(p) {
			return fmt.Errorf("invalid host pattern %q", p)
		}
	}
	return nil
}

var keyRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]*$`)

// ValidateParams checks keyword names and the handful of values with a shape
// we can check without guessing at ssh's own validation.
func ValidateParams(params []Param) error {
	for _, p := range params {
		if !keyRe.MatchString(p.Key) {
			return fmt.Errorf("invalid keyword %q", p.Key)
		}
		if strings.EqualFold(p.Key, "Port") && p.Value != "" {
			if !regexp.MustCompile(`^[0-9]{1,5}$`).MatchString(p.Value) {
				return fmt.Errorf("port must be a number, got %q", p.Value)
			}
		}
	}
	return nil
}

// ParseParams reads free-form "Keyword value" lines into params, so the form
// can expose keywords the fixed fields do not cover.
func ParseParams(text string) ([]Param, error) {
	var out []Param
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		m := kwRe.FindStringSubmatch(line)
		if m == nil || strings.TrimSpace(m[2]) == "" {
			return nil, fmt.Errorf("cannot parse %q — expected \"Keyword value\"", strings.TrimSpace(line))
		}
		out = append(out, Param{Key: CanonicalKey(m[1]), Value: strings.TrimSpace(m[2])})
	}
	return out, nil
}

// RenderParams is the inverse of ParseParams.
func RenderParams(params []Param) string {
	var b strings.Builder
	for _, p := range params {
		b.WriteString(p.Key + " " + p.Value + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// Keywords are the ssh_config keywords we know how to spell, used to
// canonicalise case and to offer completions in the extra-fields editor.
var Keywords = []string{
	"AddKeysToAgent", "AddressFamily", "BatchMode", "BindAddress", "CertificateFile",
	"Ciphers", "ClearAllForwardings", "Compression", "ConnectTimeout",
	"ConnectionAttempts", "ControlMaster", "ControlPath", "ControlPersist",
	"DynamicForward", "EscapeChar", "ExitOnForwardFailure", "ForwardAgent",
	"ForwardX11", "ForwardX11Trusted", "GatewayPorts", "GlobalKnownHostsFile",
	"GSSAPIAuthentication", "HostKeyAlgorithms", "HostName", "IdentitiesOnly",
	"IdentityAgent", "IdentityFile", "IPQoS", "KbdInteractiveAuthentication",
	"KexAlgorithms", "LocalCommand", "LocalForward", "LogLevel", "MACs",
	"NumberOfPasswordPrompts", "PasswordAuthentication", "PermitLocalCommand",
	"PKCS11Provider", "Port", "PreferredAuthentications", "ProxyCommand",
	"ProxyJump", "PubkeyAcceptedAlgorithms", "PubkeyAuthentication",
	"RekeyLimit", "RemoteCommand", "RemoteForward", "RequestTTY", "SendEnv",
	"ServerAliveCountMax", "ServerAliveInterval", "SetEnv", "StrictHostKeyChecking",
	"TCPKeepAlive", "Tunnel", "UpdateHostKeys", "User", "UserKnownHostsFile",
	"VerifyHostKeyDNS", "VisualHostKey", "XAuthLocation",
}

var canonical = func() map[string]string {
	m := make(map[string]string, len(Keywords))
	for _, k := range Keywords {
		m[strings.ToLower(k)] = k
	}
	return m
}()

// CanonicalKey fixes the capitalisation of a known keyword; unknown keywords
// are left exactly as the user typed them.
func CanonicalKey(k string) string {
	if c, ok := canonical[strings.ToLower(strings.TrimSpace(k))]; ok {
		return c
	}
	return strings.TrimSpace(k)
}
