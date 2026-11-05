package gitstore

// openPacks is none on Windows: a file open there can't be deleted, and a
// server keeping its packs open would fail git gc's. Each read opens its
// pack again.
const openPacks = 0
