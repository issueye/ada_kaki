//go:build windows

package procjob

import (
	"fmt"
	"os"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// directChildren lists this process's children from a Toolhelp snapshot.
func directChildren() (map[int]struct{}, error) {
	parents, err := processParents()
	if err != nil {
		return nil, err
	}
	self := uint32(os.Getpid())
	out := make(map[int]struct{})
	for pid, parent := range parents {
		if parent == self {
			out[int(pid)] = struct{}{}
		}
	}
	return out, nil
}

// newChild returns one of this process's new children, preferring the
// one whose executable is exe: ConPTY may start conhost before the shell.
func newChild(before map[int]struct{}, exe string) int {
	children, names, err := childEntries()
	if err != nil {
		return 0
	}
	found := 0
	for pid := range children {
		if _, ok := before[pid]; ok {
			continue
		}
		base := strings.ToLower(names[pid])
		switch {
		case exe != "" && strings.EqualFold(base, exe):
			return pid
		case !strings.EqualFold(base, "conhost.exe") && !strings.EqualFold(base, "openconsole.exe"):
			if found == 0 {
				found = pid
			}
		}
	}
	return found
}

// childEntries returns this process's children and their executable names.
func childEntries() (map[int]struct{}, map[int]string, error) {
	parents, err := processParents()
	if err != nil {
		return nil, nil, err
	}
	names, err := processNames()
	if err != nil {
		return nil, nil, err
	}
	self := uint32(os.Getpid())
	out := make(map[int]struct{})
	for pid, parent := range parents {
		if parent == self {
			out[int(pid)] = struct{}{}
		}
	}
	return out, names, nil
}

// killTree terminates pid and every descendant, children first.
func killTree(pid int) error {
	parents, err := processParents()
	if err != nil {
		return err
	}
	// Only a direct child is ours to end: a reused PID of some other
	// process must not be killed in a session's name.
	if parents[uint32(pid)] != uint32(os.Getpid()) {
		return fmt.Errorf("process %d is not a child of this app", pid)
	}
	children := make(map[int][]int)
	for child, parent := range parents {
		children[int(parent)] = append(children[int(parent)], int(child))
	}
	var order []int
	var walk func(int)
	walk = func(p int) {
		for _, c := range children[p] {
			walk(c)
		}
		order = append(order, p)
	}
	walk(pid)

	var errs []error
	for _, p := range order {
		h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(p))
		if err != nil {
			if p == pid {
				errs = append(errs, err)
			}
			continue // it exited meanwhile
		}
		if err := windows.TerminateProcess(h, 1); err != nil && p == pid {
			errs = append(errs, err)
		}
		_ = windows.CloseHandle(h)
	}
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// processNames maps a PID to its executable name.
func processNames() (map[int]string, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snap)
	entry := windows.ProcessEntry32{}
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(snap, &entry); err != nil {
		return nil, err
	}
	out := make(map[int]string)
	for {
		out[int(entry.ProcessID)] = windows.UTF16ToString(entry.ExeFile[:])
		if err := windows.Process32Next(snap, &entry); err != nil {
			if err == windows.ERROR_NO_MORE_FILES {
				return out, nil
			}
			return nil, err
		}
	}
}

// processParents maps a PID to its parent's PID.
func processParents() (map[uint32]uint32, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snap)
	entry := windows.ProcessEntry32{}
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(snap, &entry); err != nil {
		return nil, err
	}
	out := make(map[uint32]uint32)
	for {
		out[entry.ProcessID] = entry.ParentProcessID
		if err := windows.Process32Next(snap, &entry); err != nil {
			if err == windows.ERROR_NO_MORE_FILES {
				return out, nil
			}
			return nil, err
		}
	}
}
