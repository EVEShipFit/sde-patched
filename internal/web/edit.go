package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/EVEShipFit/sde-patched/internal/patch"
)

// The editor reads a file as a list of entries per section and writes single
// entries back, so comments and layout outside the changed entry are kept.
//
// A file is named after the attribute it is for, except "selectors" and
// "effects".

// fileRef is where one name lives, and what it is allowed to say.
type fileRef struct {
	path string
	kind string
}

func (s *Server) file(name string) (fileRef, error) {
	if name != filepath.Base(name) || name == "" || strings.HasSuffix(name, ".yaml") {
		return fileRef{}, fmt.Errorf("%q is not a name I can write to", name)
	}

	switch name {
	case "selectors":
		return fileRef{filepath.Join(s.patchesDir, patch.SelectorsFile), patch.KindSelectors}, nil
	case "effects":
		return fileRef{filepath.Join(s.patchesDir, patch.EffectsFile), patch.KindEffects}, nil
	case "units":
		return fileRef{filepath.Join(s.patchesDir, patch.UnitsFile), patch.KindUnits}, nil
	}
	return fileRef{filepath.Join(s.patchesDir, patch.AttributesDir, name+".yaml"), patch.KindAttribute}, nil
}

func (s *Server) document(name string) (*patch.Document, error) {
	where, err := s.file(name)
	if err != nil {
		return nil, err
	}

	raw, err := os.ReadFile(where.path)
	if os.IsNotExist(err) {
		raw = nil
	} else if err != nil {
		return nil, err
	}
	return patch.ParseDocument(raw, where.kind)
}

func (s *Server) save(name string, doc *patch.Document) error {
	where, err := s.file(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(where.path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(where.path, doc.Bytes(), 0o644)
}

// handlePatch is everything one file declares, in the order it is written
// down, with what each declaration ended up doing.
func (s *Server) handlePatch(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	doc, err := s.document(name)
	if err != nil {
		fail(w, http.StatusNotFound, err)
		return
	}

	body, err := s.view(name, doc)
	if err != nil {
		fail(w, http.StatusConflict, err)
		return
	}
	write(w, body)
}

// view is a file as the editor sees it: what it says, and what the last good
// run made of it. The declarations are included even when the run failed.
func (s *Server) view(name string, doc *patch.Document) (map[string]any, error) {
	current, err := s.reload()

	body := map[string]any{
		"name":  name,
		"kind":  doc.Kind(),
		"notes": doc.Notes(),
	}
	if err != nil {
		body["error"] = err.Error()
	}

	for _, section := range patch.Sections(doc.Kind()) {
		if section == "new" || section == "change" {
			definition := &patch.Definition{}
			if found, err := doc.Mapping(section, definition); err != nil {
				return nil, err
			} else if found {
				body[section] = definition
			}
			continue
		}

		entries := make([]any, 0, doc.Count(section))
		for i := range doc.Count(section) {
			fields, err := doc.Entry(section, i)
			if err != nil {
				return nil, err
			}

			entry := map[string]any{
				"index":   i,
				"comment": doc.Comment(section, i),
				"fields":  fields,
			}
			if current != nil {
				s.explain(current, name, section, i, entry)
			}
			entries = append(entries, entry)
		}
		body[section] = entries
	}
	return body, nil
}

// explain adds what the last good run made of one declaration: the ID it was
// given, what it matched, what else mentions it.
func (s *Server) explain(current *state, name, section string, index int, entry map[string]any) {
	switch section {
	case "selectors":
		if index >= len(current.spec.Selectors) {
			return
		}
		selector := current.spec.Selectors[index]
		entry["at"] = selector.At()
		if matched, err := current.ctx.Match(selector.Match.Text); err == nil {
			entry["count"] = len(matched)
		}

	case "effects":
		declared := effectsOf(current, name)
		if index >= len(declared) {
			return
		}
		effect := declared[index]
		entry["at"] = effect.At()
		entry["name"] = effect.EffectName()
		entry["id"] = current.ctx.EffectID(effect.EffectName())
		entry["count"] = len(current.ctx.Applied[effect.EffectName()])

	case "addTo":
		declared := addTosOf(current, name)
		if index >= len(declared) {
			return
		}
		entry["at"] = declared[index].At()
		entry["description"] = declared[index].String()

	case "changes":
		if index >= len(current.spec.Changes) {
			return
		}
		entry["at"] = current.spec.Changes[index].At()
		entry["description"] = current.spec.Changes[index].String()

	case "actions":
		if index >= len(current.spec.Actions) {
			return
		}
		action := current.spec.Actions[index]
		entry["at"] = action.At()
		entry["description"] = action.String()
		entry["count"] = len(current.ctx.Matched[index])
		entry["types"] = s.tree(current, current.ctx.Matched[index])
	}
}

// effectsOf and addTosOf are the entries of one attribute's file, in the order
// they are written, which is the order the editor works in.
func effectsOf(current *state, attribute string) []*patch.Effect {
	if entry := declared(current, attribute); entry != nil {
		return entry.Effects
	}
	return nil
}

func addTosOf(current *state, attribute string) []*patch.AddTo {
	if entry := declared(current, attribute); entry != nil {
		return entry.AddTo
	}
	return nil
}

// usedBy lists the effects whose rules mention an attribute.
func usedBy(current *state, attribute int32) []any {
	var result []any
	for _, effect := range sortedEffects(current) {
		for _, modifier := range effect.Modifiers {
			if modifier.ModifiedAttributeID == attribute || modifier.ModifyingAttributeID == attribute {
				result = append(result, map[string]any{"id": effect.Key, "name": effect.Name})
				break
			}
		}
	}
	return result
}

type edit struct {
	Fields  json.RawMessage `json:"fields"`
	Comment string          `json:"comment"`
}

// entry reads a declaration the editor sends. It is decoded into the same
// thing the YAML decodes into, so a bad name or a broken expression is
// refused here rather than written to the file.
func (e edit) entry(section string) (any, error) {
	value, err := patch.NewEntry(section)
	if err != nil {
		return nil, err
	}
	if len(e.Fields) == 0 {
		return nil, fmt.Errorf("no fields to write")
	}
	if err := strict(e.Fields, value); err != nil {
		return nil, err
	}
	return value, nil
}

// strict refuses a field it does not know, so a typo is not silently dropped.
func strict(raw json.RawMessage, into any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(into)
}

func read(r *http.Request) (edit, error) {
	var body edit
	err := json.NewDecoder(r.Body).Decode(&body)
	return body, err
}

func (s *Server) handleInsert(w http.ResponseWriter, r *http.Request) {
	body, err := read(r)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	section := r.PathValue("section")

	value, err := body.entry(section)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}

	s.change(w, r.PathValue("name"), func(doc *patch.Document) error {
		return doc.Insert(section, value, body.Comment)
	})
}

func (s *Server) handleReplace(w http.ResponseWriter, r *http.Request) {
	body, err := read(r)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	section := r.PathValue("section")

	index, err := strconv.Atoi(r.PathValue("index"))
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	value, err := body.entry(section)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}

	s.change(w, r.PathValue("name"), func(doc *patch.Document) error {
		return doc.Replace(section, index, value, body.Comment)
	})
}

func (s *Server) handleRemove(w http.ResponseWriter, r *http.Request) {
	section := r.PathValue("section")
	index, err := strconv.Atoi(r.PathValue("index"))
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}

	s.change(w, r.PathValue("name"), func(doc *patch.Document) error {
		return doc.Remove(section, index)
	})
}

// handleDefine writes the "new" or "change" block: what the attribute is,
// apart from the sum that fills it in. Sending nothing takes the block out.
func (s *Server) handleDefine(w http.ResponseWriter, r *http.Request) {
	section := r.PathValue("section")
	if section != "new" && section != "change" {
		fail(w, http.StatusBadRequest, fmt.Errorf("%q is not new or change", section))
		return
	}

	body, err := read(r)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}

	var definition any
	if len(body.Fields) > 0 && string(body.Fields) != "null" {
		value := &patch.Definition{}
		if err := strict(body.Fields, value); err != nil {
			fail(w, http.StatusBadRequest, err)
			return
		}
		definition = value
	}

	s.change(w, r.PathValue("name"), func(doc *patch.Document) error {
		return doc.SetMapping(section, definition)
	})
}

func (s *Server) handleNotes(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}

	s.change(w, r.PathValue("name"), func(doc *patch.Document) error {
		return doc.SetNotes(body.Text)
	})
}

// handleCreate starts a file for an attribute that has none yet.
func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	s.writing.Lock()
	defer s.writing.Unlock()

	name := r.PathValue("name")
	where, err := s.file(name)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	if _, err := os.Stat(where.path); err == nil {
		fail(w, http.StatusConflict, fmt.Errorf("there is already a file for %q", name))
		return
	}

	// An empty body is fine: the file starts out with nothing in it.
	var body struct {
		Notes string            `json:"notes"`
		New   *patch.Definition `json:"new"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		fail(w, http.StatusBadRequest, err)
		return
	}

	doc, err := patch.ParseDocument(nil, where.kind)
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	if err := doc.SetNotes(body.Notes); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	if body.New != nil {
		if err := doc.SetMapping("new", body.New); err != nil {
			fail(w, http.StatusBadRequest, err)
			return
		}
	}
	if err := s.save(name, doc); err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}

	// A new attribute needs a number, and it has to be the same number next
	// time, so it is written down before anything can read it back.
	if err := s.recordIDs(); err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	write(w, map[string]any{"name": name})
}

// handleDelete deletes an attribute. Its ID stays written down, so the
// number can never come back meaning something else.
func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	s.writing.Lock()
	defer s.writing.Unlock()

	name := r.PathValue("name")
	where, err := s.file(name)
	if err != nil || where.kind != patch.KindAttribute {
		fail(w, http.StatusBadRequest, fmt.Errorf("only an attribute can be deleted"))
		return
	}
	if err := os.Remove(where.path); err != nil {
		fail(w, http.StatusNotFound, err)
		return
	}

	body := map[string]any{"removed": name}
	if _, err := s.reload(); err != nil {
		body["error"] = err.Error()
	}
	write(w, body)
}

// recordIDs gives a number to anything new and writes it down.
func (s *Server) recordIDs() error {
	spec, err := patch.Load(s.patchesDir)
	if err != nil {
		return err
	}
	if spec.IDs.Record(spec) == 0 {
		return nil
	}
	return spec.IDs.Save()
}

// change applies one edit and saves it. The file is read again first, so that
// an edit is always made against what is on disk.
func (s *Server) change(w http.ResponseWriter, name string, apply func(*patch.Document) error) {
	s.writing.Lock()
	defer s.writing.Unlock()

	doc, err := s.document(name)
	if err != nil {
		fail(w, http.StatusNotFound, err)
		return
	}
	if err := apply(doc); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	if err := s.save(name, doc); err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	if err := s.recordIDs(); err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}

	// The edit is already saved; a conflict here means the patches no longer
	// apply, which the editor shows so the next edit can fix it.
	body, err := s.view(name, doc)
	if err != nil {
		fail(w, http.StatusConflict, err)
		return
	}
	write(w, body)
}
