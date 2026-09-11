package sde

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
)

// Load reads the parts of the SDE zip we need. Files are streamed, as the
// archive does not fit comfortably in memory.
func Load(filename string, build int32) (*Data, error) {
	reader, err := zip.OpenReader(filename)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	data := &Data{
		BuildNumber:     build,
		Types:           map[int32]*Type{},
		Groups:          map[int32]*Group{},
		Categories:      map[int32]*Category{},
		DogmaAttributes: map[int32]*DogmaAttribute{},
		DogmaEffects:    map[int32]*DogmaEffect{},
	}

	if err := decode(&reader.Reader, "categories.jsonl", func(entry *Category) {
		data.Categories[entry.Key] = entry
	}); err != nil {
		return nil, err
	}
	if err := decode(&reader.Reader, "groups.jsonl", func(entry *Group) {
		data.Groups[entry.Key] = entry
	}); err != nil {
		return nil, err
	}
	if err := decode(&reader.Reader, "types.jsonl", func(entry *Type) {
		data.Types[entry.Key] = entry
	}); err != nil {
		return nil, err
	}
	if err := decode(&reader.Reader, "dogmaAttributes.jsonl", func(entry *DogmaAttribute) {
		data.DogmaAttributes[entry.Key] = entry
	}); err != nil {
		return nil, err
	}
	if err := decode(&reader.Reader, "dogmaEffects.jsonl", func(entry *DogmaEffect) {
		data.DogmaEffects[entry.Key] = entry
	}); err != nil {
		return nil, err
	}

	if err := decode(&reader.Reader, "typeDogma.jsonl", func(entry *typeDogmaEntry) {
		item, ok := data.Types[entry.Key]
		if !ok {
			return
		}
		item.DogmaAttributes = entry.DogmaAttributes
		item.DogmaEffects = entry.DogmaEffects
	}); err != nil {
		return nil, err
	}

	for _, item := range data.Types {
		if group, ok := data.Groups[item.GroupID]; ok {
			item.CategoryID = group.CategoryID
		}
	}

	return data, nil
}

type typeDogmaEntry struct {
	Key             int32                `json:"_key"`
	DogmaAttributes []TypeDogmaAttribute `json:"dogmaAttributes"`
	DogmaEffects    []TypeDogmaEffect    `json:"dogmaEffects"`
}

func decode[T any](reader *zip.Reader, name string, add func(*T)) error {
	file, err := reader.Open(name)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	for {
		var entry T
		if err := decoder.Decode(&entry); err == io.EOF {
			return nil
		} else if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		add(&entry)
	}
}
