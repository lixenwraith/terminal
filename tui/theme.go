package tui

import (
	"github.com/lixenwraith/color"
)

// Theme defines semantic colors for TUI components
type Theme struct {
	Bg       color.RGB
	Fg       color.RGB
	FocusBg  color.RGB
	CursorBg color.RGB

	Selected   color.RGB
	Unselected color.RGB
	Partial    color.RGB
	Error      color.RGB
	Warning    color.RGB

	Border   color.RGB
	HeaderBg color.RGB
	HeaderFg color.RGB
	StatusFg color.RGB
	HintFg   color.RGB
	InputBg  color.RGB

	DirFg    color.RGB
	FileFg   color.RGB
	SymbolFg color.RGB

	SyntaxComment color.RGB
	SyntaxString  color.RGB
	SyntaxKeyword color.RGB
	SyntaxType    color.RGB
	SyntaxNumber  color.RGB
	SyntaxSymbol  color.RGB
}

// DefaultTheme provides reasonable defaults
var DefaultTheme = Theme{
	Bg:            color.RGB{R: 20, G: 20, B: 30},
	Fg:            color.RGB{R: 200, G: 200, B: 200},
	FocusBg:       color.RGB{R: 30, G: 35, B: 45},
	CursorBg:      color.RGB{R: 50, G: 50, B: 70},
	Selected:      color.RGB{R: 80, G: 200, B: 80},
	Unselected:    color.RGB{R: 100, G: 100, B: 100},
	Partial:       color.RGB{R: 80, G: 160, B: 220},
	Error:         color.RGB{R: 255, G: 80, B: 80},
	Warning:       color.RGB{R: 255, G: 80, B: 80},
	Border:        color.RGB{R: 60, G: 80, B: 100},
	HeaderBg:      color.RGB{R: 40, G: 60, B: 90},
	HeaderFg:      color.RGB{R: 255, G: 255, B: 255},
	StatusFg:      color.RGB{R: 140, G: 140, B: 140},
	HintFg:        color.RGB{R: 100, G: 180, B: 200},
	InputBg:       color.RGB{R: 30, G: 30, B: 50},
	DirFg:         color.RGB{R: 130, G: 170, B: 220},
	FileFg:        color.RGB{R: 200, G: 200, B: 200},
	SymbolFg:      color.RGB{R: 180, G: 220, B: 220},
	SyntaxComment: color.RGB{R: 100, G: 110, B: 120},
	SyntaxString:  color.RGB{R: 180, G: 220, B: 140},
	SyntaxKeyword: color.RGB{R: 180, G: 140, B: 220},
	SyntaxType:    color.RGB{R: 80, G: 200, B: 200},
	SyntaxNumber:  color.RGB{R: 220, G: 180, B: 120},
	SyntaxSymbol:  color.RGB{R: 220, G: 180, B: 80},
}

