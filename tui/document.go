package tui

// DocKind classifies a document block
type DocKind uint8

const (
	DocSection DocKind = iota // Section header with an inline rule
	DocEntry                  // Key + description in aligned columns
	DocPara                   // Wrapped paragraph across the document width
	DocGap                    // One blank row
)

// DocBlock is one logical unit of a document
type DocBlock struct {
	Kind DocKind
	Key  string // DocEntry only
	Text string
}

// DocOpts configures document layout and rendering
// Layout-affecting fields must not change between Layout and Doc
type DocOpts struct {
	HeaderStyle Style
	KeyStyle    Style
	TextStyle   Style
	RuleStyle   Style
	Rule        LineType // Header rule; LineNone draws the header alone
	KeyWidth    int      // Fixed key column, 0 = measured from the entries
	KeyMaxW     int      // Cap for the measured key column, 0 = 40% of width
	MinTextW    int      // Text column narrower than this stacks every entry
	Gap         int      // Columns between key and text, minimum 1
	Indent      int      // Left indent of section content
	StackIndent int      // Indent of a stacked description
	SectionGap  int      // Blank rows before a section header
	MaxWidth    int      // Document width cap, 0 = region width
	Center      bool     // Center the document when the cap applies
}

// Row kinds in the flattened document
const (
	docRowBlank uint8 = iota
	docRowSection
	docRowKey   // Stacked entry key, left-aligned
	docRowEntry // Key column plus the first description line
	docRowText  // Continuation or stacked description line
)

type docRow struct {
	key  string
	text string
	x    int
	kind uint8
}

// DocState holds a laid-out document and its scroll position
type DocState struct {
	Viewport *ViewportScroll
	rows     []docRow
	width    int // Document width used by the layout
	xOff     int // Document offset within the render region
	keyW     int
	gap      int
}

// NewDocState creates document state
func NewDocState() *DocState {
	return &DocState{Viewport: NewViewportScroll()}
}

// SetViewport updates viewport height
func (d *DocState) SetViewport(h int) {
	d.Viewport.ViewportH = h
	d.Viewport.ScrollTo(d.Viewport.Offset)
}

// Layout flattens blocks into rows for the given width; the row count becomes
// the viewport content height. Entries stack when the text column is too
// narrow, and an over-long key stacks on its own rather than truncating.
func (d *DocState) Layout(blocks []DocBlock, width int, opts DocOpts) {
	d.rows = d.rows[:0]
	if width < 1 {
		d.width, d.Viewport.ContentH = 0, 0
		return
	}

	docW := width
	if opts.MaxWidth > 0 && docW > opts.MaxWidth {
		docW = opts.MaxWidth
	}
	d.width = docW
	d.xOff = 0
	if opts.Center {
		d.xOff = (width - docW) / 2
	}

	d.gap = max(opts.Gap, 1)
	indent := min(max(opts.Indent, 0), docW-1)

	keyMax := opts.KeyMaxW
	if keyMax <= 0 {
		keyMax = docW * 2 / 5
	}
	keyMax = min(keyMax, max(docW/2, 1))

	keyW := opts.KeyWidth
	if keyW <= 0 {
		for i := range blocks {
			if blocks[i].Kind != DocEntry {
				continue
			}
			if n := RuneLen(blocks[i].Key); n > keyW && n <= keyMax {
				keyW = n
			}
		}
	}
	d.keyW = min(max(keyW, 1), keyMax)

	textX := indent + d.keyW + d.gap
	textW := docW - textX
	stackX := min(indent+max(opts.StackIndent, 0), docW-1)
	stackW := max(docW-stackX, 1)
	stacked := textW < opts.MinTextW || textW < 1

	for i := range blocks {
		b := &blocks[i]
		switch b.Kind {
		case DocGap:
			d.rows = append(d.rows, docRow{kind: docRowBlank})

		case DocSection:
			for range opts.SectionGap {
				if len(d.rows) > 0 {
					d.rows = append(d.rows, docRow{kind: docRowBlank})
				}
			}
			d.rows = append(d.rows, docRow{kind: docRowSection, text: b.Text})

		case DocPara:
			for _, line := range WrapText(b.Text, max(docW-indent, 1)) {
				d.rows = append(d.rows, docRow{kind: docRowText, x: indent, text: line})
			}

		case DocEntry:
			// Narrow document: every entry stacks at the stack indent
			if stacked {
				d.rows = append(d.rows, docRow{kind: docRowKey, x: indent, key: b.Key})
				for _, line := range WrapText(b.Text, stackW) {
					d.rows = append(d.rows, docRow{kind: docRowText, x: stackX, text: line})
				}
				continue
			}
			// Over-long key stacks alone but keeps the shared text column
			if RuneLen(b.Key) > d.keyW {
				d.rows = append(d.rows, docRow{kind: docRowKey, x: indent, key: b.Key})
				for _, line := range WrapText(b.Text, textW) {
					d.rows = append(d.rows, docRow{kind: docRowText, x: textX, text: line})
				}
				continue
			}
			lines := WrapText(b.Text, textW)
			d.rows = append(d.rows, docRow{kind: docRowEntry, x: indent, key: b.Key, text: lines[0]})
			for _, line := range lines[1:] {
				d.rows = append(d.rows, docRow{kind: docRowText, x: textX, text: line})
			}
		}
	}

	d.Viewport.ContentH = len(d.rows)
}

// Doc renders the visible window of a laid-out document
func (r Region) Doc(d *DocState, opts DocOpts) {
	if d == nil || r.H < 1 || r.W < 1 {
		return
	}
	d.Viewport.SetDimensions(len(d.rows), r.H)

	for y := range r.H {
		i := d.Viewport.Offset + y
		if i >= len(d.rows) {
			break
		}
		row := &d.rows[i]
		x := d.xOff + row.x

		switch row.kind {
		case docRowSection:
			r.docSection(y, d.xOff, d.width, row.text, opts)
		case docRowKey:
			r.TextStyled(x, y, row.key, opts.KeyStyle)
		case docRowEntry:
			r.TextStyled(x+d.keyW-RuneLen(row.key), y, row.key, opts.KeyStyle)
			r.TextStyled(x+d.keyW+d.gap, y, row.text, opts.TextStyle)
		case docRowText:
			r.TextStyled(x, y, row.text, opts.TextStyle)
		}
	}
}

// docSection draws a full-width rule with the label inset from the left
func (r Region) docSection(y, x, w int, label string, opts DocOpts) {
	if opts.Rule == LineNone {
		r.TextStyled(x, y, label, opts.HeaderStyle)
		return
	}
	line := opts.Rule
	if line >= LineType(len(boxChars)) {
		line = LineSingle
	}
	ch := boxChars[line][boxH]
	for i := range w {
		r.Cell(x+i, y, ch, opts.RuleStyle.Fg, opts.RuleStyle.Bg, opts.RuleStyle.Attr)
	}
	if label == "" {
		return
	}

	const lead = 2
	text := " " + label + " "
	if RuneLen(text)+lead > w {
		text = Truncate(text, max(w-lead, 1))
	}
	r.TextStyled(x+lead, y, text, opts.HeaderStyle)
}
