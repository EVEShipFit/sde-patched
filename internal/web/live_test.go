package web

import (
	"net/http"
	"testing"

	"github.com/EVEShipFit/sde-patched/internal/sde"
)

// The attribute under test divides one value by another, which is what most
// of the real ones do, and is the shape that goes wrong when the second one
// is missing.
const ehpPatch = `
new:
  displayName: Armor EHP
  category: Armor
  highIsGood: true

effects:
  - on: isShip
    category: passive
    rules:
      - {from: armorHP}
      - {div: armorEmDamageResonance}
      - {add: armorEhp, domain: shipID}
`

const shipSelector = `
selectors:
  - {name: isShip, match: category("Ship")}
`

// The Punisher carries no resonance, so it divides by a default of zero.
func liveData() *sde.Data {
	data := &sde.Data{
		BuildNumber: 42,
		Categories:  map[int32]*sde.Category{6: {Key: 6, Name: sde.Localized{En: "Ship"}, Published: true}},
		Groups:      map[int32]*sde.Group{25: {Key: 25, Name: sde.Localized{En: "Frigate"}, CategoryID: 6, Published: true}},
		Types: map[int32]*sde.Type{
			587: {Key: 587, Name: sde.Localized{En: "Rifter"}, GroupID: 25, CategoryID: 6, Published: true,
				DogmaAttributes: []sde.TypeDogmaAttribute{{AttributeID: 265, Value: 450}, {AttributeID: 267, Value: 0.5}}},
			588: {Key: 588, Name: sde.Localized{En: "Punisher"}, GroupID: 25, CategoryID: 6, Published: true,
				DogmaAttributes: []sde.TypeDogmaAttribute{{AttributeID: 265, Value: 600}}},
		},
		DogmaAttributes: map[int32]*sde.DogmaAttribute{
			265: {Key: 265, Name: "armorHP", DisplayName: sde.Localized{En: "Armor hitpoints"}, HighIsGood: true, Stackable: true, UnitID: 9},
			267: {Key: 267, Name: "armorEmDamageResonance", Stackable: true},
		},
		DogmaCategories: map[int32]*sde.DogmaAttributeCategory{3: {Key: 3, Name: "Armor"}},
		DogmaEffects:    map[int32]*sde.DogmaEffect{},
	}
	return data
}

func newLiveServer(t *testing.T) (http.Handler, string) {
	t.Helper()

	dir := writePatches(t, map[string]string{"armorEhp": ehpPatch, "selectors": shipSelector})
	return New(liveData(), dir).Handler(), dir
}

func find(t *testing.T, values []any, name string) map[string]any {
	t.Helper()

	for _, entry := range values {
		if entry.(map[string]any)["name"] == name {
			return entry.(map[string]any)
		}
	}
	t.Fatalf("no value named %q", name)
	return nil
}

func TestLive(t *testing.T) {
	handler, _ := newLiveServer(t)

	status, body := get(t, handler, "/api/live/587")
	if status != http.StatusOK {
		t.Fatalf("status %d: %v", status, body)
	}
	if body["name"] != "Rifter" {
		t.Errorf("name = %v", body["name"])
	}

	values := body["values"].([]any)
	ehp := find(t, values, "armorEhp")
	if ehp["value"].(float64) != 900 {
		t.Errorf("armorEhp = %v, want 900", ehp["value"])
	}
	if ehp["origin"] != "computed" || ehp["added"] != nil {
		t.Errorf("armorEhp = %v", ehp)
	}

	// The working is what the editor shows, so it has to name every rule.
	steps := ehp["steps"].([]any)
	if len(steps) != 3 {
		t.Fatalf("want 3 steps, got %v", steps)
	}
	if first := steps[0].(map[string]any); first["op"] != "base" || first["from"] != "computed" {
		t.Errorf("first step = %v", first)
	}
	if second := steps[1].(map[string]any); second["modifying"] != "armorHP" || second["after"].(float64) != 450 || second["ours"] != true {
		t.Errorf("second step = %v", second)
	}
	if third := steps[2].(map[string]any); third["op"] != "postDiv" || third["from"] != "type" {
		t.Errorf("third step = %v", third)
	}

	// An attribute the item carries itself is still reported, with its unit.
	hp := find(t, values, "armorHP")
	if hp["value"].(float64) != 450 || hp["origin"] != "type" || hp["unitID"].(float64) != 9 {
		t.Errorf("armorHP = %v", hp)
	}

	// A rule that reaches for the ship cannot be worked out alone.
	if unfit := body["needsFit"].([]any); len(unfit) != 1 {
		t.Errorf("needsFit = %v", unfit)
	}
	if effects := body["effects"].([]any); len(effects) != 1 || effects[0].(map[string]any)["added"] != true {
		t.Errorf("effects = %v", effects)
	}

	if status, _ := get(t, handler, "/api/live/1"); status != http.StatusNotFound {
		t.Errorf("unknown type: status %d", status)
	}
}

// A divide by zero cannot be encoded in JSON, so it comes back as a word.
func TestLiveBroken(t *testing.T) {
	handler, _ := newLiveServer(t)

	status, body := get(t, handler, "/api/live/588")
	if status != http.StatusOK {
		t.Fatalf("status %d: %v", status, body)
	}
	if ehp := find(t, body["values"].([]any), "armorEhp"); ehp["value"] != "Infinity" {
		t.Errorf("armorEhp = %v, want Infinity", ehp["value"])
	}
}

func TestHealth(t *testing.T) {
	handler, _ := newLiveServer(t)

	status, body := get(t, handler, "/api/health")
	if status != http.StatusOK {
		t.Fatalf("status %d: %v", status, body)
	}

	entry := find(t, body["attributes"].([]any), "armorEhp")
	if entry["count"].(float64) != 2 || entry["broken"].(float64) != 1 {
		t.Errorf("armorEhp = %v", entry)
	}
	if entry["max"].(float64) != 900 {
		t.Errorf("max = %v, want 900", entry["max"])
	}
	if entry["ours"] != true || entry["needsFit"] != true {
		t.Errorf("armorEhp = %v", entry)
	}

	kinds := map[string]bool{}
	for _, flag := range entry["flags"].([]any) {
		kinds[flag.(map[string]any)["kind"].(string)] = true
	}
	if !kinds["broken"] {
		t.Errorf("flags = %v, want a broken one", entry["flags"])
	}

	if declared := entry["declaredBy"].(map[string]any); declared["patch"] != "armorEhp" {
		t.Errorf("declaredBy = %v", declared)
	}
	if written := entry["writtenBy"].([]any); len(written) != 3 {
		t.Errorf("writtenBy = %v", written)
	}
}

func TestHealthAttribute(t *testing.T) {
	handler, _ := newLiveServer(t)

	status, body := get(t, handler, "/api/health/armorEhp")
	if status != http.StatusOK {
		t.Fatalf("status %d: %v", status, body)
	}

	// Broken first.
	items := body["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("want 2 items, got %v", items)
	}
	if first := items[0].(map[string]any); first["name"] != "Punisher" || first["value"] != "Infinity" {
		t.Errorf("first item = %v", first)
	}
	if second := items[1].(map[string]any); second["name"] != "Rifter" || second["value"].(float64) != 900 {
		t.Errorf("second item = %v", second)
	}

	if status, _ := get(t, handler, "/api/health/nonsense"); status != http.StatusNotFound {
		t.Errorf("unknown attribute: status %d", status)
	}
}

// Unpublished items carry almost nothing, so they would drown the counts.
func TestHealthPublishedOnly(t *testing.T) {
	data := liveData()
	data.Types[588].Published = false

	dir := writePatches(t, map[string]string{"armorEhp": ehpPatch, "selectors": shipSelector})
	handler := New(data, dir).Handler()

	_, body := get(t, handler, "/api/health")
	if entry := find(t, body["attributes"].([]any), "armorEhp"); entry["count"].(float64) != 1 {
		t.Errorf("count = %v, want 1", entry["count"])
	}

	_, body = get(t, handler, "/api/health?published=0")
	if entry := find(t, body["attributes"].([]any), "armorEhp"); entry["count"].(float64) != 2 {
		t.Errorf("count = %v, want 2", entry["count"])
	}
}
