// Package procjob ties the processes the console starts to its own
// lifetime: when the app goes away, the shells it opened and whatever
// they started go with it.
package procjob

// Guard puts the current process, and so every process it starts, in a
// job object that the system kills when the app exits. It is a no-op
// where the platform has no such object.
func Guard() error { return guard() }

// DirectChildren returns the PIDs of the processes of which this process
// is the parent, as a set: the shells this app started.
func DirectChildren() (map[int]struct{}, error) { return directChildren() }

// NewChild returns a direct child that is not in before, preferring one
// whose executable is exe (a shell's base name), or 0 when the process
// has not started yet. Callers use it around starting a shell to learn
// the shell's PID, not ConPTY's console host, which starts beside it.
func NewChild(before map[int]struct{}, exe string) int { return newChild(before, exe) }

// KillTree ends a process and all of its descendants, immediate and
// without a hangup: what a force-close of a session does.
func KillTree(pid int) error { return killTree(pid) }
