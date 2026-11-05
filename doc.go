// Package gitboard is git-board's module root (docs/rewrite/design.md;
// ADR 0021). It rewrote v1 and kept the data: the formats of docs/spec are
// its contract, and v1's outputs on the golden boards (testdata/golden)
// are what it prints again (make compat).
//
// The layers, from the bottom: store (the datastores) and kernel (packs,
// entities, search); view (the index); conf; domain (issues, people,
// releases, the cycle); forge and sync; tools; app, which opens a board;
// surface (cli, http, tui) on app; cmd/git-board, the binary. Which
// package may import which is layers_test.go's table. Nothing imports
// contrib/, modules of their own that speak to git-board through its CLI
// and HTTP API.
package gitboard
