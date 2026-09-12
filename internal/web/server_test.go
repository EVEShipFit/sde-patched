package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/EVEShipFit/sde-patched/internal/patch"
	"github.com/EVEShipFit/sde-patched/internal/sde"
)

const goodPatch = `
new:
  default: 1.5

effects:
  - on: isShip
    category: passive
    rules:
      - {mul: agility}
`

const selectorsPatch = `
selectors:
  - {name: isShip, match: category("Ship")}
`

// writePatches lays out a patches directory: one file per attribute, plus the
// two files that are not an attribute.
func writePatches(t *testing.T, files map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, patch.AttributesDir), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, text := range files {
		path := filepath.Join(dir, patch.AttributesDir, name+".yaml")
		if name == "selectors" || name == "effects" {
			path = filepath.Join(dir, name+".yaml")
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// The editor writes the IDs down as it goes; a test starts as if it had.
	spec, err := patch.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if spec.IDs.Record(spec) > 0 {
		if err := spec.IDs.Save(); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func testData() *sde.Data {
	return &sde.Data{
		BuildNumber: 42,
		Categories:  map[int32]*sde.Category{6: {Key: 6, Name: sde.Localized{En: "Ship"}}},
		Groups:      map[int32]*sde.Group{25: {Key: 25, Name: sde.Localized{En: "Frigate"}, CategoryID: 6}},
		Types: map[int32]*sde.Type{
			587: {Key: 587, Name: sde.Localized{En: "Rifter"}, GroupID: 25, CategoryID: 6, Published: true},
		},
		DogmaAttributes: map[int32]*sde.DogmaAttribute{70: {Key: 70, Name: "agility"}},
		DogmaEffects:    map[int32]*sde.DogmaEffect{},
	}
}

func newTestServer(t *testing.T) (http.Handler, string) {
	t.Helper()

	dir := writePatches(t, map[string]string{"alignTime": goodPatch, "selectors": selectorsPatch})
	return New(testData(), dir).Handler(), dir
}

func get(t *testing.T, handler http.Handler, path string) (int, map[string]any) {
	t.Helper()

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest("GET", path, nil))

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("%s: %v (%s)", path, err, recorder.Body.String())
	}
	return recorder.Code, body
}

func send(t *testing.T, handler http.Handler, method, path, payload string) (int, map[string]any) {
	t.Helper()

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(method, path, strings.NewReader(payload)))

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("%s %s: %v (%s)", method, path, err, recorder.Body.String())
	}
	return recorder.Code, body
}

func fileText(t *testing.T, dir, name string) string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(dir, patch.AttributesDir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestState(t *testing.T) {
	handler, _ := newTestServer(t)

	code, body := get(t, handler, "/api/state")
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if body["error"] != nil {
		t.Fatalf("error = %v", body["error"])
	}
	if got := body["build"]; got != float64(42) {
		t.Errorf("build = %v, want 42", got)
	}
	if got := body["types"]; got != float64(1) {
		t.Errorf("types = %v, want 1", got)
	}
}

func TestPatchAndType(t *testing.T) {
	handler, _ := newTestServer(t)

	_, body := get(t, handler, "/api/patch/alignTime")
	if body["kind"] != "attribute" {
		t.Errorf("kind = %v, want attribute", body["kind"])
	}
	if got := body["new"].(map[string]any)["default"]; got != float64(1.5) {
		t.Errorf("default = %v, want 1.5", got)
	}

	effects := body["effects"].([]any)
	if len(effects) != 1 {
		t.Fatalf("effects = %v", effects)
	}
	if got := effects[0].(map[string]any)["count"]; got != float64(1) {
		t.Errorf("applied to = %v, want 1", got)
	}
	if got := effects[0].(map[string]any)["name"]; got != "alignTime" {
		t.Errorf("effect name = %v, want the attribute's", got)
	}

	_, shown := get(t, handler, "/api/type/587")
	onRifter := shown["effects"].([]any)
	if len(onRifter) != 1 || onRifter[0].(map[string]any)["added"] != true {
		t.Errorf("effects on the Rifter = %v, want one marked as added", onRifter)
	}
	if got := shown["patches"].([]any); len(got) != 1 {
		t.Errorf("patches touching the Rifter = %v, want one", got)
	}
}

func TestPreview(t *testing.T) {
	handler, _ := newTestServer(t)

	code, body := get(t, handler, "/api/preview?on="+`category%28%22Ship%22%29`)
	if code != http.StatusOK {
		t.Fatalf("status = %d, body %v", code, body)
	}
	if got := body["count"]; got != float64(1) {
		t.Errorf("count = %v, want 1", got)
	}

	code, body = get(t, handler, "/api/preview?on=nonsense%28")
	if code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", code)
	}
	if body["error"] == nil {
		t.Error("want an error explaining the expression")
	}
}

// A half-written file must not take the rest of the editor down with it.
func TestBrokenFileKeepsTheLastGoodRun(t *testing.T) {
	handler, dir := newTestServer(t)
	get(t, handler, "/api/state")

	file := filepath.Join(dir, patch.AttributesDir, "alignTime.yaml")
	if err := os.WriteFile(file, []byte("attributes: [{name: a, wobble: 1}]"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, body := get(t, handler, "/api/state")
	if body["error"] == nil {
		t.Error("state should report the error")
	}

	code, shown := get(t, handler, "/api/type/587")
	if code != http.StatusOK {
		t.Fatalf("type view broke too: %d %v", code, shown)
	}
	if shown["name"] != "Rifter" {
		t.Errorf("type = %v, want the Rifter from the last good run", shown["name"])
	}

	// Putting it back must recover.
	if err := os.WriteFile(file, []byte(goodPatch), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, body := get(t, handler, "/api/state"); body["error"] != nil {
		t.Errorf("error after fixing the file = %v", body["error"])
	}
}

func TestFilePathIsGuarded(t *testing.T) {
	handler, dir := newTestServer(t)

	for _, name := range []string{"..%2Fgo.mod", "..%2F..%2Fescape", "alignTime.yaml"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest("GET", "/api/patch/"+name, nil))
		if recorder.Code == http.StatusOK {
			t.Errorf("%q was served, want it refused", name)
		}
	}

	// Nothing outside the patches directory was created either.
	names, _ := filepath.Glob(filepath.Join(dir, patch.AttributesDir, "*"))
	if len(names) != 1 {
		t.Errorf("files in the attributes directory = %v, want only the one", names)
	}
}

// The editor works on declarations. Writing one back must change that one
// line, and leave the file it lives in alone.
func TestEditOneDeclaration(t *testing.T) {
	handler, dir := newTestServer(t)

	code, body := send(t, handler, "PUT", "/api/patch/alignTime/effects/0",
		`{"fields":{"on":"isShip","category":"passive","rules":[{"op":"div","by":"agility"}]},"comment":"Why it divides."}`)
	if code != http.StatusOK {
		t.Fatalf("status = %d, body %v", code, body)
	}

	text := fileText(t, dir, "alignTime.yaml")
	if !strings.Contains(text, "{div: agility}") {
		t.Errorf("the new rule is not in the file:\n%s", text)
	}
	if !strings.Contains(text, "# Why it divides.") {
		t.Errorf("the comment was not written:\n%s", text)
	}
	if !strings.Contains(text, "default: 1.5") {
		t.Errorf("the rest of the file went missing:\n%s", text)
	}

	// What comes back is the file as it now stands.
	effects := body["effects"].([]any)
	fields := effects[0].(map[string]any)["fields"].(map[string]any)
	if rules := fields["rules"].([]any); rules[0].(map[string]any)["op"] != "div" {
		t.Errorf("rules = %v, want a div", rules)
	}
}

// What an attribute is, is one block at the top of its file, so it is written
// on its own rather than as an entry in a list.
func TestEditWhatTheAttributeIs(t *testing.T) {
	handler, dir := newTestServer(t)

	code, body := send(t, handler, "PUT", "/api/patch/alignTime/define/new",
		`{"fields":{"default":9,"highIsGood":true}}`)
	if code != http.StatusOK {
		t.Fatalf("status = %d, body %v", code, body)
	}
	if got := body["new"].(map[string]any)["default"]; got != float64(9) {
		t.Errorf("default = %v, want 9", got)
	}
	if text := fileText(t, dir, "alignTime.yaml"); !strings.Contains(text, "default: 9") {
		t.Errorf("the new value is not in the file:\n%s", text)
	}
}

func TestAddAndRemove(t *testing.T) {
	handler, dir := newTestServer(t)

	code, body := send(t, handler, "POST", "/api/patch/alignTime/effects",
		`{"fields":{"name":"alignTime2","on":"category(\"Ship\")","category":"online","rules":[{"op":"add","by":"agility"}]}}`)
	if code != http.StatusOK {
		t.Fatalf("status = %d, body %v", code, body)
	}
	if got := len(body["effects"].([]any)); got != 2 {
		t.Fatalf("effects = %d, want 2", got)
	}

	code, body = send(t, handler, "DELETE", "/api/patch/alignTime/effects/1", "")
	if code != http.StatusOK {
		t.Fatalf("status = %d, body %v", code, body)
	}
	if got := len(body["effects"].([]any)); got != 1 {
		t.Errorf("effects after removing one = %d, want 1", got)
	}
	if text := fileText(t, dir, "alignTime.yaml"); strings.Contains(text, "alignTime2") {
		t.Errorf("the effect is still there:\n%s", text)
	}
}

// A declaration that would not read back is refused before it reaches the
// file.
func TestBadDeclarationIsRefused(t *testing.T) {
	handler, dir := newTestServer(t)
	before := fileText(t, dir, "alignTime.yaml")

	for _, payload := range []string{
		`{"fields":{"on":"category(\"Ship\"","category":"passive"}}`,
		`{"fields":{"on":"isShip","category":"passive","wobble":1}}`,
	} {
		code, body := send(t, handler, "POST", "/api/patch/alignTime/effects", payload)
		if code != http.StatusBadRequest {
			t.Errorf("status = %d for %s, want 400", code, payload)
		}
		if body["error"] == nil {
			t.Errorf("want an error for %s", payload)
		}
	}

	if after := fileText(t, dir, "alignTime.yaml"); after != before {
		t.Errorf("the file changed anyway:\n%s", after)
	}
}

func TestNotesAndNewPatch(t *testing.T) {
	handler, dir := newTestServer(t)

	if _, body := send(t, handler, "PUT", "/api/patch/alignTime/notes", `{"text":"What this is for."}`); body["notes"] != "What this is for." {
		t.Errorf("notes = %v", body["notes"])
	}
	if text := fileText(t, dir, "alignTime.yaml"); !strings.HasPrefix(text, "# What this is for.\n") {
		t.Errorf("the notes are not at the top:\n%s", text)
	}

	code, _ := send(t, handler, "POST", "/api/patch/newIdea", `{"notes":"An idea.","new":{"highIsGood":true}}`)
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if text := fileText(t, dir, "newIdea.yaml"); text != "# An idea.\n\nnew:\n  highIsGood: true\n" {
		t.Errorf("the new attribute reads %q", text)
	}

	// A new attribute gets a number, and it is written down straight away.
	spec, err := patch.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, known := spec.IDs.Attribute("newIdea"); !known {
		t.Error("the new attribute has no ID written down")
	}

	// Taking it out again leaves the number where it is.
	if code, _ := send(t, handler, "DELETE", "/api/patch/newIdea", ""); code != http.StatusOK {
		t.Fatalf("status = %d taking it out", code)
	}
	if spec, err := patch.Load(dir); err != nil {
		t.Fatal(err)
	} else if _, known := spec.IDs.Attribute("newIdea"); !known {
		t.Error("the ID was given back, so it could come back meaning something else")
	}

	// A name that is not a file name must not make a file.
	if code, _ := send(t, handler, "POST", "/api/patch/..%2Fescape", `{}`); code == http.StatusOK {
		t.Error("a path was served, want it refused")
	}

	// A body that does not read is refused rather than taken as empty.
	if code, _ := send(t, handler, "POST", "/api/patch/halfWritten", `{"notes":`); code != http.StatusBadRequest {
		t.Errorf("status = %d for a broken body, want 400", code)
	}
}

// The type view answers "what did the patches do to this?".
func TestTypeShowsWhatChanged(t *testing.T) {
	handler, _ := newTestServer(t)

	_, body := get(t, handler, "/api/type/587")
	effects := body["effects"].([]any)
	if len(effects) != 1 || effects[0].(map[string]any)["added"] != true {
		t.Errorf("effects = %v, want one marked as added", effects)
	}
	if got := body["category"].(map[string]any)["name"]; got != "Ship" {
		t.Errorf("category = %v, want a name to click on", got)
	}
}

func TestLookups(t *testing.T) {
	handler, _ := newTestServer(t)

	code, body := get(t, handler, "/api/attribute/alignTime")
	if code != http.StatusOK {
		t.Fatalf("status = %d, body %v", code, body)
	}
	if body["declaredBy"] == nil {
		t.Error("want the patch that declared it")
	}
	if got := body["usedBy"].([]any); len(got) != 1 {
		t.Errorf("usedBy = %v, want the effect that uses it", got)
	}

	code, body = get(t, handler, "/api/effect/alignTime")
	if code != http.StatusOK {
		t.Fatalf("status = %d, body %v", code, body)
	}
	if got := body["types"].(map[string]any)["count"]; got != float64(1) {
		t.Errorf("types with the effect = %v, want 1", got)
	}
	if got := body["modifiers"].([]any)[0].(map[string]any)["modifying"]; got != "agility" {
		t.Errorf("modifying = %v, want the attribute name", got)
	}

	code, body = get(t, handler, "/api/selector/isShip")
	if code != http.StatusOK {
		t.Fatalf("status = %d, body %v", code, body)
	}
	if got := body["usedBy"].([]any); len(got) != 1 {
		t.Errorf("usedBy = %v, want the action that uses it", got)
	}

	if code, _ := get(t, handler, "/api/selector/nothing"); code != http.StatusNotFound {
		t.Errorf("status for an unknown selector = %d, want 404", code)
	}
}

// The types a filter matched are sorted into their categories and groups.
func TestTypesComeBackAsATree(t *testing.T) {
	handler, _ := newTestServer(t)

	_, body := get(t, handler, "/api/preview?on="+`category%28%22Ship%22%29`)
	tree := body["types"].(map[string]any)
	categories := tree["categories"].([]any)
	if len(categories) != 1 {
		t.Fatalf("categories = %v", categories)
	}

	category := categories[0].(map[string]any)
	if category["name"] != "Ship" || category["count"] != float64(1) {
		t.Errorf("category = %v, want the Ship category with one type", category)
	}
	groups := category["groups"].([]any)
	if len(groups) != 1 || groups[0].(map[string]any)["name"] != "Frigate" {
		t.Errorf("groups = %v, want the Frigate group", groups)
	}
}
