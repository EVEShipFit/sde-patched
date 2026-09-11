package patch

import (
	"errors"
	"fmt"
	"sort"

	"github.com/EVEShipFit/sde-patched/internal/sde"
)

// Everything declared by the patches package lands in these, in whatever order
// Go initialises the variables. Apply sorts them by file and line, so the
// result never depends on that order.
var (
	newAttributes []*AttributeRef
	newEffects    []*EffectRef
	lookups       []resolvable
	changes       []change
	actions       []action
)

// Context is the SDE plus the bookkeeping needed to explain what happened.
type Context struct {
	Data *sde.Data

	Changes []change
	Actions []action

	attributeByName map[string]*sde.DogmaAttribute
	effectByName    map[string]*sde.DogmaEffect
	categoryByName  map[string]*sde.Category
	groupsByName    map[string][]*sde.Group
	typesByName     map[string][]*sde.Type

	sortedTypes []*sde.Type
	errs        []error
}

func (ctx *Context) errorf(at source, format string, args ...any) {
	ctx.errs = append(ctx.errs, fmt.Errorf("%s: %s", at, fmt.Sprintf(format, args...)))
}

// Apply runs every declared patch against the data, in place.
func Apply(data *sde.Data) (*Context, error) {
	ctx := &Context{Data: data, Changes: changes, Actions: actions}
	ctx.index()

	ctx.create()
	for _, ref := range lookups {
		ref.resolve(ctx)
	}
	ctx.fillModifiers()

	// Stop here on unresolved names; applying anything now would only pile
	// confusing errors on top of clear ones.
	if err := ctx.err(); err != nil {
		return ctx, err
	}

	sort.SliceStable(ctx.Changes, func(i, j int) bool { return before(ctx.Changes[i].source(), ctx.Changes[j].source()) })
	sort.SliceStable(ctx.Actions, func(i, j int) bool { return before(ctx.Actions[i].source(), ctx.Actions[j].source()) })

	for _, item := range ctx.Changes {
		item.apply(ctx)
	}

	for _, item := range ctx.Actions {
		if len(item.selection()) == 0 {
			ctx.errorf(item.source(), "%s: no selectors, which would hit every type", item)
			continue
		}

		for _, selector := range item.selection() {
			selector.prepare(ctx)
		}

		matched := ctx.match(item.selection())
		item.setMatched(ids(matched))

		if len(matched) == 0 {
			ctx.errorf(item.source(), "%s: matches no types", item)
			continue
		}
		item.apply(ctx, matched)
	}

	return ctx, ctx.err()
}

func before(a, b source) bool {
	if a.file != b.file {
		return a.file < b.file
	}
	return a.line < b.line
}

func (ctx *Context) err() error {
	if len(ctx.errs) == 0 {
		return nil
	}
	return errors.Join(ctx.errs...)
}

func (ctx *Context) index() {
	ctx.attributeByName = map[string]*sde.DogmaAttribute{}
	ctx.effectByName = map[string]*sde.DogmaEffect{}
	ctx.categoryByName = map[string]*sde.Category{}
	ctx.groupsByName = map[string][]*sde.Group{}
	ctx.typesByName = map[string][]*sde.Type{}

	for _, entry := range ctx.Data.DogmaAttributes {
		ctx.attributeByName[entry.Name] = entry
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

	ctx.sortedTypes = make([]*sde.Type, 0, len(ctx.Data.Types))
	for _, entry := range ctx.Data.Types {
		ctx.sortedTypes = append(ctx.sortedTypes, entry)
	}
	sort.Slice(ctx.sortedTypes, func(i, j int) bool { return ctx.sortedTypes[i].Key < ctx.sortedTypes[j].Key })
}

// create adds all new attributes and effects. IDs are handed out in file and
// line order, and are negative: that makes it obvious they are not CCP's.
func (ctx *Context) create() {
	sort.SliceStable(newAttributes, func(i, j int) bool { return before(newAttributes[i].at, newAttributes[j].at) })
	sort.SliceStable(newEffects, func(i, j int) bool { return before(newEffects[i].at, newEffects[j].at) })

	nextID := int32(-1)
	for _, ref := range newAttributes {
		if _, exists := ctx.attributeByName[ref.name]; exists {
			ctx.errorf(ref.at, "dogma attribute %q already exists", ref.name)
			continue
		}

		ref.id = ref.def.ID
		if ref.id == 0 {
			ref.id = nextID
			nextID--
		}
		if _, exists := ctx.Data.DogmaAttributes[ref.id]; exists {
			ctx.errorf(ref.at, "dogma attribute ID %d is already taken", ref.id)
			continue
		}

		entry := &sde.DogmaAttribute{
			Key:          ref.id,
			Name:         ref.name,
			DefaultValue: ref.def.DefaultValue,
			HighIsGood:   ref.def.HighIsGood,
			Stackable:    ref.def.Stackable,
			Published:    ref.def.Published,
			UnitID:       ref.def.UnitID,
		}
		entry.DisplayName.En = ref.def.DisplayName
		ctx.Data.DogmaAttributes[ref.id] = entry
		ctx.attributeByName[ref.name] = entry
	}

	nextID = int32(-1)
	for _, ref := range newEffects {
		if _, exists := ctx.effectByName[ref.name]; exists {
			ctx.errorf(ref.at, "dogma effect %q already exists", ref.name)
			continue
		}

		ref.id = ref.def.ID
		if ref.id == 0 {
			ref.id = nextID
			nextID--
		}
		if _, exists := ctx.Data.DogmaEffects[ref.id]; exists {
			ctx.errorf(ref.at, "dogma effect ID %d is already taken", ref.id)
			continue
		}

		entry := &sde.DogmaEffect{
			Key:              ref.id,
			Name:             ref.name,
			EffectCategoryID: int32(ref.def.Category),
			Published:        ref.def.Published,
			ElectronicChance: ref.def.ElectronicChance,
			IsAssistance:     ref.def.IsAssistance,
			IsOffensive:      ref.def.IsOffensive,
			IsWarpSafe:       ref.def.IsWarpSafe,
			PropulsionChance: ref.def.PropulsionChance,
			RangeChance:      ref.def.RangeChance,
		}
		entry.DisplayName.En = ref.def.DisplayName
		ctx.Data.DogmaEffects[ref.id] = entry
		ctx.effectByName[ref.name] = entry
	}
}

// fillModifiers runs after every reference is resolved, as modifiers point at
// attributes, groups and skills by name.
func (ctx *Context) fillModifiers() {
	for _, ref := range newEffects {
		entry, ok := ctx.Data.DogmaEffects[ref.id]
		if !ok {
			continue
		}

		entry.DischargeAttributeID = attributeID(ref.def.DischargeAttribute)
		entry.DurationAttributeID = attributeID(ref.def.DurationAttribute)
		entry.FalloffAttributeID = attributeID(ref.def.FalloffAttribute)
		entry.FittingUsageChanceAttributeID = attributeID(ref.def.FittingUsageChanceAttribute)
		entry.RangeAttributeID = attributeID(ref.def.RangeAttribute)
		entry.ResistanceAttributeID = attributeID(ref.def.ResistanceAttribute)
		entry.TrackingSpeedAttributeID = attributeID(ref.def.TrackingSpeedAttribute)

		for _, modifier := range ref.def.Modifiers {
			entry.Modifiers = append(entry.Modifiers, modifier.toSDE())
		}
	}
}

func attributeID(ref *AttributeRef) int32 {
	if ref == nil {
		return 0
	}
	return ref.id
}

func (m Modifier) toSDE() sde.Modifier {
	modifier := sde.Modifier{
		Domain:               m.Domain,
		Func:                 m.Func,
		Operation:            m.Operation,
		ModifiedAttributeID:  attributeID(m.Modified),
		ModifyingAttributeID: attributeID(m.Modifying),
	}
	if m.Group != nil {
		modifier.GroupID = m.Group.id
	}
	if m.Skill != nil {
		modifier.SkillTypeID = m.Skill.id
	}
	return modifier
}

func (ctx *Context) match(selectors []Selector) []*sde.Type {
	var matched []*sde.Type

	for _, entry := range ctx.sortedTypes {
		hit := true
		for _, selector := range selectors {
			if !selector.matches(ctx, entry) {
				hit = false
				break
			}
		}
		if hit {
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
