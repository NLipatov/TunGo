package bubble_tea

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
)

type Selector struct {
	placeholder string
	options     []string
	cursor      int
	choice      string
	checked     int
	done        bool
}

func NewSelector(placeholder string, choices []string) Selector {
	return Selector{
		placeholder: placeholder,
		options:     choices,
		checked:     -1,
	}
}

func (m Selector) Choice() string {
	return m.choice
}

func (m Selector) Init() tea.Cmd {
	return nil
}

func (m Selector) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up":
			if m.cursor > 0 && !m.done {
				m.cursor--
			}
		case "down":
			if m.cursor < len(m.options)-1 && !m.done {
				m.cursor++
			}
		case "enter":
			if !m.done {
				m.choice = m.options[m.cursor]
				m.checked = m.cursor
				m.done = true
			}
			return m, tea.Quit
		case "q":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m Selector) View() string {
	if m.done {
		return ""
	}

	s := fmt.Sprintf("%s\n", m.placeholder)
	for i, choice := range m.options {
		checked := "[]"
		if m.checked == i {
			checked = "[x]"
		}
		line := fmt.Sprintf("%s %s", checked, choice)
		if m.cursor == i {
			line = "\033[1;32m" + line + "\033[0m"
		}
		s += line + "\n"
	}
	s += "\nPress q to quit.\n"
	return s
}
