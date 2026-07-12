package tui

import "github.com/lixenwraith/terminal"

// Theme defines semantic colors for TUI components
type Theme struct {
	Bg       terminal.RGB
	Fg       terminal.RGB
	FocusBg  terminal.RGB
	CursorBg terminal.RGB

	Selected   terminal.RGB
	Unselected terminal.RGB
	Partial    terminal.RGB
	Error      terminal.RGB
	Warning    terminal.RGB

	Border   terminal.RGB
	HeaderBg terminal.RGB
	HeaderFg terminal.RGB
	StatusFg terminal.RGB
	HintFg   terminal.RGB
	InputBg  terminal.RGB

	DirFg    terminal.RGB
	FileFg   terminal.RGB
	SymbolFg terminal.RGB

	SyntaxComment terminal.RGB
	SyntaxString  terminal.RGB
	SyntaxKeyword terminal.RGB
	SyntaxType    terminal.RGB
	SyntaxNumber  terminal.RGB
	SyntaxSymbol  terminal.RGB
}

// DefaultTheme provides reasonable defaults
var DefaultTheme = Theme{
	Bg:            terminal.RGB{R: 20, G: 20, B: 30},
	Fg:            terminal.RGB{R: 200, G: 200, B: 200},
	FocusBg:       terminal.RGB{R: 30, G: 35, B: 45},
	CursorBg:      terminal.RGB{R: 50, G: 50, B: 70},
	Selected:      terminal.RGB{R: 80, G: 200, B: 80},
	Unselected:    terminal.RGB{R: 100, G: 100, B: 100},
	Partial:       terminal.RGB{R: 80, G: 160, B: 220},
	Error:         terminal.RGB{R: 255, G: 80, B: 80},
	Warning:       terminal.RGB{R: 255, G: 80, B: 80},
	Border:        terminal.RGB{R: 60, G: 80, B: 100},
	HeaderBg:      terminal.RGB{R: 40, G: 60, B: 90},
	HeaderFg:      terminal.RGB{R: 255, G: 255, B: 255},
	StatusFg:      terminal.RGB{R: 140, G: 140, B: 140},
	HintFg:        terminal.RGB{R: 100, G: 180, B: 200},
	InputBg:       terminal.RGB{R: 30, G: 30, B: 50},
	DirFg:         terminal.RGB{R: 130, G: 170, B: 220},
	FileFg:        terminal.RGB{R: 200, G: 200, B: 200},
	SymbolFg:      terminal.RGB{R: 180, G: 220, B: 220},
	SyntaxComment: terminal.RGB{R: 100, G: 110, B: 120},
	SyntaxString:  terminal.RGB{R: 180, G: 220, B: 140},
	SyntaxKeyword: terminal.RGB{R: 180, G: 140, B: 220},
	SyntaxType:    terminal.RGB{R: 80, G: 200, B: 200},
	SyntaxNumber:  terminal.RGB{R: 220, G: 180, B: 120},
	SyntaxSymbol:  terminal.RGB{R: 220, G: 180, B: 80},
}