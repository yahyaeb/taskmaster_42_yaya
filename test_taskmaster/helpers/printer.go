package helpers

import "fmt"

// ANSI escape codes for terminal output.
const (
	AnsiRed   = "\033[0;31m"
	AnsiGreen = "\033[0;32m"
	AnsiBold  = "\033[1m"
	AnsiNC    = "\033[0m"
)

// Printer wraps terminal output with colour helpers.
type Printer struct{}

func (p Printer) Pass(msg string) {
	fmt.Printf("  %s✓%s %s\n", AnsiGreen, AnsiNC, msg)
}

func (p Printer) Fail(msg string) {
	fmt.Printf("  %s✗%s %s\n", AnsiRed, AnsiNC, msg)
}

func (p Printer) Section(title string) {
	fmt.Printf("\n%s%s━━━ %s ━━━%s\n", AnsiBold, AnsiNC, title, AnsiNC)
}

func (p Printer) Banner(text string) {
	fmt.Printf("%s%s%s\n", AnsiBold, text, AnsiNC)
}

// DefaultPrinter is the shared terminal printer used by Report and main.
var DefaultPrinter = Printer{}
