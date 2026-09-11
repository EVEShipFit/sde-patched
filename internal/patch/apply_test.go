package patch

import (
	"strings"
	"testing"

	"github.com/EVEShipFit/sde-patched/internal/sde"
)

// Declarations live in package variables, so every test has to start clean.
func reset() {
	newAttributes = nil
	newEffects = nil
	lookups = nil
	changes = nil
	actions = nil
}

func testData() *sde.Data {
	return &sde.Data{
		Categories: map[int32]*sde.Category{
			6:  {Key: 6, Name: sde.Localized{En: "Ship"}},
			18: {Key: 18, Name: sde.Localized{En: "Drone"}},
		},
		Groups: map[int32]*sde.Group{
			25:  {Key: 25, Name: sde.Localized{En: "Frigate"}, CategoryID: 6},
			100: {Key: 100, Name: sde.Localized{En: "Combat Drone"}, CategoryID: 18},
		},
		Types: map[int32]*sde.Type{
			587: {
				Key: 587, Name: sde.Localized{En: "Rifter"}, GroupID: 25, CategoryID: 6, Published: true,
				DogmaAttributes: []sde.TypeDogmaAttribute{{AttributeID: 70, Value: 3.2}},
			},
			2456: {
				Key: 2456, Name: sde.Localized{En: "Hobgoblin II"}, GroupID: 100, CategoryID: 18, Published: true,
			},
			999: {
				Key: 999, Name: sde.Localized{En: "Unpublished Frigate"}, GroupID: 25, CategoryID: 6,
			},
		},
		DogmaAttributes: map[int32]*sde.DogmaAttribute{
			70:  {Key: 70, Name: "agility"},
			161: {Key: 161, Name: "volume"},
		},
		DogmaEffects: map[int32]*sde.DogmaEffect{
			16: {Key: 16, Name: "online", EffectCategoryID: 1},
		},
	}
}

func TestCreateAndApply(t *testing.T) {
	reset()

	speed := NewAttribute("alignTime", AttributeDef{DefaultValue: 1.5, Stackable: true})
	effect := NewEffect("alignTime", EffectDef{
		Category: Passive,
		Modifiers: []Modifier{
			{Domain: ItemID, Func: ItemModifier, Operation: PostMul, Modified: speed, Modifying: Attribute("agility")},
		},
	})
	action := ApplyEffect(effect).On(InCategory("Ship"), IsPublished())

	data := testData()
	ctx, err := Apply(data)
	if err != nil {
		t.Fatal(err)
	}

	if speed.ID() != -1 || effect.ID() != -1 {
		t.Errorf("new IDs = %d / %d, want -1 / -1", speed.ID(), effect.ID())
	}
	if got := data.DogmaAttributes[-1].Name; got != "alignTime" {
		t.Errorf("attribute name = %q", got)
	}
	if got := data.DogmaEffects[-1].Modifiers[0].ModifyingAttributeID; got != 70 {
		t.Errorf("modifying attribute = %d, want 70 (agility)", got)
	}

	// Only the published ship; not the drone, not the unpublished one.
	if got := action.matchedIDs(); len(got) != 1 || got[0] != 587 {
		t.Errorf("matched = %v, want [587]", got)
	}
	if got := data.Types[587].DogmaEffects; len(got) != 1 || got[0].EffectID != -1 {
		t.Errorf("effects on Rifter = %v", got)
	}
	if len(data.Types[2456].DogmaEffects) != 0 {
		t.Error("drone should not be touched")
	}
	if ctx.Data != data {
		t.Error("context lost the data")
	}
}

func TestSelectors(t *testing.T) {
	reset()

	effect := NewEffect("test", EffectDef{Category: Passive})
	byGroup := ApplyEffect(effect).On(InGroup("Frigate"), Not(IsPublished()))
	byName := ApplyEffect(NewEffect("test2", EffectDef{})).On(Any(Named("Rifter"), Named("Hobgoblin II")))
	byAttribute := ApplyEffect(NewEffect("test3", EffectDef{})).On(HasAttribute(Attribute("agility")))

	if _, err := Apply(testData()); err != nil {
		t.Fatal(err)
	}

	if got := byGroup.matchedIDs(); len(got) != 1 || got[0] != 999 {
		t.Errorf("InGroup+Not(IsPublished) = %v, want [999]", got)
	}
	if got := byName.matchedIDs(); len(got) != 2 {
		t.Errorf("Any(Named, Named) = %v, want 2 types", got)
	}
	if got := byAttribute.matchedIDs(); len(got) != 1 || got[0] != 587 {
		t.Errorf("HasAttribute = %v, want [587]", got)
	}
}

func TestChangeEffect(t *testing.T) {
	reset()

	ChangeEffect(Effect("online")).SetCategory(Online)

	data := testData()
	if _, err := Apply(data); err != nil {
		t.Fatal(err)
	}
	if got := data.DogmaEffects[16].EffectCategoryID; got != 4 {
		t.Errorf("online category = %d, want 4", got)
	}
}

func TestErrors(t *testing.T) {
	tests := []struct {
		name    string
		declare func()
		want    string
	}{
		{
			name:    "unknown attribute",
			declare: func() { NewEffect("x", EffectDef{Modifiers: []Modifier{{Modifying: Attribute("nope")}}}) },
			want:    `no dogma attribute named "nope"`,
		},
		{
			name:    "unknown category",
			declare: func() { ApplyEffect(NewEffect("x", EffectDef{})).On(InCategory("Nope")) },
			want:    `no category named "Nope"`,
		},
		{
			name:    "no selectors",
			declare: func() { ApplyEffect(NewEffect("x", EffectDef{})) },
			want:    "no selectors",
		},
		{
			name:    "no matches",
			declare: func() { ApplyEffect(NewEffect("x", EffectDef{})).On(InCategory("Ship"), InCategory("Drone")) },
			want:    "matches no types",
		},
		{
			name:    "duplicate name",
			declare: func() { NewAttribute("agility", AttributeDef{}) },
			want:    `dogma attribute "agility" already exists`,
		},
		{
			name: "effect applied twice",
			declare: func() {
				effect := NewEffect("x", EffectDef{})
				ApplyEffect(effect).On(InCategory("Ship"))
				ApplyEffect(effect).On(InCategory("Ship"))
			},
			want: "already has effect",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reset()
			test.declare()

			_, err := Apply(testData())
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("error = %q, want it to mention %q", err, test.want)
			}
			if !strings.Contains(err.Error(), "apply_test.go:") {
				t.Errorf("error = %q, want it to point at the declaring line", err)
			}
		})
	}
}
