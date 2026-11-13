// Package procjob ties the processes the console starts to its own
// lifetime: when the app goes away, the shells it opened and whatever
// they started go with it.
package procjob

// Guard puts the current process, and so every process it starts, in a
// job object that the system kills when the app exits. It is a no-op
// where the platform has no such object.
func Guard() error { return guard() }
