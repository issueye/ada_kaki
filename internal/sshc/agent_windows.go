//go:build windows

package sshc

import (
	"fmt"
	"io"
	"net"
	"os"
	"syscall"
)

// openAgent connects to the OpenSSH agent: its named pipe on Windows
// (the agent service must be running), for tests a TCP address in
// ADAKAKI_AGENT_ADDR.
func openAgent() (io.ReadWriteCloser, error) {
	if p := os.Getenv("ADAKAKI_AGENT_ADDR"); p != "" {
		return net.Dial("tcp", p)
	}
	path, err := syscall.UTF16PtrFromString(`\\.\pipe\openssh-ssh-agent`)
	if err != nil {
		return nil, err
	}
	h, err := syscall.CreateFile(path,
		syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		0, nil, syscall.OPEN_EXISTING, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("ssh: OpenSSH Agent 不可用（服务是否在运行？）: %w", err)
	}
	return os.NewFile(uintptr(h), "openssh-ssh-agent"), nil
}
