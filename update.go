package main

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
)

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {

	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-m.padding)
			m.viewport.MouseWheelEnabled = m.mouseEnabled
			m.viewport.SetYOffset(0)

			m.searchBar = textinput.New()
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - m.padding
			if m.cursorPastViewBottom() {
				m.cursor = m.viewport.YOffset + m.viewport.Height - 1
			}
		}

	case tea.MouseMsg:
		if !m.mouseEnabled {
			break
		}
		switch msg.Type {
		case tea.MouseWheelUp:
			m.cursor -= m.viewport.MouseWheelDelta
			if m.cursorPastViewTop() {
				m.viewport.LineUp(m.viewport.MouseWheelDelta)
			}
		case tea.MouseWheelDown:
			m.cursor += m.viewport.MouseWheelDelta
			if m.cursorPastViewBottom() {
				m.viewport.LineDown(m.viewport.MouseWheelDelta)
			}
		}
	}

	switch {
	case m.search && m.searchBar.Focused():
		cmds = append(cmds, m.handleSearch(msg))
	default:
		cmds = append(cmds, m.handleNormal(msg))
	}

	// cursor loop around
	if m.cursorPastBeginning() {
		m.cursorToEnd()
		m.viewport.GotoBottom()
	} else if m.cursorPastEnd() {
		m.cursorToBeginning()
		m.viewport.GotoTop()
	}

	m.viewItems()
	return m, tea.Batch(cmds...)
}

func (m *model) handleNormal(msg tea.Msg) tea.Cmd {

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			return tea.Quit

		case key.Matches(msg, m.keys.Search):
			m.search = true
			m.filterState = filtering
			m.searchBar.Focus()

		case key.Matches(msg, m.keys.Up):
			m.cursor--
			if m.cursorPastViewTop() {
				m.viewport.LineUp(1)
			}
		case key.Matches(msg, m.keys.Down):
			m.cursor++
			if m.cursorPastViewBottom() {
				m.viewport.LineDown(1)
			}

		case key.Matches(msg, m.keys.HalfUp):
			m.cursor -= m.viewport.Height / 2
			if m.cursorPastViewTop() {
				m.viewport.HalfViewUp()
			}

			// don't loop around
			if m.cursorPastBeginning() {
				m.cursorToBeginning()
				m.viewport.GotoTop()
			}
		case key.Matches(msg, m.keys.HalfDown):
			m.cursor += m.viewport.Height / 2
			if m.cursorPastViewBottom() {
				m.viewport.HalfViewDown()
			}

			// don't loop around
			if m.cursorPastEnd() {
				m.cursorToEnd()
				m.viewport.GotoBottom()
			}

		case key.Matches(msg, m.keys.FullUp):
			m.cursor -= m.viewport.Height
			if m.cursorPastViewTop() {
				m.viewport.ViewUp()
			}

			// don't loop around
			if m.cursorPastBeginning() {
				m.cursorToBeginning()
				m.viewport.GotoTop()
			}

		case key.Matches(msg, m.keys.FullDown):
			m.cursor += m.viewport.Height
			if m.cursorPastViewBottom() {
				m.viewport.ViewDown()
			}

			// don't loop around
			if m.cursorPastEnd() {
				m.cursorToEnd()
				m.viewport.GotoBottom()
			}

		case key.Matches(msg, m.keys.GoToFirstLine):
			m.cursorToBeginning()
			m.viewport.GotoTop()
		case key.Matches(msg, m.keys.GoToLastLine):
			m.cursorToEnd()
			m.viewport.GotoBottom()

		case key.Matches(msg, m.keys.GoToTop):
			m.cursorToViewTop()

		case key.Matches(msg, m.keys.GoToMiddle):
			m.cursorToViewMiddle()

		case key.Matches(msg, m.keys.GoToBottom):
			m.cursorToViewBottom()

			// case key.Matches(msg, m.keys.CenterCursor):
			// 	middle := m.viewport.Height / 2
			// 	diff := m.cursor - middle
			// 	if diff > 0 {
			// 		m.viewport.LineDown(diff)
			// 	} else {
			// 		m.viewport.LineUp(diff)
			// 	}
		}
	}
	return nil
}

func (m *model) handleSearch(msg tea.Msg) tea.Cmd {

	var (
		cmds []tea.Cmd
		cmd  tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case msg.String() == "ctrl+c":
			return tea.Quit

		case key.Matches(msg, m.keys.Normal):
			m.search = false
			m.searchBar.Blur()

			if m.filteredTable.Empty() {
				m.filterState = unfiltered
			}
			return nil
		}

		// filter with search input
		m.searchBar, cmd = m.searchBar.Update(msg)
		cmds = append(cmds, cmd)
		matches := filter(m.searchBar.Value(), m.table.Output)

		// style matched rune indices
		var styledMatches []string
		matchedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA066"))
		for _, m := range matches {
			styledMatches = append(styledMatches, highlight(m, matchedStyle))
		}

		// present new filtered rows
		m.filteredTable.Reset()
		if len(styledMatches) == 0 {
			m.filteredTable.AppendRow("")
		} else {
			m.filteredTable.AppendRows(styledMatches...)
		}
		m.cursorToBeginning()
	}

	// reset if search input is empty regardless of filterState
	if m.searchBar.Value() == "" {
		m.resetOutput()
	}
	return tea.Batch(cmds...)
}

func filter(term string, target []string) fuzzy.Matches {
	matches := fuzzy.Find(term, target)
	sort.Stable(matches)
	return matches
}

func highlight(m fuzzy.Match, style lipgloss.Style) string {
	var b strings.Builder

	for i, rune := range []rune(m.Str) {
		styled := false
		for _, mi := range m.MatchedIndexes {
			if i == mi {
				b.WriteString(style.Render(string(rune)))
				styled = true
			}
		}
		if !styled {
			b.WriteString(string(rune))
		}
	}
	return b.String()
}

func (m *model) resetOutput() {
	m.filteredTable.Reset()
	m.searchBar.Reset()
	m.filterState = unfiltered

	m.cursorToBeginning()
	m.viewItems()
}

func (m *model) viewItems() {
	if !m.filteredTable.Empty() {
		m.renderCursor(m.filteredTable.Output)
		m.maxRows = m.filteredTable.LineCount
	} else {
		m.renderCursor(m.table.Output)
		m.maxRows = m.table.LineCount
	}
}

func (m *model) renderCursor(rows []string) {
	if len(rows) == 0 {
		m.viewport.SetContent("")
		return
	}

	cursorStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(m.CursorBackground)).
		Foreground(lipgloss.Color(m.CursorForeground))

	// make a deep copy to not preserve cursor position
	cpy := make([]string, len(rows))
	copy(cpy, rows)

	cpy[m.cursor] = cursorStyle.Render(cpy[m.cursor])
	m.viewport.SetContent(strings.Join(cpy, "\n"))
}

func (m *model) cursorToBeginning() {
	m.cursor = 0
}

func (m *model) cursorToEnd() {
	m.cursor = m.maxRows - 1
}

func (m *model) cursorToViewTop() {
	m.cursor = m.viewport.YOffset + 3
}

func (m *model) cursorToViewMiddle() {
	m.cursor = (m.viewport.YOffset + m.viewport.Height) / 2
}

func (m *model) cursorToViewBottom() {
	m.cursor = m.viewport.YOffset + m.viewport.Height - 3
}

func (m *model) cursorPastViewTop() bool {
	return m.cursor < m.viewport.YOffset
}

func (m *model) cursorPastViewBottom() bool {
	return m.cursor > m.viewport.YOffset+m.viewport.Height-1
}

func (m *model) cursorPastBeginning() bool {
	return m.cursor < 0
}

func (m *model) cursorPastEnd() bool {
	return m.cursor > m.maxRows-1
}
