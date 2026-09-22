package patch

import (
	"errors"
	"fmt"
	"sort"

	"github.com/EVEShipFit/sde-patched/internal/sde"
)

// Context is the SDE plus the bookkeeping needed to explain what happened.
type Context struct {
	Data *sde.Data
	Spec *Spec

	// Matched holds the type IDs each action hit, in the same order as
	// Spec.Actions. Applied is the same for each effect, by name.
	Matched [][]int32
	Applied map[string][]int32

	attributeByName     map[string]*sde.DogmaAttribute
	dogmaCategoryByName map[string]*sde.DogmaAttributeCategory
	effectByName        map[string]*sde.DogmaEffect
	categoryByName      map[string]*sde.Category
	groupsByName        map[string][]*sde.Group
	typesByName         map[string][]*sde.Type
	selectorByName      map[string]*NamedSelector

	sortedTypes []*sde.Type
	errs        []error
}

// linkState guards against a selector that names itself, directly or through
// others.
type linkState int

const (
	unlinked linkState = iota
	linking
	linked
)

func (ctx *Context) errorf(at source, format string, args ...any) {
	ctx.errs = append(ctx.errs, fmt.Errorf("%s: %s", at, fmt.Sprintf(format, args...)))
}

func (ctx *Context) err() error {
	if len(ctx.errs) == 0 {
		return nil
	}
	return errors.Join(ctx.errs...)
}

// Apply runs every patch against the data, in place.
func Apply(spec *Spec, data *sde.Data) (*Context, error) {
	spec.sort()

	ctx := &Context{
		Data: data, Spec: spec,
		Matched: make([][]int32, len(spec.Actions)),
		Applied: map[string][]int32{},
	}
	ctx.index()

	ctx.create()
	ctx.clamp()
	ctx.fillRules()
	ctx.link()

	// Stop on unresolved names; applying would only add follow-on errors.
	if err := ctx.err(); err != nil {
		return ctx, err
	}

	for _, change := range spec.Changes {
		ctx.change(change)
	}
	for _, attribute := range spec.Attributes {
		if attribute.Change != nil {
			ctx.changeAttribute(attribute)
		}
	}
	for _, added := range spec.AddTos() {
		ctx.addTo(added)
	}

	for _, effect := range spec.Effects() {
		matched := ctx.matching(effect.On)
		ctx.Applied[effect.EffectName()] = ids(matched)

		if len(matched) == 0 {
			ctx.errorf(effect.at, "effect %q matches no types", effect.EffectName())
			continue
		}
		ctx.applyEffect(effect, matched)
	}

	for i, action := range spec.Actions {
		matched := ctx.match(action)
		ctx.Matched[i] = ids(matched)

		if len(matched) == 0 {
			ctx.errorf(action.at, "%s: matches no types", action)
			continue
		}
		ctx.act(action, matched)
	}

	return ctx, ctx.err()
}

// clamp links the attributes a definition names as its floor and its cap. It
// runs after create, so one new attribute can be capped by another.
func (ctx *Context) clamp() {
	for _, attribute := range ctx.Spec.Attributes {
		entry := ctx.attributeByName[attribute.Name]
		if entry == nil {
			continue
		}

		for _, definition := range []*Definition{attribute.New, attribute.Change} {
			if definition == nil {
				continue
			}
			if definition.Min != "" {
				entry.MinAttributeID = ctx.clampAgainst(attribute.at, definition.Min)
			}
			if definition.Max != "" {
				entry.MaxAttributeID = ctx.clampAgainst(attribute.at, definition.Max)
			}
		}
	}
}

func (ctx *Context) dogmaCategory(at source, name string) int32 {
	entry, ok := ctx.dogmaCategoryByName[name]
	if !ok {
		ctx.errorf(at, "no dogma attribute category named %q", name)
		return 0
	}
	return entry.Key
}

func (ctx *Context) clampAgainst(at source, name string) int32 {
	entry, ok := ctx.attributeByName[name]
	if !ok {
		ctx.errorf(at, "no dogma attribute named %q to clamp against", name)
		return 0
	}
	return entry.Key
}

func (ctx *Context) index() {
	ctx.attributeByName = map[string]*sde.DogmaAttribute{}
	ctx.dogmaCategoryByName = map[string]*sde.DogmaAttributeCategory{}
	ctx.effectByName = map[string]*sde.DogmaEffect{}
	ctx.categoryByName = map[string]*sde.Category{}
	ctx.groupsByName = map[string][]*sde.Group{}
	ctx.typesByName = map[string][]*sde.Type{}
	ctx.selectorByName = map[string]*NamedSelector{}

	for _, entry := range ctx.Data.DogmaAttributes {
		ctx.attributeByName[entry.Name] = entry
	}
	for _, entry := range ctx.Data.DogmaCategories {
		ctx.dogmaCategoryByName[entry.Name] = entry
	}
	for _, entry := range ctx.Data.DogmaEffects {
		ctx.effectByName[entry.Name] = entry
	}
	for _, entry := range ctx.Data.Categories {
		ctx.categoryByName[entry.Name.En] = entry
	}
	for _, entry := range ctx.Data.Groups {
		ctx.groupsByName[entry.Name.En] = append(ctx.groupsByName[entry.Name.En], entry)
	}
	for _, entry := range ctx.Data.Types {
		ctx.typesByName[entry.Name.En] = append(ctx.typesByName[entry.Name.En], entry)
	}
	for _, selector := range ctx.Spec.Selectors {
		if _, exists := ctx.selectorByName[selector.Name]; exists {
			ctx.errorf(selector.at, "there is already a selector named %q", selector.Name)
			continue
		}
		ctx.selectorByName[selector.Name] = selector
	}

	ctx.sortedTypes = make([]*sde.Type, 0, len(ctx.Data.Types))
	for _, entry := range ctx.Data.Types {
		ctx.sortedTypes = append(ctx.sortedTypes, entry)
	}
	sort.Slice(ctx.sortedTypes, func(i, j int) bool { return ctx.sortedTypes[i].Key < ctx.sortedTypes[j].Key })
}

// create adds every new attribute and effect, with the IDs from ids.yaml.
func (ctx *Context) create() {
	for _, attribute := range ctx.Spec.Attributes {
		if attribute.New == nil {
			if _, exists := ctx.attributeByName[attribute.Name]; !exists {
				ctx.errorf(attribute.at, "no dogma attribute named %q; say \"new:\" to add one", attribute.Name)
			}
			continue
		}
		if _, exists := ctx.attributeByName[attribute.Name]; exists {
			ctx.errorf(attribute.at, "dogma attribute %q already exists; say \"change:\" to edit it", attribute.Name)
			continue
		}

		id, known := ctx.Spec.IDs.Attribute(attribute.Name)
		if !known {
			ctx.errorf(attribute.at, "attribute %q has no ID yet; run \"sde-patched ids\"", attribute.Name)
			continue
		}
		if _, taken := ctx.Data.DogmaAttributes[id]; taken {
			ctx.errorf(attribute.at, "dogma attribute ID %d is already taken", id)
			continue
		}

		entry := &sde.DogmaAttribute{
			Key:          id,
			Name:         attribute.Name,
			DefaultValue: value(attribute.New.Default),
			HighIsGood:   no(attribute.New.HighIsGood),
			Stackable:    yes(attribute.New.Stackable),
			Published:    yes(attribute.New.Published),
			UnitID:       attribute.New.UnitID,
		}
		entry.DisplayName.En = attribute.New.DisplayName
		if attribute.New.Category != "" {
			entry.CategoryID = ctx.dogmaCategory(attribute.at, attribute.New.Category)
		}
		ctx.Data.DogmaAttributes[id] = entry
		ctx.attributeByName[attribute.Name] = entry
	}

	for _, effect := range ctx.Spec.Effects() {
		name := effect.EffectName()
		if _, exists := ctx.effectByName[name]; exists {
			ctx.errorf(effect.at, "dogma effect %q already exists; use \"addTo\" to add rules to it", name)
			continue
		}

		category, err := lookupEnum(effectCategories, "effect category", effect.Category)
		if err != nil {
			ctx.errorf(effect.at, "%s", err)
			continue
		}

		id, known := ctx.Spec.IDs.Effect(name)
		if !known {
			ctx.errorf(effect.at, "effect %q has no ID yet; run \"sde-patched ids\"", name)
			continue
		}
		if _, taken := ctx.Data.DogmaEffects[id]; taken {
			ctx.errorf(effect.at, "dogma effect ID %d is already taken", id)
			continue
		}

		entry := &sde.DogmaEffect{
			Key:              id,
			Name:             name,
			EffectCategoryID: int32(category),
			Published:        effect.Published,
			ElectronicChance: effect.ElectronicChance,
			IsAssistance:     effect.IsAssistance,
			IsOffensive:      effect.IsOffensive,
			IsWarpSafe:       yes(effect.IsWarpSafe),
			PropulsionChance: effect.PropulsionChance,
			RangeChance:      effect.RangeChance,
		}
		entry.DisplayName.En = effect.DisplayName
		ctx.Data.DogmaEffects[id] = entry
		ctx.effectByName[name] = entry
	}
}

// fillRules runs after create, as a rule can read an attribute another file
// has only just added.
func (ctx *Context) fillRules() {
	for _, effect := range ctx.Spec.Effects() {
		entry, ok := ctx.effectByName[effect.EffectName()]
		if !ok {
			continue
		}

		entry.DischargeAttributeID = ctx.attributeID(effect.at, effect.DischargeAttribute)
		entry.DurationAttributeID = ctx.attributeID(effect.at, effect.DurationAttribute)
		entry.FalloffAttributeID = ctx.attributeID(effect.at, effect.FalloffAttribute)
		entry.FittingUsageChanceAttributeID = ctx.attributeID(effect.at, effect.FittingUsageChanceAttribute)
		entry.RangeAttributeID = ctx.attributeID(effect.at, effect.RangeAttribute)
		entry.ResistanceAttributeID = ctx.attributeID(effect.at, effect.ResistanceAttribute)
		entry.TrackingSpeedAttributeID = ctx.attributeID(effect.at, effect.TrackingSpeedAttribute)

		entry.Modifiers = append(entry.Modifiers, ctx.rules(effect.at, effect.Attribute, effect.Rules)...)
	}
}

func (ctx *Context) link() {
	for _, selector := range ctx.Spec.Selectors {
		ctx.linkSelector(selector)
	}
	for _, effect := range ctx.Spec.Effects() {
		if effect.On.tree == nil {
			ctx.errorf(effect.at, "effect %q says nothing about who gets it, which would hit every type", effect.EffectName())
			continue
		}
		effect.On.tree.link(ctx, effect.at)
	}
	for _, action := range ctx.Spec.Actions {
		if action.On.tree == nil {
			ctx.errorf(action.at, "%s: no condition, which would hit every type", action)
			continue
		}
		action.On.tree.link(ctx, action.at)
	}
}

func (ctx *Context) linkSelector(selector *NamedSelector) {
	switch selector.linked {
	case linked:
		return
	case linking:
		ctx.errorf(selector.at, "selector %q refers back to itself", selector.Name)
		selector.linked = linked
		return
	}

	selector.linked = linking
	if selector.Match.tree == nil {
		ctx.errorf(selector.at, "selector %q has no condition", selector.Name)
	} else {
		selector.Match.tree.link(ctx, selector.at)
	}
	selector.linked = linked
}

func (ctx *Context) attributeID(at source, name string) int32 {
	if name == "" {
		return 0
	}
	entry, ok := ctx.attributeByName[name]
	if !ok {
		ctx.errorf(at, "no dogma attribute named %q", name)
		return 0
	}
	return entry.Key
}

func (ctx *Context) rules(at source, writes string, rules []Rule) []sde.Modifier {
	result := make([]sde.Modifier, 0, len(rules))
	for _, rule := range rules {
		domain, err := lookupEnum(modifierDomains, "modifier domain", rule.domain())
		if err != nil {
			ctx.errorf(at, "%s", err)
			continue
		}
		function, err := lookupEnum(modifierFuncs, "modifier func", rule.function())
		if err != nil {
			ctx.errorf(at, "%s", err)
			continue
		}
		operation, err := operationOf(rule.Op)
		if err != nil {
			ctx.errorf(at, "%s", err)
			continue
		}

		result = append(result, sde.Modifier{
			Domain:               domain,
			Func:                 function,
			Operation:            operation,
			ModifiedAttributeID:  ctx.attributeID(at, writes),
			ModifyingAttributeID: ctx.attributeID(at, rule.By),
			GroupID:              ctx.groupID(at, rule.Group),
			SkillTypeID:          ctx.skillID(at, rule.Skill),
		})
	}
	return result
}

func (ctx *Context) groupID(at source, name string) int32 {
	if name == "" {
		return 0
	}
	entries := ctx.groupsByName[name]
	switch len(entries) {
	case 0:
		ctx.errorf(at, "no group named %q", name)
	case 1:
		return entries[0].Key
	default:
		ctx.errorf(at, "group %q is ambiguous: %d groups have that name", name, len(entries))
	}
	return 0
}

func (ctx *Context) skillID(at source, name string) int32 {
	switch {
	case name == "":
		return 0
	case name == AnySkill:
		return -1
	}

	entries := ctx.typesByName[name]
	switch len(entries) {
	case 0:
		ctx.errorf(at, "no type named %q", name)
	case 1:
		return entries[0].Key
	default:
		ctx.errorf(at, "type %q is ambiguous: %d types have that name", name, len(entries))
	}
	return 0
}

func (ctx *Context) match(action *Action) []*sde.Type { return ctx.matching(action.On) }

func (ctx *Context) matching(on Expr) []*sde.Type {
	if on.tree == nil {
		return nil
	}
	var matched []*sde.Type
	for _, entry := range ctx.sortedTypes {
		if on.tree.matches(entry) {
			matched = append(matched, entry)
		}
	}
	return matched
}

func ids(types []*sde.Type) []int32 {
	result := make([]int32, 0, len(types))
	for _, entry := range types {
		result = append(result, entry.Key)
	}
	return result
}
