//go:build js && wasm

package terminal

import (
	"slices"
	"syscall/js"
	"testing"
	"time"
)

// pageWorker defines the bridge as a page's worker does, over a page that answers
// a zoom as fitted at once, and returns it with the messages it is posted
func pageWorker(t *testing.T) (worker, posted js.Value) {
	worker = js.ValueOf(map[string]any{
		"grid": map[string]any{"cols": 100, "rows": 30}, "kept": map[string]any{"settings": "cell = 14"},
		"features": []any{"menu"}, "hidden": false,
	})
	posted = js.Global().Get("Array").New()
	frame := js.FuncOf(func(js.Value, []js.Value) any { return nil })
	post := js.FuncOf(func(_ js.Value, args []js.Value) any {
		posted.Call("push", js.Global().Get("Object").Call("assign", map[string]any{"type": args[0]}, args[1]))
		if event := worker.Get("event"); args[0].String() == "zoom" && event.Type() == js.TypeFunction {
			event.Invoke(map[string]any{"type": "fitted"})
		}
		return nil
	})
	worker.Set("frame", frame)
	worker.Set("post", post)
	js.Global().Set(jsPage, worker)
	t.Cleanup(func() {
		js.Global().Delete(jsPage)
		frame.Release()
		post.Release()
	})
	return worker, posted
}

// TestAPagesBarTakesATapWithThePointerOff: with the program taking no pointer, a
// finger's press on the bar's rows is taken with its release, as a phone's view
// takes it; one above them, a mouse's anywhere and a lone release are not.
func TestAPagesBarTakesATapWithThePointerOff(t *testing.T) {
	p := &Page{barRows: 2}
	p.setGrid(10, 4)
	at := func(y int, action MouseAction, touch bool) bool {
		return p.admit(Event{Type: EventMouse, MouseBtn: MouseBtnLeft, MouseAction: action, MouseY: y, Touch: touch})
	}
	if at(1, MouseActionPress, true) || at(1, MouseActionRelease, true) {
		t.Fatal("a finger above the bar was taken")
	}
	if at(3, MouseActionPress, false) {
		t.Fatal("a mouse press on the bar was taken")
	}
	if !at(2, MouseActionPress, true) || !at(0, MouseActionRelease, true) {
		t.Fatal("a finger on the bar was not taken with its release")
	}
}

// TestAProgramsOwnMessagesCrossThePagesBridge: a page opens on its worker's bridge
// with its font, then its zoom, and once that is fitted. From then a kind the page
// handles reaches it with its fields, bytes among them, a document it kept comes
// back by name and goes to be kept, and a kind of the program's own reaches what
// Handle named for it. With no worker nothing is had, kept or posted.
func TestAProgramsOwnMessagesCrossThePagesBridge(t *testing.T) {
	PagePost("menu", nil)
	if PageHas("menu") || PageKept("settings") != nil || PageHidden() {
		t.Fatal("a page with no worker")
	}
	worker, posted := pageWorker(t)
	if !PageHas("menu") || PageHas("audio") {
		t.Fatal("the page handles other kinds than its worker lists")
	}
	if string(PageKept("settings")) != "cell = 14" || PageKept("places") != nil {
		t.Fatalf("kept %q and %q", PageKept("settings"), PageKept("places"))
	}

	start := time.Now()
	p, err := OpenPage(PageConfig{BarRows: 2, Cell: 14, CellLeast: 8, CellMost: 32, CellStep: 2, Font: []byte{1, 2, 3}})
	if err != nil || time.Since(start) >= fitWait {
		t.Fatalf("OpenPage: %v, after %v", err, time.Since(start))
	}
	if cols, rows := p.Size(); cols != 100 || rows != 30 {
		t.Fatalf("the page opened at %dx%d, its worker's grid being 100x30", cols, rows)
	}
	PagePost("menu", map[string]any{"json": "[]"})
	PagePost("audio", map[string]any{"data": PageBytes([]byte{0, 0, 1, 0})})
	PageKeep("settings", []byte("cell = 16"))
	const want = `[{"type":"font","data":{"0":1,"1":2,"2":3}},` +
		`{"type":"zoom","cell":14,"least":8,"most":32,"step":2,"bar":2},` +
		`{"type":"menu","json":"[]"},{"type":"audio","data":{"0":0,"1":0,"2":1,"3":0}},` +
		`{"type":"keep","name":"settings","text":"cell = 16"}]`
	// The names in the order each message is written out, a typed array by its indexes
	names := []any{"type", "data", "0", "1", "2", "3", "cell", "least", "most", "step", "bar", "json", "name", "text"}
	if got := js.Global().Get("JSON").Call("stringify", posted, names).String(); got != want {
		t.Fatalf("the page was posted\n%s, want\n%s", got, want)
	}

	var away []bool
	p.Handle("away", func(msg js.Value) { away = append(away, msg.Get("away").Truthy()) })
	for _, msg := range []map[string]any{{"type": "away", "away": true}, {"type": "unheard"}, {"type": "away", "away": false}} {
		worker.Get("event").Invoke(msg)
	}
	if !slices.Equal(away, []bool{true, false}) {
		t.Fatalf("away crossed as %v", away)
	}
}

// TestNewsInitOpensThePageAndFailsWhereNoneIs: New's Terminal is the page, which
// Init opens: under Node, where no worker defines the bridge, Init says so and Fini
// has nothing to end; over a worker it is the page's grid in truecolor, whatever
// colour mode was asked.
func TestNewsInitOpensThePageAndFailsWhereNoneIs(t *testing.T) {
	term := New(ColorMode256)
	if err := term.Init(); err == nil {
		t.Fatal("Init opened a page where none is")
	}
	term.Fini()

	pageWorker(t)
	if err := term.Init(); err != nil {
		t.Fatal(err)
	}
	defer term.Fini()
	if cols, rows := term.Size(); cols != 100 || rows != 30 || term.ColorMode() != ColorModeTrueColor {
		t.Fatalf("the page is %dx%d in %v", cols, rows, term.ColorMode())
	}
}
