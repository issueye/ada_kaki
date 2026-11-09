package shells

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDetectHasCmd(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows")
	}
	got := Detect()
	if len(got) == 0 {
		t.Fatal("Detect returned nothing")
	}
	cmd := got[0]
	if cmd.ID != Cmd || !cmd.Found || cmd.Path == "" {
		t.Errorf("cmd shell = %+v; want found", cmd)
	}
	if !filepath.IsAbs(cmd.Path) {
		t.Errorf("cmd path %q is not absolute", cmd.Path)
	}
}

func TestLaunchCmdForcesUTF8(t *testing.T) {
	cmd := Launch(Cmd, `C:\Windows\System32\cmd.exe`, "")
	if len(cmd) != 3 || cmd[1] != "/k" || cmd[2] != "chcp 65001 >nul" {
		t.Errorf("cmd launch = %v", cmd)
	}
}

func TestLaunchGitBash(t *testing.T) {
	cmd := Launch(GitBash, `C:\Program Files\Git\bin\bash.exe`, "")
	if len(cmd) != 3 || cmd[1] != "-i" || cmd[2] != "-l" {
		t.Errorf("gitbash launch = %v", cmd)
	}
}

func TestLaunchCustomSplitsQuotes(t *testing.T) {
	got := Launch(Custom, "", `mytool.exe --flag "a b c" x`)
	want := []string{"mytool.exe", "--flag", "a b c", "x"}
	if len(got) != len(want) {
		t.Fatalf("custom launch = %q; want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("custom launch[%d] = %q; want %q", i, got[i], want[i])
		}
	}
}

func TestExistsSkipsDirectories(t *testing.T) {
	dir := t.TempDir()
	if exists(dir) {
		t.Error("exists must be false for a directory")
	}
	p := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if !exists(p) {
		t.Error("exists must be true for a file")
	}
}
