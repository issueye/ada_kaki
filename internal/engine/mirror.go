package engine

import (
	"sync"
	"time"

	"github.com/egoist/mygo/plugins/terminal"
)

// ErrClosedPipe is what the mirror pipe's Read returns once closed.
var ErrClosedPipe = errPipeClosed{}

type errPipeClosed struct{}

func (errPipeClosed) Error() string { return "engine: mirror pipe closed" }

// MirrorView is a second, synchronized view of a session: it feeds a
// terminal of its own with the Mirror frames of the session's terminal,
// and forwards what is typed there to the session. The session's own
// view keeps running wherever it is; both show the same program and take
// input at once.
//
// Open it with Session.OpenMirror, show MirrorView.Terminal in a view,
// and Close it when the view goes away.
type MirrorView struct {
	sess   *Session
	mirror *terminal.Mirror
	term   *terminal.Terminal
	pipe   *mirrorPipe
	stop   chan struct{}
	once   sync.Once
}

// mirrorPipe is the mirror terminal's Conn: reads block (frames arrive
// through Feed), writes are the user's keys, and ResizeByConn keeps the
// popout's own size from fighting the session's.
type mirrorPipe struct {
	sess     *Session
	stop     chan struct{}
	stopOnce sync.Once
}

func (p *mirrorPipe) Read(b []byte) (int, error) {
	<-p.stop
	return 0, ErrClosedPipe
}

func (p *mirrorPipe) Write(b []byte) (int, error) {
	p.sess.UserSend(string(b))
	return len(b), nil
}

func (p *mirrorPipe) Close() error {
	p.stopOnce.Do(func() { close(p.stop) })
	return nil
}

// Resize is the popout's view asking to size the screen; the pump keeps
// the screen at the session's size instead, so both ends show the same
// grid. Ignored by design.
func (p *mirrorPipe) Resize(cols, rows int) error { return nil }

// OpenMirror starts a synchronized second view of the session.
func (s *Session) OpenMirror() (*MirrorView, error) {
	t := s.terminal()
	if t == nil {
		return nil, &sessionClosedError{id: s.id}
	}
	m, err := t.NewMirror()
	if err != nil {
		return nil, err
	}
	mv := &MirrorView{
		sess:   s,
		mirror: m,
		pipe:   &mirrorPipe{sess: s, stop: make(chan struct{})},
		stop:   make(chan struct{}),
	}
	// The mirror terminal takes its screen's size only from Resize, so
	// the pump can keep it at the session's size whatever the popout's
	// own size is.
	term, err := terminal.New(terminal.Options{
		Conn:         mv.pipe,
		ResizeByConn: true,
	})
	if err != nil {
		m.Close()
		return nil, err
	}
	mv.term = term

	go mv.pump()
	return mv, nil
}

// pump moves frames from the session's mirror to the view terminal, and
// keeps the two screens the same size, at frame rate.
func (mv *MirrorView) pump() {
	ticker := time.NewTicker(33 * time.Millisecond) // ~30 frames a second
	defer ticker.Stop()
	for {
		select {
		case <-mv.stop:
			return
		case <-mv.pipe.stop:
			return
		case <-ticker.C:
		}
		frame, due := mv.mirror.Frame()
		if due {
			continue // a synchronized update holds it back
		}
		if frame != nil {
			mv.term.Feed(frame)
		}
		cols, rows := mv.sess.Size()
		if c, r := mv.term.Size(); c != cols || r != rows {
			mv.term.Resize(cols, rows)
		}
	}
}

// Terminal returns the mirror's terminal, for the UI to show in a view.
func (mv *MirrorView) Terminal() *terminal.Terminal { return mv.term }

// Close frees the mirror and the view terminal.
func (mv *MirrorView) Close() {
	mv.once.Do(func() {
		close(mv.stop)
		mv.pipe.Close()
		mv.mirror.Close()
		mv.term.Close()
	})
}

// sessionClosedError says the session a call needed is already closed.
type sessionClosedError struct{ id string }

func (e *sessionClosedError) Error() string {
	return "engine: session " + e.id + " is closed"
}
