package patch

import (
	"fmt"
	"strings"

	"github.com/EVEShipFit/sde-patched/internal/sde"
)

// Selector picks the types an action works on. Handing several to On means
// all of them have to hold; use Any for "one of these".
type Selector interface {
	fmt.Stringer

	prepare(*Context)
	matches(*Context, *sde.Type) bool
}

// InCategory matches types whose category has one of these English names.
func InCategory(names ...string) Selector {
	return &idSelector{kind: "InCategory", names: names, at: here(1)}
}

// InGroup matches types whose group has one of these English names.
func InGroup(names ...string) Selector {
	return &idSelector{kind: "InGroup", names: names, at: here(1)}
}

// Named matches types by their English name.
func Named(names ...string) Selector {
	return &idSelector{kind: "Named", names: names, at: here(1)}
}

// HasAttribute matches types that already carry the attribute.
func HasAttribute(ref *AttributeRef) Selector {
	return &hasAttribute{ref: ref}
}

// HasEffect matches types that already carry the effect.
func HasEffect(ref *EffectRef) Selector {
	return &hasEffect{ref: ref}
}

func IsPublished() Selector {
	return &isPublished{}
}

// As gives a selector a name, so that "explain" shows that instead of the
// whole nest of conditions.
func As(name string, selector Selector) Selector {
	return &named{name: name, selector: selector}
}

type named struct {
	name     string
	selector Selector
}

func (s *named) String() string                             { return s.name }
func (s *named) prepare(ctx *Context)                       { s.selector.prepare(ctx) }
func (s *named) matches(ctx *Context, entry *sde.Type) bool { return s.selector.matches(ctx, entry) }

// All matches when every selector matches. On already does this for the
// selectors you hand it; All is for nesting inside Any.
func All(selectors ...Selector) Selector {
	return &allOf{selectors: selectors}
}

// Any matches when at least one of the selectors matches.
func Any(selectors ...Selector) Selector {
	return &anyOf{selectors: selectors}
}

func Not(selector Selector) Selector {
	return &not{selector: selector}
}

// idSelector covers the three name-based selectors; they only differ in what
// they look the name up in.
type idSelector struct {
	kind  string
	names []string
	at    source

	ids map[int32]bool
}

func (s *idSelector) String() string {
	return fmt.Sprintf("%s(%q)", s.kind, strings.Join(s.names, `", "`))
}

func (s *idSelector) prepare(ctx *Context) {
	s.ids = map[int32]bool{}

	for _, name := range s.names {
		switch s.kind {
		case "InCategory":
			entry, ok := ctx.categoryByName[name]
			if !ok {
				ctx.errorf(s.at, "no category named %q", name)
				continue
			}
			s.ids[entry.Key] = true
		case "InGroup":
			entries := ctx.groupsByName[name]
			if len(entries) == 0 {
				ctx.errorf(s.at, "no group named %q", name)
				continue
			}
			for _, entry := range entries {
				s.ids[entry.Key] = true
			}
		case "Named":
			entries := ctx.typesByName[name]
			if len(entries) == 0 {
				ctx.errorf(s.at, "no type named %q", name)
				continue
			}
			for _, entry := range entries {
				s.ids[entry.Key] = true
			}
		}
	}
}

func (s *idSelector) matches(_ *Context, entry *sde.Type) bool {
	switch s.kind {
	case "InCategory":
		return s.ids[entry.CategoryID]
	case "InGroup":
		return s.ids[entry.GroupID]
	default:
		return s.ids[entry.Key]
	}
}

type hasAttribute struct {
	ref *AttributeRef
}

func (s *hasAttribute) String() string     { return fmt.Sprintf("HasAttribute(%q)", s.ref.name) }
func (s *hasAttribute) prepare(_ *Context) {}
func (s *hasAttribute) matches(_ *Context, entry *sde.Type) bool {
	for _, attribute := range entry.DogmaAttributes {
		if attribute.AttributeID == s.ref.id {
			return true
		}
	}
	return false
}

type hasEffect struct {
	ref *EffectRef
}

func (s *hasEffect) String() string     { return fmt.Sprintf("HasEffect(%q)", s.ref.name) }
func (s *hasEffect) prepare(_ *Context) {}
func (s *hasEffect) matches(_ *Context, entry *sde.Type) bool {
	for _, effect := range entry.DogmaEffects {
		if effect.EffectID == s.ref.id {
			return true
		}
	}
	return false
}

type isPublished struct{}

func (s *isPublished) String() string                           { return "IsPublished()" }
func (s *isPublished) prepare(_ *Context)                       {}
func (s *isPublished) matches(_ *Context, entry *sde.Type) bool { return entry.Published }

type anyOf struct {
	selectors []Selector
}

func (s *anyOf) String() string {
	parts := make([]string, 0, len(s.selectors))
	for _, selector := range s.selectors {
		parts = append(parts, selector.String())
	}
	return "Any(" + strings.Join(parts, ", ") + ")"
}

func (s *anyOf) prepare(ctx *Context) {
	for _, selector := range s.selectors {
		selector.prepare(ctx)
	}
}

func (s *anyOf) matches(ctx *Context, entry *sde.Type) bool {
	for _, selector := range s.selectors {
		if selector.matches(ctx, entry) {
			return true
		}
	}
	return false
}

type allOf struct {
	selectors []Selector
}

func (s *allOf) String() string {
	parts := make([]string, 0, len(s.selectors))
	for _, selector := range s.selectors {
		parts = append(parts, selector.String())
	}
	return "All(" + strings.Join(parts, ", ") + ")"
}

func (s *allOf) prepare(ctx *Context) {
	for _, selector := range s.selectors {
		selector.prepare(ctx)
	}
}

func (s *allOf) matches(ctx *Context, entry *sde.Type) bool {
	for _, selector := range s.selectors {
		if !selector.matches(ctx, entry) {
			return false
		}
	}
	return true
}

type not struct {
	selector Selector
}

func (s *not) String() string       { return "Not(" + s.selector.String() + ")" }
func (s *not) prepare(ctx *Context) { s.selector.prepare(ctx) }
func (s *not) matches(ctx *Context, entry *sde.Type) bool {
	return !s.selector.matches(ctx, entry)
}
