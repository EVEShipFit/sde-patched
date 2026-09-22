package patch

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

// Document is one patch file, kept as the text it was read as. The parsed
// nodes are only used to find where each declaration starts and ends: editing
// splices lines in and out, so the comments, the blank lines and the layout of
// everything the edit did not touch survive it.
type Document struct {
	kind  string
	lines []string
	root  *yaml.Node
}

// The four kinds of file a patches directory holds. Which one a document is
// decides what it may say, and in what order.
const (
	KindAttribute = "attribute"
	KindSelectors = "selectors"
	KindEffects   = "effects"
	KindUnits     = "units"
)

// sections are the keys each kind of file may hold, in the order a new one is
// added in. The ones that are a list can be edited entry by entry.
var sections = map[string][]string{
	KindAttribute: {"new", "change", "effects", "addTo"},
	KindSelectors: {"selectors"},
	KindEffects:   {"changes", "actions"},
	KindUnits:     {"units"},
}

var lists = map[string]bool{"effects": true, "addTo": true, "selectors": true, "changes": true, "actions": true, "units": true}

// Sections are the keys one kind of file may hold.
func Sections(kind string) []string { return sections[kind] }

func ParseDocument(raw []byte, kind string) (*Document, error) {
	if _, known := sections[kind]; !known {
		return nil, fmt.Errorf("unknown kind of file %q", kind)
	}
	doc := &Document{kind: kind, lines: split(raw)}
	if err := doc.parse(); err != nil {
		return nil, err
	}
	return doc, nil
}

func (d *Document) Kind() string { return d.kind }

func split(raw []byte) []string {
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

// parse re-reads the text. It runs after every edit, so that the indexes an
// editor holds keep pointing at the same declarations, and so that a change
// that would not read back is refused before it reaches the disk.
func (d *Document) parse() error {
	raw := d.Bytes()

	var document yaml.Node
	if err := yaml.Unmarshal(raw, &document); err != nil {
		return err
	}

	d.root = &yaml.Node{}
	if len(document.Content) > 0 {
		d.root = document.Content[0]
	}
	if d.root.Kind != 0 && d.root.Kind != yaml.MappingNode {
		return fmt.Errorf("a patch file is a mapping of sections, not %s", kindName(d.root.Kind))
	}

	var parsed any
	switch d.kind {
	case KindAttribute:
		parsed = &Attribute{}
	case KindSelectors:
		parsed = &selectorFile{}
	case KindEffects:
		parsed = &effectFile{}
	case KindUnits:
		parsed = &unitFile{}
	}

	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	if err := decoder.Decode(parsed); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func kindName(kind yaml.Kind) string {
	switch kind {
	case yaml.SequenceNode:
		return "a list"
	case yaml.ScalarNode:
		return "a single value"
	default:
		return "that"
	}
}

func (d *Document) Bytes() []byte {
	if len(d.lines) == 0 {
		return nil
	}
	return []byte(strings.Join(d.lines, "\n") + "\n")
}

// Count is how many entries a section holds. A section that is not there
// holds none.
func (d *Document) Count(section string) int {
	seq := d.section(section)
	if seq == nil {
		return 0
	}
	return len(seq.Content)
}

// Comment is the comment block written directly above an entry.
func (d *Document) Comment(section string, index int) string {
	seq := d.section(section)
	if seq == nil || index < 0 || index >= len(seq.Content) {
		return ""
	}
	start := seq.Content[index].Line - 1
	return uncomment(d.lines[d.commentStart(start):start])
}

// Notes is the comment block at the top of the file.
func (d *Document) Notes() string {
	end := 0
	for end < len(d.lines) && isComment(d.lines[end]) {
		end++
	}
	return uncomment(d.lines[:end])
}

// SetNotes replaces the comment block at the top of the file.
func (d *Document) SetNotes(text string) error {
	end := 0
	for end < len(d.lines) && isComment(d.lines[end]) {
		end++
	}

	// Keep the blank line that separates the notes from the first section.
	rest := d.lines[end:]
	if text != "" && len(rest) > 0 && strings.TrimSpace(rest[0]) != "" {
		rest = append([]string{""}, rest...)
	}
	if text == "" {
		for len(rest) > 0 && strings.TrimSpace(rest[0]) == "" {
			rest = rest[1:]
		}
	}

	d.lines = append(comment(text, 0), rest...)
	return d.parse()
}

// Replace writes one entry again, keeping the layout of the entry it replaces:
// one written on one line stays on one line. An unchanged entry is left alone,
// so saving without changes does not touch the file.
func (d *Document) Replace(section string, index int, value any, note string) error {
	seq := d.section(section)
	if seq == nil || index < 0 || index >= len(seq.Content) {
		return fmt.Errorf("no %s %d in this patch", strings.TrimSuffix(section, "s"), index)
	}

	start, end := d.span(seq, index)
	indent := indentOf(seq)
	block, err := render(value, seq.Content[index], indent)
	if err != nil {
		return err
	}
	if same, err := d.unchanged(section, index, block, note); err != nil || same {
		return err
	}

	replaced := append(comment(note, indent), block...)
	return d.splice(d.commentStart(start), end, replaced)
}

// Insert adds an entry to the end of a section, and adds the section itself
// when the patch does not have one yet.
func (d *Document) Insert(section string, value any, note string) error {
	seq := d.section(section)
	if seq == nil {
		return d.insertSection(section, value, note)
	}

	indent := indentOf(seq)
	var like *yaml.Node
	if len(seq.Content) > 0 {
		like = &yaml.Node{Style: seq.Content[len(seq.Content)-1].Style}
	}
	block, err := render(value, like, indent)
	if err != nil {
		return err
	}

	_, end := d.span(seq, len(seq.Content)-1)
	if len(seq.Content) == 0 {
		end = seq.Line - 1
	}

	added := append(comment(note, indent), block...)
	if d.spaced(seq) {
		added = append([]string{""}, added...)
	}
	return d.splice(end+1, end, added)
}

// unchanged says whether an entry would be written exactly as it stands. The
// entry in the file is rendered the same way as the one replacing it, so that
// only a real difference counts.
func (d *Document) unchanged(section string, index int, block []string, note string) (bool, error) {
	if d.Comment(section, index) != note {
		return false, nil
	}

	seq := d.section(section)
	old, err := d.Entry(section, index)
	if err != nil {
		return false, err
	}
	was, err := render(old, seq.Content[index], indentOf(seq))
	if err != nil {
		return false, err
	}
	return strings.Join(was, "\n") == strings.Join(block, "\n"), nil
}

// insertSection adds a section that is not there yet, in the order the
// sections are listed in.
func (d *Document) insertSection(section string, value any, note string) error {
	block, err := render(value, nil, 2)
	if err != nil {
		return err
	}

	added := append([]string{section + ":"}, append(comment(note, 2), block...)...)
	at := d.sectionStart(section)
	if at > 0 && strings.TrimSpace(d.lines[at-1]) != "" {
		added = append([]string{""}, added...)
	}
	if at < len(d.lines) {
		added = append(added, "")
	}
	return d.splice(at, at-1, added)
}

// sectionStart is the line a new section belongs on: before the first section
// that comes after it, or at the end of the file.
func (d *Document) sectionStart(section string) int {
	rank := func(name string) int {
		for i, known := range sections[d.kind] {
			if known == name {
				return i
			}
		}
		return len(sections[d.kind])
	}

	at := len(d.lines)
	for i := 0; i < len(d.root.Content); i += 2 {
		key := d.root.Content[i]
		if rank(key.Value) <= rank(section) {
			continue
		}
		if line := d.commentStart(key.Line - 1); line < at {
			at = line
		}
	}

	// At the end of the file there is nothing to keep a blank line away from.
	for at == len(d.lines) && at > 0 && strings.TrimSpace(d.lines[at-1]) == "" {
		at--
	}
	return at
}

// Remove takes an entry out, along with its comment and the blank line that
// separated it. The section goes too when it was the last entry.
func (d *Document) Remove(section string, index int) error {
	seq := d.section(section)
	if seq == nil || index < 0 || index >= len(seq.Content) {
		return fmt.Errorf("no %s %d in this patch", strings.TrimSuffix(section, "s"), index)
	}

	if len(seq.Content) == 1 {
		key := d.root.Content[d.keyIndex(section)]
		_, end := d.span(seq, 0)
		return d.splice(d.commentStart(key.Line-1), end, nil)
	}

	start, end := d.span(seq, index)
	start = d.commentStart(start)
	for start > 0 && strings.TrimSpace(d.lines[start-1]) == "" {
		start--
	}
	return d.splice(start, end, nil)
}

// splice replaces the lines from start to end, both included, and reads the
// result back. An end below start inserts without replacing anything.
func (d *Document) splice(start, end int, with []string) error {
	before := append([]string{}, d.lines[:start]...)
	after := append([]string{}, d.lines[min(end+1, len(d.lines)):]...)

	saved := d.lines
	d.lines = append(before, append(with, after...)...)
	d.tidy(start + len(with))
	d.tidy(start)

	if err := d.parse(); err != nil {
		d.lines = saved
		d.parse()
		return err
	}
	return nil
}

// tidy closes the gap an edit leaves behind: two blank lines where there was
// one, or a file that now ends in nothing.
func (d *Document) tidy(at int) {
	for at > 0 && at < len(d.lines) && blank(d.lines[at-1]) && blank(d.lines[at]) {
		d.lines = append(d.lines[:at], d.lines[at+1:]...)
	}
	for at == 0 && len(d.lines) > 0 && blank(d.lines[0]) {
		d.lines = d.lines[1:]
	}
	for len(d.lines) > 0 && blank(d.lines[len(d.lines)-1]) {
		d.lines = d.lines[:len(d.lines)-1]
	}
}

func blank(line string) bool { return strings.TrimSpace(line) == "" }

// Mapping reads back a section that is a mapping rather than a list, which is
// how an attribute file says what the attribute is.
func (d *Document) Mapping(section string, value any) (bool, error) {
	index := d.keyIndex(section)
	if index < 0 {
		return false, nil
	}
	return true, d.root.Content[index+1].Decode(value)
}

// SetMapping writes such a section, adding it when the file has none yet and
// taking it out when there is nothing left to say.
func (d *Document) SetMapping(section string, value any) error {
	index := d.keyIndex(section)

	var block []string
	if value != nil {
		var node yaml.Node
		if err := node.Encode(value); err != nil {
			return err
		}
		// A comment written beside a value it still has belongs to that value,
		// not to the file, so it is carried over.
		if index >= 0 {
			carry(d.root.Content[index+1], &node)
			node.Style = 0
		}
		if len(node.Content) > 0 {
			var out bytes.Buffer
			encoder := yaml.NewEncoder(&out)
			encoder.SetIndent(2)
			if err := encoder.Encode(&node); err != nil {
				return err
			}
			encoder.Close()
			for _, line := range split(out.Bytes()) {
				block = append(block, strings.TrimRight("  "+unquoteOn(line), " "))
			}
		}
	}

	if index < 0 {
		if len(block) == 0 {
			return nil
		}
		at := d.sectionStart(section)
		added := append([]string{section + ":"}, block...)
		if at > 0 && strings.TrimSpace(d.lines[at-1]) != "" {
			added = append([]string{""}, added...)
		}
		if at < len(d.lines) {
			added = append(added, "")
		}
		return d.splice(at, at-1, added)
	}

	start := d.root.Content[index].Line - 1
	end := d.keyEnd(index) - 1
	for end > start && isGap(d.lines[end]) {
		end--
	}
	if len(block) == 0 {
		return d.splice(start, end, nil)
	}
	return d.splice(start, end, append([]string{section + ":"}, block...))
}

// keyEnd is the line the next section starts on.
func (d *Document) keyEnd(index int) int {
	here := d.root.Content[index].Line - 1
	end := len(d.lines)
	for i := 0; i < len(d.root.Content); i += 2 {
		line := d.commentStart(d.root.Content[i].Line - 1)
		if line > here && line < end {
			end = line
		}
	}
	return end
}

func (d *Document) section(name string) *yaml.Node {
	if index := d.keyIndex(name); index >= 0 {
		value := d.root.Content[index+1]
		if value.Kind == yaml.SequenceNode {
			return value
		}
	}
	return nil
}

func (d *Document) keyIndex(name string) int {
	for i := 0; i < len(d.root.Content); i += 2 {
		if d.root.Content[i].Value == name {
			return i
		}
	}
	return -1
}

// span is the first and last line of one entry, not counting the comment
// above it.
func (d *Document) span(seq *yaml.Node, index int) (int, int) {
	if index < 0 {
		return seq.Line - 1, seq.Line - 2
	}

	start := seq.Content[index].Line - 1
	end := d.limit(seq, index) - 1
	for end > start && isGap(d.lines[end]) {
		end--
	}
	return start, end
}

// limit is the line the next entry, or the next section, starts on.
func (d *Document) limit(seq *yaml.Node, index int) int {
	if index+1 < len(seq.Content) {
		return d.commentStart(seq.Content[index+1].Line - 1)
	}

	end := len(d.lines)
	for i := 0; i < len(d.root.Content); i += 2 {
		line := d.commentStart(d.root.Content[i].Line - 1)
		if line > seq.Line-1 && line < end {
			end = line
		}
	}
	return end
}

// commentStart is the first line of the comment block written above a line.
func (d *Document) commentStart(line int) int {
	for line > 0 && isComment(d.lines[line-1]) {
		line--
	}
	return line
}

// spaced says whether a section keeps its entries a blank line apart, so that
// an added entry is written the same way.
func (d *Document) spaced(seq *yaml.Node) bool {
	if len(seq.Content) < 2 {
		return false
	}
	_, end := d.span(seq, len(seq.Content)-2)
	return strings.TrimSpace(d.lines[end+1]) == ""
}

func indentOf(seq *yaml.Node) int {
	if len(seq.Content) == 0 {
		return 2
	}
	return max(seq.Content[0].Column-3, 0)
}

// render writes one entry as the lines of a list item. The entry it is
// replacing, when there is one, decides whether it stays on one line and keeps
// the comments written inside it.
func render(value any, like *yaml.Node, indent int) ([]string, error) {
	var node yaml.Node
	if err := node.Encode(value); err != nil {
		return nil, err
	}
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("an entry is a mapping, not %s", kindName(node.Kind))
	}

	flowPlain(&node)
	if like != nil {
		node.Style = like.Style
		carry(like, &node)
	}
	dropFlowComments(&node, false)

	// The comment above the entry is written separately, so that an entry on
	// one line can have one too.
	node.HeadComment, node.LineComment, node.FootComment = "", "", ""

	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(&node); err != nil {
		return nil, err
	}
	encoder.Close()

	lines := split(out.Bytes())
	for i, line := range lines {
		line = unquoteOn(line)
		lead := strings.Repeat(" ", indent) + "  "
		if i == 0 {
			lead = strings.Repeat(" ", indent) + "- "
		}
		lines[i] = strings.TrimRight(lead+line, " ")
	}
	return lines, nil
}

// flowPlain puts a mapping of plain values on one line, which is how the
// patches are written. Anything with a list in it stays spread out.
func flowPlain(node *yaml.Node) {
	switch node.Kind {
	case yaml.MappingNode:
		plain := true
		for _, child := range node.Content {
			flowPlain(child)
			if child.Kind != yaml.ScalarNode {
				plain = false
			}
		}
		if plain {
			node.Style = yaml.FlowStyle
		}
	case yaml.SequenceNode:
		for _, child := range node.Content {
			flowPlain(child)
		}
	}
}

// carry copies the comments and the layout written inside an entry over to
// the entry that replaces it. A value that changed keeps neither, as its
// comment may not describe it anymore.
func carry(from, to *yaml.Node) {
	if from.Kind != to.Kind {
		return
	}

	switch to.Kind {
	case yaml.MappingNode:
		copyComments(from, to)
		to.Style = from.Style
		for i := 0; i+1 < len(to.Content); i += 2 {
			for j := 0; j+1 < len(from.Content); j += 2 {
				if from.Content[j].Value == to.Content[i].Value {
					copyComments(from.Content[j], to.Content[i])
					carry(from.Content[j+1], to.Content[i+1])
				}
			}
		}

	case yaml.SequenceNode:
		copyComments(from, to)
		to.Style = from.Style
		for i := range min(len(from.Content), len(to.Content)) {
			carry(from.Content[i], to.Content[i])
		}

	case yaml.ScalarNode:
		if from.Value == to.Value {
			copyComments(from, to)
			to.Style = from.Style
		}
	}
}

func copyComments(from, to *yaml.Node) {
	to.HeadComment, to.LineComment, to.FootComment = from.HeadComment, from.LineComment, from.FootComment
}

// dropFlowComments throws away the comments inside anything written on one
// line, as YAML has nowhere to put them there. A comment above such a thing is
// fine: it is on a line of its own.
func dropFlowComments(node *yaml.Node, inside bool) {
	if inside {
		node.HeadComment, node.LineComment, node.FootComment = "", "", ""
	}
	for _, child := range node.Content {
		dropFlowComments(child, inside || node.Style&yaml.FlowStyle != 0)
	}
}

// unquoteOn undoes the quotes yaml.v3 puts around the key "on", which YAML 1.1
// reads as a boolean. Nothing here reads YAML 1.1.
func unquoteOn(line string) string {
	return strings.Replace(line, `"on":`, "on:", 1)
}

func comment(text string, indent int) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}

	var lines []string
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		lines = append(lines, strings.TrimRight(strings.Repeat(" ", indent)+"# "+line, " "))
	}
	return lines
}

func uncomment(lines []string) string {
	var text []string
	for _, line := range lines {
		text = append(text, strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(line), "#"), " "))
	}
	return strings.Join(text, "\n")
}

func isComment(line string) bool { return strings.HasPrefix(strings.TrimSpace(line), "#") }

func isGap(line string) bool { return strings.TrimSpace(line) == "" || isComment(line) }

// NewEntry is an empty declaration of one kind, for something outside this
// package to fill in and hand back to Insert or Replace.
func NewEntry(section string) (any, error) {
	switch section {
	case "selectors":
		return &NamedSelector{}, nil
	case "effects":
		return &Effect{}, nil
	case "addTo":
		return &AddTo{}, nil
	case "changes":
		return &Change{}, nil
	case "actions":
		return &Action{}, nil
	case "new", "change":
		return &Definition{}, nil
	}
	return nil, fmt.Errorf("unknown section %q", section)
}

// Entry reads one declaration back, as it stands in the file.
func (d *Document) Entry(section string, index int) (any, error) {
	seq := d.section(section)
	if seq == nil || index < 0 || index >= len(seq.Content) {
		return nil, fmt.Errorf("no %s %d in this patch", strings.TrimSuffix(section, "s"), index)
	}

	value, err := NewEntry(section)
	if err != nil {
		return nil, err
	}
	if err := seq.Content[index].Decode(value); err != nil {
		return nil, err
	}
	return value, nil
}
