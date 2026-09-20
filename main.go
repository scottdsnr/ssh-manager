// Command ssh-manager is a terminal UI for viewing, creating, editing,
// grouping and disabling the Host blocks in your ssh config.
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/scotthellings/ssh-manager/internal/config"
	"github.com/scotthellings/ssh-manager/internal/ui"
)

// versionFile is the VERSION file baked in at build time; release builds
// override version via -ldflags "-X main.version=...".
//
//go:embed VERSION
var versionFile string

var version string

func init() {
	if version == "" {
		version = strings.TrimSpace(versionFile)
	}
}

func main() {
	setup := flag.Bool("setup", false, "open the settings screen on start")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("ssh-manager", version)
		return
	}

	cfg, configured, err := config.Load()
	if err != nil {
		fail(err)
	}

	m, err := ui.New(cfg, !configured || *setup)
	if err != nil {
		fail(err)
	}
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "ssh-manager:", err)
	os.Exit(1)
}
