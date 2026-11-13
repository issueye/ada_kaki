//go:build windows

package procjob

// The app's children have to die with it: a shell's program may be a
// long-running command that ConPTY's close does not reach before the app
// process ends. A job object with KILL_ON_JOB_CLOSE lets the system end
// every descendant when the app's last handle closes.

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// job is the app's job object. It is kept for the life of the process:
// closing it would kill the app itself along with its children.
var job windows.Handle

func guard() error {
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return fmt.Errorf("create job: %w", err)
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		h, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)),
	); err != nil {
		_ = windows.CloseHandle(h)
		return fmt.Errorf("limit job: %w", err)
	}
	if err := windows.AssignProcessToJobObject(h, windows.CurrentProcess()); err != nil {
		_ = windows.CloseHandle(h)
		return fmt.Errorf("assign job: %w", err)
	}
	job = h
	return nil
}
