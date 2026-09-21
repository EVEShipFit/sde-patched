package sde

// Clone makes a copy deep enough to patch. Patching adds attributes and
// effects, appends to the dogma of a type, and edits attributes and effects in
// place; everything it can reach is copied, and the rest is shared.
//
// It exists so a long-running process can apply the patches again without
// reading the SDE from disk a second time.
func (d *Data) Clone() *Data {
	clone := &Data{
		BuildNumber:      d.BuildNumber,
		Types:            make(map[int32]*Type, len(d.Types)),
		Groups:           d.Groups,
		Categories:       d.Categories,
		MarketGroups:     d.MarketGroups,
		MetaGroups:       d.MetaGroups,
		DogmaAttributes:  make(map[int32]*DogmaAttribute, len(d.DogmaAttributes)),
		DogmaEffects:     make(map[int32]*DogmaEffect, len(d.DogmaEffects)),
		DbuffCollections: d.DbuffCollections,
		Mutaplasmids:     d.Mutaplasmids,
	}

	for id, entry := range d.Types {
		copied := *entry
		copied.DogmaAttributes = clip(entry.DogmaAttributes)
		copied.DogmaEffects = clip(entry.DogmaEffects)
		clone.Types[id] = &copied
	}
	for id, entry := range d.DogmaAttributes {
		copied := *entry
		clone.DogmaAttributes[id] = &copied
	}
	for id, entry := range d.DogmaEffects {
		copied := *entry
		copied.Modifiers = clip(entry.Modifiers)
		clone.DogmaEffects[id] = &copied
	}

	return clone
}

// clip copies a slice with no spare capacity, so that appending to the copy
// can never write into the original.
func clip[T any](values []T) []T {
	if values == nil {
		return nil
	}
	return append(make([]T, 0, len(values)), values...)
}
