package web

import (
	"fmt"
	"maps"
	"math"
	"net/http"
	"slices"
	"sort"

	"github.com/EVEShipFit/sde-patched/internal/fbs/eve"
	"github.com/EVEShipFit/sde-patched/internal/patch"
	"github.com/EVEShipFit/sde-patched/internal/sde"
)

// One attribute is one page: what it is, the sum that works it out, which
// items get it, and what it came to on each of them.
//
// Which file a declaration is written in is only carried as somewhere to write
// an edit back to. It is not how anything is grouped.

// place is where a declaration lives, and the declaration itself, so a form
// can be filled in and put straight back.
type place struct {
	Patch   string `json:"patch"`
	Section string `json:"section"`
	Index   int    `json:"index"`
	Fields  any    `json:"fields"`
}

func (s *Server) sweptBy(current *state, published, active bool) map[int32][]reading {
	key := fmt.Sprintf("%t/%t", published, active)

	current.mu.Lock()
	defer current.mu.Unlock()
	if found, done := current.swept[key]; done {
		return found
	}
	found := s.sweep(current, published, active)
	current.swept[key] = found
	return found
}

// subjects is every attribute the patches are answerable for, which is the
// files in patches/attributes and nothing else.
func subjects(current *state) []int32 {
	ids := make([]int32, 0, len(current.spec.Attributes))
	for _, attribute := range current.spec.Attributes {
		if id := current.ctx.AttributeID(attribute.Name); id != 0 && current.data.DogmaAttributes[id] != nil {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		return attributeName(current.data, ids[i]) < attributeName(current.data, ids[j])
	})
	return ids
}

// declared is the file for one attribute, which is where every edit to it
// goes.
func declared(current *state, name string) *patch.Attribute {
	for _, attribute := range current.spec.Attributes {
		if attribute.Name == name {
			return attribute
		}
	}
	return nil
}

// handleSheets is the list down the side: every attribute the patches own,
// whether it was invented or only written into, and whether it looks wrong.
func (s *Server) handleSheets(w http.ResponseWriter, r *http.Request) {
	current, err := s.reload()
	if current == nil {
		fail(w, http.StatusConflict, err)
		return
	}

	published, active := settings(r)
	found := s.sweptBy(current, published, active)
	fit, read := needsFit(current), readBy(current)

	list := []any{}
	for _, id := range subjects(current) {
		attribute := current.data.DogmaAttributes[id]
		state := health(current, id, found[id], fit, read)
		list = append(list, map[string]any{
			"id": id, "name": attribute.Name, "displayName": attribute.DisplayName.En,
			"kind": kindOf(current, attribute.Name), "unitID": attribute.UnitID,
			"count": len(found[id]), "severity": state["severity"], "flags": state["flags"],
		})
	}

	body := map[string]any{"attributes": list, "published": published, "active": active}
	if err != nil {
		body["error"] = err.Error()
	}
	write(w, body)
}

// kindOf says what the patches did to an attribute: invented it, wrote into
// one that already existed, or nothing at all.
func kindOf(current *state, name string) string {
	entry := declared(current, name)
	switch {
	case entry == nil:
		return ""
	case entry.New != nil:
		return "new"
	}
	return "patched"
}

// ------------------------------------------------------------- the formula

// term is one step of the sum that works an attribute out. Read in dogma's
// order they spell the whole formula.
type term struct {
	effect    *sde.DogmaEffect
	modifier  sde.Modifier
	index     int
	ours      bool
	elsewhere bool
}

// termsFor gathers every rule that writes an attribute, in the order dogma
// applies them rather than the order they happen to be written down.
func termsFor(current *state, pristine *sde.Data, id int32) []term {
	var found []term
	for _, effect := range sortedEffects(current) {
		ccp := 0
		if before := pristine.DogmaEffects[effect.Key]; before != nil {
			ccp = len(before.Modifiers)
		}
		for i, modifier := range effect.Modifiers {
			if modifier.ModifiedAttributeID != id {
				continue
			}
			found = append(found, term{
				effect: effect, modifier: modifier, index: i,
				ours:      effect.Key < 0 || i >= ccp,
				elsewhere: modifier.Domain != eve.ModifierDomainItemID || modifier.Func != eve.ModifierFuncItemModifier,
			})
		}
	}

	rank := map[eve.ModifierOperation]int{}
	for i, op := range order {
		rank[op] = i
	}
	sort.SliceStable(found, func(i, j int) bool {
		return rank[found[i].modifier.Operation] < rank[found[j].modifier.Operation]
	})
	return found
}

// ------------------------------------------------------------ one attribute

func (s *Server) handleSheet(w http.ResponseWriter, r *http.Request) {
	current, err := s.reload()
	if current == nil {
		fail(w, http.StatusConflict, err)
		return
	}

	name := r.PathValue("name")
	attribute := findAttribute(current, name)
	if attribute == nil {
		fail(w, http.StatusNotFound, fmt.Errorf("no attribute named %q", name))
		return
	}

	published, active := settings(r)
	all := r.URL.Query().Get("all") == "1"

	terms := termsFor(current, s.pristine, attribute.Key)
	inputs := inputsOf(current, terms)

	values, rows := s.workOut(current, attribute.Key, terms, inputs, published, active, all)
	state := health(current, attribute.Key, values, needsFit(current), readBy(current))

	body := map[string]any{
		"id": attribute.Key, "name": attribute.Name, "displayName": attribute.DisplayName.En,
		"kind": kindOf(current, attribute.Name), "unitID": attribute.UnitID,
		"defaultValue": number(attribute.DefaultValue),
		"highIsGood":   attribute.HighIsGood, "stackable": attribute.Stackable,
		"published": attribute.Published,

		"definition": definitionOf(current, attribute.Name),
		"formula":    formulaOf(current, terms),
		"appliesTo":  appliesTo(current, terms),
		"alsoWrites": alsoWrites(current, terms, attribute.Key),
		"readBy":     readsOf(current, attribute.Key),

		"inputs": inputNames(current, inputs),
		"rows":   rows, "total": len(values), "showingAll": all,

		"count": state["count"], "broken": state["broken"], "needsFit": state["needsFit"],
		"min": state["min"], "median": state["median"], "max": state["max"],
		"spread": state["spread"], "bands": state["bands"],
		"flags": state["flags"], "severity": state["severity"],
	}
	if err != nil {
		body["error"] = err.Error()
	}
	write(w, body)
}

// definitionOf is what the attribute is, and where to write a change to it.
// It is one block at the top of the attribute's own file, so it has no index.
func definitionOf(current *state, name string) *place {
	entry := declared(current, name)
	if entry == nil {
		return nil
	}
	switch {
	case entry.New != nil:
		return &place{Patch: name, Section: "new", Index: -1, Fields: entry.New}
	case entry.Change != nil:
		return &place{Patch: name, Section: "change", Index: -1, Fields: entry.Change}
	}
	// Upstream owns what it is; the file only fills it in. A change block can
	// still be started.
	return &place{Patch: name, Section: "change", Index: -1, Fields: &patch.Definition{}}
}

func formulaOf(current *state, terms []term) []any {
	out := []any{}
	for _, at := range terms {
		out = append(out, map[string]any{
			"effect": at.effect.Name, "ours": at.ours, "elsewhere": at.elsewhere,
			"op":        patch.OperationWord(int32(at.modifier.Operation)),
			"domain":    enumName("modifierDomain", int32(at.modifier.Domain)),
			"func":      enumName("modifierFunc", int32(at.modifier.Func)),
			"modifying": attributeName(current.data, at.modifier.ModifyingAttributeID),
			"where":     whereModifier(current, at),
		})
	}
	return out
}

// whereModifier points at the declaration holding one rule, and at the rule
// inside it, so a form can be opened on exactly that line.
func whereModifier(current *state, at term) any {
	for _, entry := range current.spec.Attributes {
		for i, effect := range entry.Effects {
			if effect.EffectName() != at.effect.Name {
				continue
			}
			return map[string]any{
				"place": &place{Patch: entry.Name, Section: "effects", Index: i, Fields: effect},
				"rule":  at.index,
			}
		}

		// A rule appended to an upstream effect lives in the addTo that appended
		// it, counted from the end of what upstream wrote.
		for i, added := range entry.AddTo {
			if added.Effect != at.effect.Name {
				continue
			}
			return map[string]any{
				"place": &place{Patch: entry.Name, Section: "addTo", Index: i, Fields: added},
				"rule":  at.index - (len(at.effect.Modifiers) - len(added.Rules)),
			}
		}
	}
	return nil
}

// appliesTo is every effect that writes the attribute, and who each of them
// is given to.
func appliesTo(current *state, terms []term) []any {
	seen := map[string]bool{}
	out := []any{}
	for _, at := range terms {
		if seen[at.effect.Name] {
			continue
		}
		seen[at.effect.Name] = true

		entry := map[string]any{
			"effect": at.effect.Name, "ours": at.effect.Key < 0,
			"category": effectCategory(at.effect.EffectCategoryID),
			"count":    len(typesWithEffect(current, at.effect.Key)),
		}
		for _, owner := range current.spec.Attributes {
			for i, effect := range owner.Effects {
				if effect.EffectName() != at.effect.Name {
					continue
				}
				entry["on"] = effect.On.Text
				entry["asDefault"] = effect.AsDefault
				entry["matched"] = len(current.ctx.Applied[effect.EffectName()])
				entry["where"] = &place{Patch: owner.Name, Section: "effects", Index: i, Fields: effect}
			}
		}
		out = append(out, entry)
	}
	return out
}

// alsoWrites warns that an effect on this page is shared: editing it moves
// numbers on another attribute too.
func alsoWrites(current *state, terms []term, id int32) []any {
	seen := map[string]bool{}
	out := []any{}
	for _, at := range terms {
		for _, modifier := range at.effect.Modifiers {
			name := attributeName(current.data, modifier.ModifiedAttributeID)
			if modifier.ModifiedAttributeID == id || modifier.ModifiedAttributeID == 0 || seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, map[string]any{"name": name, "effect": at.effect.Name})
		}
	}
	return out
}

// readsOf is what reads this attribute.
func readsOf(current *state, id int32) []any {
	seen := map[string]bool{}
	out := []any{}
	for _, effect := range sortedEffects(current) {
		for _, modifier := range effect.Modifiers {
			name := attributeName(current.data, modifier.ModifiedAttributeID)
			if modifier.ModifyingAttributeID != id || modifier.ModifiedAttributeID == id || seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, map[string]any{"name": name, "effect": effect.Name, "ours": modifier.ModifiedAttributeID < 0})
		}
	}
	return out
}

// inputsOf is the columns of the table: every attribute the sum reads, once
// each, in the order the sum reads them.
func inputsOf(current *state, terms []term) []int32 {
	seen := map[int32]bool{}
	var ids []int32
	for _, at := range terms {
		id := at.modifier.ModifyingAttributeID
		if id == 0 || seen[id] || at.elsewhere {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}

func inputNames(current *state, ids []int32) []any {
	out := []any{}
	for _, id := range ids {
		attribute := current.data.DogmaAttributes[id]
		if attribute == nil {
			continue
		}
		out = append(out, map[string]any{
			"id": id, "name": attribute.Name, "displayName": attribute.DisplayName.En,
			"unitID": attribute.UnitID, "ours": id < 0,
		})
	}
	return out
}

// ------------------------------------------------------------- the numbers

// workOut runs every item that gets the attribute, keeping the answer and the
// numbers that went into it.
func (s *Server) workOut(current *state, id int32, terms []term, inputs []int32, published, active, all bool) ([]reading, []any) {
	mine := map[int32]bool{}
	for _, at := range terms {
		mine[at.effect.Key] = true
	}

	type row struct {
		entry *sde.Type
		value float64
		in    []float64
	}

	var found []row
	var values []reading
	for _, entry := range sortedTypes(current) {
		if published && !entry.Published || !carries(entry, mine) {
			continue
		}

		it := newLive(current, s.pristine, entry, active)
		if len(it.rules[id]) == 0 {
			continue
		}
		value := it.value(id)
		values = append(values, reading{id: entry.Key, value: value})

		in := make([]float64, 0, len(inputs))
		for _, input := range inputs {
			in = append(in, it.value(input))
		}
		found = append(found, row{entry: entry, value: value, in: in})
	}

	// Broken answers first, then largest to smallest.
	sort.SliceStable(found, func(i, j int) bool {
		a, b := found[i].value, found[j].value
		if bad, other := odd(a), odd(b); bad != other {
			return bad
		}
		return a > b
	})

	keep := found
	if !all {
		keep = pick(len(found), func(i int) row { return found[i] })
	}

	rows := []any{}
	for _, at := range keep {
		in := make([]any, 0, len(at.in))
		for _, value := range at.in {
			in = append(in, number(value))
		}
		rows = append(rows, map[string]any{
			"id": at.entry.Key, "name": at.entry.Name.En, "published": at.entry.Published,
			"group": nameOfGroup(current.data.Groups[at.entry.GroupID]),
			"value": number(at.value), "in": in,
		})
	}
	return values, rows
}

func odd(value float64) bool { return math.IsNaN(value) || math.IsInf(value, 0) }

// sample is how many rows are shown unless every row is asked for.
const sample = 14

// pick keeps the first half of the sample, which is where broken answers sort
// to, and spreads the other half evenly over the whole range, so the top, the
// middle and the bottom are all on screen.
func pick[T any](count int, at func(int) T) []T {
	if count <= sample {
		out := make([]T, 0, count)
		for i := range count {
			out = append(out, at(i))
		}
		return out
	}

	wanted := map[int]bool{}
	for i := range sample / 2 {
		wanted[i] = true
		wanted[i*(count-1)/(sample/2-1)] = true
	}

	out := make([]T, 0, len(wanted))
	for _, i := range slices.Sorted(maps.Keys(wanted)) {
		out = append(out, at(i))
	}
	return out
}
