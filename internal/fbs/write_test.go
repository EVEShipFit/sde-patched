package fbs

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/EVEShipFit/sde-patched/internal/fbs/eve"
	"github.com/EVEShipFit/sde-patched/internal/sde"
)

func ptr[T any](value T) *T { return &value }

func testData() *sde.Data {
	return &sde.Data{
		BuildNumber: 42,
		Categories: map[int32]*sde.Category{
			18: {Key: 18, Name: sde.Localized{En: "Drone"}, Published: true},
		},
		Groups: map[int32]*sde.Group{
			100: {Key: 100, Name: sde.Localized{En: "Combat Drone"}, CategoryID: 18, Published: true},
		},
		Types: map[int32]*sde.Type{
			2456: {
				Key:        2456,
				Name:       sde.Localized{En: "Hobgoblin II", De: "Hobgoblin II", Ja: "ホブゴブリンII"},
				GroupID:    100,
				CategoryID: 18,
				Published:  true,
				Mass:       ptr(4000.0),
				DogmaAttributes: []sde.TypeDogmaAttribute{
					{AttributeID: 9, Value: 168},
				},
				DogmaEffects: []sde.TypeDogmaEffect{
					{EffectID: -5, IsDefault: false},
				},
			},
			1: {Key: 1, Name: sde.Localized{En: "Nothing"}},
		},
		DogmaAttributes: map[int32]*sde.DogmaAttribute{
			9: {Key: 9, Name: "hp", DefaultValue: 0, HighIsGood: true, Stackable: true},
		},
		DogmaEffects: map[int32]*sde.DogmaEffect{
			-5: {
				Key:              -5,
				Name:             "droneLoad",
				EffectCategoryID: int32(eve.EffectCategoryPassive),
				IsWarpSafe:       true,
				Modifiers: []sde.Modifier{{
					Domain:               eve.ModifierDomainShipID,
					Func:                 eve.ModifierFuncItemModifier,
					Operation:            eve.ModifierOperationModAdd,
					ModifiedAttributeID:  -9,
					ModifyingAttributeID: 161,
				}},
			},
		},
	}
}

// lookupName is what a consumer of the file has to write: lowercase, then a
// binary search.
func lookupName(root *eve.Names, name string) int32 {
	wanted := strings.ToLower(name)

	index := sort.Search(root.NamesLength(), func(i int) bool { return string(root.Names(i)) >= wanted })
	if index >= root.NamesLength() || string(root.Names(index)) != wanted {
		return 0
	}
	return root.TypeIds(index)
}

func TestWriteRoundTrip(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "sde.dat")
	if err := Write(testData(), filename); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if !eve.SdeBufferHasIdentifier(raw) {
		t.Fatal("file identifier missing")
	}

	root := eve.GetRootAsSde(raw, 0)
	if root.BuildNumber() != 42 {
		t.Errorf("build number = %d, want 42", root.BuildNumber())
	}

	var entry eve.Type
	if !root.TypesByKey(&entry, 2456) {
		t.Fatal("type 2456 not found")
	}
	if got := string(entry.Name()); got != "Hobgoblin II" {
		t.Errorf("name = %q", got)
	}
	if got := entry.Mass(); got == nil || *got != 4000 {
		t.Errorf("mass = %v, want 4000", got)
	}
	if entry.Volume() != nil {
		t.Error("volume should be absent, not zero")
	}
	if got := entry.CategoryId(); got != 18 {
		t.Errorf("categoryID = %d, want 18", got)
	}

	if entry.DogmaAttributesLength() != 1 || entry.DogmaEffectsLength() != 1 {
		t.Fatalf("dogma: %d attributes, %d effects", entry.DogmaAttributesLength(), entry.DogmaEffectsLength())
	}

	var effect eve.DogmaEffect
	if !root.DogmaEffectsByKey(&effect, -5) {
		t.Fatal("effect -5 not found")
	}
	if got := string(effect.Name()); got != "droneLoad" {
		t.Errorf("effect name = %q", got)
	}
	if got := effect.EffectCategory(); got != eve.EffectCategoryPassive {
		t.Errorf("effect category = %v", got)
	}

	var modifier eve.Modifier
	if !effect.Modifiers(&modifier, 0) {
		t.Fatal("modifier missing")
	}
	if modifier.Domain() != eve.ModifierDomainShipID || modifier.Operation() != eve.ModifierOperationModAdd {
		t.Errorf("modifier = %v / %v", modifier.Domain(), modifier.Operation())
	}
	if modifier.ModifyingAttributeId() != 161 {
		t.Errorf("modifyingAttributeID = %d", modifier.ModifyingAttributeId())
	}
}

func TestWriteNamesRoundTrip(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "names.dat")
	if err := WriteNames(testData(), filename); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if !eve.NamesBufferHasIdentifier(raw) {
		t.Fatal("file identifier missing")
	}

	root := eve.GetRootAsNames(raw, 0)
	if root.BuildNumber() != 42 {
		t.Errorf("build number = %d, want 42", root.BuildNumber())
	}

	// The Japanese name has to lead back to the type, same as the English one.
	if got := lookupName(root, "ホブゴブリンII"); got != 2456 {
		t.Errorf("lookup of japanese name = %d, want 2456", got)
	}
	if got := lookupName(root, "Hobgoblin II"); got != 2456 {
		t.Errorf("lookup of english name = %d, want 2456", got)
	}
	if got := lookupName(root, "hObGoBlIn ii"); got != 2456 {
		t.Errorf("lookup ignoring case = %d, want 2456", got)
	}
	if got := lookupName(root, "Nothing at all"); got != 0 {
		t.Errorf("lookup of unknown name = %d, want 0", got)
	}

	// German repeats the English name; it must not show up twice.
	if got := root.NamesLength(); got != 3 {
		t.Errorf("names length = %d, want 3", got)
	}
	for i := 1; i < root.NamesLength(); i++ {
		if string(root.Names(i-1)) > string(root.Names(i)) {
			t.Fatalf("names are not sorted at %d", i)
		}
	}
}
