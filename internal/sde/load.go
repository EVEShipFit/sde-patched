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
		BuildNumber:      build,
		Types:            map[int32]*Type{},
		Groups:           map[int32]*Group{},
		Categories:       map[int32]*Category{},
		MarketGroups:     map[int32]*MarketGroup{},
		MetaGroups:       map[int32]*MetaGroup{},
		DogmaAttributes:  map[int32]*DogmaAttribute{},
		DogmaEffects:     map[int32]*DogmaEffect{},
		DbuffCollections: map[int32]*DbuffCollection{},
		Mutaplasmids:     map[int32]*Mutaplasmid{},
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
	if err := decode(&reader.Reader, "marketGroups.jsonl", func(entry *MarketGroup) {
		data.MarketGroups[entry.Key] = entry
	}); err != nil {
		return nil, err
	}
	if err := decode(&reader.Reader, "metaGroups.jsonl", func(entry *MetaGroup) {
		data.MetaGroups[entry.Key] = entry
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

	if err := decode(&reader.Reader, "dbuffCollections.jsonl", func(entry *DbuffCollection) {
		data.DbuffCollections[entry.Key] = entry
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

	if err := decode(&reader.Reader, "fighterAbilitiesByType.jsonl", func(entry *fighterAbilitiesEntry) {
		item, ok := data.Types[entry.Key]
		if !ok {
			return
		}
		for slot, ability := range []*fighterAbilitySlot{entry.Slot0, entry.Slot1, entry.Slot2} {
			if ability == nil {
				continue
			}
			item.FighterAbilities = append(item.FighterAbilities, TypeFighterAbility{
				Slot:             int8(slot),
				AbilityID:        ability.AbilityID,
				CooldownSeconds:  ability.CooldownSeconds,
				ChargeCount:      ability.Charges.ChargeCount,
				RearmTimeSeconds: ability.Charges.RearmTimeSeconds,
			})
		}
	}); err != nil {
		return nil, err
	}

	if err := decode(&reader.Reader, "dynamicItemAttributes.jsonl", func(entry *Mutaplasmid) {
		data.Mutaplasmids[entry.Key] = entry
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

// The SDE gives each ability slot its own key rather than a list.
type fighterAbilitiesEntry struct {
	Key   int32               `json:"_key"`
	Slot0 *fighterAbilitySlot `json:"abilitySlot0"`
	Slot1 *fighterAbilitySlot `json:"abilitySlot1"`
	Slot2 *fighterAbilitySlot `json:"abilitySlot2"`
}

type fighterAbilitySlot struct {
	AbilityID       int32   `json:"abilityID"`
	CooldownSeconds float64 `json:"cooldownSeconds"`
	Charges         struct {
		ChargeCount      int32   `json:"chargeCount"`
		RearmTimeSeconds float64 `json:"rearmTimeSeconds"`
	} `json:"charges"`
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
