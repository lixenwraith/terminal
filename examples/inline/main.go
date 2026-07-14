package main

import (
	"fmt"
	"os"
	"time"

	"github.com/lixenwraith/color"
	"github.com/lixenwraith/terminal"
	"github.com/lixenwraith/terminal/inline"
)

func main() {
	p := inline.New(os.Stdout)

	name := inline.Fg(color.LightSkyBlue).Attr(terminal.AttrBold)
	okSt := inline.Fg(color.LimeGreen).Attr(terminal.AttrBold)
	dim := inline.Fg(color.IronGray)

	pkgs := []string{"openssl", "zlib", "curl", "git", "go"}
	frame := 0

	for i, pkg := range pkgs {
		const steps = 25
		for s := range steps {
			pct := (float64(i) + float64(s)/steps) / float64(len(pkgs))
			p.Update(
				inline.Spinner(frame)+" installing "+p.Paint(pkg, name),
				"["+inline.Bar(32, pct, inline.BarBlock)+"] "+
					p.Paint(fmt.Sprintf("%d/%d", i+1, len(pkgs)), dim),
			)
			frame++
			time.Sleep(40 * time.Millisecond)
		}
		p.Log("%s %s", p.Paint("✓", okSt), pkg)
	}

	p.Done(p.Paint("✓ 5 packages installed", okSt))
}
