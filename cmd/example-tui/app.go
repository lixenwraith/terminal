package main

import (
	"cmp"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/lixenwraith/terminal"
	"github.com/lixenwraith/terminal/tui"
)

const (
	hexLoadCap  = 8 << 20 // full-view byte cap; larger files load truncated
	previewCap  = 4096    // preview read size
	maxMatches  = 5000    // search result cap
	toastFrames = 25      // ~2.5 s at 10 fps
)

// --- model -----------------------------------------------------------------

type sortMode uint8

const (
	sortName sortMode = iota
	sortSize
	sortTime
	sortModeCount
)

func (s sortMode) String() string { return [...]string{"name", "size", "time"}[s] }

type entry struct {
	name    string
	dir     bool
	symlink bool
	size    int64
	mode    fs.FileMode
	mtime   time.Time
	broken  bool // stat failed
}

type browser struct {
	cwd        string
	all        []entry // sorted superset
	view       []int   // indices into all, after hidden+filter
	cursor     int     // index into view
	scroll     int
	marks      map[string]bool
	filter     string
	sortBy     sortMode
	showHidden bool

	parent    []entry // left column
	parentSel int
}

type previewKind uint8

const (
	pvNone previewKind = iota
	pvText
	pvHex
	pvDir
	pvErr
)

type preview struct {
	kind    previewKind
	path    string // cache key
	lines   []string
	raw     []byte
	entries []entry
	info    fs.FileInfo
	errMsg  string
}

type hexView struct {
	path      string
	data      []byte
	truncated bool
	cursor    int
	scrollRow int
	bpr       int // bytes/row, set by render
	visRows   int // set by render
	starts    []int
	patLen    int
	matchIdx  int
	query     string
	queryHex  bool
}

type viewMode uint8

const (
	viewBrowse viewMode = iota
	viewHex
)

type overlayMode uint8

const (
	ovNone overlayMode = iota
	ovPrompt
	ovConfirm
	ovHelp
)

type promptKind uint8

const (
	prFilter promptKind = iota
	prRename
	prMkdir
	prHexSearch
)

type rect struct{ x, y, w, h int }

func (r rect) contains(x, y int) bool { return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h }

type appState struct {
	term      terminal.Terminal
	width     int
	height    int
	frame     int
	quit      bool
	execShell bool

	themeIdx int
	view     viewMode
	overlay  overlayMode

	br browser
	pv preview
	hx hexView

	prompt     promptKind
	promptTF   *tui.TextFieldState
	confirm    *tui.ConfirmState
	confirmMsg string
	onConfirm  func()

	toast    tui.ToastState
	pendingG bool
	clip     []string

	geom struct{ list, parent rect } // last-rendered hit-test rects
}

func newApp(t terminal.Terminal, cwd string) *appState {
	w, h := t.Size()
	return &appState{
		term: t, width: w, height: h,
		br: browser{cwd: cwd, marks: map[string]bool{}},
	}
}

// --- filesystem ------------------------------------------------------------

func readEntries(dir string) ([]entry, error) {
	des, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]entry, 0, len(des))
	for _, de := range des {
		e := entry{name: de.Name(), dir: de.IsDir(), symlink: de.Type()&fs.ModeSymlink != 0}
		if fi, err := de.Info(); err == nil {
			e.size, e.mode, e.mtime = fi.Size(), fi.Mode(), fi.ModTime()
		} else {
			e.broken = true
		}
		out = append(out, e)
	}
	return out, nil
}

func (a *appState) sortEntries(es []entry) {
	by := a.br.sortBy
	slices.SortFunc(es, func(x, y entry) int {
		if x.dir != y.dir { // dirs first, always
			if x.dir {
				return -1
			}
			return 1
		}
		switch by {
		case sortSize:
			if c := cmp.Compare(y.size, x.size); c != 0 {
				return c
			}
		case sortTime:
			if c := y.mtime.Compare(x.mtime); c != 0 {
				return c
			}
		}
		return cmp.Compare(strings.ToLower(x.name), strings.ToLower(y.name))
	})
}

// loadDir replaces browser state; keep selects the cursor entry by name.
func (a *appState) loadDir(dir, keep string) error {
	es, err := readEntries(dir)
	if err != nil {
		return err
	}
	a.br.cwd = dir
	a.br.all = es
	a.sortEntries(a.br.all)
	a.br.filter = ""
	clear(a.br.marks)
	a.rebuildView(keep)
	a.loadParent()
	a.pv.path = "" // invalidate preview cache
	return nil
}

func (a *appState) loadParent() {
	p := filepath.Dir(a.br.cwd)
	if p == a.br.cwd { // at root
		a.br.parent = []entry{{name: "/", dir: true}}
		a.br.parentSel = 0
		return
	}
	es, err := readEntries(p)
	if err != nil {
		a.br.parent, a.br.parentSel = nil, 0
		return
	}
	a.sortEntries(es)
	// parent column shows dirs only (lf-style)
	dirs := es[:0]
	for _, e := range es {
		if e.dir {
			dirs = append(dirs, e)
		}
	}
	a.br.parent = dirs
	base := filepath.Base(a.br.cwd)
	a.br.parentSel = 0
	for i, e := range dirs {
		if e.name == base {
			a.br.parentSel = i
			break
		}
	}
}

func (a *appState) rebuildView(keep string) {
	b := &a.br
	b.view = b.view[:0]
	f := strings.ToLower(b.filter)
	for i, e := range b.all {
		if !b.showHidden && strings.HasPrefix(e.name, ".") {
			continue
		}
		if f != "" && !strings.Contains(strings.ToLower(e.name), f) {
			continue
		}
		b.view = append(b.view, i)
	}
	b.cursor = 0
	if keep != "" {
		for vi, i := range b.view {
			if b.all[i].name == keep {
				b.cursor = vi
				break
			}
		}
	}
	if b.cursor >= len(b.view) {
		b.cursor = max(0, len(b.view)-1)
	}
	b.scroll = 0
}

func (a *appState) cursorEntry() (entry, bool) {
	b := &a.br
	if b.cursor < 0 || b.cursor >= len(b.view) {
		return entry{}, false
	}
	return b.all[b.view[b.cursor]], true
}

func (a *appState) cursorPath() (string, bool) {
	e, ok := a.cursorEntry()
	if !ok {
		return "", false
	}
	return filepath.Join(a.br.cwd, e.name), true
}

// targetSet returns marked entries, or the cursor entry if none marked.
func (a *appState) targetSet() []entry {
	var out []entry
	for _, i := range a.br.view {
		if a.br.marks[a.br.all[i].name] {
			out = append(out, a.br.all[i])
		}
	}
	if len(out) == 0 {
		if e, ok := a.cursorEntry(); ok {
			out = append(out, e)
		}
	}
	return out
}

// --- preview ---------------------------------------------------------------

func (a *appState) refreshPreview() {
	path, ok := a.cursorPath()
	if !ok {
		a.pv = preview{kind: pvNone, path: ""}
		return
	}
	if a.pv.path == path {
		return // cached
	}
	pv := preview{path: path}
	fi, err := os.Lstat(path)
	if err != nil {
		pv.kind, pv.errMsg = pvErr, err.Error()
		a.pv = pv
		return
	}
	pv.info = fi
	e, _ := a.cursorEntry()
	if e.dir {
		es, err := readEntries(path)
		if err != nil {
			pv.kind, pv.errMsg = pvErr, err.Error()
		} else {
			a.sortEntries(es)
			if len(es) > 64 {
				es = es[:64]
			}
			pv.kind, pv.entries = pvDir, es
		}
		a.pv = pv
		return
	}
	f, err := os.Open(path)
	if err != nil {
		pv.kind, pv.errMsg = pvErr, err.Error()
		a.pv = pv
		return
	}
	buf := make([]byte, previewCap)
	n, _ := io.ReadFull(f, buf)
	f.Close()
	pv.raw = buf[:n]
	if isBinary(pv.raw) {
		pv.kind = pvHex
	} else {
		pv.kind = pvText
		pv.lines = textLines(pv.raw, 200)
	}
	a.pv = pv
}

func isBinary(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	n := min(len(b), 1024)
	bad := 0
	for _, c := range b[:n] {
		if c == 0 {
			return true
		}
		if c < 0x09 || (c > 0x0d && c < 0x20) {
			bad++
		}
	}
	return bad*10 > n*3
}

func textLines(b []byte, maxLines int) []string {
	s := strings.ReplaceAll(string(b), "\t", "    ")
	lines := strings.Split(s, "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	return lines
}

// --- hex viewer ------------------------------------------------------------

// openHex loads the file (capped), rendering a determinate progress overlay
// between chunks — synchronous, so no cross-goroutine state.
func (a *appState) openHex(path string) {
	fi, err := os.Stat(path)
	if err != nil {
		a.say(tui.ToastError, err.Error())
		return
	}
	f, err := os.Open(path)
	if err != nil {
		a.say(tui.ToastError, err.Error())
		return
	}
	defer f.Close()

	total := fi.Size()
	loadN := min(total, int64(hexLoadCap))
	data := make([]byte, 0, loadN)
	buf := make([]byte, 256<<10)
	var read int64
	for read < loadN {
		want := min(int64(len(buf)), loadN-read)
		n, err := f.Read(buf[:want])
		if n > 0 {
			data = append(data, buf[:n]...)
			read += int64(n)
		}
		a.drawLoadFrame(filepath.Base(path), float64(read)/float64(max(loadN, 1)))
		if err != nil {
			break
		}
	}
	a.hx = hexView{path: path, data: data, truncated: total > loadN, matchIdx: -1}
	a.view = viewHex
}

func decodeHexQuery(s string) ([]byte, bool) {
	t := strings.ToLower(strings.NewReplacer(" ", "", "\t", "", "0x", "").Replace(s))
	if t == "" || len(t)%2 != 0 {
		return nil, false
	}
	out := make([]byte, len(t)/2)
	for i := 0; i < len(t); i += 2 {
		hi, ok1 := hexNibble(t[i])
		lo, ok2 := hexNibble(t[i+1])
		if !ok1 || !ok2 {
			return nil, false
		}
		out[i/2] = hi<<4 | lo
	}
	return out, true
}

func hexNibble(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	}
	return 0, false
}

func (a *appState) hexSearch(q string) {
	h := &a.hx
	pat, isHex := decodeHexQuery(q)
	if !isHex {
		pat = []byte(q)
	}
	h.query, h.queryHex, h.patLen = q, isHex, len(pat)
	h.starts = h.starts[:0]
	for off := 0; len(h.starts) < maxMatches; {
		i := indexBytes(h.data[off:], pat)
		if i < 0 {
			break
		}
		h.starts = append(h.starts, off+i)
		off += i + 1
	}
	if len(h.starts) == 0 {
		h.matchIdx = -1
		a.say(tui.ToastWarning, "no match: "+q)
		return
	}
	// jump to first match at or after cursor, wrapping
	i := sort.SearchInts(h.starts, h.cursor) % len(h.starts)
	h.matchIdx = i
	h.cursor = h.starts[i]
	a.hexEnsureVisible()
	a.say(tui.ToastSuccess, fmt.Sprintf("%d match(es)", len(h.starts)))
}

func indexBytes(hay, pat []byte) int {
	if len(pat) == 0 || len(pat) > len(hay) {
		return -1
	}
	return strings.Index(string(hay), string(pat)) // zero-copy in map/index contexts is not needed here; sizes are capped
}

func (a *appState) hexEnsureVisible() {
	h := &a.hx
	if h.bpr <= 0 || h.visRows <= 0 {
		return
	}
	row := h.cursor / h.bpr
	if row < h.scrollRow {
		h.scrollRow = row
	}
	if row >= h.scrollRow+h.visRows {
		h.scrollRow = row - h.visRows + 1
	}
	maxRow := (len(h.data) + h.bpr - 1) / h.bpr
	h.scrollRow = max(0, min(h.scrollRow, max(0, maxRow-h.visRows)))
}

func (a *appState) hexMove(delta int) {
	h := &a.hx
	if len(h.data) == 0 {
		return
	}
	h.cursor = max(0, min(len(h.data)-1, h.cursor+delta))
	a.hexEnsureVisible()
}

// --- prompts / confirm / toast --------------------------------------------

func (a *appState) openPrompt(k promptKind, initial string) {
	a.prompt = k
	a.promptTF = tui.NewTextFieldState(initial)
	a.overlay = ovPrompt
}

func (a *appState) openConfirm(msg string, fn func()) {
	a.confirm = tui.NewConfirmState(false)
	a.confirmMsg = msg
	a.onConfirm = fn
	a.overlay = ovConfirm
}

func (a *appState) say(sev tui.ToastSeverity, msg string) {
	o := tui.DefaultToastOpts(msg, sev)
	o.Position = tui.ToastBottomRight
	o.Style = tui.ToastStyleRounded
	a.toast.Show(o, toastFrames)
}

// --- operations ------------------------------------------------------------

func (a *appState) opEnter() {
	e, ok := a.cursorEntry()
	if !ok {
		return
	}
	path := filepath.Join(a.br.cwd, e.name)
	if e.dir {
		if err := a.loadDir(path, ""); err != nil {
			a.say(tui.ToastError, err.Error())
		}
		return
	}
	a.openHex(path)
}

func (a *appState) opParent() {
	p := filepath.Dir(a.br.cwd)
	if p == a.br.cwd {
		return
	}
	keep := filepath.Base(a.br.cwd)
	if err := a.loadDir(p, keep); err != nil {
		a.say(tui.ToastError, err.Error())
	}
}

func (a *appState) opDelete() {
	ts := a.targetSet()
	if len(ts) == 0 {
		return
	}
	msg := fmt.Sprintf("Delete %d item(s)? (files and empty dirs only)", len(ts))
	if len(ts) == 1 {
		msg = "Delete '" + ts[0].name + "'?"
	}
	names := make([]string, len(ts))
	for i, e := range ts {
		names[i] = e.name
	}
	a.openConfirm(msg, func() {
		ok, fail := 0, 0
		for _, n := range names {
			if err := os.Remove(filepath.Join(a.br.cwd, n)); err != nil {
				fail++
			} else {
				ok++
			}
		}
		keep := ""
		if e, has := a.cursorEntry(); has {
			keep = e.name
		}
		_ = a.loadDir(a.br.cwd, keep)
		if fail > 0 {
			a.say(tui.ToastWarning, fmt.Sprintf("deleted %d, failed %d", ok, fail))
		} else {
			a.say(tui.ToastSuccess, fmt.Sprintf("deleted %d", ok))
		}
	})
}

func (a *appState) commitPrompt() {
	val := strings.TrimSpace(a.promptTF.Value())
	k := a.prompt
	a.overlay = ovNone
	switch k {
	case prFilter:
		// live filter already applied per keystroke
	case prHexSearch:
		if val != "" {
			a.hexSearch(val)
		}
	case prMkdir:
		if val == "" || strings.ContainsRune(val, os.PathSeparator) {
			a.say(tui.ToastWarning, "invalid name")
			return
		}
		if err := os.Mkdir(filepath.Join(a.br.cwd, val), 0o755); err != nil {
			a.say(tui.ToastError, err.Error())
			return
		}
		_ = a.loadDir(a.br.cwd, val)
		a.say(tui.ToastSuccess, "created "+val)
	case prRename:
		e, ok := a.cursorEntry()
		if !ok || val == "" || val == e.name || strings.ContainsRune(val, os.PathSeparator) {
			return
		}
		if err := os.Rename(filepath.Join(a.br.cwd, e.name), filepath.Join(a.br.cwd, val)); err != nil {
			a.say(tui.ToastError, err.Error())
			return
		}
		_ = a.loadDir(a.br.cwd, val)
		a.say(tui.ToastSuccess, "renamed → "+val)
	}
}

// --- event handling ---------------------------------------------------------

func (a *appState) handleEvent(ev terminal.Event) {
	switch ev.Type {
	case terminal.EventResize:
		a.width, a.height = ev.Width, ev.Height
		return
	case terminal.EventClosed, terminal.EventError:
		a.quit = true
		return
	case terminal.EventMouse:
		a.handleMouse(ev)
		return
	}
	if ev.Type != terminal.EventKey {
		return
	}
	if ev.Key == terminal.KeyNone { // synthetic tick
		a.frame++
		if a.toast.Visible {
			a.toast.Tick()
		}
		return
	}

	// Global
	switch ev.Key {
	case terminal.KeyCtrlC, terminal.KeyCtrlQ:
		a.quit = true
		return
	case terminal.KeyCtrlL:
		a.term.Sync()
		return
	}

	switch a.overlay {
	case ovHelp:
		a.overlay = ovNone
		return
	case ovConfirm:
		if a.confirm.HandleKey(ev.Key, ev.Rune) {
			a.overlay = ovNone
			if a.confirm.Result == tui.ConfirmYes && a.onConfirm != nil {
				a.onConfirm()
			}
			a.onConfirm = nil
		}
		return
	case ovPrompt:
		switch ev.Key {
		case terminal.KeyEscape:
			if a.prompt == prFilter {
				a.br.filter = ""
				a.rebuildView(currentName(a))
			}
			a.overlay = ovNone
		case terminal.KeyEnter:
			a.commitPrompt()
		default:
			if a.promptTF.HandleKey(ev.Key, ev.Rune, ev.Modifiers) && a.prompt == prFilter {
				a.br.filter = a.promptTF.Value()
				a.rebuildView("")
			}
		}
		return
	}

	if a.view == viewHex {
		a.handleHexKey(ev)
	} else {
		a.handleBrowseKey(ev)
	}
}

func currentName(a *appState) string {
	if e, ok := a.cursorEntry(); ok {
		return e.name
	}
	return ""
}

func (a *appState) moveCursor(delta int) {
	b := &a.br
	if len(b.view) == 0 {
		return
	}
	b.cursor = max(0, min(len(b.view)-1, b.cursor+delta))
}

func (a *appState) handleBrowseKey(ev terminal.Event) {
	b := &a.br
	pendG := a.pendingG
	a.pendingG = false

	switch ev.Key {
	case terminal.KeyUp:
		a.moveCursor(-1)
	case terminal.KeyDown:
		a.moveCursor(1)
	case terminal.KeyLeft, terminal.KeyBackspace:
		a.opParent()
	case terminal.KeyRight, terminal.KeyEnter:
		a.opEnter()
	case terminal.KeyHome:
		b.cursor = 0
	case terminal.KeyEnd:
		b.cursor = max(0, len(b.view)-1)
	case terminal.KeyPageUp:
		a.moveCursor(-tui.PageDelta(a.listVisible()) * 2)
	case terminal.KeyPageDown:
		a.moveCursor(tui.PageDelta(a.listVisible()) * 2)
	case terminal.KeyCtrlU:
		a.moveCursor(-tui.PageDelta(a.listVisible()))
	case terminal.KeyCtrlD:
		a.moveCursor(tui.PageDelta(a.listVisible()))
	case terminal.KeyEscape:
		if b.filter != "" {
			b.filter = ""
			a.rebuildView(currentName(a))
		} else {
			clear(b.marks)
		}
	case terminal.KeyRune:
		switch ev.Rune {
		case 'q':
			a.quit = true
		case 'Q':
			a.quit, a.execShell = true, true
		case 'j':
			a.moveCursor(1)
		case 'k':
			a.moveCursor(-1)
		case 'h':
			a.opParent()
		case 'l':
			a.opEnter()
		case 'g':
			if pendG {
				b.cursor = 0
			} else {
				a.pendingG = true
			}
		case 'G':
			b.cursor = max(0, len(b.view)-1)
		case '~':
			if home, err := os.UserHomeDir(); err == nil {
				if err := a.loadDir(home, ""); err != nil {
					a.say(tui.ToastError, err.Error())
				}
			}
		case ' ':
			if e, ok := a.cursorEntry(); ok {
				if b.marks[e.name] {
					delete(b.marks, e.name)
				} else {
					b.marks[e.name] = true
				}
				a.moveCursor(1)
			}
		case 'a':
			for _, i := range b.view {
				b.marks[b.all[i].name] = true
			}
		case 'u':
			clear(b.marks)
		case '.':
			b.showHidden = !b.showHidden
			a.rebuildView(currentName(a))
		case 's':
			b.sortBy = (b.sortBy + 1) % sortModeCount
			keep := currentName(a)
			a.sortEntries(b.all)
			a.rebuildView(keep)
		case '/':
			a.openPrompt(prFilter, b.filter)
		case 'r':
			if e, ok := a.cursorEntry(); ok {
				a.openPrompt(prRename, e.name)
			}
		case 'm':
			a.openPrompt(prMkdir, "")
		case 'D':
			a.opDelete()
		case 'y':
			ts := a.targetSet()
			a.clip = a.clip[:0]
			for _, e := range ts {
				a.clip = append(a.clip, filepath.Join(b.cwd, e.name))
			}
			a.say(tui.ToastInfo, fmt.Sprintf("yanked %d path(s)", len(a.clip)))
		case 'T':
			a.themeIdx = (a.themeIdx + 1) % len(palettes)
		case '?':
			a.overlay = ovHelp
		}
	}
	// keep cursor entry sane after any mutation
	if b.cursor >= len(b.view) {
		b.cursor = max(0, len(b.view)-1)
	}
}

func (a *appState) handleHexKey(ev terminal.Event) {
	h := &a.hx
	pendG := a.pendingG
	a.pendingG = false
	half := max(1, h.visRows/2) * h.bpr

	switch ev.Key {
	case terminal.KeyEscape:
		a.view = viewBrowse
	case terminal.KeyUp:
		a.hexMove(-h.bpr)
	case terminal.KeyDown:
		a.hexMove(h.bpr)
	case terminal.KeyLeft:
		a.hexMove(-1)
	case terminal.KeyRight:
		a.hexMove(1)
	case terminal.KeyHome:
		h.cursor = 0
		a.hexEnsureVisible()
	case terminal.KeyEnd:
		h.cursor = max(0, len(h.data)-1)
		a.hexEnsureVisible()
	case terminal.KeyPageUp:
		a.hexMove(-h.visRows * h.bpr)
	case terminal.KeyPageDown:
		a.hexMove(h.visRows * h.bpr)
	case terminal.KeyCtrlU:
		a.hexMove(-half)
	case terminal.KeyCtrlD:
		a.hexMove(half)
	case terminal.KeyRune:
		switch ev.Rune {
		case 'q', 'h':
			if ev.Rune == 'h' {
				a.hexMove(-1)
			} else {
				a.view = viewBrowse
			}
		case 'j':
			a.hexMove(h.bpr)
		case 'k':
			a.hexMove(-h.bpr)
		case 'l':
			a.hexMove(1)
		case '0':
			a.hexMove(-(h.cursor % max(1, h.bpr)))
		case '$':
			if h.bpr > 0 {
				a.hexMove(h.bpr - 1 - h.cursor%h.bpr)
			}
		case 'g':
			if pendG {
				h.cursor = 0
				a.hexEnsureVisible()
			} else {
				a.pendingG = true
			}
		case 'G':
			h.cursor = max(0, len(h.data)-1)
			a.hexEnsureVisible()
		case '/':
			a.openPrompt(prHexSearch, h.query)
		case 'n', 'N':
			if len(h.starts) == 0 {
				break
			}
			d := 1
			if ev.Rune == 'N' {
				d = -1
			}
			h.matchIdx = (h.matchIdx + d + len(h.starts)) % len(h.starts)
			h.cursor = h.starts[h.matchIdx]
			a.hexEnsureVisible()
		}
	}
}

func (a *appState) handleMouse(ev terminal.Event) {
	if ev.MouseAction != terminal.MouseActionPress {
		return
	}
	switch ev.MouseBtn {
	case terminal.MouseBtnWheelUp:
		if a.view == viewHex {
			a.hexMove(-3 * a.hx.bpr)
		} else {
			a.moveCursor(-3)
		}
	case terminal.MouseBtnWheelDown:
		if a.view == viewHex {
			a.hexMove(3 * a.hx.bpr)
		} else {
			a.moveCursor(3)
		}
	case terminal.MouseBtnLeft:
		if a.view != viewBrowse || a.overlay != ovNone {
			return
		}
		if a.geom.list.contains(ev.MouseX, ev.MouseY) {
			idx := a.br.scroll + (ev.MouseY - a.geom.list.y)
			if idx >= 0 && idx < len(a.br.view) {
				if a.br.cursor == idx {
					a.opEnter() // click on cursor row = open
				} else {
					a.br.cursor = idx
				}
			}
		} else if a.geom.parent.contains(ev.MouseX, ev.MouseY) {
			a.opParent()
		}
	}
}

func (a *appState) listVisible() int { return a.geom.list.h }

// --- misc helpers -----------------------------------------------------------

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	v := float64(n) / float64(div)
	suffix := "KMGTPE"[exp]
	if v < 10 {
		return fmt.Sprintf("%.1f%c", v, suffix)
	}
	return fmt.Sprintf("%.0f%c", v, suffix)
}

func padRight(s string, w int) string {
	n := 0
	for range s {
		n++
	}
	if n >= w {
		return s
	}
	return s + strings.Repeat(" ", w-n)
}

func classify(r rune) bool { return unicode.IsPrint(r) }
