// Package terminal draws a grid of cells and reads keys, the pointer and resizes:
// direct ANSI on a terminal, with cell-level diffing and no allocation a frame,
// and the same Terminal over a host that draws the cells itself, as a browser
// page does on js/wasm. README.md holds the rest.
package terminal
