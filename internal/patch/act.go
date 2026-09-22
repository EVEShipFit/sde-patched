package patch

import (
	"errors"
	"fmt"

	"github.com/EVEShipFit/sde-patched/internal/sde"
)

func (ctx *Context) act(action *Action, types []*sde.Type) {
	switch {
	case action.RemoveEffect != "":
		ctx.removeEffect(action, types)
	case action.SetAttribute != "":
		ctx.setAttribute(action, types)
	}
}

func (ctx *Context) applyEffect(effect *Effect, types []*sde.Type) {
	entry, ok := ctx.effectByName[effect.EffectName()]
	if !ok {
		return
	}

	for _, item := range types {
		for _, has := range item.DogmaEffects {
			if has.EffectID == entry.Key {
				ctx.errorf(effect.at, "type %q already has effect %q", item.Name.En, effect.EffectName())
				return
			}
		}
		item.DogmaEffects = append(item.DogmaEffects, sde.TypeDogmaEffect{
			EffectID:  entry.Key,
			IsDefault: effect.AsDefault,
		})
	}
}

func (ctx *Context) removeEffect(action *Action, types []*sde.Type) {
	entry, ok := ctx.effectByName[action.RemoveEffect]
	if !ok {
		ctx.errorf(action.at, "no dogma effect named %q", action.RemoveEffect)
		return
	}

	for _, item := range types {
		kept := item.DogmaEffects[:0]
		found := false
		for _, effect := range item.DogmaEffects {
			if effect.EffectID == entry.Key {
				found = true
				continue
			}
			kept = append(kept, effect)
		}
		if !found {
			ctx.errorf(action.at, "type %q does not have effect %q", item.Name.En, action.RemoveEffect)
			return
		}
		item.DogmaEffects = kept
	}
}

func (ctx *Context) setAttribute(action *Action, types []*sde.Type) {
	entry, ok := ctx.attributeByName[action.SetAttribute]
	if !ok {
		ctx.errorf(action.at, "no dogma attribute named %q", action.SetAttribute)
		return
	}

	for _, item := range types {
		found := false
		for i, attribute := range item.DogmaAttributes {
			if attribute.AttributeID == entry.Key {
				item.DogmaAttributes[i].Value = action.Value
				found = true
				break
			}
		}
		if !found {
			item.DogmaAttributes = append(item.DogmaAttributes, sde.TypeDogmaAttribute{
				AttributeID: entry.Key,
				Value:       action.Value,
			})
		}
	}
}

// change edits an effect of CCP's in a way no single attribute owns.
func (ctx *Context) change(change *Change) {
	entry, ok := ctx.effectByName[change.Effect]
	if !ok {
		ctx.errorf(change.at, "no dogma effect named %q", change.Effect)
		return
	}

	if change.Category != nil {
		category, err := lookupEnum(effectCategories, "effect category", *change.Category)
		if err != nil {
			ctx.errorf(change.at, "%s", err)
		} else {
			entry.EffectCategoryID = int32(category)
		}
	}
}

// addTo appends rules to an effect CCP already has.
func (ctx *Context) addTo(added *AddTo) {
	entry, ok := ctx.effectByName[added.Effect]
	if !ok {
		ctx.errorf(added.at, "no dogma effect named %q", added.Effect)
		return
	}
	entry.Modifiers = append(entry.Modifiers, ctx.rules(added.at, added.Attribute, added.Rules)...)
}

// changeAttribute edits what one of CCP's attributes is, which an attribute
// file says with "change".
func (ctx *Context) changeAttribute(attribute *Attribute) {
	entry, ok := ctx.attributeByName[attribute.Name]
	if !ok {
		ctx.errorf(attribute.at, "no dogma attribute named %q", attribute.Name)
		return
	}

	edit := attribute.Change
	if edit.Default != nil {
		entry.DefaultValue = *edit.Default
	}
	if edit.HighIsGood != nil {
		entry.HighIsGood = *edit.HighIsGood
	}
	if edit.Stackable != nil {
		entry.Stackable = *edit.Stackable
	}
	if edit.DisplayName != "" {
		entry.DisplayName.En = edit.DisplayName
	}
	if edit.UnitID != 0 {
		entry.UnitID = edit.UnitID
	}
	if edit.Category != "" {
		entry.CategoryID = ctx.dogmaCategory(attribute.at, edit.Category)
	}
}

// Validate checks what can be checked without the SDE: that every declaration
// says one thing, and that its names are spelled right. Apply does the rest,
// once the names can be looked up.
func (spec *Spec) Validate() error {
	var errs []error
	report := func(at source, format string, args ...any) {
		errs = append(errs, fmt.Errorf("%s: %s", at, fmt.Sprintf(format, args...)))
	}

	for _, action := range spec.Actions {
		if count(action.RemoveEffect, action.SetAttribute) != 1 {
			report(action.at, "an action needs exactly one of removeEffect or setAttribute")
		}
	}
	for _, change := range spec.Changes {
		if change.Effect == "" {
			report(change.at, "a change needs an effect to change")
		}
	}

	seen := map[string]source{}
	for _, attribute := range spec.Attributes {
		if attribute.New != nil && attribute.Change != nil {
			report(attribute.at, "%q says both new and change; it is one or the other", attribute.Name)
		}

		// A single effect defaults to the attribute's name; with more, each
		// must be named.
		for i, effect := range attribute.Effects {
			if effect.Name == "" && len(attribute.Effects) > 1 {
				report(effect.at, "%q declares %d effects, so effect %d needs a name of its own",
					attribute.Name, len(attribute.Effects), i+1)
			}
			if effect.Category == "" {
				report(effect.at, "effect %q needs a category: passive, active, online or one of the rest", effect.EffectName())
			}
			if was, taken := seen[effect.EffectName()]; taken {
				report(effect.at, "there is already an effect named %q, at %s", effect.EffectName(), was)
			}
			seen[effect.EffectName()] = effect.at
		}

		for _, added := range attribute.AddTo {
			if added.Effect == "" {
				report(added.at, "%q adds rules to an effect without saying which", attribute.Name)
			}
		}
	}

	return errors.Join(errs...)
}

func count(values ...string) int {
	set := 0
	for _, value := range values {
		if value != "" {
			set++
		}
	}
	return set
}

func (a *Action) String() string {
	switch {
	case a.RemoveEffect != "":
		return fmt.Sprintf("remove effect %q from %s", a.RemoveEffect, a.On.Text)
	case a.SetAttribute != "":
		return fmt.Sprintf("set attribute %q to %g on %s", a.SetAttribute, a.Value, a.On.Text)
	default:
		return "an action that does nothing"
	}
}

func (c *Change) String() string {
	if c.Category != nil {
		return fmt.Sprintf("change effect %q to category %s", c.Effect, *c.Category)
	}
	return fmt.Sprintf("change effect %q", c.Effect)
}

func (a *AddTo) String() string {
	return fmt.Sprintf("add %d rule(s) for %q to effect %q", len(a.Rules), a.Attribute, a.Effect)
}
