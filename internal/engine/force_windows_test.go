//go:build windows

package engine

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unsafe"

	"adakaki/internal/shells"
	"golang.org/x/sys/windows"
)

// TestForceCloseKillsShellTree starts a command in a cmd session, then
// force-closes the session: both the shell and the command must go at
// once, without the hangup's wait.
func TestForceCloseKillsShellTree(t *testing.T) {
	m := NewManager()
	defer m.CloseAll()
	s, err := m.CreateLocal(shells.Cmd, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if s.rootPID == 0 {
		t.Fatal("the session has no root PID")
	}
	s.Send("ping -n 60 127.0.0.1 >nul\r\n")

	var ping int
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
		if ping = childNamed(s.rootPID, "PING.EXE"); ping != 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if ping == 0 {
		t.Fatalf("the shell did not start ping: root=%d (%s) state=%v text=%q tree=%v",
			s.rootPID, exeOf(s.rootPID), s.State(), s.Text(), treeNames(s.rootPID))
	}

	if err := s.ForceClose(); err != nil {
		t.Fatalf("ForceClose: %v", err)
	}
	waitFor(t, 3*time.Second, func() bool {
		return !processAlive(s.rootPID) && !processAlive(ping)
	})
}

func exeOf(pid int) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return "?"
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, 260)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return "?"
	}
	return windows.UTF16ToString(buf[:n])
}

func treeNames(root int) []string {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	children := make(map[int][]int)
	var entries []windows.ProcessEntry32
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	if err := windows.Process32First(snap, &entry); err != nil {
		return nil
	}
	for {
		entries = append(entries, entry)
		children[int(entry.ParentProcessID)] = append(children[int(entry.ParentProcessID)], int(entry.ProcessID))
		if err := windows.Process32Next(snap, &entry); err != nil {
			break
		}
	}
	var out []string
	var walk func(int)
	walk = func(p int) {
		for _, c := range children[p] {
			for _, e := range entries {
				if int(e.ProcessID) == c {
					out = append(out, fmt.Sprintf("%d:%s(%d)", c, windows.UTF16ToString(e.ExeFile[:]), e.ParentProcessID))
				}
			}
			walk(c)
		}
	}
	walk(root)
	return out
}

func processAlive(pid int) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	_ = windows.CloseHandle(h)
	return true
}

// childNamed finds a descendant of root whose executable is name.
func childNamed(root int, name string) int {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0
	}
	defer windows.CloseHandle(snap)
	children := make(map[int][]int)
	var entries []windows.ProcessEntry32
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	if err := windows.Process32First(snap, &entry); err != nil {
		return 0
	}
	for {
		entries = append(entries, entry)
		children[int(entry.ParentProcessID)] = append(children[int(entry.ParentProcessID)], int(entry.ProcessID))
		if err := windows.Process32Next(snap, &entry); err != nil {
			break
		}
	}
	var found int
	var walk func(int)
	walk = func(p int) {
		for _, c := range children[p] {
			for _, e := range entries {
				if int(e.ProcessID) == c && strings.EqualFold(windows.UTF16ToString(e.ExeFile[:]), name) {
					found = c
				}
			}
			walk(c)
		}
	}
	walk(root)
	return found
}
