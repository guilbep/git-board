//go:build !windows

package gitstore

// openPacks is how many pack files a Store keeps open between reads. Each
// read would open its pack again otherwise, and a listing reads thousands
// of objects. Close lets them go.
const openPacks = 8
