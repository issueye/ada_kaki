// Session lifecycle: restoring and creating sessions, SSH profiles
// and the dialog, closing and reconnecting, and popouts.

package ui

import (
	"adakaki/internal/engine"
	"adakaki/internal/shells"
	"adakaki/internal/sshc"
	"adakaki/internal/store"
	"fmt"
	"log"

	"github.com/egoist/mygo"
	myui "github.com/egoist/mygo/ui"
)

// saveRestoreList writes the sessions worth reopening into the config,
// so a restart brings them back. It runs on the main thread.
func (a *app) saveRestoreList() {
	specs := a.mgr.RestoreSpecs()
	out := make([]store.RestoreSpec, len(specs))
	for i, sp := range specs {
		out[i] = store.RestoreSpec{
			Kind: sp.Kind, Shell: sp.Shell, Cwd: sp.Cwd,
			Custom: sp.Custom, Profile: sp.Profile,
		}
	}
	a.store.Restore = out
	if err := a.store.Save(); err != nil {
		log.Printf("save restore list: %v", err)
	}
}

// restoreSessions reopens what the last run had, off the main thread per
// session; the last one takes the view, as it had at exit.
func (a *app) restoreSessions() {
	if a.store.Settings.RestoreDisabled {
		return
	}
	specs := a.store.Restore
	if len(specs) == 0 {
		return
	}
	for _, spec := range specs {
		switch spec.Kind {
		case "local":
			shell := shells.ID(spec.Shell)
			if shell == "" {
				continue
			}
			a.newSession(shell, spec.Cwd, spec.Custom)
		case "ssh":
			if spec.Profile == "" {
				continue // a direct-password session cannot re-auth
			}
			a.connectSSHProfile(spec.Profile)
		}
	}

}

// sshSourceFor builds the dial for an SSH profile: the stored secret
// (or the agent, or the key) and the host key policy.
func (a *app) sshSourceFor(p store.Profile) engine.SSHSource {
	auth := sshc.Auth{Kind: p.AuthKind, KeyPath: p.KeyPath}
	if auth.Kind == "" {
		auth.Kind = "password"
	}
	if p.Secret != "" {
		if plain, err := store.Unprotect(p.Secret); err == nil {
			if auth.Kind == "password" {
				auth.Password = plain
			} else if auth.Kind == "key" {
				auth.Passphrase = plain
			}
		}
	}
	policy := a.sshPolicy()
	return func(s *engine.Session) (engine.Conn, string, error) {
		client, err := sshc.Dial(p.Host, p.Port, p.User, auth, policy)
		if err != nil {
			return nil, "", err
		}
		conn, err := client.OpenShell(80, 24)
		if err != nil {
			_ = client.Close()
			return nil, "", err
		}
		return conn, p.User + "@" + p.Host, nil
	}
}

// connectSSHProfile opens a session from a saved profile, by name.
func (a *app) connectSSHProfile(name string) {
	p, ok := a.store.Profile(name)
	if !ok {
		return
	}
	go func() {
		source := a.sshSourceFor(p)
		if _, err := a.mgr.CreateSSH(p.Name, source); err != nil {
			a.win.Update(func() { a.pendingToast = err.Error() })
		}
	}()
}

// defaultShellID is the shell new panes start, the setting or the first
// shell found.
func (a *app) defaultShellID() shells.ID {
	if s := shells.ID(a.store.Settings.DefaultShell); s != "" {
		if sh, ok := shells.Get(s); ok && sh.Found {
			return s
		}
	}
	for _, sh := range a.shells {
		if sh.Found {
			return sh.ID
		}
	}
	return shells.Cmd
}

// newSession starts a local shell session off the main thread; the engine
// event activates it, and a failure lands in a pending toast.
func (a *app) newSession(id shells.ID, cwd, custom string) {
	win := a.win
	if win == nil {
		return
	}
	pending := a.pending
	go func() {
		_, err := a.mgr.CreateLocal(id, cwd, custom)
		if err != nil {
			win.Update(func() {
				if pending != nil && a.pending == pending {
					a.pending = nil // the split's pane stays empty
				}
				a.pendingToast = err.Error()
			})
		}
	}()
}

func (a *app) closeSession(s *engine.Session) {
	// The agent's session is its workspace: closing it would pull the
	// work out from under the agent.
	if on, _ := s.AgentLock(); on {
		a.pendingToast = "该会话正被 AI Agent 操作，已阻止关闭"
		return
	}
	if w := a.popouts[s.ID()]; w != nil {
		delete(a.popouts, s.ID())
		w.Close()
	}
	s.Close()
}

// forceCloseSession ends a local shell's whole process tree at once, for
// a program that ignores the hangup (a Java service, say) or takes too
// long to stop. The agent's lock still protects its workspace.
func (a *app) forceCloseSession(s *engine.Session) {
	if s.State() == engine.Exited {
		a.closeSession(s)
		return
	}
	if on, _ := s.AgentLock(); on {
		a.pendingToast = "该会话正被 AI Agent 操作，已阻止关闭"
		return
	}
	if w := a.popouts[s.ID()]; w != nil {
		delete(a.popouts, s.ID())
		w.Close()
	}
	if err := s.ForceClose(); err != nil {
		a.pendingToast = "强制关闭失败：" + err.Error()
		a.refreshUI()
	}
}

// popout opens a synchronized window for the session: a second terminal
// fed by the session's Mirror frames, whose keys go to the session. The
// main window's tab keeps the session's own view, so both ends show the
// same program and take input at once.
func (a *app) popout(s *engine.Session) {
	if a.win == nil || a.popouts[s.ID()] != nil {
		return
	}
	mv, err := s.OpenMirror()
	if err != nil {
		a.pendingToast = err.Error()
		return
	}
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:         s.Title() + " — 同屏",
		TitleBarStyle: mygo.TitleBarHidden,
		Width:         940,
		Height:        600,
		Content: myui.View(func(c *myui.Context) {
			a.popoutView(c, mv, s.Title())
		}),
	})
	iconWindow(w)
	a.popouts[s.ID()] = w
	w.OnClosed(func() {
		mv.Close()
		delete(a.popouts, s.ID())
		if a.win != nil {
			a.win.Invalidate()
		}
	})
}

// recall closes the popped-out window of a session, back to its tab.
func (a *app) recall(s *engine.Session) {
	if w := a.popouts[s.ID()]; w != nil {
		w.Close()
	}
}

// reconnect restarts an exited session in place: an SSH session dials
// again, a local one starts its shell or command again; the pane, the
// tab and the sidebar entry all stay where they are.
func (a *app) reconnect(s *engine.Session) {
	if on, _ := s.AgentLock(); on {
		a.pendingToast = "该会话正被 AI Agent 操作，已阻止重连"
		return
	}
	go func() {
		err := s.Reconnect()
		if err == nil {
			// The old mirror froze at the last screen: a fresh window
			// after the reconnect shows the fresh session.
			a.win.Update(func() {
				if w := a.popouts[s.ID()]; w != nil {
					delete(a.popouts, s.ID())
					w.Close()
				}
			})
			return
		}
		a.win.Update(func() { a.pendingToast = "重连失败：" + err.Error() })
	}()
}

// connectSSH starts an SSH session off the main thread, from the dialog's
// fields; the profile name goes to the session, and Save archives it.
func (a *app) connectSSH() {
	authKind := a.sshAuth
	if authKind == "" {
		authKind = "password"
	}
	p := store.Profile{
		Name:     a.sshProfile,
		Host:     a.sshHost,
		User:     a.sshUser,
		AuthKind: authKind,
		KeyPath:  a.sshKeyPath,
	}
	fmt.Sscanf(a.sshPort, "%d", &p.Port)
	if p.Host == "" || p.User == "" {
		a.pendingToast = "SSH：主机和用户名不能为空"
		return
	}
	if a.sshSave && a.sshSaveAs != "" {
		p.Name = a.sshSaveAs
		if authKind != "agent" {
			if c, err := store.Protect(a.sshPass); err == nil {
				p.Secret = c // the password, or the key's passphrase
			}
		}
		if err := a.store.PutProfile(p); err != nil {
			log.Printf("save profile: %v", err)
		}
	}

	auth := sshc.Auth{Kind: p.AuthKind, KeyPath: p.KeyPath, Passphrase: a.sshPass}
	if authKind == "password" {
		auth.Password = a.sshPass
		// A saved password stays hidden: an empty field uses the archive.
		if a.sshPass == "" && a.sshProfile != "" {
			if saved, ok := a.store.Profile(a.sshProfile); ok && saved.AuthKind == "password" {
				if plain, err := store.Unprotect(saved.Secret); err == nil {
					auth.Password = plain
				}
			}
		}
	}
	name := p.Name
	a.showSSH = false

	go func() {
		source := func(s *engine.Session) (engine.Conn, string, error) {
			client, err := sshc.Dial(p.Host, p.Port, p.User, auth, a.sshPolicy())
			if err != nil {
				return nil, "", err
			}
			conn, err := client.OpenShell(80, 24)
			if err != nil {
				_ = client.Close()
				return nil, "", err
			}
			return conn, p.User + "@" + p.Host, nil
		}
		if _, err := a.mgr.CreateSSH(name, source); err != nil {
			a.win.Update(func() { a.pendingToast = err.Error() })
		}
	}()
}
