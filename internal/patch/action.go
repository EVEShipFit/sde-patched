package patch

import (
	"fmt"
	"strings"

	"github.com/EVEShipFit/sde-patched/internal/sde"
)

type action interface {
	fmt.Stringer

	source() source
	selection() []Selector
	setMatched([]int32)
	matchedIDs() []int32
	apply(*Context, []*sde.Type)
}

// onTypes is shared by every action that works on a set of types.
type onTypes struct {
	at        source
	selectors []Selector
	matched   []int32
}

func (a *onTypes) source() source         { return a.at }
func (a *onTypes) selection() []Selector  { return a.selectors }
func (a *onTypes) setMatched(ids []int32) { a.matched = ids }
func (a *onTypes) matchedIDs() []int32    { return a.matched }

func selectorsString(selectors []Selector) string {
	parts := make([]string, 0, len(selectors))
	for _, selector := range selectors {
		parts = append(parts, selector.String())
	}
	return strings.Join(parts, ", ")
}

// ApplyEffect gives an effect to types. Use On to say which.
func ApplyEffect(ref *EffectRef) *applyEffect {
	a := &applyEffect{ref: ref}
	a.at = here(1)
	actions = append(actions, a)
	return a
}

type applyEffect struct {
	onTypes
	ref       *EffectRef
	isDefault bool
}

// AsDefault marks the effect as one the item runs on its own.
func (a *applyEffect) AsDefault() *applyEffect {
	a.isDefault = true
	return a
}

func (a *applyEffect) On(selectors ...Selector) *applyEffect {
	a.selectors = selectors
	return a
}

func (a *applyEffect) String() string {
	return fmt.Sprintf("apply effect %q on %s", a.ref.name, selectorsString(a.selectors))
}

func (a *applyEffect) apply(ctx *Context, types []*sde.Type) {
	for _, entry := range types {
		for _, effect := range entry.DogmaEffects {
			if effect.EffectID == a.ref.id {
				ctx.errorf(a.at, "type %q already has effect %q", entry.Name.En, a.ref.name)
				return
			}
		}
		entry.DogmaEffects = append(entry.DogmaEffects, sde.TypeDogmaEffect{
			EffectID:  a.ref.id,
			IsDefault: a.isDefault,
		})
	}
}

// RemoveEffect takes an effect away from types. Use From to say which.
func RemoveEffect(ref *EffectRef) *removeEffect {
	a := &removeEffect{ref: ref}
	a.at = here(1)
	actions = append(actions, a)
	return a
}

type removeEffect struct {
	onTypes
	ref *EffectRef
}

func (a *removeEffect) From(selectors ...Selector) *removeEffect {
	a.selectors = selectors
	return a
}

func (a *removeEffect) String() string {
	return fmt.Sprintf("remove effect %q from %s", a.ref.name, selectorsString(a.selectors))
}

func (a *removeEffect) apply(ctx *Context, types []*sde.Type) {
	for _, entry := range types {
		kept := entry.DogmaEffects[:0]
		found := false
		for _, effect := range entry.DogmaEffects {
			if effect.EffectID == a.ref.id {
				found = true
				continue
			}
			kept = append(kept, effect)
		}
		if !found {
			ctx.errorf(a.at, "type %q does not have effect %q", entry.Name.En, a.ref.name)
			return
		}
		entry.DogmaEffects = kept
	}
}

// SetAttribute sets the value of an attribute on types, whether or not they
// already have it. Use On to say which.
func SetAttribute(ref *AttributeRef, value float64) *setAttribute {
	a := &setAttribute{ref: ref, value: value}
	a.at = here(1)
	actions = append(actions, a)
	return a
}

type setAttribute struct {
	onTypes
	ref   *AttributeRef
	value float64
}

func (a *setAttribute) On(selectors ...Selector) *setAttribute {
	a.selectors = selectors
	return a
}

func (a *setAttribute) String() string {
	return fmt.Sprintf("set attribute %q to %g on %s", a.ref.name, a.value, selectorsString(a.selectors))
}

func (a *setAttribute) apply(_ *Context, types []*sde.Type) {
	for _, entry := range types {
		found := false
		for i, attribute := range entry.DogmaAttributes {
			if attribute.AttributeID == a.ref.id {
				entry.DogmaAttributes[i].Value = a.value
				found = true
				break
			}
		}
		if !found {
			entry.DogmaAttributes = append(entry.DogmaAttributes, sde.TypeDogmaAttribute{
				AttributeID: a.ref.id,
				Value:       a.value,
			})
		}
	}
}

// ChangeEffect changes an effect itself, for all types that have it.
func ChangeEffect(ref *EffectRef) *changeEffect {
	a := &changeEffect{ref: ref, at: here(1)}
	changes = append(changes, a)
	return a
}

type changeEffect struct {
	ref     *EffectRef
	at      source
	changes []string

	category  *EffectCategory
	modifiers []Modifier
}

func (a *changeEffect) SetCategory(category EffectCategory) *changeEffect {
	a.category = &category
	a.changes = append(a.changes, "category = "+category.String())
	return a
}

// AddModifiers appends modifiers; it never replaces the ones CCP made.
func (a *changeEffect) AddModifiers(modifiers ...Modifier) *changeEffect {
	a.modifiers = append(a.modifiers, modifiers...)
	a.changes = append(a.changes, fmt.Sprintf("+%d modifier(s)", len(modifiers)))
	return a
}

func (a *changeEffect) source() source { return a.at }

func (a *changeEffect) String() string {
	return fmt.Sprintf("change effect %q: %s", a.ref.name, strings.Join(a.changes, ", "))
}

func (a *changeEffect) apply(ctx *Context) {
	entry, ok := ctx.Data.DogmaEffects[a.ref.id]
	if !ok {
		return
	}

	if a.category != nil {
		entry.EffectCategoryID = int32(*a.category)
	}
	for _, modifier := range a.modifiers {
		entry.Modifiers = append(entry.Modifiers, modifier.toSDE())
	}
}

func ChangeAttribute(ref *AttributeRef) *changeAttribute {
	a := &changeAttribute{ref: ref, at: here(1)}
	changes = append(changes, a)
	return a
}

type changeAttribute struct {
	ref     *AttributeRef
	at      source
	changes []string

	defaultValue *float64
	highIsGood   *bool
	stackable    *bool
}

func (a *changeAttribute) SetDefaultValue(value float64) *changeAttribute {
	a.defaultValue = &value
	a.changes = append(a.changes, fmt.Sprintf("defaultValue = %g", value))
	return a
}

func (a *changeAttribute) SetHighIsGood(value bool) *changeAttribute {
	a.highIsGood = &value
	a.changes = append(a.changes, fmt.Sprintf("highIsGood = %t", value))
	return a
}

func (a *changeAttribute) SetStackable(value bool) *changeAttribute {
	a.stackable = &value
	a.changes = append(a.changes, fmt.Sprintf("stackable = %t", value))
	return a
}

func (a *changeAttribute) source() source { return a.at }

func (a *changeAttribute) String() string {
	return fmt.Sprintf("change attribute %q: %s", a.ref.name, strings.Join(a.changes, ", "))
}

func (a *changeAttribute) apply(ctx *Context) {
	entry, ok := ctx.Data.DogmaAttributes[a.ref.id]
	if !ok {
		return
	}

	if a.defaultValue != nil {
		entry.DefaultValue = *a.defaultValue
	}
	if a.highIsGood != nil {
		entry.HighIsGood = *a.highIsGood
	}
	if a.stackable != nil {
		entry.Stackable = *a.stackable
	}
}

type change interface {
	fmt.Stringer

	source() source
	apply(*Context)
}
