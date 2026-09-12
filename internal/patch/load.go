package patch

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// The three things a patches directory holds.
const (
	AttributesDir = "attributes"
	SelectorsFile = "selectors.yaml"
	EffectsFile   = "effects.yaml"
)

// Load reads a patches directory into one Spec. Attribute files are read in
// name order and everything keeps the order it was written in, which is what
// fixes the order the patches are applied in.
func Load(dir string) (*Spec, error) {
	ids, err := LoadIDs(dir)
	if err != nil {
		return nil, err
	}
	spec := &Spec{IDs: ids}

	if err := spec.addSelectors(filepath.Join(dir, SelectorsFile)); err != nil {
		return nil, err
	}
	if err := spec.addEffects(filepath.Join(dir, EffectsFile)); err != nil {
		return nil, err
	}

	names, err := filepath.Glob(filepath.Join(dir, AttributesDir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	sort.Strings(names)

	for _, name := range names {
		if err := spec.addAttribute(name); err != nil {
			return nil, err
		}
	}

	if len(spec.Attributes) == 0 && len(spec.Selectors) == 0 && len(spec.Changes) == 0 && len(spec.Actions) == 0 {
		return nil, fmt.Errorf("%s holds no patches", dir)
	}
	return spec, nil
}

// AttributeName is the attribute a file in patches/attributes is for. The
// file is named after it, so nothing inside has to repeat it.
func AttributeName(filename string) string {
	return strings.TrimSuffix(filepath.Base(filename), ".yaml")
}

func (spec *Spec) addAttribute(filename string) error {
	raw, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	where := filepath.Join(AttributesDir, filepath.Base(filename))
	name := AttributeName(filename)

	attribute := &Attribute{Name: name}
	if err := decode(raw, attribute); err != nil {
		return atLine(where, err)
	}
	attribute.at = source{file: where, line: 1}

	for _, effect := range attribute.Effects {
		effect.at.file = where
		effect.Attribute = name
	}
	for _, added := range attribute.AddTo {
		added.at.file = where
		added.Attribute = name
	}

	spec.Attributes = append(spec.Attributes, attribute)
	return nil
}

func (spec *Spec) addSelectors(filename string) error {
	raw, err := os.ReadFile(filename)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	var parsed selectorFile
	if err := decode(raw, &parsed); err != nil {
		return atLine(SelectorsFile, err)
	}
	for _, selector := range parsed.Selectors {
		selector.at.file = SelectorsFile
		spec.Selectors = append(spec.Selectors, selector)
	}
	return nil
}

func (spec *Spec) addEffects(filename string) error {
	raw, err := os.ReadFile(filename)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	var parsed effectFile
	if err := decode(raw, &parsed); err != nil {
		return atLine(EffectsFile, err)
	}
	for _, change := range parsed.Changes {
		change.at.file = EffectsFile
		spec.Changes = append(spec.Changes, change)
	}
	for _, action := range parsed.Actions {
		action.at.file = EffectsFile
		spec.Actions = append(spec.Actions, action)
	}
	return nil
}

func decode(raw []byte, target any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	if err := decoder.Decode(target); err != nil && err.Error() != "EOF" {
		return err
	}
	return nil
}

// atLine turns the "line 8: ..." that decoding produces into the same
// file:line form every other error uses.
func atLine(file string, err error) error {
	if err == nil {
		return nil
	}
	if rest, ok := strings.CutPrefix(err.Error(), "line "); ok {
		if line, message, found := strings.Cut(rest, ": "); found {
			return fmt.Errorf("patches/%s:%s: %s", file, line, message)
		}
	}
	return fmt.Errorf("patches/%s: %w", file, err)
}

// sort puts everything in file and line order, so that the result never
// depends on the order the files happened to be read in.
func (spec *Spec) sort() {
	sort.SliceStable(spec.Attributes, func(i, j int) bool { return before(spec.Attributes[i].at, spec.Attributes[j].at) })
	sort.SliceStable(spec.Changes, func(i, j int) bool { return before(spec.Changes[i].at, spec.Changes[j].at) })
	sort.SliceStable(spec.Actions, func(i, j int) bool { return before(spec.Actions[i].at, spec.Actions[j].at) })
}
