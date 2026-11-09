//go:build !windows

package sshc

import (
	"fmt"
	"io"
	"net"
	"os"
)

// openAgent connects to the OpenSSH agent over $SSH_AUTH_SOCK; for
// tests, a TCP address in ADAKAKI_AGENT_ADDR.
func openAgent() (io.ReadWriteCloser, error) {
	if p := os.Getenv("ADAKAKI_AGENT_ADDR"); p != "" {
		return net.Dial("tcp", p)
	}
	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" {
		return nil, fmt.Errorf("ssh: OpenSSH Agent 不可用：SSH_AUTH_SOCK 未设置")
	}
	return net.Dial("unix", sock)
}
