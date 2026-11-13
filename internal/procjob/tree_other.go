//go:build !windows

package procjob

func directChildren() (map[int]struct{}, error) { return nil, nil }

func newChild(map[int]struct{}, string) int { return 0 }

func killTree(int) error { return nil }
