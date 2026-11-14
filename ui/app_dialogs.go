// The dialogs' host side: the guard and host key questions that
// block a background call until the window answers.

package ui

import (
	"adakaki/internal/guard"
	"adakaki/internal/sshc"
)

// confirmGuard runs on a tool-call goroutine: the dialog on the main
// thread, the answer back through a channel.
func (a *app) confirmGuard(req guard.Request, rule guard.Rule) bool {
	if a.win == nil {
		return false
	}
	ch := make(chan bool, 1)
	a.win.Update(func() {
		r, rl := req, rule
		a.guardReq, a.guardRule, a.guardCh = &r, &rl, ch
		a.showGuardAsk = true
	})
	return <-ch
}

func (a *app) answerGuard(ok bool) {
	if a.guardCh != nil {
		a.guardCh <- ok
		a.guardCh = nil
	}
	a.showGuardAsk = false
}

// sshPolicy builds the host key policy from the settings: "prompt" asks
// in a dialog (the default), "accept_new" stores unknown keys, "off"
// verifies nothing. A CHANGED key is refused under every setting.
func (a *app) sshPolicy() *sshc.HostKeyPolicy {
	p := &sshc.HostKeyPolicy{Store: a.hostKeys}
	switch a.store.Settings.StrictHostKey {
	case "off":
		p.Insecure = true
	case "accept_new":
		p.AcceptNew = true
	default:
		p.Prompt = a.promptHostKey
	}
	return p
}

// promptHostKey runs on a dial goroutine: it raises the dialog on the
// main thread and blocks until the user answers.
func (a *app) promptHostKey(host string, port int, fp string) bool {
	ch := make(chan bool, 1)
	if a.win == nil {
		return false
	}
	a.win.Update(func() {
		a.hkHost, a.hkPort, a.hkFP, a.hkCh = host, port, fp, ch
		a.showHostKey = true
	})
	return <-ch
}

func (a *app) answerHostKey(ok bool) {
	if a.hkCh != nil {
		a.hkCh <- ok
		a.hkCh = nil
	}
	a.showHostKey = false
}
