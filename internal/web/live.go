package web

import (
	"fmt"
	"math"
	"net/http"
	"slices"
	"sort"
	"strings"

	"github.com/EVEShipFit/sde-patched/internal/fbs/eve"
	"github.com/EVEShipFit/sde-patched/internal/sde"
)

// Dogma is worked out one item at a time: the item view reads one, the
// attributes view reads every item a patch touches.
//
// An item on its own can only answer rules that read itself. Rules that reach
// for the ship it is fitted to, or for the pilot, need a fit; they are
// reported rather than dropped.

// fields are the four attributes a dogma engine fills in from the type
// itself, as the SDE keeps them on the type and not in its dogma. A rule that
// reads one of them is not reading a default.
func fields(entry *sde.Type) map[int32]float64 {
	result := map[int32]float64{}
	if entry.Mass != nil {
		result[4] = *entry.Mass
	}
	if entry.Capacity != nil {
		result[38] = *entry.Capacity
	}
	if entry.Volume != nil {
		result[161] = *entry.Volume
	}
	if entry.Radius != nil {
		result[162] = *entry.Radius
	}
	return result
}

// order is the sequence dogma applies operations in. Two rules on the same
// attribute only commute inside one step of it.
var order = []eve.ModifierOperation{
	eve.ModifierOperationPreAssign,
	eve.ModifierOperationPreMul,
	eve.ModifierOperationPreDiv,
	eve.ModifierOperationModAdd,
	eve.ModifierOperationModSub,
	eve.ModifierOperationPostMul,
	eve.ModifierOperationPostDiv,
	eve.ModifierOperationPostPercent,
	eve.ModifierOperationPostAssign,
}

func penalised(op eve.ModifierOperation) bool {
	return op == eve.ModifierOperationPostMul || op == eve.ModifierOperationPostDiv || op == eve.ModifierOperationPostPercent
}

type rule struct {
	effect *sde.DogmaEffect
	mod    sde.Modifier
	ours   bool
}

type step struct {
	op        string
	effect    string
	modifying int32
	x         float64
	from      string
	before    float64
	after     float64
	penalty   float64
	ours      bool
}

// live is one type, worked out.
type live struct {
	data    *sde.Data
	entry   *sde.Type
	base    map[int32]float64
	origin  map[int32]string
	field   map[int32]bool
	rules   map[int32][]rule
	memo    map[int32]float64
	trail   map[int32][]step
	open    map[int32]bool
	loops   map[int32]bool
	effects []*sde.DogmaEffect
	unfit   []rule
}

// newLive gathers what the item is worth before any rule runs, and sorts the
// rules of its effects by the attribute they write.
func newLive(current *state, pristine *sde.Data, entry *sde.Type, active bool) *live {
	it := &live{
		data: current.data, entry: entry,
		base: map[int32]float64{}, origin: map[int32]string{}, field: map[int32]bool{},
		rules: map[int32][]rule{}, memo: map[int32]float64{},
		trail: map[int32][]step{}, open: map[int32]bool{}, loops: map[int32]bool{},
	}

	for _, attribute := range entry.DogmaAttributes {
		it.base[attribute.AttributeID] = attribute.Value
		it.origin[attribute.AttributeID] = "type"
	}
	for id, value := range fields(entry) {
		if _, have := it.base[id]; !have {
			it.base[id] = value
			it.origin[id] = "type"
			it.field[id] = true
		}
	}

	for _, applied := range entry.DogmaEffects {
		effect := current.data.DogmaEffects[applied.EffectID]
		if effect == nil || !runs(effect, active) {
			continue
		}
		it.effects = append(it.effects, effect)

		ccp := 0
		if before := pristine.DogmaEffects[effect.Key]; before != nil {
			ccp = len(before.Modifiers)
		}
		for i, mod := range effect.Modifiers {
			at := rule{effect: effect, mod: mod, ours: i >= ccp}
			if mod.Func == eve.ModifierFuncItemModifier && mod.Domain == eve.ModifierDomainItemID && mod.ModifiedAttributeID != 0 {
				it.rules[mod.ModifiedAttributeID] = append(it.rules[mod.ModifiedAttributeID], at)
			} else {
				it.unfit = append(it.unfit, at)
			}
		}
	}
	sort.Slice(it.effects, func(i, j int) bool { return it.effects[i].Name < it.effects[j].Name })
	return it
}

// runs says whether an effect is on. Passive and online always are; active
// ones only when the item is asked to be running.
func runs(effect *sde.DogmaEffect, active bool) bool {
	switch eve.EffectCategory(effect.EffectCategoryID) {
	case eve.EffectCategoryPassive, eve.EffectCategoryOnline:
		return true
	case eve.EffectCategoryActive:
		return active
	}
	return false
}

func (it *live) baseOf(id int32) float64 {
	if value, have := it.base[id]; have {
		return value
	}
	if attribute := it.data.DogmaAttributes[id]; attribute != nil {
		return attribute.DefaultValue
	}
	return 0
}

// source is where a value came from, so a rule that read an unset default
// can be told apart.
func (it *live) source(id int32) string {
	if len(it.rules[id]) > 0 {
		return "computed"
	}
	if it.origin[id] != "" {
		return "type"
	}
	if it.data.DogmaAttributes[id] != nil {
		return "default"
	}
	return "missing"
}

// value works an attribute out, and every attribute it leans on with it. A
// rule that ends up reading its own attribute is a loop; it gets the base
// value, and the loop is remembered so it can be shown.
func (it *live) value(id int32) float64 {
	if value, done := it.memo[id]; done {
		return value
	}
	if it.open[id] {
		it.loops[id] = true
		return it.baseOf(id)
	}
	rules := it.rules[id]
	if len(rules) == 0 {
		value := it.baseOf(id)
		it.memo[id] = value
		return value
	}

	it.open[id] = true
	value := it.baseOf(id)
	trail := []step{{op: "base", after: value, from: it.source(id)}}
	stackable := true
	if attribute := it.data.DogmaAttributes[id]; attribute != nil {
		stackable = attribute.Stackable
	}

	for _, op := range order {
		var group []rule
		for _, at := range rules {
			if at.mod.Operation == op {
				group = append(group, at)
			}
		}
		if len(group) == 0 {
			continue
		}

		stacked := !stackable && penalised(op)
		if stacked {
			sort.SliceStable(group, func(i, j int) bool {
				return weight(op, it.value(group[i].mod.ModifyingAttributeID)) > weight(op, it.value(group[j].mod.ModifyingAttributeID))
			})
		}

		for i, at := range group {
			x := it.value(at.mod.ModifyingAttributeID)
			before, penalty := value, 1.0
			if stacked {
				penalty = math.Exp(-math.Pow(float64(i)/2.67, 2))
				value = before * (1 + (factor(op, x)-1)*penalty)
			} else {
				value = apply(op, before, x)
			}
			trail = append(trail, step{
				op: enumName("modifierOperation", int32(op)), effect: at.effect.Name,
				modifying: at.mod.ModifyingAttributeID, x: x, from: it.source(at.mod.ModifyingAttributeID),
				before: before, after: value, penalty: penalty, ours: at.ours,
			})
		}
	}

	delete(it.open, id)
	it.memo[id] = value
	it.trail[id] = trail
	return value
}

func apply(op eve.ModifierOperation, value, x float64) float64 {
	switch op {
	case eve.ModifierOperationPreAssign, eve.ModifierOperationPostAssign:
		return x
	case eve.ModifierOperationPreMul, eve.ModifierOperationPostMul:
		return value * x
	case eve.ModifierOperationPreDiv, eve.ModifierOperationPostDiv:
		return value / x
	case eve.ModifierOperationModAdd:
		return value + x
	case eve.ModifierOperationModSub:
		return value - x
	case eve.ModifierOperationPostPercent:
		return value * (1 + x/100)
	}
	return value
}

// factor is what a penalised rule would multiply by on its own, which is how
// the rules are ranked before the penalty is handed out.
func factor(op eve.ModifierOperation, x float64) float64 {
	switch op {
	case eve.ModifierOperationPostMul:
		return x
	case eve.ModifierOperationPostDiv:
		if x == 0 {
			return math.Inf(1)
		}
		return 1 / x
	case eve.ModifierOperationPostPercent:
		return 1 + x/100
	}
	return 1
}

func weight(op eve.ModifierOperation, x float64) float64 {
	return math.Abs(math.Log(factor(op, x)))
}

// touched is every attribute the item has an answer for: the ones it carries
// and the ones its rules write.
func (it *live) touched() []int32 {
	seen := map[int32]bool{}
	var ids []int32
	for id := range it.base {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for id := range it.rules {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return attributeName(it.data, ids[i]) < attributeName(it.data, ids[j]) })
	return ids
}

// ---------------------------------------------------------------- one item

// handleLive is one type with every number it ends up with, and the rules
// behind each of them.
func (s *Server) handleLive(w http.ResponseWriter, r *http.Request) {
	current, err := s.reload()
	if current == nil {
		fail(w, http.StatusConflict, err)
		return
	}

	entry := findType(current, r.PathValue("id"))
	if entry == nil {
		fail(w, http.StatusNotFound, fmt.Errorf("no type %q", r.PathValue("id")))
		return
	}
	_, active := settings(r)
	it := newLive(current, s.pristine, entry, active)

	before := s.pristine.Types[entry.Key]
	was := map[int32]float64{}
	for _, attribute := range before.DogmaAttributes {
		was[attribute.AttributeID] = attribute.Value
	}
	had := map[int32]bool{}
	for _, effect := range before.DogmaEffects {
		had[effect.EffectID] = true
	}

	values := []any{}
	for _, id := range it.touched() {
		attribute := current.data.DogmaAttributes[id]
		if attribute == nil {
			continue
		}
		value := it.value(id)

		shown := map[string]any{
			"id": id, "name": attribute.Name, "displayName": attribute.DisplayName.En,
			"value": number(value), "origin": it.source(id), "unitID": attribute.UnitID,
			"highIsGood": attribute.HighIsGood, "stackable": attribute.Stackable,
			"ours": id < 0, "loop": it.loops[id],
		}
		if raw, carried := it.base[id]; carried {
			shown["base"] = number(raw)
			switch old, kept := was[id]; {
			case it.field[id]:
				shown["fromType"] = true
			case !kept:
				shown["added"] = true
			case old != raw:
				shown["was"] = number(old)
			}
		} else {
			shown["base"] = number(it.baseOf(id))
		}
		if trail := it.trail[id]; trail != nil {
			shown["steps"] = trailOf(current, trail)
		}
		values = append(values, shown)
	}

	effects := []any{}
	for _, effect := range it.effects {
		effects = append(effects, map[string]any{
			"id": effect.Key, "name": effect.Name, "ours": effect.Key < 0,
			"category": effectCategory(effect.EffectCategoryID), "added": !had[effect.Key],
			"at": whereEffect(current, effect.Name),
		})
	}

	unfit := []any{}
	for _, at := range it.unfit {
		unfit = append(unfit, map[string]any{
			"effect": at.effect.Name, "ours": at.ours,
			"domain":    enumName("modifierDomain", int32(at.mod.Domain)),
			"func":      enumName("modifierFunc", int32(at.mod.Func)),
			"op":        enumName("modifierOperation", int32(at.mod.Operation)),
			"modified":  attributeName(current.data, at.mod.ModifiedAttributeID),
			"modifying": attributeName(current.data, at.mod.ModifyingAttributeID),
		})
	}

	group := current.data.Groups[entry.GroupID]
	category := current.data.Categories[entry.CategoryID]
	body := map[string]any{
		"id": entry.Key, "name": entry.Name.En, "published": entry.Published, "active": active,
		"group": named(group.Key, group.Name.En), "category": named(category.Key, category.Name.En),
		"values": values, "effects": effects, "needsFit": unfit, "patches": touching(current, entry.Key),
	}
	if err != nil {
		body["error"] = err.Error()
	}
	write(w, body)
}

func trailOf(current *state, trail []step) []any {
	out := make([]any, 0, len(trail))
	for _, at := range trail {
		if at.op == "base" {
			out = append(out, map[string]any{"op": "base", "value": number(at.after), "from": at.from})
			continue
		}
		out = append(out, map[string]any{
			"op": at.op, "effect": at.effect, "ours": at.ours,
			"modifying": attributeName(current.data, at.modifying), "x": number(at.x), "from": at.from,
			"before": number(at.before), "after": number(at.after), "penalty": number(at.penalty),
		})
	}
	return out
}

// whereEffect is the line an effect is written on, when a patch wrote it.
func whereEffect(current *state, name string) any {
	for _, effect := range current.spec.Effects() {
		if effect.Name == name {
			return effect.At()
		}
	}
	for _, change := range current.spec.Changes {
		if change.Effect == name {
			return change.At()
		}
	}
	return nil
}

// number keeps JSON valid: infinity and not-a-number cannot be encoded, but
// have to reach the editor.
func number(value float64) any {
	switch {
	case math.IsNaN(value):
		return "NaN"
	case math.IsInf(value, 1):
		return "Infinity"
	case math.IsInf(value, -1):
		return "-Infinity"
	}
	return value
}

// ------------------------------------------------------------- every item

// ours is every attribute the patches have a file for.
func ours(current *state) map[int32]bool {
	wanted := map[int32]bool{}
	for _, attribute := range current.spec.Attributes {
		wanted[current.ctx.AttributeID(attribute.Name)] = true
	}
	for _, action := range current.spec.Actions {
		if action.SetAttribute != "" {
			wanted[current.ctx.AttributeID(action.SetAttribute)] = true
		}
	}
	delete(wanted, 0)
	return wanted
}

// ourEffects are the effects a patch declared or added rules to.
func ourEffects(current *state) []*sde.DogmaEffect {
	names := map[string]bool{}
	for _, effect := range current.spec.Effects() {
		names[effect.EffectName()] = true
	}
	for _, added := range current.spec.AddTos() {
		names[added.Effect] = true
	}

	var found []*sde.DogmaEffect
	for _, effect := range sortedEffects(current) {
		if names[effect.Name] {
			found = append(found, effect)
		}
	}
	return found
}

type reading struct {
	id    int32
	value float64
}

// sweep works out every type a patch reaches, and keeps what the attributes
// the patches own came to. One pass over the types answers for all of them.
func (s *Server) sweep(current *state, published, active bool) map[int32][]reading {
	wanted := ours(current)
	mine := map[int32]bool{}
	for _, effect := range ourEffects(current) {
		mine[effect.Key] = true
	}

	found := map[int32][]reading{}
	for id := range wanted {
		found[id] = nil
	}

	for _, entry := range sortedTypes(current) {
		if published && !entry.Published || !carries(entry, mine) {
			continue
		}

		it := newLive(current, s.pristine, entry, active)
		for id := range it.rules {
			if wanted[id] {
				found[id] = append(found[id], reading{id: entry.Key, value: it.value(id)})
			}
		}
	}
	return found
}

// carries says whether a type has any of the effects.
func carries(entry *sde.Type, effects map[int32]bool) bool {
	return slices.ContainsFunc(entry.DogmaEffects, func(has sde.TypeDogmaEffect) bool {
		return effects[has.EffectID]
	})
}

func sortedTypes(current *state) []*sde.Type {
	types := make([]*sde.Type, 0, len(current.data.Types))
	for _, entry := range current.data.Types {
		types = append(types, entry)
	}
	sort.Slice(types, func(i, j int) bool { return types[i].Name.En < types[j].Name.En })
	return types
}

// bands group values by magnitude, so an outlier stands out.
var bands = []struct {
	name  string
	below float64
}{
	{"< 0", 0}, {"0", 0}, {"< 1", 1}, {"< 10", 10}, {"< 100", 100},
	{"< 1k", 1e3}, {"< 10k", 1e4}, {"< 1M", 1e6}, {"≥ 1M", math.Inf(1)},
}

func bandOf(value float64) int {
	switch {
	case value < 0:
		return 0
	case value == 0:
		return 1
	}
	for i := 2; i < len(bands); i++ {
		if value < bands[i].below {
			return i
		}
	}
	return len(bands) - 1
}

// health is what one attribute came to over everything that has it, and what
// about that looks wrong.
func health(current *state, id int32, list []reading, fit map[int32]bool, read map[int32]int) map[string]any {
	attribute := current.data.DogmaAttributes[id]

	spread := make([]int, len(bands))
	var finite []float64
	broken, zero, negative, atDefault := 0, 0, 0, 0
	distinct := map[float64]bool{}

	for _, at := range list {
		if odd(at.value) {
			broken++
			continue
		}
		finite = append(finite, at.value)
		distinct[at.value] = true
		spread[bandOf(at.value)]++
		switch {
		case at.value == 0:
			zero++
		case at.value < 0:
			negative++
		}
		if at.value == attribute.DefaultValue {
			atDefault++
		}
	}
	sort.Float64s(finite)

	// An attribute that other items add to only holds its base on an item
	// standing alone, so a flat zero there is the right answer, not a fault.
	fed := fit[id]

	flags := []flag{}
	raise := func(kind, why string) { flags = append(flags, flag{Kind: kind, Why: why}) }
	switch {
	case len(list) == 0 && fed:
		raise("onlyOnFits", "no rule sets this on a single item; other items add to it, so it only has a value on a full fit")
	case len(list) == 0 && read[id] > 0:
		raise("input", fmt.Sprintf("no rule sets this; %d rules read it, so it is an input and always holds its default", read[id]))
	case len(list) == 0:
		raise("orphan", "no rule sets this and nothing reads it, so it has no effect")
	case broken > 0:
		raise("broken", fmt.Sprintf("%d of %d are infinite or not a number, usually a divide by zero", broken, len(list)))
	}
	if len(finite) > 0 && !fed {
		if zero == len(finite) {
			raise("zero", "every item is zero")
		} else if len(distinct) == 1 && len(finite) > 1 {
			raise("flat", "every item has the same value, so the rules may read something no item has")
		}
		if share := float64(atDefault) / float64(len(finite)); atDefault >= 3 && share >= 0.2 && attribute.DefaultValue != 0 {
			raise("default", fmt.Sprintf("%d of %d are exactly the default of %s, so a rule may be reading nothing", atDefault, len(finite), trim(attribute.DefaultValue)))
		}
	}
	if len(finite) > 0 {
		if negative > 0 && attribute.HighIsGood {
			raise("negative", fmt.Sprintf("%d are below zero, while higher is better", negative))
		}
		if middle, top := finite[len(finite)/2], finite[len(finite)-1]; middle > 0 && top/middle > 1e5 {
			raise("spread", "the largest is over 100,000× the median, so a factor may be off")
		}
	}

	body := map[string]any{
		"id": id, "name": attribute.Name, "displayName": attribute.DisplayName.En,
		"default": number(attribute.DefaultValue), "unitID": attribute.UnitID,
		"highIsGood": attribute.HighIsGood, "stackable": attribute.Stackable, "ours": id < 0,
		"count": len(list), "broken": broken, "zero": zero, "negative": negative, "needsFit": fed,
		"atDefault": atDefault, "distinct": len(distinct), "flags": flags, "severity": severity(flags),
		"spread": spread, "bands": bandNames(),
		"declaredBy": declaredAttribute(current, attribute.Name),
		"writtenBy":  writtenBy(current, id),
	}
	if len(finite) > 0 {
		body["min"] = number(finite[0])
		body["max"] = number(finite[len(finite)-1])
		body["median"] = number(finite[len(finite)/2])
	}
	return body
}

// flag is one thing about an attribute's numbers that looks wrong.
type flag struct {
	Kind string `json:"kind"`
	Why  string `json:"why"`
}

// severities rank the flags so the worst one decides where an attribute lands
// in the list.
var severities = map[string]int{
	"broken": 3, "negative": 3,
	"zero": 2, "flat": 2, "default": 2, "spread": 2,
	"orphan": 1,
	"input":  0, "onlyOnFits": 0,
}

func severity(flags []flag) int {
	worst := 0
	for _, entry := range flags {
		if rank := severities[entry.Kind]; rank > worst {
			worst = rank
		}
	}
	return worst
}

func bandNames() []string {
	names := make([]string, len(bands))
	for i, band := range bands {
		names[i] = band.name
	}
	return names
}

// needsFit is every attribute a whole fit is needed to answer: the ones other
// items write, and then everything worked out from one of those. Without the
// second half, a value that is only ever zero on a bare hull reads as a fault
// rather than as a value waiting for modules.
func needsFit(current *state) map[int32]bool {
	needs := map[int32]bool{}
	reads := map[int32][]int32{}
	for _, effect := range current.data.DogmaEffects {
		for _, modifier := range effect.Modifiers {
			if modifier.ModifiedAttributeID == 0 {
				continue
			}
			if modifier.Domain != eve.ModifierDomainItemID {
				needs[modifier.ModifiedAttributeID] = true
			}
			reads[modifier.ModifiedAttributeID] = append(reads[modifier.ModifiedAttributeID], modifier.ModifyingAttributeID)
		}
	}

	for spreading := true; spreading; {
		spreading = false
		for modified, inputs := range reads {
			if needs[modified] {
				continue
			}
			for _, input := range inputs {
				if needs[input] {
					needs[modified], spreading = true, true
					break
				}
			}
		}
	}
	return needs
}

// readBy counts the rules that read an attribute. One that nothing works out
// but something reads is an input, not a mistake.
func readBy(current *state) map[int32]int {
	count := map[int32]int{}
	for _, effect := range ourEffects(current) {
		for _, modifier := range effect.Modifiers {
			count[modifier.ModifyingAttributeID]++
		}
	}
	return count
}

func trim(value float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.4f", value), "0"), ".")
}

// writtenBy is the rules that give an attribute its value.
func writtenBy(current *state, id int32) []any {
	var result []any
	for _, effect := range ourEffects(current) {
		for _, modifier := range effect.Modifiers {
			if modifier.ModifiedAttributeID != id {
				continue
			}
			result = append(result, map[string]any{
				"effect": effect.Name, "at": whereEffect(current, effect.Name),
				"op":        enumName("modifierOperation", int32(modifier.Operation)),
				"domain":    enumName("modifierDomain", int32(modifier.Domain)),
				"modifying": attributeName(current.data, modifier.ModifyingAttributeID),
			})
		}
	}
	return result
}

// handleHealth is every number the patches are answerable for, over every
// item that has it, worst first. The attributes view is this list.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	current, err := s.reload()
	if current == nil {
		fail(w, http.StatusConflict, err)
		return
	}

	published, active := settings(r)
	found := s.sweptBy(current, published, active)
	fit, read := needsFit(current), readBy(current)

	attributes := []any{}
	for id, list := range found {
		if current.data.DogmaAttributes[id] == nil {
			continue
		}
		attributes = append(attributes, health(current, id, list, fit, read))
	}
	// Worst first.
	sort.Slice(attributes, func(i, j int) bool {
		a, b := attributes[i].(map[string]any), attributes[j].(map[string]any)
		if x, y := a["severity"].(int), b["severity"].(int); x != y {
			return x > y
		}
		if x, y := len(a["flags"].([]flag)), len(b["flags"].([]flag)); x != y {
			return x > y
		}
		return a["name"].(string) < b["name"].(string)
	})

	body := map[string]any{
		"attributes": attributes, "published": published, "active": active,
		"types": len(current.data.Types),
	}
	if err != nil {
		body["error"] = err.Error()
	}
	write(w, body)
}

// handleHealthAttribute is the items behind one of those numbers, sorted by
// value.
func (s *Server) handleHealthAttribute(w http.ResponseWriter, r *http.Request) {
	current, err := s.reload()
	if current == nil {
		fail(w, http.StatusConflict, err)
		return
	}

	name := r.PathValue("name")
	attribute := findAttribute(current, name)
	if attribute == nil {
		fail(w, http.StatusNotFound, fmt.Errorf("no dogma attribute named %q", name))
		return
	}
	id := attribute.Key

	// The sweep is shared with every other request on this run, so it is
	// sorted on a copy.
	published, active := settings(r)
	list := slices.Clone(s.sweptBy(current, published, active)[id])

	// Broken answers first, then the biggest.
	sort.Slice(list, func(i, j int) bool {
		a, b := list[i].value, list[j].value
		if bad, other := odd(a), odd(b); bad != other {
			return bad
		}
		return a > b
	})

	items := []any{}
	for _, at := range list {
		entry := current.data.Types[at.id]
		if entry == nil {
			continue
		}
		items = append(items, map[string]any{
			"id": entry.Key, "name": entry.Name.En, "published": entry.Published,
			"group": nameOfGroup(current.data.Groups[entry.GroupID]),
			"value": number(at.value),
		})
	}

	body := health(current, id, list, needsFit(current), readBy(current))
	body["items"] = items
	if err != nil {
		body["error"] = err.Error()
	}
	write(w, body)
}
