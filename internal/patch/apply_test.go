package patch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/EVEShipFit/sde-patched/internal/sde"
)

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
		DogmaUnits:      map[int32]*sde.DogmaUnit{113: {Key: 113, Name: "Hitpoints"}},
		DogmaCategories: map[int32]*sde.DogmaAttributeCategory{3: {Key: 3, Name: "Armor"}},
	}
}

func TestUnitsAndCategories(t *testing.T) {
	spec := load(t, tree{
		"units":     "units: [{name: hitpointsPerSecond, displayName: HP/s}]\n",
		"repairing": "new: {displayName: Repairing, category: Armor, unit: hitpointsPerSecond, highIsGood: true}\n",
		"armorHP":   "new: {displayName: Armor HP, category: Armor, unit: Hitpoints, highIsGood: true}\n",
	})
	if err := spec.Validate(); err != nil {
		t.Fatal(err)
	}

	data := testData()
	ctx, err := Apply(spec, data)
	if err != nil {
		t.Fatal(err)
	}

	unit := data.DogmaUnits[spec.IDs.Units["hitpointsPerSecond"]]
	if unit == nil || unit.DisplayName.En != "HP/s" {
		t.Fatalf("the unit we added reads %+v", unit)
	}

	ours := data.DogmaAttributes[ctx.AttributeID("repairing")]
	if ours.UnitID != unit.Key || ours.CategoryID != 3 {
		t.Errorf("repairing has unit %d and category %d", ours.UnitID, ours.CategoryID)
	}
	if theirs := data.DogmaAttributes[ctx.AttributeID("armorHP")]; theirs.UnitID != 113 {
		t.Errorf("an attribute naming one of CCP's units has unit %d", theirs.UnitID)
	}
}

// tree is a patches directory: one file per attribute, plus the selectors
// and effects files when a test needs them.
type tree map[string]string

// load writes the tree to disk and reads it back, handing out IDs the way the
// editor would so that a test never has to write one down.
func load(t *testing.T, files tree) *Spec {
	t.Helper()

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, AttributesDir), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, text := range files {
		path := filepath.Join(dir, AttributesDir, name+".yaml")
		if name == "selectors" || name == "effects" || name == "units" {
			path = filepath.Join(dir, name+".yaml")
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	spec, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	spec.IDs.Record(spec)
	return spec
}

func TestCreateAndApply(t *testing.T) {
	spec := load(t, tree{"alignTime": `
new:
  default: 1.5

effects:
  - on: category("Ship") and published()
    category: passive
    rules:
      - {mul: agility}
`})

	data := testData()
	ctx, err := Apply(spec, data)
	if err != nil {
		t.Fatal(err)
	}

	if got := data.DogmaAttributes[-1].Name; got != "alignTime" {
		t.Errorf("attribute -1 = %q, want alignTime", got)
	}
	if got := data.DogmaEffects[-1].Modifiers[0].ModifyingAttributeID; got != 70 {
		t.Errorf("modifying attribute = %d, want 70 (agility)", got)
	}
	// A rule writes the attribute whose file it is in, and nothing else.
	if got := data.DogmaEffects[-1].Modifiers[0].ModifiedAttributeID; got != -1 {
		t.Errorf("modified attribute = %d, want -1 (alignTime)", got)
	}

	// Only the published ship; not the drone, not the unpublished one.
	if got := ctx.Applied["alignTime"]; len(got) != 1 || got[0] != 587 {
		t.Errorf("applied to = %v, want [587]", got)
	}
	if got := data.Types[587].DogmaEffects; len(got) != 1 || got[0].EffectID != -1 {
		t.Errorf("effects on Rifter = %v", got)
	}
	if len(data.Types[2456].DogmaEffects) != 0 {
		t.Error("drone should not be touched")
	}
}

// An attribute can name another as its cap, including one the patches add.
func TestClamp(t *testing.T) {
	spec := load(t, tree{
		"maxTargets": `
new:
  max: maxTargetsCharacter

effects:
  - on: category("Ship")
    category: passive
    rules:
      - {from: agility}
`,
		"maxTargetsCharacter": "new: {default: 1000000}\n",
		"volume":              "change: {min: agility}\n",
	})

	data := testData()
	if _, err := Apply(spec, data); err != nil {
		t.Fatal(err)
	}

	capID, _ := spec.IDs.Attribute("maxTargetsCharacter")
	if got := data.DogmaAttributes[-1].MaxAttributeID; got != capID {
		t.Errorf("cap of maxTargets = %d, want %d", got, capID)
	}
	// One of CCP's can be capped too, against one of CCP's.
	if got := data.DogmaAttributes[161].MinAttributeID; got != 70 {
		t.Errorf("floor of volume = %d, want 70 (agility)", got)
	}
}

// A cap that names nothing is a mistake worth reporting.
func TestClampAgainstUnknown(t *testing.T) {
	spec := load(t, tree{"maxTargets": "new: {max: nosuchattribute}\n"})

	_, err := Apply(spec, testData())
	if err == nil || !strings.Contains(err.Error(), "nosuchattribute") {
		t.Errorf("error = %v, want one naming nosuchattribute", err)
	}
}

// An ID written down once is the ID that name keeps, whatever is added later.
func TestIDsNeverMove(t *testing.T) {
	files := tree{
		"first":  "new: {}\n",
		"second": "new: {}\n",
	}
	spec := load(t, files)
	first, _ := spec.IDs.Attribute("first")
	second, _ := spec.IDs.Attribute("second")
	if first != -1 || second != -2 {
		t.Fatalf("IDs = %d and %d, want -1 and -2", first, second)
	}

	if err := spec.IDs.Save(); err != nil {
		t.Fatal(err)
	}

	// A third attribute sorting before both of them must not move either.
	again := load(t, tree{"aardvark": "new: {}\n", "first": "new: {}\n", "second": "new: {}\n"})
	again.IDs.Attributes["first"] = first
	again.IDs.Attributes["second"] = second
	if got, _ := again.IDs.Attribute("first"); got != -1 {
		t.Errorf("first = %d, want -1", got)
	}
}

// An attribute of CCP's is only filled in, and says so by having no "new".
func TestWritingIntoOneOfCCPs(t *testing.T) {
	spec := load(t, tree{"agility": `
effects:
  - on: category("Ship")
    category: passive
    rules:
      - {mul: agility}
`})

	data := testData()
	if _, err := Apply(spec, data); err != nil {
		t.Fatal(err)
	}
	if got := data.DogmaEffects[-1].Modifiers[0].ModifiedAttributeID; got != 70 {
		t.Errorf("modified attribute = %d, want 70 (CCP's agility)", got)
	}
}

func TestSelectors(t *testing.T) {
	spec := load(t, tree{
		"selectors": `
selectors:
  - {name: isFrigate, match: group("Frigate")}
  - {name: isHidden, match: isFrigate and not published()}
`,
		"a": "new: {}\neffects: [{on: isHidden, category: passive, rules: [{mul: agility}]}]\n",
		"b": `new: {}
effects: [{on: name("Rifter") or name("Hobgoblin II"), category: passive, rules: [{mul: agility}]}]
`,
		"c": `new: {}
effects: [{on: attribute("agility"), category: passive, rules: [{mul: agility}]}]
`,
	})

	ctx, err := Apply(spec, testData())
	if err != nil {
		t.Fatal(err)
	}

	want := map[string][]int32{"a": {999}, "b": {587, 2456}, "c": {587}}
	for name, expected := range want {
		got := ctx.Applied[name]
		if len(got) != len(expected) {
			t.Errorf("%s applied to %v, want %v", name, got, expected)
			continue
		}
		for j := range got {
			if got[j] != expected[j] {
				t.Errorf("%s applied to %v, want %v", name, got, expected)
				break
			}
		}
	}
}

// Changing what one of CCP's attributes is, and adding rules to one of its
// effects, both belong in the attribute's own file.
func TestChange(t *testing.T) {
	spec := load(t, tree{
		"effects": "changes:\n  - {effect: online, category: online}\n",
		"agility": `
change:
  default: 2
  highIsGood: true

addTo:
  - effect: online
    rules:
      - {mul: agility}
`,
	})

	data := testData()
	if _, err := Apply(spec, data); err != nil {
		t.Fatal(err)
	}

	if got := data.DogmaEffects[16].EffectCategoryID; got != 4 {
		t.Errorf("online category = %d, want 4", got)
	}
	if got := data.DogmaAttributes[70]; got.DefaultValue != 2 || !got.HighIsGood {
		t.Errorf("agility = %+v, want default 2 and highIsGood", got)
	}
	if got := data.DogmaEffects[16].Modifiers; len(got) != 1 || got[0].ModifiedAttributeID != 70 {
		t.Errorf("rules on online = %v, want one writing agility", got)
	}
}

func TestErrors(t *testing.T) {
	tests := []struct {
		name  string
		files tree
		want  string
	}{
		{
			name:  "unknown attribute",
			files: tree{"x": "new: {}\neffects: [{on: published(), category: passive, rules: [{mul: nope}]}]\n"},
			want:  `no dogma attribute named "nope"`,
		},
		{
			name:  "unknown category",
			files: tree{"x": `new: {}` + "\n" + `effects: [{on: category("Nope"), category: passive, rules: [{mul: agility}]}]` + "\n"},
			want:  `no category named "Nope"`,
		},
		{
			name:  "unknown effect category",
			files: tree{"x": "new: {}\neffects: [{on: published(), category: sideways, rules: [{mul: agility}]}]\n"},
			want:  `unknown effect category "sideways"`,
		},
		{
			name:  "unknown selector",
			files: tree{"x": "new: {}\neffects: [{on: whatIsThis, category: passive, rules: [{mul: agility}]}]\n"},
			want:  `no selector named "whatIsThis"`,
		},
		{
			name: "selector loop",
			files: tree{
				"selectors": "selectors: [{name: a, match: b}, {name: b, match: a}]\n",
				"x":         "new: {}\neffects: [{on: a, category: passive, rules: [{mul: agility}]}]\n",
			},
			want: "refers back to itself",
		},
		{
			name:  "no matches",
			files: tree{"x": `new: {}` + "\n" + `effects: [{on: category("Ship") and category("Drone"), category: passive, rules: [{mul: agility}]}]` + "\n"},
			want:  "matches no types",
		},
		{
			name:  "a new attribute CCP already has",
			files: tree{"agility": "new: {}\n"},
			want:  `dogma attribute "agility" already exists`,
		},
		{
			name:  "an attribute nobody has",
			files: tree{"nothingLikeThis": "effects: [{on: published(), category: passive, rules: [{mul: agility}]}]\n"},
			want:  `no dogma attribute named "nothingLikeThis"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Apply(load(t, test.files), testData())
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("error = %q, want it to mention %q", err, test.want)
			}
			if !strings.Contains(err.Error(), "patches/") {
				t.Errorf("error = %q, want it to point at the declaring line", err)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name  string
		files tree
		want  string
	}{
		{
			name:  "two verbs",
			files: tree{"effects": `actions: [{removeEffect: a, setAttribute: b, on: published()}]` + "\n"},
			want:  "exactly one of removeEffect",
		},
		{
			name:  "no verb",
			files: tree{"effects": "actions: [{on: published()}]\n"},
			want:  "exactly one of removeEffect",
		},
		{
			name:  "both new and change",
			files: tree{"x": "new: {}\nchange: {default: 1}\n"},
			want:  "it is one or the other",
		},
		{
			name:  "two effects, one name",
			files: tree{"x": "effects:\n  - {on: published(), category: passive, rules: [{mul: agility}]}\n  - {on: published(), category: passive, rules: [{div: agility}]}\n"},
			want:  "needs a name of its own",
		},
		{
			name:  "no category",
			files: tree{"x": "effects: [{on: published(), rules: [{mul: agility}]}]\n"},
			want:  "needs a category",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := load(t, test.files).Validate()
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("error = %q, want it to mention %q", err, test.want)
			}
		})
	}
}

// An unknown key is almost always a typo, so loading has to fail.
func TestUnknownKey(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, AttributesDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, AttributesDir, "a.yaml"), []byte("new: {wobble: 3}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(dir)
	if err == nil || !strings.Contains(err.Error(), "wobble") {
		t.Errorf("error = %v, want it to mention the unknown key", err)
	}
}
