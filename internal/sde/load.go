package sde

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

var tables = []string{
	"_sde",
	"categories",
	"groups",
	"marketGroups",
	"metaGroups",
	"types",
	"dogmaAttributes",
	"dogmaUnits",
	"dogmaAttributeCategories",
	"dogmaEffects",
	"dbuffCollections",
	"typeDogma",
	"fighterAbilities",
	"fighterAbilitiesByType",
	"dynamicItemAttributes",
	"typeBonus",
}

// Load reads the tables Download put in dir.
func Load(dir string, build int32) (*Data, error) {
	data := &Data{
		BuildNumber:      build,
		Types:            map[int32]*Type{},
		Groups:           map[int32]*Group{},
		Categories:       map[int32]*Category{},
		MarketGroups:     map[int32]*MarketGroup{},
		MetaGroups:       map[int32]*MetaGroup{},
		FighterAbilities: map[int32]*FighterAbility{},
		DogmaAttributes:  map[int32]*DogmaAttribute{},
		DogmaUnits:       map[int32]*DogmaUnit{},
		DogmaCategories:  map[int32]*DogmaAttributeCategory{},
		DogmaEffects:     map[int32]*DogmaEffect{},
		DbuffCollections: map[int32]*DbuffCollection{},
		Mutaplasmids:     map[int32]*Mutaplasmid{},
		TypeBonuses:      map[int32]*TypeBonuses{},
	}

	if err := decode(dir, "_sde", func(entry *sdeEntry) {
		if entry.Key == "sde" {
			data.ReleaseDate = entry.ReleaseDate
		}
	}); err != nil {
		return nil, err
	}
	if err := decode(dir, "categories", func(entry *Category) {
		data.Categories[entry.Key] = entry
	}); err != nil {
		return nil, err
	}
	if err := decode(dir, "groups", func(entry *Group) {
		data.Groups[entry.Key] = entry
	}); err != nil {
		return nil, err
	}
	if err := decode(dir, "marketGroups", func(entry *MarketGroup) {
		data.MarketGroups[entry.Key] = entry
	}); err != nil {
		return nil, err
	}
	if err := decode(dir, "metaGroups", func(entry *MetaGroup) {
		data.MetaGroups[entry.Key] = entry
	}); err != nil {
		return nil, err
	}
	if err := decode(dir, "types", func(entry *Type) {
		data.Types[entry.Key] = entry
	}); err != nil {
		return nil, err
	}
	if err := decode(dir, "dogmaAttributes", func(entry *DogmaAttribute) {
		data.DogmaAttributes[entry.Key] = entry
	}); err != nil {
		return nil, err
	}
	if err := decode(dir, "dogmaUnits", func(entry *DogmaUnit) {
		data.DogmaUnits[entry.Key] = entry
	}); err != nil {
		return nil, err
	}
	if err := decode(dir, "dogmaAttributeCategories", func(entry *DogmaAttributeCategory) {
		data.DogmaCategories[entry.Key] = entry
	}); err != nil {
		return nil, err
	}
	if err := decode(dir, "dogmaEffects", func(entry *DogmaEffect) {
		data.DogmaEffects[entry.Key] = entry
	}); err != nil {
		return nil, err
	}

	if err := decode(dir, "dbuffCollections", func(entry *DbuffCollection) {
		data.DbuffCollections[entry.Key] = entry
	}); err != nil {
		return nil, err
	}

	if err := decode(dir, "typeDogma", func(entry *typeDogmaEntry) {
		item, ok := data.Types[entry.Key]
		if !ok {
			return
		}
		item.DogmaAttributes = entry.DogmaAttributes
		item.DogmaEffects = entry.DogmaEffects
	}); err != nil {
		return nil, err
	}

	if err := decode(dir, "fighterAbilities", func(entry *FighterAbility) {
		data.FighterAbilities[entry.Key] = entry
	}); err != nil {
		return nil, err
	}

	if err := decode(dir, "fighterAbilitiesByType", func(entry *fighterAbilitiesEntry) {
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

	if err := decode(dir, "dynamicItemAttributes", func(entry *Mutaplasmid) {
		data.Mutaplasmids[entry.Key] = entry
	}); err != nil {
		return nil, err
	}

	if err := decode(dir, "typeBonus", func(entry *TypeBonuses) {
		data.TypeBonuses[entry.Key] = entry
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

type sdeEntry struct {
	Key         string    `json:"key"`
	ReleaseDate time.Time `json:"releaseDate"`
}

type typeDogmaEntry struct {
	Key             int32                `json:"typeID"`
	DogmaAttributes []TypeDogmaAttribute `json:"dogmaAttributes"`
	DogmaEffects    []TypeDogmaEffect    `json:"dogmaEffects"`
}

// The SDE gives each ability slot its own key rather than a list.
type fighterAbilitiesEntry struct {
	Key   int32               `json:"typeID"`
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

func decode[T any](dir string, table string, add func(*T)) error {
	file, err := os.Open(filepath.Join(dir, table+".jsonl.gz"))
	if err != nil {
		return err
	}
	defer file.Close()

	reader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("%s: %w", table, err)
	}
	defer reader.Close()

	decoder := json.NewDecoder(reader)
	for {
		var entry T
		if err := decoder.Decode(&entry); err == io.EOF {
			return nil
		} else if err != nil {
			return fmt.Errorf("%s: %w", table, err)
		}
		add(&entry)
	}
}
