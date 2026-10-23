package tui

import (
	"errors"

	tea "github.com/charmbracelet/bubbletea"

	"mdl/internal/library"
)

// Run shows the browser full-screen and blocks until the user quits or asks
// for a download. Bubble Tea restores the terminal (and stops reading stdin)
// before Run returns, so a follow-up program such as beet can prompt normally.
func Run(lib *library.Library) (Result, error) {
	final, err := tea.NewProgram(NewModel(lib), tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
	if err != nil {
		return Result{}, err
	}
	m, ok := final.(*Model)
	if !ok {
		return Result{}, errors.New("tui: unexpected final model")
	}
	return m.Result(), nil
}
