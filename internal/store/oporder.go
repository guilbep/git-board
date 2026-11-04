package store

// OpOrder is the place of each of an issue's operations in the order its
// log applies them, by operation id (an Event's or a Version's ID): finer
// than their times, which are to the second.
func (b *Backend) OpOrder(idPrefix string) (map[string]int, error) {
	_, l, err := b.issue(idPrefix)
	if err != nil {
		return nil, err
	}
	res := make(map[string]int, len(l.Entries))
	for i, e := range l.Entries {
		res[e.ID] = i
	}
	return res, nil
}
