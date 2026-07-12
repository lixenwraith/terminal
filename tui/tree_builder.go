package tui

// TreeBuilder helps construct flattened visible node list from hierarchical data
type TreeBuilder struct {
	nodes     []TreeNode
	expansion *TreeExpansion
}

// NewTreeBuilder creates a builder with expansion state
func NewTreeBuilder(expansion *TreeExpansion) *TreeBuilder {
	return &TreeBuilder{
		expansion: expansion,
	}
}

// Reset clears accumulated nodes
func (b *TreeBuilder) Reset() {
	b.nodes = b.nodes[:0]
}

// Add adds a node if visible (parent expanded)
// parentExpanded should be true for root-level nodes
func (b *TreeBuilder) Add(node TreeNode, parentExpanded bool) {
	if !parentExpanded {
		return
	}
	node.Expanded = b.expansion.IsExpanded(node.Key)
	b.nodes = append(b.nodes, node)
}

// Nodes returns accumulated visible nodes
func (b *TreeBuilder) Nodes() []TreeNode {
	return b.nodes
}

// MarkLastSiblings sets IsLast flag on nodes that are last at their depth
// Call after all nodes added, before rendering
func (b *TreeBuilder) MarkLastSiblings() {
	// Backward scan: seen[d] tracks whether a sibling at depth d was encountered
	// within the current subtree. Crossing depth d invalidates deeper entries.
	seen := make([]bool, 0, 8)
	for i := len(b.nodes) - 1; i >= 0; i-- {
		d := b.nodes[i].Depth
		for len(seen) <= d {
			seen = append(seen, false)
		}
		seen = seen[:d+1] // deeper entries belong to a later subtree
		b.nodes[i].IsLast = !seen[d]
		seen[d] = true
	}
}

