package mode_selection

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"tungo/domain/mode"
)

type PromptAppMode struct {
	arguments []string
}

func NewPromptAppMode(arguments []string) AppMode {
	return &PromptAppMode{
		arguments: arguments,
	}
}

func (p *PromptAppMode) Mode() (mode.Mode, error) {
	if len(p.arguments) == 0 {
		return mode.Unknown, mode.NewInvalidExecPathProvided()
	}

	if len(p.arguments) < 2 {
		selectedMode := p.promptForMode()
		if selectedMode == "" {
			return mode.Unknown, mode.NewInvalidModeProvided("empty string")
		}

		p.arguments = []string{p.arguments[0], selectedMode}
	}

	appModeFromArgs := NewArgsAppMode(p.arguments)
	return appModeFromArgs.Mode()
}

func (p *PromptAppMode) promptForMode() string {
	fmt.Println("Please select mode:")
	fmt.Println("\ts - Server")
	fmt.Println("\tc - Client")
	fmt.Println("---")
	fmt.Print("👉 Your choice: ")

	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text())
	}

	return ""
}
