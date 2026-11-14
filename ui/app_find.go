// The terminal find state behind the find bar.

package ui

import (
	"fmt"
)

// openFind shows the find bar and searches for what it holds.
func (a *app) openFind() {
	a.findOpen = true
	if a.findText != "" {
		a.searchFor(a.findText)
	}
}

func (a *app) closeFind() {
	a.findOpen = false
	if s := a.active(); s != nil {
		s.Search("") // end the search, clear the highlights
	}
}

func (a *app) searchFor(text string) {
	if s := a.active(); s != nil {
		s.Search(text)
	}
}

func (a *app) searchNext() {
	if s := a.active(); s != nil {
		s.SearchNext()
	}
}

func (a *app) searchPrev() {
	if s := a.active(); s != nil {
		s.SearchPrevious()
	}
}

// findStatus renders the find bar's counter.
func (a *app) findStatus() string {
	s := a.active()
	if s == nil {
		return ""
	}
	st := s.SearchStatus()
	switch {
	case st.Text == "":
		return ""
	case st.Searching:
		return "搜索中…"
	case st.Total == 0:
		return "无匹配"
	default:
		return fmt.Sprintf("%d / %d", st.Selected, st.Total)
	}
}
