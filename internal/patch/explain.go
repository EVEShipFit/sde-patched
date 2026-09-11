package patch

import (
	"fmt"
	"io"
	"sort"
)

const explainLimit = 20

// Patches lists the name of every patch that declared something.
func (ctx *Context) Patches() []string {
	seen := map[string]bool{}
	for _, ref := range newAttributes {
		seen[ref.at.patchName()] = true
	}
	for _, ref := range newEffects {
		seen[ref.at.patchName()] = true
	}
	for _, item := range ctx.Changes {
		seen[item.source().patchName()] = true
	}
	for _, item := range ctx.Actions {
		seen[item.source().patchName()] = true
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Explain writes what a single patch declared and what it matched.
func (ctx *Context) Explain(w io.Writer, name string, full bool) error {
	found := false
	for _, patch := range ctx.Patches() {
		if patch == name {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("no patch named %q", name)
	}

	fmt.Fprintf(w, "%s\n\n", name)

	for _, ref := range newAttributes {
		if ref.at.patchName() == name {
			fmt.Fprintf(w, "  new attribute  %6d  %s\n", ref.id, ref.name)
		}
	}
	for _, ref := range newEffects {
		if ref.at.patchName() == name {
			fmt.Fprintf(w, "  new effect     %6d  %s\n", ref.id, ref.name)
		}
	}

	for _, item := range ctx.Changes {
		if item.source().patchName() != name {
			continue
		}
		fmt.Fprintf(w, "\n  %s\n    %s\n", item.source(), item)
	}

	for _, item := range ctx.Actions {
		if item.source().patchName() != name {
			continue
		}

		matched := item.matchedIDs()
		fmt.Fprintf(w, "\n  %s\n    %s\n    %d types\n", item.source(), item, len(matched))
		ctx.writeTypes(w, matched, full)
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

// ExplainType writes every patch that touched a single type.
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
		for _, item := range ctx.Actions {
			if !contains(item.matchedIDs(), entry.Key) {
				continue
			}
			hits++
			fmt.Fprintf(w, "  %-24s %s\n    %s\n", item.source().patchName(), item.source(), item)
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
