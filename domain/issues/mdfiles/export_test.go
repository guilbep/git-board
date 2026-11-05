package mdfiles

// Parsed is how many files f read the bytes of, to read them in.
func Parsed(f *Folder) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.parsed
}
