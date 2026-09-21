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
		MarketGroups: map[int32]*sde.MarketGroup{
			157: {Key: 157, Name: sde.Localized{En: "Drones"}},
			837: {Key: 837, Name: sde.Localized{En: "Light Scout Drones"}, ParentGroupID: 157},
		},
		MetaGroups: map[int32]*sde.MetaGroup{
			1: {Key: 1, Name: sde.Localized{En: "Tech I"}},
			2: {Key: 2, Name: sde.Localized{En: "Tech II"}},
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
				FighterAbilities: []sde.TypeFighterAbility{
					{Slot: 0, AbilityID: 22},
					{Slot: 2, AbilityID: 33, ChargeCount: 18, RearmTimeSeconds: 4},
				},
			},
			1: {Key: 1, Name: sde.Localized{En: "Nothing"}},
		},
		DogmaAttributes: map[int32]*sde.DogmaAttribute{
			9: {Key: 9, Name: "hp", DefaultValue: 0, HighIsGood: true, Stackable: true, UnitID: 113, CategoryID: 4},
		},
		DogmaUnits: map[int32]*sde.DogmaUnit{
			113: {Key: 113, Name: "Hitpoints", DisplayName: sde.Localized{En: "HP"}},
			122: {Key: 122, Name: "Fitting slots"},
		},
		DogmaCategories: map[int32]*sde.DogmaAttributeCategory{
			4: {Key: 4, Name: "Structure"},
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
		DbuffCollections: map[int32]*sde.DbuffCollection{
			12: {
				Key:           12,
				DisplayName:   sde.Localized{En: "Shield HP Bonus"},
				AggregateMode: eve.DbuffAggregateModeMaximum,
				Operation:     eve.ModifierOperationPostPercent,
				Display:       eve.DbuffDisplayInverted,
				Modifiers: []sde.DbuffModifier{
					{Func: eve.ModifierFuncItemModifier, ModifiedAttributeID: 263},
					{Func: eve.ModifierFuncLocationGroupModifier, ModifiedAttributeID: 54, GroupID: 208},
				},
			},
		},
		Mutaplasmids: map[int32]*sde.Mutaplasmid{
			60461: {
				Key:        60461,
				Attributes: []sde.MutaplasmidAttribute{{AttributeID: 9, Min: 0.7, Max: 1.15}},
				Mappings:   []sde.MutaplasmidMapping{{ApplicableTypes: []int32{2456, 2454}, ResultingType: 60478}},
			},
		},
	}
}

// lookupName searches the file the way a consumer would: lowercase, then a
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

	var ability eve.TypeFighterAbility
	if entry.FighterAbilitiesLength() != 2 || !entry.FighterAbilities(&ability, 1) {
		t.Fatalf("fighter abilities: %d", entry.FighterAbilitiesLength())
	}
	if ability.Slot() != 2 || ability.AbilityId() != 33 || ability.ChargeCount() != 18 || ability.RearmTimeSeconds() != 4 {
		t.Errorf("fighter ability = slot %d, ability %d, %d charges, %v rearm", ability.Slot(), ability.AbilityId(), ability.ChargeCount(), ability.RearmTimeSeconds())
	}

	var marketGroup eve.MarketGroup
	if !root.MarketGroupsByKey(&marketGroup, 837) {
		t.Fatal("market group 837 not found")
	}
	if got := string(marketGroup.Name()); got != "Light Scout Drones" {
		t.Errorf("market group name = %q", got)
	}
	if marketGroup.ParentGroupId() != 157 {
		t.Errorf("market group parent = %d, want 157", marketGroup.ParentGroupId())
	}

	var metaGroup eve.MetaGroup
	if !root.MetaGroupsByKey(&metaGroup, 2) {
		t.Fatal("meta group 2 not found")
	}
	if got := string(metaGroup.Name()); got != "Tech II" {
		t.Errorf("meta group name = %q", got)
	}

	var attribute eve.DogmaAttribute
	if !root.DogmaAttributesByKey(&attribute, 9) {
		t.Fatal("attribute 9 not found")
	}
	if attribute.UnitId() != 113 || attribute.CategoryId() != 4 {
		t.Errorf("attribute unit = %d, category = %d", attribute.UnitId(), attribute.CategoryId())
	}

	var unit eve.DogmaUnit
	if !root.DogmaUnitsByKey(&unit, 113) {
		t.Fatal("unit 113 not found")
	}
	if got := string(unit.Name()); got != "Hitpoints" {
		t.Errorf("unit name = %q", got)
	}
	if got := string(unit.DisplayName()); got != "HP" {
		t.Errorf("unit display name = %q", got)
	}

	if !root.DogmaUnitsByKey(&unit, 122) {
		t.Fatal("unit 122 not found")
	}
	if got := string(unit.DisplayName()); got != "" {
		t.Errorf("unit without a display name = %q", got)
	}

	var attributeCategory eve.DogmaAttributeCategory
	if !root.DogmaAttributeCategoriesByKey(&attributeCategory, 4) {
		t.Fatal("attribute category 4 not found")
	}
	if got := string(attributeCategory.Name()); got != "Structure" {
		t.Errorf("attribute category name = %q", got)
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

	var buff eve.DbuffCollection
	if !root.DbuffCollectionsByKey(&buff, 12) {
		t.Fatal("buff 12 not found")
	}
	if got := string(buff.DisplayName()); got != "Shield HP Bonus" {
		t.Errorf("buff display name = %q", got)
	}
	if buff.AggregateMode() != eve.DbuffAggregateModeMaximum || buff.Operation() != eve.ModifierOperationPostPercent {
		t.Errorf("buff = %v / %v", buff.AggregateMode(), buff.Operation())
	}
	if buff.Display() != eve.DbuffDisplayInverted {
		t.Errorf("buff display = %v", buff.Display())
	}

	var buffModifier eve.DbuffModifier
	if buff.ModifiersLength() != 2 || !buff.Modifiers(&buffModifier, 1) {
		t.Fatalf("buff modifiers: %d", buff.ModifiersLength())
	}
	if buffModifier.Func() != eve.ModifierFuncLocationGroupModifier || buffModifier.ModifiedAttributeId() != 54 || buffModifier.GroupId() != 208 {
		t.Errorf("buff modifier = %v, attribute %d, group %d", buffModifier.Func(), buffModifier.ModifiedAttributeId(), buffModifier.GroupId())
	}

	var mutaplasmid eve.Mutaplasmid
	if !root.MutaplasmidsByKey(&mutaplasmid, 60461) {
		t.Fatal("mutaplasmid 60461 not found")
	}

	var roll eve.MutaplasmidAttribute
	if mutaplasmid.AttributesLength() != 1 || !mutaplasmid.Attributes(&roll, 0) {
		t.Fatalf("mutaplasmid attributes: %d", mutaplasmid.AttributesLength())
	}
	if roll.AttributeId() != 9 || roll.Min() != 0.7 || roll.Max() != 1.15 {
		t.Errorf("mutaplasmid attribute = %d, %v to %v", roll.AttributeId(), roll.Min(), roll.Max())
	}

	var mapping eve.MutaplasmidMapping
	if mutaplasmid.MappingsLength() != 1 || !mutaplasmid.Mappings(&mapping, 0) {
		t.Fatalf("mutaplasmid mappings: %d", mutaplasmid.MappingsLength())
	}
	if mapping.ResultingTypeId() != 60478 {
		t.Errorf("resulting type = %d, want 60478", mapping.ResultingTypeId())
	}
	if mapping.ApplicableTypeIdsLength() != 2 || mapping.ApplicableTypeIds(0) != 2454 || mapping.ApplicableTypeIds(1) != 2456 {
		t.Errorf("applicable types are not the sorted input")
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
