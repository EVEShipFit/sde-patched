package patch

import (
	"errors"

	"github.com/EVEShipFit/sde-patched/internal/fbs/eve"
)

// EnumName is the word a patch writes an enum value as, for reading back what
// is already in the SDE.
func EnumName(kind string, value int32) string {
	switch kind {
	case "effectCategory":
		return effectCategoryNames[eve.EffectCategory(value)]
	case "modifierDomain":
		return modifierDomainNames[eve.ModifierDomain(value)]
	case "modifierFunc":
		return modifierFuncNames[eve.ModifierFunc(value)]
	case "modifierOperation":
		return modifierOpNames[eve.ModifierOperation(value)]
	}
	return ""
}

// Enums lists the words a patch may use where the flatbuffer wants an enum,
// for an editor to offer as choices.
func Enums() map[string][]string {
	return map[string][]string{
		"effectCategory":    sortedKeys(effectCategories),
		"modifierDomain":    sortedKeys(modifierDomains),
		"modifierFunc":      sortedKeys(modifierFuncs),
		"modifierOperation": sortedKeys(modifierOps),
	}
}

func (s *NamedSelector) At() string { return s.at.String() }
func (a *Attribute) At() string     { return a.at.String() }
func (e *Effect) At() string        { return e.at.String() }
func (a *AddTo) At() string         { return a.at.String() }
func (c *Change) At() string        { return c.at.String() }
func (a *Action) At() string        { return a.at.String() }

func (s *NamedSelector) Patch() string { return s.at.patchName() }
func (a *Attribute) Patch() string     { return a.at.patchName() }
func (e *Effect) Patch() string        { return e.at.patchName() }
func (a *AddTo) Patch() string         { return a.at.patchName() }
func (c *Change) Patch() string        { return c.at.patchName() }
func (a *Action) Patch() string        { return a.at.patchName() }

// AttributeID is the ID an attribute ended up with, which is only the one in
// the YAML when it was pinned there.
func (ctx *Context) AttributeID(name string) int32 {
	if entry, ok := ctx.attributeByName[name]; ok {
		return entry.Key
	}
	return 0
}

func (ctx *Context) EffectID(name string) int32 {
	if entry, ok := ctx.effectByName[name]; ok {
		return entry.Key
	}
	return 0
}

// Match runs an expression that is not in any patch, so a filter can be tried
// before it is written down. The IDs come back sorted.
func (ctx *Context) Match(text string) ([]int32, error) {
	tree, err := parse(text)
	if err != nil {
		return nil, err
	}

	// Linking reports through the context; keep those errors out of the run
	// that actually produced the data.
	saved := ctx.errs
	ctx.errs = nil
	tree.link(ctx, source{})
	found := ctx.errs
	ctx.errs = saved

	if err := errors.Join(found...); err != nil {
		return nil, err
	}

	var matched []int32
	for _, entry := range ctx.sortedTypes {
		if tree.matches(entry) {
			matched = append(matched, entry.Key)
		}
	}
	return matched, nil
}
