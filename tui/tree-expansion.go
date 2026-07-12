package tui

// TreeExpansion manages expand/collapse state
type TreeExpansion struct {
	State map[string]bool
}

// NewTreeExpansion creates initialized expansion state
func NewTreeExpansion() *TreeExpansion {
	return &TreeExpansion{
		State: make(map[string]bool),
	}
}

// --- State queries ---

// IsExpanded returns expansion state for key
func (e *TreeExpansion) IsExpanded(key string) bool {
	return e.State[key]
}

// --- State modification ---

// SetExpanded sets expansion state for key
func (e *TreeExpansion) SetExpanded(key string, expanded bool) {
	e.State[key] = expanded
}

// Toggle toggles expansion state for key
func (e *TreeExpansion) Toggle(key string) bool {
	e.State[key] = !e.State[key]
	return e.State[key]
}

// Expand sets key to expanded
func (e *TreeExpansion) Expand(key string) {
	e.State[key] = true
}

// Collapse sets key to collapsed
func (e *TreeExpansion) Collapse(key string) {
	e.State[key] = false
}

// --- Bulk operations ---

// ExpandAll expands all provided keys
func (e *TreeExpansion) ExpandAll(keys []string) {
	for _, k := range keys {
		e.State[k] = true
	}
}

// CollapseAll collapses all keys
func (e *TreeExpansion) CollapseAll() {
	for k := range e.State {
		e.State[k] = false
	}
}

// Clear removes all expansion state
func (e *TreeExpansion) Clear() {
	e.State = make(map[string]bool)
}