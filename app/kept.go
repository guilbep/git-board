package app

import "sync"

// kept is a value kept while what it was made from holds, its stamp (a
// file's size and time, the refs' folders' times): what a server's
// requests read again and again, read once a change. A stamp that can't
// be told ("") keeps nothing. The board's: forges found, remotes, the
// code's reads; the issues' list is issues.KeptList, the Work page's rows
// workKept (kept by issue). A cache that isn't one of these says why.
type kept[T any] struct {
	mu sync.Mutex
	at string
	v  T
}

// get is the value kept at stamp, else build's, kept.
func (k *kept[T]) get(stamp string, build func() (T, error)) (T, error) {
	k.mu.Lock()
	if stamp != "" && k.at == stamp {
		v := k.v
		k.mu.Unlock()
		return v, nil
	}
	k.mu.Unlock()
	v, err := build()
	if err == nil && stamp != "" {
		k.mu.Lock()
		k.at, k.v = stamp, v
		k.mu.Unlock()
	}
	return v, err
}
