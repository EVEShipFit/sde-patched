package patch

import (
	"fmt"
	"io"
	"sort"
)

const explainLimit = 20

// Patches lists everything that can be explained: one name per attribute, and
// the two files that are not an attribute.
func (ctx *Context) Patches() []string {
	names := make([]string, 0, len(ctx.Spec.Attributes)+2)
	for _, attribute := range ctx.Spec.Attributes {
		names = append(names, attribute.Name)
	}
	if len(ctx.Spec.Selectors) > 0 {
		names = append(names, "selectors")
	}
	if len(ctx.Spec.Changes) > 0 || len(ctx.Spec.Actions) > 0 {
		names = append(names, "effects")
	}
	sort.Strings(names)
	return names
}

// Explain writes what one file declared and what it matched.
func (ctx *Context) Explain(w io.Writer, name string, full bool) error {
	switch name {
	case "selectors":
		return ctx.explainSelectors(w)
	case "effects":
		return ctx.explainEffects(w)
	}

	for _, attribute := range ctx.Spec.Attributes {
		if attribute.Name == name {
			return ctx.explainAttribute(w, attribute, full)
		}
	}
	return fmt.Errorf("nothing named %q", name)
}

// attributeName reads back the name of an attribute a floor or a cap points
// at, falling back to its number when it is gone.
func (ctx *Context) attributeName(id int32) string {
	if entry, ok := ctx.Data.DogmaAttributes[id]; ok {
		return entry.Name
	}
	return fmt.Sprintf("%d", id)
}

func (ctx *Context) explainAttribute(w io.Writer, attribute *Attribute, full bool) error {
	fmt.Fprintf(w, "%s\n\n", attribute.Name)

	entry := ctx.attributeByName[attribute.Name]
	switch {
	case entry == nil:
		fmt.Fprintf(w, "  no such dogma attribute\n")
	case attribute.New != nil:
		fmt.Fprintf(w, "  new attribute  %6d  default %g\n", entry.Key, entry.DefaultValue)
	case attribute.Change != nil:
		fmt.Fprintf(w, "  CCP's          %6d  changed, default %g\n", entry.Key, entry.DefaultValue)
	default:
		fmt.Fprintf(w, "  CCP's          %6d  only filled in\n", entry.Key)
	}

	if entry != nil {
		if entry.MinAttributeID != 0 {
			fmt.Fprintf(w, "  never below            %s\n", ctx.attributeName(entry.MinAttributeID))
		}
		if entry.MaxAttributeID != 0 {
			fmt.Fprintf(w, "  never above            %s\n", ctx.attributeName(entry.MaxAttributeID))
		}
	}

	for _, effect := range attribute.Effects {
		matched := ctx.Applied[effect.EffectName()]
		fmt.Fprintf(w, "\n  %s\n    on %s\n    %d types\n", effect.at, effect.On.Text, len(matched))
		for _, rule := range effect.Rules {
			fmt.Fprintf(w, "      %s %s\n", rule.Op, rule.By)
		}
		ctx.writeTypes(w, matched, full)
	}

	for _, added := range attribute.AddTo {
		fmt.Fprintf(w, "\n  %s\n    %s\n", added.at, added)
	}
	return nil
}

func (ctx *Context) explainSelectors(w io.Writer) error {
	fmt.Fprintf(w, "selectors\n\n")
	for _, selector := range ctx.Spec.Selectors {
		matched, err := ctx.Match(selector.Match.Text)
		if err != nil {
			fmt.Fprintf(w, "  %-28s %s\n", selector.Name, err)
			continue
		}
		fmt.Fprintf(w, "  %-28s %7d types  %s\n", selector.Name, len(matched), selector.Match.Text)
	}
	return nil
}

func (ctx *Context) explainEffects(w io.Writer) error {
	fmt.Fprintf(w, "effects\n\n")
	for _, change := range ctx.Spec.Changes {
		fmt.Fprintf(w, "  %s\n    %s\n", change.at, change)
	}
	for i, action := range ctx.Spec.Actions {
		fmt.Fprintf(w, "  %s\n    %s\n    %d types\n", action.at, action, len(ctx.Matched[i]))
	}
	return nil
}

func (ctx *Context) writeTypes(w io.Writer, matched []int32, full bool) {
	shown := matched
	if !full && len(shown) > explainLimit {
		shown = shown[:explainLimit]
	}

	for _, id := range shown {
		fmt.Fprintf(w, "      %7d  %s\n", id, ctx.Data.Types[id].Name.En)
	}
	if len(shown) < len(matched) {
		fmt.Fprintf(w, "      ... and %d more (use --full)\n", len(matched)-len(shown))
	}
}

// ExplainType writes every attribute a patch works out on a single type.
func (ctx *Context) ExplainType(w io.Writer, name string) error {
	entries := ctx.typesByName[name]
	if len(entries) == 0 {
		return fmt.Errorf("no type named %q", name)
	}

	for _, entry := range entries {
		group := ctx.Data.Groups[entry.GroupID]
		category := ctx.Data.Categories[entry.CategoryID]
		fmt.Fprintf(w, "%s (type %d, group %q, category %q)\n\n", entry.Name.En, entry.Key, group.Name.En, category.Name.En)

		hits := 0
		for _, effect := range ctx.Spec.Effects() {
			if !contains(ctx.Applied[effect.EffectName()], entry.Key) {
				continue
			}
			hits++
			fmt.Fprintf(w, "  %-32s %s\n    on %s\n", effect.Attribute, effect.at, effect.On.Text)
		}
		for i, action := range ctx.Spec.Actions {
			if !contains(ctx.Matched[i], entry.Key) {
				continue
			}
			hits++
			fmt.Fprintf(w, "  %-32s %s\n    %s\n", action.at.patchName(), action.at, action)
		}
		if hits == 0 {
			fmt.Fprintf(w, "  no patch touches this type\n")
		}
		fmt.Fprintln(w)
	}
	return nil
}

func contains(ids []int32, id int32) bool {
	index := sort.Search(len(ids), func(i int) bool { return ids[i] >= id })
	return index < len(ids) && ids[index] == id
}
