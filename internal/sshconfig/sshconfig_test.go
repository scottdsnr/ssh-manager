package sshconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sample = `# my ssh config
Host scott-1
    HostName notes.dsnr.co.uk
    User scott
    Port 1337

Host github.com
    HostName github.com
    User git
    IdentityFile ~/.ssh/id_ed25519

# ===== Tunnels =====
Host forge-tunnel # work db
    Hostname 68.183.254.60
    User forge
    LocalForward 8000 localhost:3306
    LocalForward 9000 localhost:6379

#!Host github.dsnr
#!    HostName github.com
#!    User git

Match host *.internal
    ProxyJump bastion
`

func load(t *testing.T, text string) *Doc {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	d, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestParse(t *testing.T) {
	d := load(t, sample)
	if got := len(d.Hosts(Ungrouped)); got != 2 {
		t.Fatalf("ungrouped hosts = %d, want 2", got)
	}
	n := d.Find("scott-1")
	if n == nil || n.Get("HostName") != "notes.dsnr.co.uk" || n.Get("Port") != "1337" {
		t.Fatalf("scott-1 parsed as %+v", n)
	}
	f := d.Find("forge-tunnel")
	if f.Group != "Tunnels" {
		t.Errorf("group = %q, want Tunnels", f.Group)
	}
	if f.Comment != "work db" {
		t.Errorf("comment = %q", f.Comment)
	}
	// Hostname/HostName differ only in case; both must reach the same key.
	if f.Get("HostName") != "68.183.254.60" {
		t.Errorf("hostname = %q", f.Get("HostName"))
	}
	if got := f.All("LocalForward"); len(got) != 2 {
		t.Errorf("forwards = %v, want 2", got)
	}
	if dis := d.Find("github.dsnr"); dis == nil || dis.Enabled {
		t.Errorf("disabled block = %+v", dis)
	}
}

func TestMatchBlockIsNotSwallowed(t *testing.T) {
	d := load(t, sample)
	for _, n := range d.Nodes {
		if n.Kind == KindHost && n.Alias == "*.internal" {
			t.Fatal("Match block parsed as a Host")
		}
	}
	if !strings.Contains(d.Render(), "Match host *.internal\n    ProxyJump bastion") {
		t.Fatal("Match block did not survive the round-trip")
	}
}

func TestRoundTrip(t *testing.T) {
	d := load(t, sample)
	// Keyword capitalisation is normalised ("Hostname" -> "HostName");
	// everything else must come back byte for byte.
	want := strings.Replace(sample, "    Hostname ", "    HostName ", 1)
	if got := d.Render(); got != want {
		t.Fatalf("round-trip changed the file:\n%s", got)
	}
}

func TestToggleDisable(t *testing.T) {
	d := load(t, sample)
	n := d.Find("scott-1")
	n.Enabled = false
	out := d.Render()
	if !strings.Contains(out, "#!Host scott-1\n#!    HostName notes.dsnr.co.uk") {
		t.Fatalf("disable rendered as:\n%s", out)
	}
	d2 := load(t, out)
	if d2.Find("scott-1").Enabled {
		t.Fatal("re-parsed as enabled")
	}
	d2.Find("scott-1").Enabled = true
	if d2.Render() != strings.Replace(sample, "    Hostname ", "    HostName ", 1) {
		t.Fatalf("enable did not restore the original:\n%s", d2.Render())
	}
}

func TestUpsertAndGroups(t *testing.T) {
	d := load(t, sample)
	n := Node{Alias: "box", Group: "Tunnels", Enabled: true, Params: []Param{{"HostName", "10.0.0.1"}, {"User", "root"}}}
	if err := d.Upsert("", n); err != nil {
		t.Fatal(err)
	}
	if d.Find("box").Group != "Tunnels" {
		t.Fatal("new host landed outside its group")
	}
	if err := d.Upsert("", n); err == nil {
		t.Fatal("duplicate alias accepted")
	}
	// Editing keeps the host in place and renames it.
	n.Alias = "box2"
	if err := d.Upsert("box", n); err != nil {
		t.Fatal(err)
	}
	if d.Find("box") != nil || d.Find("box2") == nil {
		t.Fatal("rename failed")
	}
	if err := d.MoveToGroup([]string{"box2"}, Ungrouped); err != nil {
		t.Fatal(err)
	}
	if d.Find("box2").Group != Ungrouped {
		t.Fatal("move failed")
	}
	d.Delete("box2")
	if d.Find("box2") != nil {
		t.Fatal("delete failed")
	}
}

func TestValidation(t *testing.T) {
	if err := ValidateAlias("a b *.c"); err != nil {
		t.Errorf("pattern list rejected: %v", err)
	}
	if err := ValidateAlias("bad name!/x"); err == nil {
		t.Error("slash accepted")
	}
	if err := ValidateParams([]Param{{"Port", "nope"}}); err == nil {
		t.Error("non-numeric port accepted")
	}
}

func TestParseParams(t *testing.T) {
	got, err := ParseParams("forwardagent yes\n\nServerAliveInterval=60")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Key != "ForwardAgent" || got[1].Value != "60" {
		t.Fatalf("got %+v", got)
	}
	if _, err := ParseParams("nonsense"); err == nil {
		t.Error("bare word accepted")
	}
}

func TestSaveKeepsBackupAndMode(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config")
	if err := os.WriteFile(p, []byte(sample), 0o600); err != nil {
		t.Fatal(err)
	}
	d, _ := Load(p)
	d.Delete("scott-1")
	if err := d.Save(); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(p + ".bak"); err != nil || string(b) != sample {
		t.Fatal("backup missing or wrong")
	}
	fi, _ := os.Stat(p)
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", fi.Mode().Perm())
	}
}
