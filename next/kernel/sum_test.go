package kernel_test

import (
	"fmt"
	"testing"

	"github.com/guilbep/git-board/next/kernel"
)

// A listing's sum is its heads' sums added: the order doesn't count, and
// one head changed moves it by its own.
func TestSumMovesByAHead(t *testing.T) {
	t.Parallel()
	var heads []kernel.Head
	for i := range 300 {
		heads = append(heads, kernel.Head{ID: kernel.ID(fmt.Sprintf("%040x", i)), Version: kernel.Version(fmt.Sprintf("%040x", i*7))})
	}
	sum := kernel.Sum(heads)
	reversed := make([]kernel.Head, len(heads))
	for i, h := range heads {
		reversed[len(heads)-1-i] = h
	}
	if kernel.Sum(reversed) != sum {
		t.Fatal("the order of the heads moves the sum")
	}
	moved := kernel.Head{ID: heads[42].ID, Version: "ffff"}
	by := kernel.AddSum(kernel.SubSum(sum, kernel.HeadSum(heads[42])), kernel.HeadSum(moved))
	heads[42] = moved
	if kernel.Sum(heads) != by {
		t.Fatal("a head changed doesn't move the sum by its own")
	}
	if kernel.Sum(heads[:299]) != kernel.SubSum(by, kernel.HeadSum(heads[299])) {
		t.Fatal("a head dropped doesn't move the sum by its own")
	}
	var zero [32]byte
	if kernel.Sum(nil) != zero || kernel.SubSum(kernel.AddSum(zero, sum), sum) != zero {
		t.Fatal("SubSum doesn't undo AddSum")
	}
}
