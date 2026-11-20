// Package terminal draws a grid of cells and reads keys, the pointer and resizes:
// direct ANSI on a terminal, with cell-level diffing and no allocation a frame,
// and the same Terminal over a host that draws the cells itself: a browser page,
// which New opens on js/wasm, a desktop window or an Android view (NewWithHost).
// README.md holds the rest.
package terminal
