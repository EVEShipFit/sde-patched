package sde

import "testing"

func TestClone(t *testing.T) {
	original := &Data{
		BuildNumber: 7,
		Types: map[int32]*Type{
			1: {
				Key:             1,
				Name:            Localized{En: "Rifter"},
				DogmaAttributes: []TypeDogmaAttribute{{AttributeID: 70, Value: 3.2}},
				DogmaEffects:    []TypeDogmaEffect{{EffectID: 16}},
			},
		},
		Groups:          map[int32]*Group{2: {Key: 2}},
		Categories:      map[int32]*Category{3: {Key: 3}},
		DogmaAttributes: map[int32]*DogmaAttribute{70: {Key: 70, Name: "agility", DefaultValue: 1}},
		DogmaEffects:    map[int32]*DogmaEffect{16: {Key: 16, Name: "online", Modifiers: []Modifier{{GroupID: 5}}}},
	}

	clone := original.Clone()

	// Everything a patch can do, done to the clone.
	clone.Types[1].DogmaEffects = append(clone.Types[1].DogmaEffects, TypeDogmaEffect{EffectID: -1})
	clone.Types[1].DogmaAttributes[0].Value = 99
	clone.DogmaAttributes[70].DefaultValue = 99
	clone.DogmaEffects[16].Modifiers = append(clone.DogmaEffects[16].Modifiers, Modifier{GroupID: 6})
	clone.DogmaEffects[16].EffectCategoryID = 4
	clone.DogmaAttributes[-1] = &DogmaAttribute{Key: -1}

	if got := len(original.Types[1].DogmaEffects); got != 1 {
		t.Errorf("original type effects = %d, want 1", got)
	}
	if got := original.Types[1].DogmaAttributes[0].Value; got != 3.2 {
		t.Errorf("original type attribute = %v, want 3.2", got)
	}
	if got := original.DogmaAttributes[70].DefaultValue; got != 1 {
		t.Errorf("original attribute default = %v, want 1", got)
	}
	if got := len(original.DogmaEffects[16].Modifiers); got != 1 {
		t.Errorf("original effect modifiers = %d, want 1", got)
	}
	if got := original.DogmaEffects[16].EffectCategoryID; got != 0 {
		t.Errorf("original effect category = %d, want 0", got)
	}
	if _, added := original.DogmaAttributes[-1]; added {
		t.Error("the new attribute leaked into the original")
	}
	if clone.BuildNumber != 7 {
		t.Errorf("build number = %d, want 7", clone.BuildNumber)
	}
}
