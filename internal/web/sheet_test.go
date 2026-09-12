package web

import (
	"net/http"
	"testing"
)

// An attribute page has to answer all four questions at once.
func TestSheetAnswersEverythingAboutOneAttribute(t *testing.T) {
	handler, _ := newTestServer(t)

	code, body := get(t, handler, "/api/sheet/alignTime")
	if code != http.StatusOK {
		t.Fatalf("status = %d: %v", code, body)
	}

	if body["kind"] != "new" {
		t.Errorf("kind = %v, want new", body["kind"])
	}
	if body["defaultValue"] != 1.5 {
		t.Errorf("defaultValue = %v, want 1.5", body["defaultValue"])
	}

	formula, _ := body["formula"].([]any)
	if len(formula) != 1 {
		t.Fatalf("formula = %v, want the one rule", formula)
	}
	step := formula[0].(map[string]any)
	if step["op"] != "mul" || step["modifying"] != "agility" {
		t.Errorf("step = %v, want a mul by agility", step)
	}
	if step["ours"] != true {
		t.Error("the rule came from a patch, so it should say so")
	}

	// An effect says itself who gets it.
	applies, _ := body["appliesTo"].([]any)
	if len(applies) != 1 {
		t.Fatalf("appliesTo = %v, want the one effect", applies)
	}
	if on := applies[0].(map[string]any)["on"]; on != "isShip" {
		t.Errorf("on = %v, want isShip", on)
	}

	// A column per input, the answer last, one row per item.
	inputs, _ := body["inputs"].([]any)
	if len(inputs) != 1 || inputs[0].(map[string]any)["name"] != "agility" {
		t.Errorf("inputs = %v, want agility", inputs)
	}
	rows, _ := body["rows"].([]any)
	if len(rows) != 1 {
		t.Fatalf("rows = %v, want the Rifter", rows)
	}
	if row := rows[0].(map[string]any); row["name"] != "Rifter" {
		t.Errorf("row = %v, want the Rifter", row)
	}
}

// Every edit the page offers is addressed by where the declaration stands in
// its own patch, so the page has to carry that.
func TestSheetSaysWhereToWriteAnEditBack(t *testing.T) {
	handler, _ := newTestServer(t)
	_, body := get(t, handler, "/api/sheet/alignTime")

	// What the attribute is, is one block at the top of its own file, so it
	// has no index.
	where, _ := body["definition"].(map[string]any)
	if where == nil || where["patch"] != "alignTime" || where["section"] != "new" {
		t.Fatalf("definition = %v, want the new block of the alignTime file", where)
	}
	if where["fields"].(map[string]any)["default"] != float64(1.5) {
		t.Errorf("fields = %v, want what the attribute is", where["fields"])
	}

	formula := body["formula"].([]any)[0].(map[string]any)
	place := formula["where"].(map[string]any)["place"].(map[string]any)
	if place["patch"] != "alignTime" || place["section"] != "effects" || place["index"] != float64(0) {
		t.Errorf("rule lives at %v, want the first effect of the alignTime file", place)
	}
}

func TestSheetsSplitWhatWeInventedFromWhatWeWriteInto(t *testing.T) {
	handler, _ := newTestServer(t)

	_, body := get(t, handler, "/api/sheets")
	kinds := map[string]string{}
	for _, entry := range body["attributes"].([]any) {
		row := entry.(map[string]any)
		kinds[row["name"].(string)] = row["kind"].(string)
	}

	if kinds["alignTime"] != "new" {
		t.Errorf("alignTime = %q, want new", kinds["alignTime"])
	}
	if _, listed := kinds["agility"]; listed {
		t.Error("agility is only read, never written, so it does not belong in the list")
	}
}

func TestUnknownAttributeIsRefused(t *testing.T) {
	handler, _ := newTestServer(t)

	if code, _ := get(t, handler, "/api/sheet/nothingLikeThis"); code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", code)
	}
}
