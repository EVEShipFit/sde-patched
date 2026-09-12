package web

import (
	"net/http"
	"sort"
	"strings"
)

// handleSearch looks a word up in everything at once.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	current, err := s.reload()
	if current == nil {
		fail(w, http.StatusConflict, err)
		return
	}

	query := strings.ToLower(r.URL.Query().Get("q"))
	if query == "" {
		write(w, map[string]any{})
		return
	}
	const limit = 300

	var attributes, effects, selectors, groups, categories, types []any

	for _, entry := range current.data.DogmaAttributes {
		if len(attributes) < limit && strings.Contains(strings.ToLower(entry.Name), query) {
			attributes = append(attributes, map[string]any{
				"id": entry.Key, "name": entry.Name, "displayName": entry.DisplayName.En,
				"defaultValue": entry.DefaultValue, "ours": entry.Key < 0,
			})
		}
	}
	for _, entry := range current.data.DogmaEffects {
		if len(effects) < limit && strings.Contains(strings.ToLower(entry.Name), query) {
			effects = append(effects, map[string]any{
				"id": entry.Key, "name": entry.Name, "modifiers": len(entry.Modifiers), "ours": entry.Key < 0,
			})
		}
	}
	for _, entry := range current.spec.Selectors {
		if len(selectors) < limit && strings.Contains(strings.ToLower(entry.Name), query) {
			selectors = append(selectors, map[string]any{
				"name": entry.Name, "patch": entry.Patch(), "match": entry.Match.Text,
			})
		}
	}
	for _, entry := range current.data.Groups {
		if len(groups) < limit && strings.Contains(strings.ToLower(entry.Name.En), query) {
			groups = append(groups, map[string]any{"id": entry.Key, "name": entry.Name.En})
		}
	}
	for _, entry := range current.data.Categories {
		if len(categories) < limit && strings.Contains(strings.ToLower(entry.Name.En), query) {
			categories = append(categories, map[string]any{"id": entry.Key, "name": entry.Name.En})
		}
	}
	for _, entry := range current.data.Types {
		if len(types) < limit && strings.Contains(strings.ToLower(entry.Name.En), query) {
			types = append(types, map[string]any{"id": entry.Key, "name": entry.Name.En})
		}
	}

	for _, list := range [][]any{attributes, effects, selectors, groups, categories, types} {
		sortByName(list)
	}

	write(w, map[string]any{
		"attributes": attributes, "effects": effects, "selectors": selectors,
		"groups": groups, "categories": categories, "types": types,
	})
}

// handleComplete feeds the name boxes in the editor.
func (s *Server) handleComplete(w http.ResponseWriter, r *http.Request) {
	current, err := s.reload()
	if current == nil {
		fail(w, http.StatusConflict, err)
		return
	}

	query := strings.ToLower(r.URL.Query().Get("q"))
	var found []string

	switch r.URL.Query().Get("kind") {
	case "attribute":
		for _, entry := range current.data.DogmaAttributes {
			found = keep(found, entry.Name, query)
		}
	case "effect":
		for _, entry := range current.data.DogmaEffects {
			found = keep(found, entry.Name, query)
		}
	case "selector":
		for _, entry := range current.spec.Selectors {
			found = keep(found, entry.Name, query)
		}
	case "group":
		for _, entry := range current.data.Groups {
			found = keep(found, entry.Name.En, query)
		}
	case "category":
		for _, entry := range current.data.Categories {
			found = keep(found, entry.Name.En, query)
		}
	case "type":
		for _, entry := range current.data.Types {
			found = keep(found, entry.Name.En, query)
		}
	}

	sort.Strings(found)
	sort.SliceStable(found, func(i, j int) bool {
		return strings.EqualFold(found[i], query) || starts(found[i], query) && !starts(found[j], query)
	})
	if len(found) > 25 {
		found = found[:25]
	}
	write(w, map[string]any{"names": found})
}

// keep collects the names that hold the query, up to a cap.
func keep(found []string, name, query string) []string {
	if len(found) < 200 && strings.Contains(strings.ToLower(name), query) {
		return append(found, name)
	}
	return found
}

func starts(name, query string) bool {
	return strings.HasPrefix(strings.ToLower(name), query)
}
