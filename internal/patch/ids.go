package patch

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Every attribute and effect a patch adds gets an ID that never changes, as
// consumers store it. The IDs are kept in one file rather than in each
// declaration.
//
// An ID is never reused: a name taken out of the patches keeps its number.

// IDsFile is the one file in the patches directory that is not a patch.
const IDsFile = "ids.yaml"

const idsNotes = `# Every ID the patches have handed out, kept so that none of them ever moves.
# A name taken out of the patches keeps its number here, so a number can never
# come back meaning something else.
#
# The editor writes this. Nothing here is edited by hand.
`

// IDs is the ID each name was given, for attributes and for effects. The two
// are numbered apart, so the same name can be in both.
type IDs struct {
	Attributes map[string]int32 `yaml:"attributes"`
	Effects    map[string]int32 `yaml:"effects"`

	path string
}

func LoadIDs(dir string) (*IDs, error) {
	ids := &IDs{
		Attributes: map[string]int32{},
		Effects:    map[string]int32{},
		path:       filepath.Join(dir, IDsFile),
	}

	raw, err := os.ReadFile(ids.path)
	if os.IsNotExist(err) {
		return ids, nil
	}
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(raw, ids); err != nil {
		return nil, fmt.Errorf("patches/%s: %w", IDsFile, err)
	}
	if ids.Attributes == nil {
		ids.Attributes = map[string]int32{}
	}
	if ids.Effects == nil {
		ids.Effects = map[string]int32{}
	}
	return ids, nil
}

func (ids *IDs) Attribute(name string) (int32, bool) {
	id, known := ids.Attributes[name]
	return id, known
}

func (ids *IDs) Effect(name string) (int32, bool) {
	id, known := ids.Effects[name]
	return id, known
}

// take hands out the next free number. IDs are negative to set them apart
// from CCP's.
func take(handed map[string]int32, name string) int32 {
	id := int32(-1)
	for {
		used := false
		for _, taken := range handed {
			if taken == id {
				used = true
				break
			}
		}
		if !used {
			break
		}
		id--
	}
	handed[name] = id
	return id
}

func (ids *IDs) TakeAttribute(name string) int32 { return take(ids.Attributes, name) }
func (ids *IDs) TakeEffect(name string) int32    { return take(ids.Effects, name) }

// Record gives a number to everything in the spec that has not got one yet,
// and says how many it handed out. Names are taken in file and line order, so
// the same patches always come out the same way.
func (ids *IDs) Record(spec *Spec) int {
	handed := 0
	for _, attribute := range spec.Attributes {
		if attribute.New == nil {
			continue
		}
		if _, known := ids.Attributes[attribute.Name]; !known && attribute.Name != "" {
			ids.TakeAttribute(attribute.Name)
			handed++
		}
	}
	for _, effect := range spec.Effects() {
		if _, known := ids.Effects[effect.EffectName()]; !known && effect.EffectName() != "" {
			ids.TakeEffect(effect.EffectName())
			handed++
		}
	}
	return handed
}

// Missing is what has no ID yet. Build refuses to run on any of these rather
// than hand out a number that might not be the one used last time.
func (ids *IDs) Missing(spec *Spec) error {
	var errs []string
	for _, attribute := range spec.Attributes {
		if attribute.New == nil {
			continue
		}
		if _, known := ids.Attributes[attribute.Name]; !known {
			errs = append(errs, fmt.Sprintf("%s: attribute %q has no ID yet", attribute.at, attribute.Name))
		}
	}
	for _, effect := range spec.Effects() {
		if _, known := ids.Effects[effect.EffectName()]; !known {
			errs = append(errs, fmt.Sprintf("%s: effect %q has no ID yet", effect.at, effect.EffectName()))
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("%s\nrun \"sde-patched ids\" to give them one", strings.Join(errs, "\n"))
}

func (ids *IDs) Save() error {
	if ids.path == "" {
		return fmt.Errorf("these IDs were not read from a file")
	}

	var out strings.Builder
	out.WriteString(idsNotes)
	out.WriteString("\nattributes:\n")
	write(&out, ids.Attributes)
	out.WriteString("\neffects:\n")
	write(&out, ids.Effects)

	return os.WriteFile(ids.path, []byte(out.String()), 0o644)
}

// write lists the names by the number they were given, so the file reads as
// the order they were handed out in and a new one always lands at the end.
func write(out *strings.Builder, handed map[string]int32) {
	names := make([]string, 0, len(handed))
	for name := range handed {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool { return handed[names[i]] > handed[names[j]] })

	for _, name := range names {
		fmt.Fprintf(out, "  %s: %d\n", name, handed[name])
	}
}
