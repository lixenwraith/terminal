// Package atlas is the font an application hands its renderer: Hack as a bitmap
// face at three cell sizes, which a window or a view draws with, and as a web
// font, which a page does. The application imports it and no renderer does, so a
// binary carries the one it names. LICENSE is the font's.
package atlas

import (
	_ "embed"
	"sync"

	"github.com/lixenwraith/terminal/glyph"
)

//go:embed hack.bin
var hackBin []byte

//go:embed hack.woff2
var hackWeb []byte

var (
	hackOnce sync.Once
	hack     *glyph.Font
)

// Hack is the face at 6x12, 8x16 and 12x24, every rune of the font a cell draws
// as itself
func Hack() *glyph.Font {
	hackOnce.Do(func() {
		font, err := glyph.Parse(hackBin)
		if err != nil {
			panic("atlas: hack.bin: " + err.Error())
		}
		hack = font
	})
	return hack
}

// HackWeb is the same runes as a woff2, what terminal.PageConfig.Font takes; the
// caller leaves it as it is
func HackWeb() []byte { return hackWeb }
