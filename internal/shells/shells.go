// Package shells detects the shells available on this machine and builds
// the launch commands the terminal plugin runs.
package shells

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ID names a shell type.
type ID string

const (
	Cmd        ID = "cmd"
	PowerShell ID = "powershell"
	Pwsh       ID = "pwsh"
	GitBash    ID = "gitbash"
	Custom     ID = "custom"
)

// Shell is one shell type the console can open, found or not.
type Shell struct {
	ID      ID
	Name    string // the display name
	Path    string // the executable, empty when not found
	Found   bool
}

// Detect returns the local shells in display order, custom last.
func Detect() []Shell {
	shells := []Shell{
		{ID: Cmd, Name: "Command Prompt"},
		{ID: PowerShell, Name: "Windows PowerShell"},
		{ID: Pwsh, Name: "PowerShell"},
		{ID: GitBash, Name: "Git Bash"},
	}
	for i := range shells {
		shells[i].Path = find(shells[i].ID)
		shells[i].Found = shells[i].Path != ""
	}
	return shells
}

// Get returns one shell by ID, found or not. Custom is not in the list.
func Get(id ID) (Shell, bool) {
	for _, s := range Detect() {
		if s.ID == id {
			return s, true
		}
	}
	return Shell{}, false
}

// CustomTitle names a custom session after its command.
func CustomTitle(command string) string {
	if parts := splitCommandLine(command); len(parts) > 0 {
		return filepath.Base(parts[0])
	}
	return "Custom"
}

func find(id ID) string {
	switch id {
	case Cmd:
		if p := os.Getenv("COMSPEC"); p != "" && exists(p) {
			return p
		}
		return underSystemRoot("System32", "cmd.exe")
	case PowerShell:
		return underSystemRoot("System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	case Pwsh:
		if p, err := exec.LookPath("pwsh.exe"); err == nil {
			return p
		}
		return ""
	case GitBash:
		return findGitBash()
	}
	return ""
}

func underSystemRoot(parts ...string) string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	p := filepath.Join(append([]string{root}, parts...)...)
	if exists(p) {
		return p
	}
	return ""
}

// findGitBash looks for Git for Windows' bash, in this order: the PATH
// (excluding WSL's System32 stub), git.exe's own location, the registry,
// then the usual install roots.
func findGitBash() string {
	if p, err := exec.LookPath("bash.exe"); err == nil && !strings.Contains(strings.ToLower(p), `\system32\`) {
		return p
	}
	if git, err := exec.LookPath("git.exe"); err == nil {
		// <git>\cmd\git.exe → <git>\bin\bash.exe
		if p := filepath.Join(filepath.Dir(filepath.Dir(git)), "bin", "bash.exe"); exists(p) {
			return p
		}
	}
	for _, hive := range []string{"HKCU", "HKLM"} {
		if p := fromRegistry(`reg`, "query", hive+`\SOFTWARE\GitForWindows`, "/v", "InstallPath"); p != "" {
			if bash := filepath.Join(p, "bin", "bash.exe"); exists(bash) {
				return bash
			}
		}
	}
	for _, root := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), filepath.Join(os.Getenv("LocalAppData"), "Programs")} {
		if root == "" {
			continue
		}
		if p := filepath.Join(root, "Git", "bin", "bash.exe"); exists(p) {
			return p
		}
	}
	return ""
}

func fromRegistry(name string, args ...string) string {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if i := strings.Index(line, "REG_SZ"); i >= 0 {
			return strings.TrimSpace(line[i+len("REG_SZ"):])
		}
	}
	return ""
}

func exists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

// Launch builds the terminal's command for a shell. custom is the user's
// command line for Custom, split the way Windows splits one; dir is
// unused here (the terminal takes it as Options.Dir).
func Launch(id ID, path, custom string) []string {
	switch id {
	case Cmd:
		// Force UTF-8 so Chinese output arrives clean whatever the
		// system's code page.
		return []string{path, "/k", "chcp 65001 >nul"}
	case PowerShell:
		return []string{path, "-NoLogo", "-NoExit", "-Command",
			`$OutputEncoding=[Console]::OutputEncoding=[Text.Encoding]::UTF8; Clear-Host`}
	case Pwsh:
		return []string{path, "-NoLogo"}
	case GitBash:
		return []string{path, "-i", "-l"}
	case Custom:
		return splitCommandLine(custom)
	}
	return nil
}

// splitCommandLine splits a Windows command line on spaces, keeping
// quoted parts together; a leading program and its arguments.
func splitCommandLine(line string) []string {
	var parts []string
	var cur strings.Builder
	inQuote := false
	flush := func() {
		if cur.Len() > 0 {
			parts = append(parts, cur.String())
			cur.Reset()
		}
	}
	for i := 0; i < len(line); i++ {
		switch c := line[i]; c {
		case '"':
			inQuote = !inQuote
		case ' ', '\t':
			if inQuote {
				cur.WriteByte(c)
			} else {
				flush()
			}
		default:
			cur.WriteByte(c)
		}
	}
	flush()
	return parts
}
