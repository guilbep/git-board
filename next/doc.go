// Package next is git-board v2, growing beside v1 until it replaces it
// (docs/rewrite/design.md). It rewrites the code and keeps the data: the
// formats of docs/spec are its contract, and v1's outputs on the golden
// boards (next/testdata/golden) are what it must print again (make compat).
//
// Nothing in next/ imports core/, internal/ or cmd/, and nothing outside
// imports next/ (guard_test.go): deleting v1 is one git rm. The one
// exception is next/golden/v1gen, which writes the golden boards with v1's
// code, as goldens made by anything else would prove nothing; it goes
// with v1. Code of v1 that was right is copied in and owned, not imported.
package next
