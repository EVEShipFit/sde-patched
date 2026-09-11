package patch

import (
	"fmt"
	"path/filepath"
	"runtime"
)

// source is where a declaration was made, so errors and "explain" can point at
// the patch that caused them.
type source struct {
	file string
	line int
}

func here(skip int) source {
	_, file, line, ok := runtime.Caller(skip + 1)
	if !ok {
		return source{file: "?"}
	}
	return source{file: filepath.Base(file), line: line}
}

func (s source) String() string {
	return fmt.Sprintf("patches/%s:%d", s.file, s.line)
}

// patchName is the file a declaration lives in, without extension. It is how
// patches are grouped and named; there is nothing else to name them by.
func (s source) patchName() string {
	name := s.file
	if ext := filepath.Ext(name); ext != "" {
		name = name[:len(name)-len(ext)]
	}
	return name
}

// AttributeRef points at a dogma attribute. It either creates one (when made
// with NewAttribute) or finds an existing one by name.
type AttributeRef struct {
	name string
	def  *AttributeDef
	at   source
	id   int32
}

// EffectRef points at a dogma effect, either new or existing.
type EffectRef struct {
	name string
	def  *EffectDef
	at   source
	id   int32
}

// GroupRef points at an existing group, by its English name.
type GroupRef struct {
	name string
	at   source
	id   int32
}

// TypeRef points at an existing type, by its English name.
type TypeRef struct {
	name string
	at   source
	id   int32
}

// NewAttribute declares a dogma attribute that the SDE does not have.
func NewAttribute(name string, def AttributeDef) *AttributeRef {
	ref := &AttributeRef{name: name, def: &def, at: here(1)}
	newAttributes = append(newAttributes, ref)
	return ref
}

// Attribute finds an existing dogma attribute by its internal name.
func Attribute(name string) *AttributeRef {
	ref := &AttributeRef{name: name, at: here(1)}
	lookups = append(lookups, ref)
	return ref
}

// NewEffect declares a dogma effect that the SDE does not have.
func NewEffect(name string, def EffectDef) *EffectRef {
	ref := &EffectRef{name: name, def: &def, at: here(1)}
	newEffects = append(newEffects, ref)
	return ref
}

// Effect finds an existing dogma effect by its internal name.
func Effect(name string) *EffectRef {
	ref := &EffectRef{name: name, at: here(1)}
	lookups = append(lookups, ref)
	return ref
}

// Group finds an existing group by its English name, for use in a
// LocationGroupModifier.
func Group(name string) *GroupRef {
	ref := &GroupRef{name: name, at: here(1)}
	lookups = append(lookups, ref)
	return ref
}

// Skill finds an existing type by its English name, for use in a
// required-skill modifier.
func Skill(name string) *TypeRef {
	ref := &TypeRef{name: name, at: here(1)}
	lookups = append(lookups, ref)
	return ref
}

// AnySkill means "whatever skill this item happens to require": the wildcard
// the dogma-engine knows as skill ID -1.
func AnySkill() *TypeRef {
	return &TypeRef{name: "<any required skill>", at: here(1), id: -1}
}

type resolvable interface {
	resolve(*Context)
	source() source
}

func (r *AttributeRef) source() source { return r.at }
func (r *EffectRef) source() source    { return r.at }
func (r *GroupRef) source() source     { return r.at }
func (r *TypeRef) source() source      { return r.at }

func (r *AttributeRef) resolve(ctx *Context) {
	entry, ok := ctx.attributeByName[r.name]
	if !ok {
		ctx.errorf(r.at, "no dogma attribute named %q", r.name)
		return
	}
	r.id = entry.Key
}

func (r *EffectRef) resolve(ctx *Context) {
	entry, ok := ctx.effectByName[r.name]
	if !ok {
		ctx.errorf(r.at, "no dogma effect named %q", r.name)
		return
	}
	r.id = entry.Key
}

func (r *GroupRef) resolve(ctx *Context) {
	entries := ctx.groupsByName[r.name]
	switch len(entries) {
	case 0:
		ctx.errorf(r.at, "no group named %q", r.name)
	case 1:
		r.id = entries[0].Key
	default:
		ctx.errorf(r.at, "group %q is ambiguous: %d groups have that name", r.name, len(entries))
	}
}

func (r *TypeRef) resolve(ctx *Context) {
	if r.id == -1 {
		return
	}

	entries := ctx.typesByName[r.name]
	switch len(entries) {
	case 0:
		ctx.errorf(r.at, "no type named %q", r.name)
	case 1:
		r.id = entries[0].Key
	default:
		ctx.errorf(r.at, "type %q is ambiguous: %d types have that name", r.name, len(entries))
	}
}

// ID returns the resolved ID. It is only valid after Apply.
func (r *AttributeRef) ID() int32 { return r.id }
func (r *EffectRef) ID() int32    { return r.id }

func (r *AttributeRef) String() string { return r.name }
func (r *EffectRef) String() string    { return r.name }
