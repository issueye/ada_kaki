//go:build windows

package procjob

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// TestGuardKillsChildrenOnExit runs a helper in a guard job, has it start
// a long-running grandchild, then lets the helper exit: the grandchild must
// go with it.
func TestGuardKillsChildrenOnExit(t *testing.T) {
	if inJob() {
		// A job nested in another does not kill on close here (the test
		// runner of an IDE or agent often lives in one). The app itself
		// starts outside a job, where this holds.
		t.Skip("the test process already runs in a job")
	}
	if os.Getenv("PROCJOB_HELPER") == "1" {
		helperProcess(t)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestGuardKillsChildrenOnExit", "-test.count=1")
	cmd.Env = append(os.Environ(), "PROCJOB_HELPER=1")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("helper: %v\n%s", err, out)
	}
	pid := 0
	for _, line := range strings.Split(string(out), "\n") {
		if v, ok := strings.CutPrefix(line, "PID="); ok {
			pid, _ = strconv.Atoi(v)
		}
	}
	if pid == 0 {
		t.Fatalf("helper gave no pid:\n%s", out)
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
		if err != nil {
			return // gone
		}
		_ = windows.CloseHandle(h)
		if time.Now().After(deadline) {
			t.Fatalf("grandchild %d still runs after the helper exited", pid)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func helperProcess(t *testing.T) {
	t.Helper()
	if err := Guard(); err != nil {
		t.Fatal(err)
	}
	proc := exec.Command("cmd.exe", "/c", "ping", "-n", "30", "127.0.0.1")
	if err := proc.Start(); err != nil {
		t.Fatal(err)
	}
	fmt.Printf("PID=%d\n", proc.Process.Pid)
	os.Exit(0)
}

// inJob reports whether this process already belongs to a job object.
var procIsProcessInJob = windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob")

func inJob() bool {
	var in int32
	r, _, _ := procIsProcessInJob.Call(uintptr(windows.CurrentProcess()), 0, uintptr(unsafe.Pointer(&in)))
	return r != 0 && in != 0
}
