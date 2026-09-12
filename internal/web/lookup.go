package web

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/EVEShipFit/sde-patched/internal/patch"
	"github.com/EVEShipFit/sde-patched/internal/sde"
)

// The pages behind the editor's links: types, attributes, effects, selectors,
// groups and categories.

func (s *Server) handleType(w http.ResponseWriter, r *http.Request) {
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
	before := s.pristine.Types[entry.Key]

	group := current.data.Groups[entry.GroupID]
	category := current.data.Categories[entry.CategoryID]
	write(w, map[string]any{
		"id": entry.Key, "name": entry.Name.En,
		"group": named(group.Key, group.Name.En), "category": named(category.Key, category.Name.En),
		"published":  entry.Published,
		"effects":    typeEffects(current, entry, before),
		"attributes": typeAttributes(current, entry, before),
		"patches":    touching(current, entry.Key),
	})
}

func typeEffects(current *state, entry, before *sde.Type) []any {
	had := map[int32]bool{}
	for _, effect := range before.DogmaEffects {
		had[effect.EffectID] = true
	}
	has := map[int32]bool{}
	for _, effect := range entry.DogmaEffects {
		has[effect.EffectID] = true
	}

	var effects []any
	for _, effect := range entry.DogmaEffects {
		effects = append(effects, map[string]any{
			"id": effect.EffectID, "name": effectName(current.data, effect.EffectID),
			"isDefault": effect.IsDefault, "added": !had[effect.EffectID],
		})
	}
	for _, effect := range before.DogmaEffects {
		if !has[effect.EffectID] {
			effects = append(effects, map[string]any{
				"id": effect.EffectID, "name": effectName(current.data, effect.EffectID),
				"isDefault": effect.IsDefault, "removed": true,
			})
		}
	}
	return effects
}

// typeAttributes puts the values a patch set next to the ones the SDE had.
func typeAttributes(current *state, entry, before *sde.Type) []any {
	was := map[int32]float64{}
	for _, attribute := range before.DogmaAttributes {
		was[attribute.AttributeID] = attribute.Value
	}

	var result []any
	for _, attribute := range entry.DogmaAttributes {
		shown := map[string]any{
			"id":    attribute.AttributeID,
			"name":  attributeName(current.data, attribute.AttributeID),
			"value": attribute.Value,
		}
		if old, had := was[attribute.AttributeID]; !had {
			shown["added"] = true
		} else if old != attribute.Value {
			shown["was"] = old
		}
		result = append(result, shown)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].(map[string]any)["name"].(string) < result[j].(map[string]any)["name"].(string)
	})
	return result
}

// touching is which attributes a patch works out on one item, which is the
// list of files that had anything to say about it.
func touching(current *state, id int32) []any {
	var result []any
	for _, effect := range current.spec.Effects() {
		if !contains(current.ctx.Applied[effect.EffectName()], id) {
			continue
		}
		result = append(result, map[string]any{
			"patch": effect.Attribute,
			"at":    effect.At(),
			"on":    effect.On.Text,
		})
	}
	for i, action := range current.spec.Actions {
		if !contains(current.ctx.Matched[i], id) {
			continue
		}
		result = append(result, map[string]any{
			"patch":       action.Patch(),
			"at":          action.At(),
			"description": action.String(),
			"on":          action.On.Text,
		})
	}
	return result
}

// handleAttribute is one dogma attribute: where it comes from, what uses it,
// and what has it.
func (s *Server) handleAttribute(w http.ResponseWriter, r *http.Request) {
	current, err := s.reload()
	if current == nil {
		fail(w, http.StatusConflict, err)
		return
	}

	name := r.PathValue("name")
	entry := findAttribute(current, name)
	if entry == nil {
		fail(w, http.StatusNotFound, fmt.Errorf("no dogma attribute named %q", name))
		return
	}

	var setBy []any
	for _, action := range current.spec.Actions {
		if action.SetAttribute == name {
			setBy = append(setBy, map[string]any{
				"patch": action.Patch(), "at": action.At(),
				"description": action.String(), "on": action.On.Text,
			})
		}
	}

	var changedBy []any
	for _, attribute := range current.spec.Attributes {
		if attribute.Name == name && attribute.Change != nil {
			changedBy = append(changedBy, map[string]any{
				"patch": attribute.Name, "at": attribute.At(),
			})
		}
	}

	write(w, map[string]any{
		"id": entry.Key, "name": entry.Name, "displayName": entry.DisplayName.En,
		"defaultValue": entry.DefaultValue, "highIsGood": entry.HighIsGood,
		"stackable": entry.Stackable, "published": entry.Published, "unitID": entry.UnitID,
		"ours":       entry.Key < 0,
		"declaredBy": declaredAttribute(current, name),
		"usedBy":     usedBy(current, entry.Key),
		"setBy":      setBy,
		"changedBy":  changedBy,
		"types":      s.tree(current, typesWithAttribute(current, entry.Key)),
	})
}

func declaredAttribute(current *state, name string) any {
	for _, attribute := range current.spec.Attributes {
		if attribute.Name == name && attribute.New != nil {
			return map[string]any{"patch": attribute.Name, "at": attribute.At()}
		}
	}
	return nil
}

// handleEffect is one dogma effect, with its modifiers spelled out in names
// rather than in the IDs they are stored as.
func (s *Server) handleEffect(w http.ResponseWriter, r *http.Request) {
	current, err := s.reload()
	if current == nil {
		fail(w, http.StatusConflict, err)
		return
	}

	name := r.PathValue("name")
	var entry *sde.DogmaEffect
	for _, effect := range current.data.DogmaEffects {
		if effect.Name == name {
			entry = effect
			break
		}
	}
	if entry == nil {
		fail(w, http.StatusNotFound, fmt.Errorf("no dogma effect named %q", name))
		return
	}

	var appliedBy []any
	for _, effect := range current.spec.Effects() {
		if effect.EffectName() != name {
			continue
		}
		appliedBy = append(appliedBy, map[string]any{
			"patch": effect.Attribute, "at": effect.At(),
			"on": effect.On.Text, "count": len(current.ctx.Applied[name]),
		})
	}
	for i, action := range current.spec.Actions {
		if action.RemoveEffect != name {
			continue
		}
		appliedBy = append(appliedBy, map[string]any{
			"patch": action.Patch(), "at": action.At(), "description": action.String(),
			"on": action.On.Text, "count": len(current.ctx.Matched[i]),
		})
	}

	var changedBy []any
	for _, change := range current.spec.Changes {
		if change.Effect == name {
			changedBy = append(changedBy, map[string]any{
				"patch": change.Patch(), "at": change.At(), "description": change.String(),
			})
		}
	}

	write(w, map[string]any{
		"id": entry.Key, "name": entry.Name, "displayName": entry.DisplayName.En,
		"category": effectCategory(entry.EffectCategoryID), "ours": entry.Key < 0,
		"published": entry.Published, "isAssistance": entry.IsAssistance,
		"isOffensive": entry.IsOffensive, "isWarpSafe": entry.IsWarpSafe,
		"attributes": map[string]any{
			"discharge":          attributeName(current.data, entry.DischargeAttributeID),
			"duration":           attributeName(current.data, entry.DurationAttributeID),
			"falloff":            attributeName(current.data, entry.FalloffAttributeID),
			"fittingUsageChance": attributeName(current.data, entry.FittingUsageChanceAttributeID),
			"range":              attributeName(current.data, entry.RangeAttributeID),
			"resistance":         attributeName(current.data, entry.ResistanceAttributeID),
			"trackingSpeed":      attributeName(current.data, entry.TrackingSpeedAttributeID),
		},
		"modifiers":  modifiers(current, entry.Modifiers),
		"declaredBy": declaredEffect(current, name),
		"appliedBy":  appliedBy,
		"changedBy":  changedBy,
		"types":      s.tree(current, typesWithEffect(current, entry.Key)),
	})
}

func declaredEffect(current *state, name string) any {
	for _, effect := range current.spec.Effects() {
		if effect.EffectName() == name {
			return map[string]any{"patch": effect.Patch(), "at": effect.At()}
		}
	}
	return nil
}

// modifiers spells out the rules of an effect by name.
func modifiers(current *state, list []sde.Modifier) []any {
	var result []any
	for _, modifier := range list {
		// Zero is "not used by this kind of rule", and the SDE happens to have
		// a group and a type with that ID.
		var group *sde.Group
		if modifier.GroupID != 0 {
			group = current.data.Groups[modifier.GroupID]
		}
		var skill *sde.Type
		if modifier.SkillTypeID > 0 {
			skill = current.data.Types[modifier.SkillTypeID]
		}

		entry := map[string]any{
			"domain":    enumName("modifierDomain", int32(modifier.Domain)),
			"func":      enumName("modifierFunc", int32(modifier.Func)),
			"op":        enumName("modifierOperation", int32(modifier.Operation)),
			"modified":  attributeName(current.data, modifier.ModifiedAttributeID),
			"modifying": attributeName(current.data, modifier.ModifyingAttributeID),
		}
		if group != nil {
			entry["group"] = named(group.Key, group.Name.En)
		}
		switch {
		case modifier.SkillTypeID == -1:
			entry["skill"] = named(-1, "whatever skill it needs")
		case skill != nil:
			entry["skill"] = named(skill.Key, skill.Name.En)
		}
		result = append(result, entry)
	}
	return result
}

// handleSelector is one named filter: what it says, what it matches, and who
// leans on it.
func (s *Server) handleSelector(w http.ResponseWriter, r *http.Request) {
	current, err := s.reload()
	if current == nil {
		fail(w, http.StatusConflict, err)
		return
	}

	name := r.PathValue("name")
	for _, selector := range current.spec.Selectors {
		if selector.Name != name {
			continue
		}

		matched, err := current.ctx.Match(selector.Match.Text)
		body := map[string]any{
			"name": selector.Name, "description": selector.Description,
			"match": selector.Match.Text, "patch": selector.Patch(), "at": selector.At(),
			"usedBy": mentions(current, name),
		}
		if err != nil {
			body["error"] = err.Error()
		} else {
			body["types"] = s.tree(current, matched)
		}
		write(w, body)
		return
	}
	fail(w, http.StatusNotFound, fmt.Errorf("no selector named %q", name))
}

// mentions finds everything whose expression uses a name. Reading the text is
// enough: a selector is a bare word in it.
func mentions(current *state, name string) []any {
	var result []any
	for _, selector := range current.spec.Selectors {
		if selector.Name != name && mentionsWord(selector.Match.Text, name) {
			result = append(result, map[string]any{
				"kind": "selector", "name": selector.Name,
				"patch": selector.Patch(), "at": selector.At(), "match": selector.Match.Text,
			})
		}
	}
	for _, effect := range current.spec.Effects() {
		if mentionsWord(effect.On.Text, name) {
			result = append(result, map[string]any{
				"kind": "attribute", "name": effect.Attribute,
				"patch": effect.Attribute, "at": effect.At(), "match": effect.On.Text,
			})
		}
	}
	for _, action := range current.spec.Actions {
		if mentionsWord(action.On.Text, name) {
			result = append(result, map[string]any{
				"kind": "action", "description": action.String(),
				"patch": action.Patch(), "at": action.At(), "match": action.On.Text,
			})
		}
	}
	return result
}

func mentionsWord(text, word string) bool {
	for at := 0; ; {
		found := strings.Index(text[at:], word)
		if found < 0 {
			return false
		}
		found += at
		before := found == 0 || !isWordByte(text[found-1])
		end := found + len(word)
		after := end == len(text) || !isWordByte(text[end])
		if before && after {
			return true
		}
		at = found + 1
	}
}

func isWordByte(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

func (s *Server) handleGroup(w http.ResponseWriter, r *http.Request) {
	current, err := s.reload()
	if current == nil {
		fail(w, http.StatusConflict, err)
		return
	}

	entry := findGroup(current, r.PathValue("id"))
	if entry == nil {
		fail(w, http.StatusNotFound, fmt.Errorf("no group %q", r.PathValue("id")))
		return
	}

	var ids []int32
	for _, item := range current.data.Types {
		if item.GroupID == entry.Key {
			ids = append(ids, item.Key)
		}
	}
	category := current.data.Categories[entry.CategoryID]

	write(w, map[string]any{
		"id": entry.Key, "name": entry.Name.En, "published": entry.Published,
		"category": named(category.Key, category.Name.En),
		"types":    s.tree(current, ids),
	})
}

func (s *Server) handleCategory(w http.ResponseWriter, r *http.Request) {
	current, err := s.reload()
	if current == nil {
		fail(w, http.StatusConflict, err)
		return
	}

	entry := findCategory(current, r.PathValue("id"))
	if entry == nil {
		fail(w, http.StatusNotFound, fmt.Errorf("no category %q", r.PathValue("id")))
		return
	}

	var ids []int32
	for _, item := range current.data.Types {
		if item.CategoryID == entry.Key {
			ids = append(ids, item.Key)
		}
	}

	write(w, map[string]any{
		"id": entry.Key, "name": entry.Name.En, "published": entry.Published,
		"types": s.tree(current, ids),
	})
}

// handlePreview matches an expression that has not been saved yet, so a filter
// can be tried before it is written down.
func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	current, err := s.reload()
	if current == nil {
		fail(w, http.StatusConflict, err)
		return
	}

	text := r.URL.Query().Get("on")
	matched, bad := current.ctx.Match(text)
	if bad != nil {
		fail(w, http.StatusBadRequest, bad)
		return
	}
	write(w, map[string]any{"count": len(matched), "types": s.tree(current, matched)})
}

// tree sorts the types a filter matched into their categories and groups.
// Every name is kept, under its count.
func (s *Server) tree(current *state, ids []int32) map[string]any {
	type bucket struct {
		name   string
		count  int
		groups map[int32]*bucket
		types  []*sde.Type
	}

	categories := map[int32]*bucket{}
	for _, id := range ids {
		entry := current.data.Types[id]
		if entry == nil {
			continue
		}

		category := categories[entry.CategoryID]
		if category == nil {
			category = &bucket{name: nameOf(current.data.Categories[entry.CategoryID]), groups: map[int32]*bucket{}}
			categories[entry.CategoryID] = category
		}
		group := category.groups[entry.GroupID]
		if group == nil {
			group = &bucket{name: nameOfGroup(current.data.Groups[entry.GroupID])}
			category.groups[entry.GroupID] = group
		}

		category.count++
		group.count++
		group.types = append(group.types, entry)
	}

	var out []any
	for id, category := range categories {
		var groups []any
		for groupID, group := range category.groups {
			sort.Slice(group.types, func(i, j int) bool { return group.types[i].Name.En < group.types[j].Name.En })
			groups = append(groups, map[string]any{
				"id": groupID, "name": group.name, "count": group.count, "types": typeRefs(group.types),
			})
		}
		sortByName(groups)
		out = append(out, map[string]any{
			"id": id, "name": category.name, "count": category.count, "groups": groups,
		})
	}
	sortByName(out)

	return map[string]any{"count": len(ids), "categories": out}
}

func typeRefs(types []*sde.Type) []any {
	result := make([]any, 0, len(types))
	for _, entry := range types {
		result = append(result, named(entry.Key, entry.Name.En))
	}
	return result
}

func sortByName(list []any) {
	sort.Slice(list, func(i, j int) bool {
		return list[i].(map[string]any)["name"].(string) < list[j].(map[string]any)["name"].(string)
	})
}

func named(id int32, name string) map[string]any {
	return map[string]any{"id": id, "name": name}
}

func nameOf(category *sde.Category) string {
	if category == nil {
		return "no category"
	}
	return category.Name.En
}

func nameOfGroup(group *sde.Group) string {
	if group == nil {
		return "no group"
	}
	return group.Name.En
}

// A link in the editor carries whichever of the two it has: a type is known by
// its ID, a group inside a modifier only by its name. Both lead here.
//
// The lowest ID wins when a name is shared, so that the same link always ends
// up in the same place; the search shows the others.

func findType(current *state, key string) *sde.Type {
	if id, err := strconv.Atoi(key); err == nil {
		return current.data.Types[int32(id)]
	}

	var found *sde.Type
	for _, entry := range current.data.Types {
		if entry.Name.En == key && (found == nil || entry.Key < found.Key) {
			found = entry
		}
	}
	return found
}

func findGroup(current *state, key string) *sde.Group {
	if id, err := strconv.Atoi(key); err == nil {
		return current.data.Groups[int32(id)]
	}

	var found *sde.Group
	for _, entry := range current.data.Groups {
		if entry.Name.En == key && (found == nil || entry.Key < found.Key) {
			found = entry
		}
	}
	return found
}

func findCategory(current *state, key string) *sde.Category {
	if id, err := strconv.Atoi(key); err == nil {
		return current.data.Categories[int32(id)]
	}

	var found *sde.Category
	for _, entry := range current.data.Categories {
		if entry.Name.En == key && (found == nil || entry.Key < found.Key) {
			found = entry
		}
	}
	return found
}

func findAttribute(current *state, name string) *sde.DogmaAttribute {
	for _, entry := range current.data.DogmaAttributes {
		if entry.Name == name {
			return entry
		}
	}
	return nil
}

func enumName(kind string, value int32) string { return patch.EnumName(kind, value) }

func effectCategory(id int32) string { return patch.EnumName("effectCategory", id) }

func attributeName(data *sde.Data, id int32) string {
	if entry := data.DogmaAttributes[id]; entry != nil {
		return entry.Name
	}
	return ""
}

func effectName(data *sde.Data, id int32) string {
	if entry := data.DogmaEffects[id]; entry != nil {
		return entry.Name
	}
	return "?"
}

func typesWithEffect(current *state, id int32) []int32 {
	var ids []int32
	for _, entry := range current.data.Types {
		for _, effect := range entry.DogmaEffects {
			if effect.EffectID == id {
				ids = append(ids, entry.Key)
				break
			}
		}
	}
	return ids
}

func typesWithAttribute(current *state, id int32) []int32 {
	var ids []int32
	for _, entry := range current.data.Types {
		for _, attribute := range entry.DogmaAttributes {
			if attribute.AttributeID == id {
				ids = append(ids, entry.Key)
				break
			}
		}
	}
	return ids
}

func sortedEffects(current *state) []*sde.DogmaEffect {
	effects := make([]*sde.DogmaEffect, 0, len(current.data.DogmaEffects))
	for _, effect := range current.data.DogmaEffects {
		effects = append(effects, effect)
	}
	sort.Slice(effects, func(i, j int) bool { return effects[i].Name < effects[j].Name })
	return effects
}

func contains(ids []int32, id int32) bool {
	index := sort.Search(len(ids), func(i int) bool { return ids[i] >= id })
	return index < len(ids) && ids[index] == id
}
