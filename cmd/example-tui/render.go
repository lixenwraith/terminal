package main

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lixenwraith/color"
	"github.com/lixenwraith/terminal"
	"github.com/lixenwraith/terminal/tui"
)

// --- palettes ---------------------------------------------------------------

type palette struct {
	name                                  string
	th                                    tui.Theme
	accent, accent2                       color.RGB
	hexNull, hexPrint, hexSpace, hexCtrl  color.RGB
	hexHigh, matchBg, matchCurBg, markDim color.RGB
}

var palettes = []palette{
	{ // Tokyo Night
		name: "tokyonight",
		th: tui.Theme{
			Bg: color.RGB{26, 27, 38}, Fg: color.RGB{192, 202, 245},
			FocusBg: color.RGB{35, 36, 48}, CursorBg: color.RGB{45, 50, 80},
			Selected: color.RGB{158, 206, 106}, Unselected: color.RGB{86, 95, 137},
			Partial: color.RGB{125, 207, 255}, Error: color.RGB{247, 118, 142},
			Warning: color.RGB{224, 175, 104}, Border: color.RGB{59, 66, 97},
			HeaderBg: color.RGB{22, 22, 30}, HeaderFg: color.RGB{192, 202, 245},
			StatusFg: color.RGB{140, 152, 200}, HintFg: color.RGB{86, 95, 137},
			InputBg: color.RGB{31, 32, 46}, DirFg: color.RGB{122, 162, 247},
			FileFg: color.RGB{192, 202, 245}, SymbolFg: color.RGB{125, 207, 255},
		},
		accent: color.RGB{122, 162, 247}, accent2: color.RGB{187, 154, 247},
		hexNull: color.RGB{65, 72, 104}, hexPrint: color.RGB{192, 202, 245},
		hexSpace: color.RGB{125, 207, 255}, hexCtrl: color.RGB{187, 154, 247},
		hexHigh: color.RGB{255, 158, 100},
		matchBg: color.RGB{40, 70, 45}, matchCurBg: color.RGB{60, 110, 60},
		markDim: color.RGB{86, 95, 137},
	},
	{ // One Dark
		name: "onedark",
		th: tui.Theme{
			Bg: color.RGB{40, 44, 52}, Fg: color.RGB{171, 178, 191},
			FocusBg: color.RGB{47, 52, 63}, CursorBg: color.RGB{62, 68, 81},
			Selected: color.RGB{152, 195, 121}, Unselected: color.RGB{92, 99, 112},
			Partial: color.RGB{86, 182, 194}, Error: color.RGB{224, 108, 117},
			Warning: color.RGB{229, 192, 123}, Border: color.RGB{76, 82, 99},
			HeaderBg: color.RGB{33, 37, 43}, HeaderFg: color.RGB{171, 178, 191},
			StatusFg: color.RGB{130, 140, 155}, HintFg: color.RGB{92, 99, 112},
			InputBg: color.RGB{33, 37, 43}, DirFg: color.RGB{97, 175, 239},
			FileFg: color.RGB{171, 178, 191}, SymbolFg: color.RGB{86, 182, 194},
		},
		accent: color.RGB{97, 175, 239}, accent2: color.RGB{198, 120, 221},
		hexNull: color.RGB{73, 80, 94}, hexPrint: color.RGB{171, 178, 191},
		hexSpace: color.RGB{86, 182, 194}, hexCtrl: color.RGB{198, 120, 221},
		hexHigh: color.RGB{209, 154, 102},
		matchBg: color.RGB{45, 70, 45}, matchCurBg: color.RGB{70, 110, 60},
		markDim: color.RGB{92, 99, 112},
	},
}

func (a *appState) pal() palette { return palettes[a.themeIdx] }

// pulse returns border color breathing toward the accent (10 fps clock).
func (a *appState) pulse(base, accent color.RGB) color.RGB {
	t := 0.35 + 0.35*math.Sin(float64(a.frame)*0.35)
	return base.Lerp(accent, t)
}

// --- top-level frame --------------------------------------------------------

func (a *appState) render() {
	w, h := a.width, a.height
	if w < 4 || h < 3 {
		return
	}
	p := a.pal()
	cells := make([]terminal.Cell, w*h)
	for i := range cells {
		cells[i] = terminal.Cell{Rune: ' ', Fg: p.th.Fg, Bg: p.th.Bg}
	}
	root := tui.NewRegion(cells, w, 0, 0, w, h)

	if w < 60 || h < 12 {
		root.Fill(p.th.Bg)
		root.TextCenter(h/2, "terminal too small (need ≥60x12) — q quits", p.th.Warning, p.th.Bg, terminal.AttrBold)
		a.term.Flush(cells, w, h)
		return
	}

	header, rest := tui.SplitVFixed(root, 1)
	body, tail := tui.SplitVFixed(rest, rest.H-2)
	status, footer := tui.SplitVFixed(tail, 1)

	if a.view == viewHex {
		a.renderHexHeader(header)
		a.renderHexBody(body)
		a.renderHexStatus(status)
		a.renderFooter(footer, hexHints)
	} else {
		a.refreshPreview()
		a.renderHeader(header)
		a.renderBrowse(body)
		a.renderBrowseStatus(status)
		a.renderFooter(footer, browseHints)
	}

	switch a.overlay {
	case ovPrompt:
		a.renderPrompt(footer) // command line replaces footer row
	case ovConfirm:
		root.ConfirmDialog(a.confirm, tui.ConfirmOpts{
			Title: "Confirm", Message: a.confirmMsg,
			YesLabel: "Delete", NoLabel: "Keep", Destructive: true,
		})
	case ovHelp:
		a.renderHelp(root)
	}

	if a.toast.Visible {
		root.Toast(a.toast.Opts)
	}
	a.term.Flush(cells, w, h)
}

// --- header / footer / status ----------------------------------------------

func (a *appState) renderHeader(r tui.Region) {
	p := a.pal()
	r.Fill(p.th.HeaderBg)
	r.Text(1, 0, " fv ", p.th.Bg, p.accent, terminal.AttrBold)
	r.Spinner(6, 0, a.frame, p.accent2)
	path := tailTruncate(a.br.cwd, r.W-40)
	r.Text(8, 0, path, p.th.HeaderFg, color.RGB{}, terminal.AttrBold)

	marks := len(a.br.marks)
	hidden := "off"
	if a.br.showHidden {
		hidden = "on"
	}
	pos := "0/0"
	if n := len(a.br.view); n > 0 {
		pos = fmt.Sprintf("%d/%d", a.br.cursor+1, n)
	}
	hint := tui.Style{Fg: p.th.HintFg}
	r.StatusBar(0, []tui.BarSection{
		{Label: "theme ", Value: p.name, LabelStyle: hint, ValueStyle: tui.Style{Fg: p.accent2}, Priority: 0},
		{Label: "sort ", Value: a.br.sortBy.String(), LabelStyle: hint, ValueStyle: tui.Style{Fg: p.th.StatusFg}, Priority: 1},
		{Label: "dot ", Value: hidden, LabelStyle: hint, ValueStyle: tui.Style{Fg: p.th.StatusFg}, Priority: 1},
		{Label: "sel ", Value: fmt.Sprintf("%d", marks), LabelStyle: hint, ValueStyle: tui.Style{Fg: p.th.Selected}, Priority: 2},
		{Label: "", Value: pos, ValueStyle: tui.Style{Fg: p.th.HeaderFg, Attr: terminal.AttrBold}, Priority: 3},
	}, tui.BarOpts{Bg: p.th.HeaderBg, Align: tui.BarAlignRight})
}

var browseHints = [][2]string{
	{"↵", "open"}, {"h/l", "nav"}, {"/", "filter"}, {"␣", "mark"},
	{"y", "yank"}, {"r", "ren"}, {"m", "mkdir"}, {"D", "del"},
	{"s", "sort"}, {".", "dot"}, {"T", "theme"}, {"?", "help"}, {"q/Q", "quit"},
}
var hexHints = [][2]string{
	{"hjkl", "move"}, {"0/$", "row"}, {"g/G", "ends"}, {"^D/^U", "half"},
	{"/", "search"}, {"n/N", "match"}, {"esc", "back"},
}

func (a *appState) renderFooter(r tui.Region, hints [][2]string) {
	p := a.pal()
	r.Fill(p.th.HeaderBg)
	x := 1
	for _, h := range hints {
		key, label := " "+h[0]+" ", h[1]+"  "
		if x+tui.RuneLen(key)+tui.RuneLen(label) > r.W {
			break
		}
		r.Text(x, 0, key, p.th.Bg, p.accent, terminal.AttrBold)
		x += tui.RuneLen(key)
		r.Text(x, 0, label, p.th.HintFg, p.th.HeaderBg, terminal.AttrNone)
		x += tui.RuneLen(label)
	}
}

func (a *appState) renderBrowseStatus(r tui.Region) {
	p := a.pal()
	r.Fill(p.th.HeaderBg)
	// size-distribution sparkline of current view (log scale)
	vals := make([]float64, 0, 40)
	for _, i := range a.br.view {
		if len(vals) == 40 {
			break
		}
		vals = append(vals, math.Log1p(float64(a.br.all[i].size)))
	}
	if len(vals) > 0 {
		r.Text(1, 0, "sizes ", p.th.HintFg, color.RGB{}, terminal.AttrDim)
		r.Sparkline(7, 0, min(40, len(vals)), vals, tui.SparklineOpts{Style: tui.Style{Fg: p.accent2}})
	}
	if a.br.filter != "" {
		r.Text(50, 0, "/"+a.br.filter, p.th.Warning, color.RGB{}, terminal.AttrBold)
	}
	r.ScrollIndicator(0, a.br.scroll, a.geom.list.h, len(a.br.view), p.th.StatusFg)
}

// --- browser ----------------------------------------------------------------

func (a *appState) renderBrowse(r tui.Region) {
	cols := tui.SplitH(r, 0.18, 0.42, 0.40)
	a.renderParent(cols[0])
	a.renderList(cols[1])
	a.renderPreview(cols[2])
}

func (a *appState) renderParent(r tui.Region) {
	p := a.pal()
	r.Fill(p.th.Bg)
	r.VLine(r.W-1, tui.LineSingle, p.th.Border)
	inner := r.Sub(0, 0, r.W-1, r.H)
	a.geom.parent = rect{inner.X, inner.Y, inner.W, inner.H}

	total := len(a.br.parent)
	scroll := tui.AdjustScroll(a.br.parentSel, 0, inner.H, total)
	for y := 0; y < inner.H; y++ {
		idx := scroll + y
		if idx >= total {
			break
		}
		e := a.br.parent[idx]
		bg, fg, attr := p.th.Bg, p.th.HintFg, terminal.AttrNone
		if idx == a.br.parentSel {
			bg, fg, attr = p.th.FocusBg, p.th.DirFg, terminal.AttrBold
		}
		for x := 0; x < inner.W; x++ {
			inner.Cell(x, y, ' ', fg, bg, terminal.AttrNone)
		}
		inner.Text(1, y, tui.Truncate(e.name, inner.W-2), fg, bg, attr)
	}
}

func (a *appState) renderList(r tui.Region) {
	p := a.pal()
	content := r.Pane(tui.PaneOpts{
		Title:  filepath.Base(a.br.cwd),
		Border: tui.LineRounded, BorderFg: a.pulse(p.th.Border, p.accent),
		TitleFg: p.accent, Bg: p.th.Bg,
	})
	if content.W < 12 || content.H < 1 {
		return
	}
	listR := content.Sub(0, 0, content.W-1, content.H) // reserve right col for scrollbar
	a.geom.list = rect{listR.X, listR.Y, listR.W, listR.H}

	b := &a.br
	b.scroll = tui.AdjustScroll(b.cursor, b.scroll, listR.H, len(b.view))

	if len(b.view) == 0 {
		msg := "empty"
		if b.filter != "" {
			msg = "no match for /" + b.filter
		}
		listR.TextCenter(listR.H/2, msg, p.th.HintFg, p.th.Bg, terminal.AttrDim)
	} else {
		items := make([]tui.ListItem, len(b.view))
		nameW := max(4, listR.W-15) // "[x] " + name + " " + 7-char size
		for vi, ei := range b.view {
			e := b.all[ei]
			fg, attr := p.th.FileFg, terminal.AttrNone
			suffix := ""
			switch {
			case e.dir:
				fg, attr, suffix = p.th.DirFg, terminal.AttrBold, "/"
			case e.symlink:
				fg, attr, suffix = p.th.SymbolFg, terminal.AttrItalic, "@"
			case e.mode&0o111 != 0:
				fg, suffix = p.th.Selected, "*"
			}
			if strings.HasPrefix(e.name, ".") {
				attr |= terminal.AttrDim
			}
			sz := humanSize(e.size)
			if e.dir {
				sz = ""
			}
			text := padRight(tui.Truncate(e.name+suffix, nameW), nameW) + fmt.Sprintf(" %7s", sz)
			it := tui.ListItem{Text: text, TextStyle: tui.Style{Fg: fg, Attr: attr}, CheckFg: p.markDim}
			if b.marks[e.name] {
				it.Check, it.CheckFg = tui.CheckFull, p.th.Selected
				it.TextStyle.Fg = p.th.Selected
			}
			items[vi] = it
		}
		listR.List(items, b.cursor, b.scroll, tui.ListOpts{
			CursorBg: p.th.CursorBg, DefaultBg: p.th.Bg, IconWidth: 1,
		})
	}

	// Correct ScrollBar usage: 1-cell column at the right edge,
	// visible == track height, drawn at x=0 of that sub-region.
	sb := content.Sub(content.W-1, 0, 1, content.H)
	sb.ScrollBar(0, b.scroll, sb.H, len(b.view), p.th.Border)
}

// --- preview ----------------------------------------------------------------

func (a *appState) renderPreview(r tui.Region) {
	p := a.pal()
	content := r.Pane(tui.PaneOpts{
		Title: "preview", Border: tui.LineSingle,
		BorderFg: p.th.Border, TitleFg: p.th.HintFg, Bg: p.th.Bg,
	})
	if content.H < 4 {
		return
	}
	pv := &a.pv

	// Info band: full-width FocusBg strip; zero-Bg KeyValue styles inherit it
	infoRows := 0
	if pv.info != nil {
		infoRows = 3
	}
	if infoRows > 0 {
		band := content.Sub(0, 0, content.W, infoRows)
		band.Fill(p.th.FocusBg)
		ks := tui.Style{Fg: p.th.HintFg}
		vs := tui.Style{Fg: p.th.Fg}
		band.KeyValue(0, "size", humanSize(pv.info.Size()), ks, vs, ':')
		band.KeyValue(1, "mode", pv.info.Mode().String(), ks, vs, ':')
		band.KeyValue(2, "mtime", pv.info.ModTime().Format("2006-01-02 15:04:05"), ks, vs, ':')
	}
	y := infoRows
	content.Divider(y, "", tui.LineSingle, p.th.Border)
	y++
	body := content.Sub(0, y, content.W, content.H-y)

	switch pv.kind {
	case pvErr:
		body.TextBlock(1, 0, pv.errMsg, p.th.Error, p.th.Bg, terminal.AttrNone)
	case pvDir:
		rows := make([][]string, 0, min(len(pv.entries), body.H-1))
		for _, e := range pv.entries {
			if len(rows) == body.H-1 {
				break
			}
			n := e.name
			if e.dir {
				n += "/"
			}
			sz := humanSize(e.size)
			if e.dir {
				sz = ""
			}
			rows = append(rows, []string{n, sz, e.mtime.Format("01-02 15:04")})
		}
		body.Table([]string{"name", "size", "modified"}, rows, tui.TableOpts{
			HeaderStyle: tui.Style{Fg: p.accent, Attr: terminal.AttrBold},
			RowStyle:    tui.Style{Fg: p.th.Fg},
			AltRowStyle: tui.Style{Fg: p.th.Fg, Bg: p.th.FocusBg},
			ColAligns:   []tui.Align{tui.AlignLeft, tui.AlignRight, tui.AlignRight},
		})
	case pvText:
		gut := 5
		for i, line := range pv.lines {
			if i >= body.H {
				break
			}
			body.Text(0, i, fmt.Sprintf("%4d", i+1), p.th.HintFg, p.th.Bg, terminal.AttrDim)
			body.Text(gut, i, tui.Truncate(line, body.W-gut), p.th.Fg, p.th.Bg, terminal.AttrNone)
		}
	case pvHex:
		a.renderHexRows(body, pv.raw, 0, hexBPR(body.W), -1)
	}
}

// --- hex viewer -------------------------------------------------------------

func hexBPR(w int) int {
	// 8 offset + 2 gap + bpr*3 hex + group gaps + 1 gap + bpr ascii
	if w >= 10+16*4+2 {
		return 16
	}
	return 8
}

func (a *appState) renderHexHeader(r tui.Region) {
	p := a.pal()
	r.Fill(p.th.HeaderBg)
	r.Text(1, 0, " hex ", p.th.Bg, p.accent2, terminal.AttrBold)
	name := tailTruncate(a.hx.path, r.W-30)
	r.Text(7, 0, name, p.th.HeaderFg, p.th.HeaderBg, terminal.AttrBold)
	if a.hx.truncated {
		r.TextRight(0, fmt.Sprintf("[truncated to %s] ", humanSize(hexLoadCap)), p.th.Warning, p.th.HeaderBg, terminal.AttrBold)
	}
}

func (a *appState) renderHexBody(r tui.Region) {
	p := a.pal()
	r.Fill(p.th.Bg)
	h := &a.hx
	h.bpr = hexBPR(r.W - 1)
	h.visRows = r.H
	a.hexEnsureVisible()

	body := r.Sub(0, 0, r.W-1, r.H)
	a.renderHexRows(body, h.data, h.scrollRow, h.bpr, h.cursor)

	totalRows := (len(h.data) + h.bpr - 1) / h.bpr
	sb := r.Sub(r.W-1, 0, 1, r.H)
	sb.ScrollBar(0, h.scrollRow, sb.H, totalRows, p.th.Border)

	if len(h.data) == 0 {
		body.TextCenter(r.H/2, "empty file", p.th.HintFg, p.th.Bg, terminal.AttrDim)
	}
}

// renderHexRows is shared by preview (cursor=-1, no matches) and full view.
func (a *appState) renderHexRows(r tui.Region, data []byte, scrollRow, bpr, cursor int) {
	p := a.pal()
	h := &a.hx
	asciiX := 10 + bpr*3 + bpr/8
	for y := 0; y < r.H; y++ {
		base := (scrollRow + y) * bpr
		if base >= len(data) {
			break
		}
		r.Text(0, y, fmt.Sprintf("%08x", base), p.th.HintFg, p.th.Bg, terminal.AttrDim)
		for i := 0; i < bpr; i++ {
			off := base + i
			hx := 10 + i*3 + i/8 // extra gap every 8 bytes
			if off >= len(data) {
				break
			}
			b := data[off]
			fg := a.byteColor(b)
			bg := p.th.Bg
			attr := terminal.AttrNone
			if cursor >= 0 && len(h.starts) > 0 { // match highlight (full view only)
				if mi, cur := a.matchAt(off); mi {
					bg = p.matchBg
					if cur {
						bg = p.matchCurBg
					}
				}
			}
			if off == cursor {
				fg, bg, attr = p.th.Bg, p.accent, terminal.AttrBold
			}
			hi, lo := hexDigit(b>>4), hexDigit(b&0xf)
			r.Cell(hx, y, hi, fg, bg, attr)
			r.Cell(hx+1, y, lo, fg, bg, attr)

			ac := '·'
			if b >= 0x20 && b < 0x7f {
				ac = rune(b)
			}
			afg, abg := fg, bg
			if off == cursor {
				afg, abg = p.th.Bg, p.accent
			}
			r.Cell(asciiX+i, y, ac, afg, abg, attr)
		}
	}
}

func (a *appState) matchAt(off int) (in, current bool) {
	h := &a.hx
	i := sort.SearchInts(h.starts, off+1) - 1
	if i < 0 || off >= h.starts[i]+h.patLen {
		return false, false
	}
	return true, i == h.matchIdx
}

func (a *appState) byteColor(b byte) color.RGB {
	p := a.pal()
	switch {
	case b == 0x00:
		return p.hexNull
	case b == 0x20 || b == 0x09 || b == 0x0a || b == 0x0d:
		return p.hexSpace
	case b < 0x20 || b == 0x7f:
		return p.hexCtrl
	case b >= 0x80:
		return p.hexHigh
	default:
		return p.hexPrint
	}
}

func hexDigit(n byte) rune {
	if n < 10 {
		return rune('0' + n)
	}
	return rune('a' + n - 10)
}

func (a *appState) renderHexStatus(r tui.Region) {
	p := a.pal()
	r.Fill(p.th.HeaderBg)
	h := &a.hx
	n := len(h.data)
	pct := 0
	if n > 1 {
		pct = h.cursor * 100 / (n - 1)
	}
	r.Gauge(1, 0, 22, pct, 100, p.accent, p.th.HeaderBg)

	valStr, off := "--", "--"
	if n > 0 {
		b := h.data[h.cursor]
		ch := "·"
		if b >= 0x20 && b < 0x7f {
			ch = string(rune(b))
		}
		valStr = fmt.Sprintf("0x%02x %3d 0b%08b '%s'", b, b, b, ch)
		off = fmt.Sprintf("0x%08x/%08x", h.cursor, n)
	}
	match := "-"
	if len(h.starts) > 0 {
		match = fmt.Sprintf("%d/%d", h.matchIdx+1, len(h.starts))
	}
	hint := tui.Style{Fg: p.th.HintFg}
	r.StatusBar(0, []tui.BarSection{
		{Label: "byte ", Value: valStr, LabelStyle: hint, ValueStyle: tui.Style{Fg: p.accent2, Attr: terminal.AttrBold}, Priority: 3},
		{Label: "match ", Value: match, LabelStyle: hint, ValueStyle: tui.Style{Fg: p.th.Selected}, Priority: 2},
		{Label: "off ", Value: off, LabelStyle: hint, ValueStyle: tui.Style{Fg: p.th.StatusFg}, Priority: 1},
	}, tui.BarOpts{Bg: p.th.HeaderBg, Align: tui.BarAlignRight})
}

// --- prompt / help / load frame ---------------------------------------------

func (a *appState) renderPrompt(r tui.Region) {
	p := a.pal()
	prefix := map[promptKind]string{
		prFilter: "/", prRename: "rename: ", prMkdir: "mkdir: ", prHexSearch: "search: ",
	}[a.prompt]
	st := tui.DefaultTextFieldStyle()
	st.TextBg, st.PrefixFg, st.TextFg = p.th.InputBg, p.accent, p.th.Fg
	st.CursorBg, st.CursorFg = p.th.Fg, p.th.Bg
	r.TextField(a.promptTF, tui.TextFieldOpts{
		Prefix: prefix, Border: tui.LineNone, Focused: true, Style: st,
		Placeholder: "…",
	})
}

func (a *appState) renderHelp(root tui.Region) {
	p := a.pal()
	pairs := [][2]string{
		{"j/k ↑/↓", "move"}, {"h/l ←/→", "parent / enter"}, {"Enter", "open dir · hex view file"},
		{"gg / G", "top / bottom"}, {"^D / ^U", "half page"}, {"/", "filter (live) · hex search"},
		{"Space", "mark"}, {"a / u", "mark all / clear"}, {"y", "yank paths"},
		{"r / m / D", "rename / mkdir / delete"}, {". / s", "hidden / sort"},
		{"n / N", "next / prev match (hex)"}, {"T", "theme"}, {"^L", "redraw"},
		{"q", "quit (writes lastdir)"}, {"Q", "quit + exec $SHELL in dir"},
	}
	h := len(pairs) + 4
	box := tui.Center(root, min(64, root.W-4), min(h, root.H-2))
	content := box.Modal(tui.ModalOpts{
		Title: "fv — keys", Hint: "any key closes",
		Border: tui.LineDouble, BorderFg: p.accent,
		TitleFg: p.th.HeaderFg, HintFg: p.th.HintFg, Bg: p.th.FocusBg,
	})
	ks := tui.Style{Fg: p.accent, Bg: p.th.FocusBg, Attr: terminal.AttrBold}
	vs := tui.Style{Fg: p.th.Fg, Bg: p.th.FocusBg}
	for i, kv := range pairs {
		content.KeyValue(i+1, kv[0], kv[1], ks, vs, ' ')
	}
}

// drawLoadFrame renders one determinate-progress frame during hex file load.
func (a *appState) drawLoadFrame(name string, frac float64) {
	w, h := a.width, a.height
	p := a.pal()
	cells := make([]terminal.Cell, w*h)
	for i := range cells {
		cells[i] = terminal.Cell{Rune: ' ', Fg: p.th.Fg, Bg: p.th.Bg}
	}
	root := tui.NewRegion(cells, w, 0, 0, w, h)
	opts := tui.DefaultProgressOpts("Loading", name, tui.ProgressDeterminate)
	opts.Width = min(52, w-6)
	opts.Progress = frac
	opts.Frame = a.frame
	opts.BarFg, opts.AccentFg, opts.Bg, opts.Fg = a.pal().accent, a.pal().accent2, p.th.FocusBg, p.th.Fg
	root.ProgressOverlay(opts)
	a.term.Flush(cells, w, h)
	a.frame++
}

func tailTruncate(s string, w int) string {
	if w < 4 {
		return ""
	}
	rs := []rune(s)
	if len(rs) <= w {
		return s
	}
	return "…" + string(rs[len(rs)-w+1:])
}
