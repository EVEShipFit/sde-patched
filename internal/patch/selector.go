package patch

import (
	"fmt"
	"sort"
	"strings"

	"github.com/EVEShipFit/sde-patched/internal/sde"
)

// Selector is one node of a parsed expression: it says whether a type is in
// or out.
type Selector interface {
	fmt.Stringer

	// link looks the names inside up, now that the SDE is loaded.
	link(*Context, source)
	matches(*sde.Type) bool
}

// callKind is one of the functions an expression can use.
type callKind string

const (
	inCategory   callKind = "category"
	inGroup      callKind = "group"
	named        callKind = "name"
	hasAttribute callKind = "attribute"
	hasEffect    callKind = "effect"
	isPublished  callKind = "published"
)

var callKinds = map[callKind]bool{
	inCategory: true, inGroup: true, named: true,
	hasAttribute: true, hasEffect: true, isPublished: true,
}

// call is every function in one type; they only differ in what they look the
// names up in and which field they compare.
type call struct {
	kind callKind
	args []string

	ids map[int32]bool
}

func newCall(name string, args []string, pos int) (Selector, error) {
	kind := callKind(name)
	if !callKinds[kind] {
		return nil, fmt.Errorf("unknown function %q at position %d, want one of %s", name, pos+1, strings.Join(sortedKeys(callKinds), ", "))
	}
	if kind == isPublished && len(args) != 0 {
		return nil, fmt.Errorf("%s() takes no names", name)
	}
	if kind != isPublished && len(args) == 0 {
		return nil, fmt.Errorf("%s() needs at least one name", name)
	}
	return &call{kind: kind, args: args}, nil
}

func (s *call) String() string {
	quoted := make([]string, 0, len(s.args))
	for _, arg := range s.args {
		quoted = append(quoted, fmt.Sprintf("%q", arg))
	}
	return fmt.Sprintf("%s(%s)", s.kind, strings.Join(quoted, ", "))
}

func (s *call) link(ctx *Context, at source) {
	s.ids = map[int32]bool{}

	for _, name := range s.args {
		switch s.kind {
		case inCategory:
			entry, ok := ctx.categoryByName[name]
			if !ok {
				ctx.errorf(at, "no category named %q", name)
				continue
			}
			s.ids[entry.Key] = true

		case inGroup:
			entries := ctx.groupsByName[name]
			if len(entries) == 0 {
				ctx.errorf(at, "no group named %q", name)
			}
			for _, entry := range entries {
				s.ids[entry.Key] = true
			}

		case named:
			entries := ctx.typesByName[name]
			if len(entries) == 0 {
				ctx.errorf(at, "no type named %q", name)
			}
			for _, entry := range entries {
				s.ids[entry.Key] = true
			}

		case hasAttribute:
			entry, ok := ctx.attributeByName[name]
			if !ok {
				ctx.errorf(at, "no dogma attribute named %q", name)
				continue
			}
			s.ids[entry.Key] = true

		case hasEffect:
			entry, ok := ctx.effectByName[name]
			if !ok {
				ctx.errorf(at, "no dogma effect named %q", name)
				continue
			}
			s.ids[entry.Key] = true
		}
	}
}

func (s *call) matches(entry *sde.Type) bool {
	switch s.kind {
	case inCategory:
		return s.ids[entry.CategoryID]
	case inGroup:
		return s.ids[entry.GroupID]
	case named:
		return s.ids[entry.Key]
	case isPublished:
		return entry.Published

	case hasAttribute:
		for _, attribute := range entry.DogmaAttributes {
			if s.ids[attribute.AttributeID] {
				return true
			}
		}
		return false

	default:
		for _, effect := range entry.DogmaEffects {
			if s.ids[effect.EffectID] {
				return true
			}
		}
		return false
	}
}

// ref is a bare word: the name of a selector declared elsewhere.
type ref struct {
	name   string
	target *NamedSelector
}

func (s *ref) String() string { return s.name }

func (s *ref) link(ctx *Context, at source) {
	target, ok := ctx.selectorByName[s.name]
	if !ok {
		ctx.errorf(at, "no selector named %q", s.name)
		return
	}
	ctx.linkSelector(target)
	s.target = target
}

func (s *ref) matches(entry *sde.Type) bool {
	// Only nil when linking failed, and then Apply has already given up.
	return s.target != nil && s.target.Match.tree.matches(entry)
}

type anyOf struct {
	selectors []Selector
}

func (s *anyOf) String() string { return join(s.selectors, " or ") }

func (s *anyOf) link(ctx *Context, at source) {
	for _, selector := range s.selectors {
		selector.link(ctx, at)
	}
}

func (s *anyOf) matches(entry *sde.Type) bool {
	for _, selector := range s.selectors {
		if selector.matches(entry) {
			return true
		}
	}
	return false
}

type allOf struct {
	selectors []Selector
}

func (s *allOf) String() string { return join(s.selectors, " and ") }

func (s *allOf) link(ctx *Context, at source) {
	for _, selector := range s.selectors {
		selector.link(ctx, at)
	}
}

func (s *allOf) matches(entry *sde.Type) bool {
	for _, selector := range s.selectors {
		if !selector.matches(entry) {
			return false
		}
	}
	return true
}

type not struct {
	selector Selector
}

func (s *not) String() string               { return "not " + s.selector.String() }
func (s *not) link(ctx *Context, at source) { s.selector.link(ctx, at) }
func (s *not) matches(entry *sde.Type) bool { return !s.selector.matches(entry) }

func join(selectors []Selector, sep string) string {
	parts := make([]string, 0, len(selectors))
	for _, selector := range selectors {
		parts = append(parts, selector.String())
	}
	return "(" + strings.Join(parts, sep) + ")"
}

func sortedKeys[K ~string, V any](m map[K]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, string(key))
	}
	sort.Strings(keys)
	return keys
}
