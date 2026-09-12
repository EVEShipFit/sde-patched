package patch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// everyFile is every patch file there is, with the kind each one is.
func everyFile(t *testing.T) map[string]string {
	t.Helper()

	root := filepath.Join("..", "..", "patches")
	found := map[string]string{}

	names, _ := filepath.Glob(filepath.Join(root, AttributesDir, "*.yaml"))
	for _, name := range names {
		found[name] = KindAttribute
	}
	for name, kind := range map[string]string{SelectorsFile: KindSelectors, EffectsFile: KindEffects} {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			found[filepath.Join(root, name)] = kind
		}
	}

	if len(found) == 0 {
		t.Skip("no patches to read")
	}
	return found
}

// Writing a declaration back exactly as it was read must leave the file alone,
// comments, blank lines and all. Runs over every patch in the repository.
func TestReplaceKeepsTheFile(t *testing.T) {
	for name, kind := range everyFile(t) {
		t.Run(filepath.Base(name), func(t *testing.T) {
			raw, err := os.ReadFile(name)
			if err != nil {
				t.Fatal(err)
			}

			doc, err := ParseDocument(raw, kind)
			if err != nil {
				t.Fatal(err)
			}
			for _, section := range Sections(kind) {
				if section == "new" || section == "change" {
					definition := &Definition{}
					if found, err := doc.Mapping(section, definition); err != nil {
						t.Fatalf("%s: %v", section, err)
					} else if found {
						if err := doc.SetMapping(section, definition); err != nil {
							t.Fatalf("%s: %v", section, err)
						}
					}
					continue
				}
				for i := range doc.Count(section) {
					entry, err := doc.Entry(section, i)
					if err != nil {
						t.Fatalf("%s %d: %v", section, i, err)
					}
					if err := doc.Replace(section, i, entry, doc.Comment(section, i)); err != nil {
						t.Fatalf("%s %d: %v", section, i, err)
					}
				}
			}

			if got := string(doc.Bytes()); got != string(raw) {
				t.Errorf("the file changed:\n%s", diff(string(raw), got))
			}
		})
	}
}

func diff(want, got string) string {
	a, b := strings.Split(want, "\n"), strings.Split(got, "\n")
	var out []string
	for i := 0; i < len(a) || i < len(b); i++ {
		left, right := "", ""
		if i < len(a) {
			left = a[i]
		}
		if i < len(b) {
			right = b[i]
		}
		if left != right {
			out = append(out, "-"+left, "+"+right)
		}
	}
	return strings.Join(out, "\n")
}

const oneFile = `# What this attribute is for.

new:
  default: 2
  highIsGood: true

effects:
  - name: first
    on: isShip
    category: passive
    rules:
      - {mul: agility}

  # Why the second one is there.
  - name: second
    on: isDrone
    category: passive
    rules:
      - {div: mass}
`

func edit(t *testing.T, text string, change func(*Document) error) string {
	t.Helper()

	doc, err := ParseDocument([]byte(text), KindAttribute)
	if err != nil {
		t.Fatal(err)
	}
	if err := change(doc); err != nil {
		t.Fatal(err)
	}
	return string(doc.Bytes())
}

func expr(t *testing.T, text string) Expr {
	t.Helper()

	parsed, err := ParseExpr(text)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestEdits(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Document) error
		want   string
	}{{
		name: "an added effect follows the ones already there",
		change: func(d *Document) error {
			return d.Insert("effects", &Effect{
				Name: "third", On: expr(t, "isCharge"), Category: "passive",
				Rules: []Rule{{Op: "add", By: "volume"}},
			}, "")
		},
		want: "  - name: third\n    on: isCharge\n    category: passive\n    rules:\n      - {add: volume}\n",
	}, {
		name: "a rule that writes somewhere else says so",
		change: func(d *Document) error {
			return d.Insert("effects", &Effect{
				Name: "fourth", On: expr(t, "isModule"), Category: "online",
				Rules: []Rule{{Op: "add", By: "cpu", Domain: "shipID"}},
			}, "")
		},
		want: "      - {add: cpu, domain: shipID}\n",
	}, {
		name: "a changed rule leaves the rest of the effect alone",
		change: func(d *Document) error {
			entry, err := d.Entry("effects", 0)
			if err != nil {
				return err
			}
			entry.(*Effect).Rules[0].Op = "div"
			return d.Replace("effects", 0, entry, d.Comment("effects", 0))
		},
		want: "      - {div: agility}\n",
	}, {
		name: "what the attribute is can be rewritten on its own",
		change: func(d *Document) error {
			high := true
			return d.SetMapping("new", &Definition{Default: ptr(9.5), HighIsGood: &high})
		},
		want: "new:\n  default: 9.5\n  highIsGood: true\n",
	}, {
		name: "a comment can be written from the editor",
		change: func(d *Document) error {
			entry, err := d.Entry("effects", 0)
			if err != nil {
				return err
			}
			return d.Replace("effects", 0, entry, "Note to self.")
		},
		want: "  # Note to self.\n  - name: first",
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := edit(t, oneFile, test.change)
			if !strings.Contains(got, test.want) {
				t.Errorf("want %q in:\n%s", test.want, got)
			}
		})
	}
}

func ptr[T any](value T) *T { return &value }

// Taking an entry out takes its comment and the blank line with it, and an
// empty section goes altogether.
func TestRemove(t *testing.T) {
	got := edit(t, oneFile, func(d *Document) error { return d.Remove("effects", 1) })
	if strings.Contains(got, "second") || strings.Contains(got, "Why the second") {
		t.Errorf("the entry or its comment is still there:\n%s", got)
	}
	if !strings.Contains(got, "first") {
		t.Errorf("the wrong entry went:\n%s", got)
	}

	got = edit(t, got, func(d *Document) error { return d.Remove("effects", 0) })
	if strings.Contains(got, "effects:") {
		t.Errorf("the empty section should have gone too:\n%s", got)
	}
	if !strings.Contains(got, "What this attribute is for.") || !strings.Contains(got, "new:") {
		t.Errorf("too much went:\n%s", got)
	}
}

// Saying nothing about what an attribute is takes the block out, which is how
// a file goes back to only filling one of CCP's in.
func TestDefinitionCanBeTakenOut(t *testing.T) {
	got := edit(t, oneFile, func(d *Document) error { return d.SetMapping("new", nil) })
	if strings.Contains(got, "new:") {
		t.Errorf("the block is still there:\n%s", got)
	}
	if !strings.Contains(got, "effects:") || !strings.Contains(got, "What this attribute is for.") {
		t.Errorf("too much went:\n%s", got)
	}
}

func TestNotes(t *testing.T) {
	doc, err := ParseDocument([]byte(oneFile), KindAttribute)
	if err != nil {
		t.Fatal(err)
	}
	if got := doc.Notes(); got != "What this attribute is for." {
		t.Errorf("notes = %q", got)
	}

	doc.SetNotes("A longer story,\nin two lines.")
	got := string(doc.Bytes())
	if !strings.HasPrefix(got, "# A longer story,\n# in two lines.\n\nnew:") {
		t.Errorf("notes were not written back:\n%s", got)
	}
	if doc.Count("effects") != 2 {
		t.Errorf("the rest of the file did not survive:\n%s", got)
	}
}

// A new attribute starts as an empty file, and its first declaration has to
// make its own section.
func TestEmptyFile(t *testing.T) {
	doc, err := ParseDocument(nil, KindAttribute)
	if err != nil {
		t.Fatal(err)
	}
	doc.SetNotes("Brand new.")
	if err := doc.SetMapping("new", &Definition{Default: ptr(1.0)}); err != nil {
		t.Fatal(err)
	}
	if err := doc.Insert("effects", &Effect{
		On: expr(t, "published()"), Category: "passive",
		Rules: []Rule{{Op: "mul", By: "mass"}},
	}, ""); err != nil {
		t.Fatal(err)
	}

	want := "# Brand new.\n\nnew:\n  default: 1\n\neffects:\n  - on: published()\n    category: passive\n    rules:\n      - {mul: mass}\n"
	if got := string(doc.Bytes()); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// An edit that would not read back must not reach the file.
func TestBadEditIsRefused(t *testing.T) {
	doc, err := ParseDocument([]byte(oneFile), KindAttribute)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Insert("effects", map[string]any{"wobble": 1}, ""); err == nil {
		t.Error("want an error about the unknown key")
	}
	if got := string(doc.Bytes()); got != oneFile {
		t.Errorf("the file changed anyway:\n%s", got)
	}
}

// A rule that says two operations, or none, is not a rule.
func TestRuleSaysOneThing(t *testing.T) {
	for _, text := range []string{
		"effects:\n  - {on: isShip, category: passive, rules: [{mul: mass, div: agility}]}\n",
		"effects:\n  - {on: isShip, category: passive, rules: [{domain: shipID}]}\n",
	} {
		if _, err := ParseDocument([]byte(text), KindAttribute); err == nil {
			t.Errorf("want an error for:\n%s", text)
		}
	}
}

// Adding a declaration and taking it out again has to leave the file exactly
// as it was, whether or not the section was already there.
func TestAddAndRemoveLeavesNoTrace(t *testing.T) {
	for name, kind := range everyFile(t) {
		t.Run(filepath.Base(name), func(t *testing.T) {
			raw, err := os.ReadFile(name)
			if err != nil {
				t.Fatal(err)
			}

			for _, section := range Sections(kind) {
				if section == "new" || section == "change" {
					continue
				}

				doc, err := ParseDocument(raw, kind)
				if err != nil {
					t.Fatal(err)
				}

				was := doc.Count(section)
				if err := doc.Insert(section, sample(t, section), "A note."); err != nil {
					t.Fatalf("%s: %v", section, err)
				}
				if err := doc.Remove(section, was); err != nil {
					t.Fatalf("%s: %v", section, err)
				}
				if got := string(doc.Bytes()); got != string(raw) {
					t.Errorf("%s left a trace:\n%s", section, diff(string(raw), got))
				}
			}
		})
	}
}

func sample(t *testing.T, section string) any {
	t.Helper()

	match := expr(t, `published()`)
	switch section {
	case "selectors":
		return &NamedSelector{Name: "sample", Match: match}
	case "effects":
		return &Effect{Name: "sample", On: match, Category: "passive", Rules: []Rule{{Op: "mul", By: "mass"}}}
	case "addTo":
		return &AddTo{Effect: "online", Rules: []Rule{{Op: "mul", By: "mass"}}}
	case "changes":
		return &Change{Effect: "online"}
	default:
		return &Action{RemoveEffect: "online", On: match}
	}
}
