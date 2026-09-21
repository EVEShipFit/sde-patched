// Package patch reads the patches and applies them to the dogma data of the
// SDE.
//
// One attribute is one file, in patches/attributes. The file is named after
// the attribute, so the name is never written inside it, and every rule in it
// writes that attribute and nothing else.
//
// Three files are not an attribute. patches/selectors.yaml holds the filters
// more than one attribute needs, patches/effects.yaml holds the handful of
// changes that belong to an effect of CCP's rather than to any one value, and
// patches/units.yaml holds the units the SDE has none of.
//
// Nothing is looked up while reading; a file only writes down the intent, and
// Apply resolves it against the SDE.
package patch

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"unicode"

	"github.com/EVEShipFit/sde-patched/internal/fbs/eve"

	"gopkg.in/yaml.v3"
)

// Spec is everything the patches declare, gathered from every file, plus the
// IDs that have already been handed out.
type Spec struct {
	Units      []*Unit
	Selectors  []*NamedSelector
	Attributes []*Attribute
	Changes    []*Change
	Actions    []*Action

	IDs *IDs
}

// Effects is every effect the patches declare, in file and line order. They
// belong to the attribute whose file they are written in.
func (spec *Spec) Effects() []*Effect {
	var found []*Effect
	for _, attribute := range spec.Attributes {
		found = append(found, attribute.Effects...)
	}
	return found
}

// AddTos is every set of rules appended to an effect of CCP's.
func (spec *Spec) AddTos() []*AddTo {
	var found []*AddTo
	for _, attribute := range spec.Attributes {
		found = append(found, attribute.AddTo...)
	}
	return found
}

// unitFile is patches/units.yaml.
type unitFile struct {
	Units []*Unit `yaml:"units,omitempty" json:"units"`
}

// Unit is a dogma unit the SDE has no answer for, such as a rate. It is only
// ever a label: nothing converts by it.
type Unit struct {
	at source

	Name        string `yaml:"name" json:"name"`
	DisplayName string `yaml:"displayName" json:"displayName"`
}

// selectorFile is patches/selectors.yaml.
type selectorFile struct {
	Selectors []*NamedSelector `yaml:"selectors,omitempty" json:"selectors"`
}

// effectFile is patches/effects.yaml: what belongs to an effect of CCP's
// rather than to any one attribute.
type effectFile struct {
	Changes []*Change `yaml:"changes,omitempty" json:"changes"`
	Actions []*Action `yaml:"actions,omitempty" json:"actions"`
}

// NamedSelector is a filter that more than one attribute needs.
type NamedSelector struct {
	at source

	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description,omitempty" json:"description"`
	Match       Expr   `yaml:"match" json:"match"`

	linked linkState
}

// Attribute is one file in patches/attributes: what the attribute is, and
// every rule that fills it in.
//
// New says it is not in CCP's data; change says it is, and edits it. A file
// with neither only writes into one of CCP's without touching what it is.
type Attribute struct {
	at source

	// Name is the file it was read from. It is not written inside the file.
	Name string `yaml:"-" json:"name"`

	New    *Definition `yaml:"new,omitempty" json:"new"`
	Change *Definition `yaml:"change,omitempty" json:"change"`

	Effects []*Effect `yaml:"effects,omitempty" json:"effects"`
	AddTo   []*AddTo  `yaml:"addTo,omitempty" json:"addTo"`
}

// Definition is what an attribute is, apart from the sum that fills it in.
// Every field is a pointer, because leaving one out of a change has to mean
// "leave it alone" rather than "make it zero".
type Definition struct {
	DisplayName string   `yaml:"displayName,omitempty" json:"displayName"`
	Category    string   `yaml:"category,omitempty" json:"category"`
	Unit        string   `yaml:"unit,omitempty" json:"unit"`
	Default     *float64 `yaml:"default,omitempty" json:"default"`
	HighIsGood  *bool    `yaml:"highIsGood,omitempty" json:"highIsGood"`
	Stackable   *bool    `yaml:"stackable,omitempty" json:"stackable"`
	Published   *bool    `yaml:"published,omitempty" json:"published"`
	Min         string   `yaml:"min,omitempty" json:"min"`
	Max         string   `yaml:"max,omitempty" json:"max"`
}

func value(number *float64) float64 {
	if number == nil {
		return 0
	}
	return *number
}

func no(flag *bool) bool  { return flag != nil && *flag }
func yes(flag *bool) bool { return flag == nil || *flag }

// Effect is a set of rules and the types that get them. What it runs on is
// part of the effect, because an effect is only ever given out whole.
type Effect struct {
	at source

	// Attribute is the file it was read from. Every rule in the effect writes
	// that attribute.
	Attribute string `yaml:"-" json:"attribute"`

	// Name is the attribute's name unless the file declares more than one
	// effect, in which case each has to say which it is.
	Name        string `yaml:"name,omitempty" json:"name"`
	On          Expr   `yaml:"on" json:"on"`
	DisplayName string `yaml:"displayName,omitempty" json:"displayName"`
	Category    string `yaml:"category" json:"category"`

	// AsDefault marks the effect as one the item runs on its own.
	AsDefault bool `yaml:"asDefault,omitempty" json:"asDefault"`

	Published        bool  `yaml:"published,omitempty" json:"published"`
	ElectronicChance bool  `yaml:"electronicChance,omitempty" json:"electronicChance"`
	IsAssistance     bool  `yaml:"isAssistance,omitempty" json:"isAssistance"`
	IsOffensive      bool  `yaml:"isOffensive,omitempty" json:"isOffensive"`
	IsWarpSafe       *bool `yaml:"isWarpSafe,omitempty" json:"isWarpSafe"`
	PropulsionChance bool  `yaml:"propulsionChance,omitempty" json:"propulsionChance"`
	RangeChance      bool  `yaml:"rangeChance,omitempty" json:"rangeChance"`

	DischargeAttribute          string `yaml:"dischargeAttribute,omitempty" json:"dischargeAttribute"`
	DurationAttribute           string `yaml:"durationAttribute,omitempty" json:"durationAttribute"`
	FalloffAttribute            string `yaml:"falloffAttribute,omitempty" json:"falloffAttribute"`
	FittingUsageChanceAttribute string `yaml:"fittingUsageChanceAttribute,omitempty" json:"fittingUsageChanceAttribute"`
	RangeAttribute              string `yaml:"rangeAttribute,omitempty" json:"rangeAttribute"`
	ResistanceAttribute         string `yaml:"resistanceAttribute,omitempty" json:"resistanceAttribute"`
	TrackingSpeedAttribute      string `yaml:"trackingSpeedAttribute,omitempty" json:"trackingSpeedAttribute"`

	Rules []Rule `yaml:"rules,omitempty" json:"rules"`
}

// EffectName is the name the effect ends up with.
func (e *Effect) EffectName() string {
	if e.Name != "" {
		return e.Name
	}
	return e.Attribute
}

// AddTo appends rules to an effect CCP already has. The ones CCP made stay.
type AddTo struct {
	at source

	Attribute string `yaml:"-" json:"attribute"`
	Effect    string `yaml:"effect" json:"effect"`
	Rules     []Rule `yaml:"rules" json:"rules"`
}

// Rule is one step of the sum that fills an attribute in: an operation, and
// the attribute it reads. Which attribute it writes is the file it is in.
//
// In YAML the key carrying the input is the operation.
type Rule struct {
	Op string `json:"op"`
	By string `json:"by"`

	// Domain is itemID and Func is itemModifier unless said otherwise, which
	// covers nearly every rule.
	Domain string `json:"domain"`
	Func   string `json:"func"`

	// Group is only used by locationGroupModifier, Skill only by the two
	// required-skill modifiers. Skill takes "*" for "whatever skill this item
	// happens to require".
	Group string `json:"group"`
	Skill string `json:"skill"`
}

// AnySkill is the wildcard the dogma-engine knows as skill ID -1.
const AnySkill = "*"

// Change edits an effect CCP already has, in a way no single attribute owns.
type Change struct {
	at source

	Effect   string  `yaml:"effect" json:"effect"`
	Category *string `yaml:"category,omitempty" json:"category"`
}

// Action changes every type that On matches. Giving an effect out is not an
// action: an effect says itself who gets it.
type Action struct {
	at source

	RemoveEffect string `yaml:"removeEffect,omitempty" json:"removeEffect"`
	SetAttribute string `yaml:"setAttribute,omitempty" json:"setAttribute"`

	// Value belongs to setAttribute.
	Value float64 `yaml:"value,omitempty" json:"value"`

	On Expr `yaml:"on" json:"on"`
}

// source is where a declaration was made, so errors and "explain" can point
// at the file that caused them.
type source struct {
	file string
	line int
}

func (s source) String() string {
	if s.file == "" {
		return "expression"
	}
	return fmt.Sprintf("patches/%s:%d", s.file, s.line)
}

// patchName is what a declaration belongs to: the attribute whose file it is
// in, or the name of one of the two files that are not an attribute.
func (s source) patchName() string {
	return strings.TrimSuffix(filepath.Base(s.file), filepath.Ext(s.file))
}

func before(a, b source) bool {
	if a.file != b.file {
		return a.file < b.file
	}
	return a.line < b.line
}

// The types below only override UnmarshalYAML to remember which line they came
// from. The "plain" alias stops the override from calling itself.

func (u *Unit) UnmarshalYAML(node *yaml.Node) error {
	type plain Unit
	u.at.line = node.Line
	return strictDecode(node, (*plain)(u))
}

func (s *NamedSelector) UnmarshalYAML(node *yaml.Node) error {
	type plain NamedSelector
	s.at.line = node.Line
	return strictDecode(node, (*plain)(s))
}

func (a *Attribute) UnmarshalYAML(node *yaml.Node) error {
	type plain Attribute
	a.at.line = node.Line
	return strictDecode(node, (*plain)(a))
}

func (d *Definition) UnmarshalYAML(node *yaml.Node) error {
	type plain Definition
	return strictDecode(node, (*plain)(d))
}

func (e *Effect) UnmarshalYAML(node *yaml.Node) error {
	type plain Effect
	e.at.line = node.Line
	return strictDecode(node, (*plain)(e))
}

func (a *AddTo) UnmarshalYAML(node *yaml.Node) error {
	type plain AddTo
	a.at.line = node.Line
	return strictDecode(node, (*plain)(a))
}

func (c *Change) UnmarshalYAML(node *yaml.Node) error {
	type plain Change
	c.at.line = node.Line
	return strictDecode(node, (*plain)(c))
}

func (a *Action) UnmarshalYAML(node *yaml.Node) error {
	type plain Action
	a.at.line = node.Line
	return strictDecode(node, (*plain)(a))
}

// strictDecode rejects keys the target does not have, as one is nearly always
// a typo. yaml.v3 only offers that at the top level, and the types above
// decode themselves.
func strictDecode(node *yaml.Node, target any) error {
	if node.Kind != yaml.MappingNode {
		return node.Decode(target)
	}

	known := map[string]bool{}
	fields := reflect.TypeOf(target).Elem()
	for i := range fields.NumField() {
		name, _, _ := strings.Cut(fields.Field(i).Tag.Get("yaml"), ",")
		known[name] = true
	}

	for i := 0; i < len(node.Content); i += 2 {
		if key := node.Content[i]; !known[key.Value] {
			return fmt.Errorf("line %d: unknown key %q", key.Line, key.Value)
		}
	}
	return node.Decode(target)
}

// The YAML spells the flatbuffer enums in lowerCamel: "postMul", "itemID".
// Deriving the names from the generated tables keeps the two in step.
var (
	effectCategories, effectCategoryNames = enumMaps(eve.EnumNamesEffectCategory)
	modifierDomains, modifierDomainNames  = enumMaps(eve.EnumNamesModifierDomain)
	modifierFuncs, modifierFuncNames      = enumMaps(eve.EnumNamesModifierFunc)
	modifierOps, modifierOpNames          = enumMaps(eve.EnumNamesModifierOperation)
)

func enumMaps[T comparable](names map[T]string) (map[string]T, map[T]string) {
	byName := make(map[string]T, len(names))
	byValue := make(map[T]string, len(names))
	for value, name := range names {
		runes := []rune(name)
		runes[0] = unicode.ToLower(runes[0])
		byName[string(runes)] = value
		byValue[value] = string(runes)
	}
	return byName, byValue
}

func lookupEnum[T comparable](table map[string]T, kind, name string) (T, error) {
	value, ok := table[name]
	if !ok {
		return value, fmt.Errorf("unknown %s %q, want one of %s", kind, name, strings.Join(sortedKeys(table), ", "))
	}
	return value, nil
}
