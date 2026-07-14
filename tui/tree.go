package tui

import (
	"github.com/lixenwraith/color"
	"github.com/lixenwraith/terminal"
)

// ExpandIcon chars
const (
	IconExpanded  = '▼'
	IconCollapsed = '▶'
	IconBullet    = '•'
)

// ExpandIconRune returns appropriate expand/collapse indicator
func ExpandIconRune(expanded bool) rune {
	if expanded {
		return IconExpanded
	}
	return IconCollapsed
}

// TreeLineMode specifies connector line rendering
type TreeLineMode uint8

const (
	TreeLinesNone   TreeLineMode = iota // Indent only, no lines
	TreeLinesSimple                     // │ continuation, └ for last
)

// TreeNode represents a node in a hierarchical tree
type TreeNode struct {
	Key        string // Unique identifier for expansion state
	Label      string // Display text
	Icon       rune   // Custom icon, 0 = auto (▶/▼ for expandable, • for leaf)
	IconFg     color.RGB
	Expandable bool       // Has children
	Expanded   bool       // Currently expanded
	Depth      int        // Nesting level (0 = root)
	Check      CheckState // Optional checkbox, CheckNone to skip
	CheckFg    color.RGB
	Style      Style // Text styling
	IsLast     bool  // Last sibling at this depth (for tree lines)
	Data       any   // Application payload

	Suffix      string    // Secondary text after label (e.g., "(5 files)")
	SuffixStyle Style     // Styling for suffix, zero = dimmed version of Style
	Badge       rune      // Icon between checkbox and label (e.g., '★'), 0 = none
	BadgeFg     color.RGB // Badge color
}

// ancestorHasMoreSiblings checks if there are more nodes at given depth after idx
func (r Region) ancestorHasMoreSiblings(nodes []TreeNode, idx, depth int) bool {
	targetDepth := depth
	for i := idx + 1; i < len(nodes); i++ {
		if nodes[i].Depth <= targetDepth {
			return nodes[i].Depth == targetDepth
		}
	}
	return false
}

// TreeOpts configures tree rendering
type TreeOpts struct {
	CursorBg    color.RGB
	DefaultBg   color.RGB
	IndentWidth int // Cells per depth, default 2
	IconWidth   int // Width for icon column, default 2
	LineMode    TreeLineMode
	LineFg      color.RGB
}

// DefaultTreeOpts returns sensible defaults
func DefaultTreeOpts() TreeOpts {
	return TreeOpts{
		IndentWidth: 2,
		IconWidth:   2,
		LineMode:    TreeLinesNone,
		LineFg:      color.RGB{R: 80, G: 80, B: 100},
	}
}

// Tree renders hierarchical tree nodes within region, returns number of rows rendered
// Nodes must be pre-flattened (only visible/expanded nodes included)
func (r Region) Tree(nodes []TreeNode, cursor, scroll int, opts TreeOpts) int {
	if r.H < 1 || len(nodes) == 0 {
		return 0
	}

	indentW := opts.IndentWidth
	if indentW < 1 {
		indentW = 2
	}
	iconW := opts.IconWidth
	if iconW < 1 {
		iconW = 2
	}

	lineFg := opts.LineFg
	if lineFg == (color.RGB{}) {
		lineFg = DefaultTheme.Border
	}

	rendered := 0
	for y := 0; y < r.H; y++ {
		idx := scroll + y
		if idx >= len(nodes) {
			break
		}

		node := nodes[idx]
		isCursor := idx == cursor

		bg := opts.DefaultBg
		if isCursor {
			bg = opts.CursorBg
		}

		for x := 0; x < r.W; x++ {
			r.Cell(x, y, ' ', color.RGB{}, bg, terminal.AttrNone)
		}

		x := 0

		if opts.LineMode == TreeLinesSimple && node.Depth > 0 {
			x = r.renderTreeLines(y, x, node, nodes, idx, scroll, indentW, lineFg, bg)
		} else {
			x = node.Depth * indentW
		}

		// Icon (expand indicator or bullet)
		icon := node.Icon
		iconFg := node.IconFg
		if icon == 0 {
			if node.Expandable {
				if node.Expanded {
					icon = IconExpanded
				} else {
					icon = IconCollapsed
				}
			} else {
				icon = IconBullet
			}
		}
		if iconFg == (color.RGB{}) {
			iconFg = lineFg
		}
		if x < r.W {
			r.Cell(x, y, icon, iconFg, bg, terminal.AttrNone)
		}
		x += iconW

		// Checkbox
		if node.Check != CheckNone || node.CheckFg != (color.RGB{}) {
			if x+3 <= r.W {
				checkFg := node.CheckFg
				if checkFg == (color.RGB{}) {
					checkFg = node.Style.Fg
				}
				var ch rune
				switch node.Check {
				case CheckNone:
					ch = ' '
				case CheckPartial:
					ch = 'o'
				case CheckFull:
					ch = 'x'
				case CheckPlus:
					ch = '+'
				}
				r.Cell(x, y, '[', checkFg, bg, terminal.AttrNone)
				r.Cell(x+1, y, ch, checkFg, bg, terminal.AttrNone)
				r.Cell(x+2, y, ']', checkFg, bg, terminal.AttrNone)
			}
			x += 4
		}

		// Badge (optional icon between checkbox and label)
		if node.Badge != 0 {
			if x < r.W {
				badgeFg := node.BadgeFg
				if badgeFg == (color.RGB{}) {
					badgeFg = node.Style.Fg
				}
				r.Cell(x, y, node.Badge, badgeFg, bg, terminal.AttrNone)
			}
			x += 2
		}

		// Label
		style := node.Style
		if style.Bg == (color.RGB{}) {
			style.Bg = bg
		}

		// Calculate available width for label + suffix
		availW := r.W - x - 1
		labelLen := RuneLen(node.Label)
		suffixLen := RuneLen(node.Suffix)

		label := node.Label
		suffix := node.Suffix

		// Truncate if needed, prioritizing label over suffix
		if labelLen+suffixLen > availW {
			if labelLen > availW {
				label = Truncate(label, availW)
				suffix = ""
			} else {
				suffix = Truncate(suffix, availW-labelLen)
			}
		}

		r.TextStyled(x, y, label, style)
		x += RuneLen(label)

		// Suffix
		if suffix != "" {
			suffixStyle := node.SuffixStyle
			if suffixStyle.Fg == (color.RGB{}) {
				// Default: dimmed version of label style
				suffixStyle.Fg = color.RGB{
					R: node.Style.Fg.R / 2,
					G: node.Style.Fg.G / 2,
					B: node.Style.Fg.B / 2,
				}
			}
			if suffixStyle.Bg == (color.RGB{}) {
				suffixStyle.Bg = bg
			}
			r.TextStyled(x, y, suffix, suffixStyle)
		}

		rendered++
	}

	return rendered
}

// renderTreeLines draws connector lines for tree structure, returns x position after lines
func (r Region) renderTreeLines(y, startX int, node TreeNode, nodes []TreeNode, idx, scroll, indentW int, fg color.RGB, bg color.RGB) int {
	x := startX

	for d := 0; d < node.Depth; d++ {
		// Determine if ancestor at this depth has more siblings below
		hasMore := r.ancestorHasMoreSiblings(nodes, idx, d)

		if d == node.Depth-1 {
			// Direct parent level - show branch
			if node.IsLast {
				r.Cell(x, y, '└', fg, bg, terminal.AttrNone)
			} else {
				r.Cell(x, y, '├', fg, bg, terminal.AttrNone)
			}
			// Horizontal connector
			for i := 1; i < indentW; i++ {
				if x+i < r.W {
					r.Cell(x+i, y, '─', fg, bg, terminal.AttrNone)
				}
			}
		} else {
			// Ancestor level - show continuation or space
			if hasMore {
				r.Cell(x, y, '│', fg, bg, terminal.AttrNone)
			}
			// Fill rest with spaces (already cleared)
		}
		x += indentW
	}

	return x
}

// --- Navigation helpers ---

// FindParentIndex returns index of parent node for node at idx, or -1 if root
func FindParentIndex(nodes []TreeNode, idx int) int {
	if idx <= 0 || idx >= len(nodes) {
		return -1
	}
	targetDepth := nodes[idx].Depth - 1
	if targetDepth < 0 {
		return -1
	}
	for i := idx - 1; i >= 0; i-- {
		if nodes[i].Depth == targetDepth {
			return i
		}
	}
	return -1
}

// FindFirstChildIndex returns index of first child for node at idx, or -1 if none
func FindFirstChildIndex(nodes []TreeNode, idx int) int {
	if idx < 0 || idx >= len(nodes)-1 {
		return -1
	}
	if !nodes[idx].Expandable || !nodes[idx].Expanded {
		return -1
	}
	childDepth := nodes[idx].Depth + 1
	if idx+1 < len(nodes) && nodes[idx+1].Depth == childDepth {
		return idx + 1
	}
	return -1
}

// FindNextSiblingIndex returns index of next sibling at same depth, or -1
func FindNextSiblingIndex(nodes []TreeNode, idx int) int {
	if idx < 0 || idx >= len(nodes) {
		return -1
	}
	depth := nodes[idx].Depth
	for i := idx + 1; i < len(nodes); i++ {
		if nodes[i].Depth < depth {
			return -1 // Went up, no more siblings
		}
		if nodes[i].Depth == depth {
			return i
		}
	}
	return -1
}

// FindPrevSiblingIndex returns index of previous sibling at same depth, or -1
func FindPrevSiblingIndex(nodes []TreeNode, idx int) int {
	if idx <= 0 || idx >= len(nodes) {
		return -1
	}
	depth := nodes[idx].Depth
	for i := idx - 1; i >= 0; i-- {
		if nodes[i].Depth < depth {
			return -1 // Went up, no more siblings
		}
		if nodes[i].Depth == depth {
			return i
		}
	}
	return -1
}

