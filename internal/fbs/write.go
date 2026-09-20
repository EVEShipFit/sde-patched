// Package fbs writes the SDE out as a flatbuffer.
//
// The eve sub-package is generated; re-generate it after changing the schema.
package fbs

//go:generate flatc --go -o . ../../specs/eve.fbs ../../specs/names.fbs

import (
	"encoding/binary"
	"math"
	"os"
	"slices"
	"sort"
	"strings"

	flatbuffers "github.com/google/flatbuffers/go"

	"github.com/EVEShipFit/sde-patched/internal/fbs/eve"
	"github.com/EVEShipFit/sde-patched/internal/sde"
)

// Write serialises the data into a flatbuffer file. The names of types in
// languages other than English are not in it; see WriteNames.
func Write(data *sde.Data, filename string) error {
	builder := flatbuffers.NewBuilder(64 * 1024 * 1024)

	types := writeTypes(builder, data)
	groups := writeGroups(builder, data)
	categories := writeCategories(builder, data)
	attributes := writeAttributes(builder, data)
	effects := writeEffects(builder, data)
	dbuffCollections := writeDbuffCollections(builder, data)
	mutaplasmids := writeMutaplasmids(builder, data)

	eve.SdeStart(builder)
	eve.SdeAddBuildNumber(builder, data.BuildNumber)
	eve.SdeAddTypes(builder, types)
	eve.SdeAddGroups(builder, groups)
	eve.SdeAddCategories(builder, categories)
	eve.SdeAddDogmaAttributes(builder, attributes)
	eve.SdeAddDogmaEffects(builder, effects)
	eve.SdeAddDbuffCollections(builder, dbuffCollections)
	eve.SdeAddMutaplasmids(builder, mutaplasmids)
	builder.FinishWithFileIdentifier(eve.SdeEnd(builder), []byte("ESF1"))

	return os.WriteFile(filename, builder.FinishedBytes(), 0o644)
}

func sortedKeys[T any](entries map[int32]T) []int32 {
	keys := make([]int32, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

// WriteNames serialises the name lookup into a flatbuffer file of its own.
// Every name of every type, in every language, so that a fit written in any of
// them leads back to a type.
func WriteNames(data *sde.Data, filename string) error {
	type entry struct {
		name   string
		typeID int32
	}

	entries := make([]entry, 0, len(data.Types)*4)
	seen := map[entry]bool{}

	for _, key := range sortedKeys(data.Types) {
		name := data.Types[key].Name

		for _, value := range []string{name.En, name.De, name.Es, name.Fr, name.Ja, name.Ko, name.Ru, name.Zh} {
			if value == "" {
				continue
			}

			// Most languages leave most names untouched, so the same name
			// turns up over and over for the same type.
			item := entry{name: strings.ToLower(value), typeID: key}
			if seen[item] {
				continue
			}
			seen[item] = true
			entries = append(entries, item)
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].name != entries[j].name {
			return entries[i].name < entries[j].name
		}
		return entries[i].typeID < entries[j].typeID
	})

	builder := flatbuffers.NewBuilder(64 * 1024 * 1024)

	// A vector of strings holds offsets, so every string has to be written
	// before the vector itself starts.
	offsets := make([]flatbuffers.UOffsetT, len(entries))
	for i, item := range entries {
		offsets[i] = builder.CreateSharedString(item.name)
	}

	eve.NamesStartNamesVector(builder, len(entries))
	for i := len(entries) - 1; i >= 0; i-- {
		builder.PrependUOffsetT(offsets[i])
	}
	names := builder.EndVector(len(entries))

	eve.NamesStartTypeIdsVector(builder, len(entries))
	for i := len(entries) - 1; i >= 0; i-- {
		builder.PrependInt32(entries[i].typeID)
	}
	typeIDs := builder.EndVector(len(entries))

	eve.NamesStart(builder)
	eve.NamesAddBuildNumber(builder, data.BuildNumber)
	eve.NamesAddNames(builder, names)
	eve.NamesAddTypeIds(builder, typeIDs)
	builder.FinishWithFileIdentifier(eve.NamesEnd(builder), []byte("ESFN"))

	return os.WriteFile(filename, builder.FinishedBytes(), 0o644)
}

func writeTypes(builder *flatbuffers.Builder, data *sde.Data) flatbuffers.UOffsetT {
	offsets := make([]flatbuffers.UOffsetT, 0, len(data.Types))

	// Many types carry identical dogma, so identical vectors are shared.
	attributeVectors := map[string]flatbuffers.UOffsetT{}
	effectVectors := map[string]flatbuffers.UOffsetT{}

	for _, key := range sortedKeys(data.Types) {
		entry := data.Types[key]

		name := builder.CreateSharedString(entry.Name.En)

		var attributes flatbuffers.UOffsetT
		if len(entry.DogmaAttributes) > 0 {
			cacheKey := attributeCacheKey(entry.DogmaAttributes)
			attributes = attributeVectors[cacheKey]
			if attributes == 0 {
				eve.TypeStartDogmaAttributesVector(builder, len(entry.DogmaAttributes))
				for i := len(entry.DogmaAttributes) - 1; i >= 0; i-- {
					attribute := entry.DogmaAttributes[i]
					eve.CreateTypeDogmaAttribute(builder, attribute.AttributeID, float32(attribute.Value))
				}
				attributes = builder.EndVector(len(entry.DogmaAttributes))
				attributeVectors[cacheKey] = attributes
			}
		}

		var effects flatbuffers.UOffsetT
		if len(entry.DogmaEffects) > 0 {
			cacheKey := effectCacheKey(entry.DogmaEffects)
			effects = effectVectors[cacheKey]
			if effects == 0 {
				eve.TypeStartDogmaEffectsVector(builder, len(entry.DogmaEffects))
				for i := len(entry.DogmaEffects) - 1; i >= 0; i-- {
					effect := entry.DogmaEffects[i]
					eve.CreateTypeDogmaEffect(builder, effect.EffectID, effect.IsDefault)
				}
				effects = builder.EndVector(len(entry.DogmaEffects))
				effectVectors[cacheKey] = effects
			}
		}

		var abilities flatbuffers.UOffsetT
		if len(entry.FighterAbilities) > 0 {
			eve.TypeStartFighterAbilitiesVector(builder, len(entry.FighterAbilities))
			for i := len(entry.FighterAbilities) - 1; i >= 0; i-- {
				ability := entry.FighterAbilities[i]
				eve.CreateTypeFighterAbility(builder,
					ability.Slot,
					ability.AbilityID,
					float32(ability.CooldownSeconds),
					ability.ChargeCount,
					float32(ability.RearmTimeSeconds),
				)
			}
			abilities = builder.EndVector(len(entry.FighterAbilities))
		}

		eve.TypeStart(builder)
		eve.TypeAddId(builder, entry.Key)
		eve.TypeAddName(builder, name)
		eve.TypeAddGroupId(builder, entry.GroupID)
		eve.TypeAddCategoryId(builder, entry.CategoryID)
		eve.TypeAddPublished(builder, entry.Published)
		eve.TypeAddFactionId(builder, entry.FactionID)
		eve.TypeAddMarketGroupId(builder, entry.MarketGroupID)
		eve.TypeAddMetaGroupId(builder, entry.MetaGroupID)
		eve.TypeAddRaceId(builder, entry.RaceID)
		if entry.Capacity != nil {
			eve.TypeAddCapacity(builder, float32(*entry.Capacity))
		}
		if entry.Mass != nil {
			eve.TypeAddMass(builder, float32(*entry.Mass))
		}
		if entry.Radius != nil {
			eve.TypeAddRadius(builder, float32(*entry.Radius))
		}
		if entry.Volume != nil {
			eve.TypeAddVolume(builder, float32(*entry.Volume))
		}
		if attributes != 0 {
			eve.TypeAddDogmaAttributes(builder, attributes)
		}
		if effects != 0 {
			eve.TypeAddDogmaEffects(builder, effects)
		}
		if abilities != 0 {
			eve.TypeAddFighterAbilities(builder, abilities)
		}
		offsets = append(offsets, eve.TypeEnd(builder))
	}

	return builder.CreateVectorOfSortedTables(offsets, eve.TypeKeyCompare)
}

func attributeCacheKey(entries []sde.TypeDogmaAttribute) string {
	key := make([]byte, 0, len(entries)*8)
	for _, entry := range entries {
		key = binary.LittleEndian.AppendUint32(key, uint32(entry.AttributeID))
		key = binary.LittleEndian.AppendUint32(key, math.Float32bits(float32(entry.Value)))
	}
	return string(key)
}

func effectCacheKey(entries []sde.TypeDogmaEffect) string {
	key := make([]byte, 0, len(entries)*5)
	for _, entry := range entries {
		key = binary.LittleEndian.AppendUint32(key, uint32(entry.EffectID))
		if entry.IsDefault {
			key = append(key, 1)
		} else {
			key = append(key, 0)
		}
	}
	return string(key)
}

func writeGroups(builder *flatbuffers.Builder, data *sde.Data) flatbuffers.UOffsetT {
	offsets := make([]flatbuffers.UOffsetT, 0, len(data.Groups))

	for _, key := range sortedKeys(data.Groups) {
		entry := data.Groups[key]
		name := builder.CreateString(entry.Name.En)

		eve.GroupStart(builder)
		eve.GroupAddId(builder, entry.Key)
		eve.GroupAddName(builder, name)
		eve.GroupAddCategoryId(builder, entry.CategoryID)
		eve.GroupAddPublished(builder, entry.Published)
		offsets = append(offsets, eve.GroupEnd(builder))
	}

	return builder.CreateVectorOfSortedTables(offsets, eve.GroupKeyCompare)
}

func writeCategories(builder *flatbuffers.Builder, data *sde.Data) flatbuffers.UOffsetT {
	offsets := make([]flatbuffers.UOffsetT, 0, len(data.Categories))

	for _, key := range sortedKeys(data.Categories) {
		entry := data.Categories[key]
		name := builder.CreateString(entry.Name.En)

		eve.CategoryStart(builder)
		eve.CategoryAddId(builder, entry.Key)
		eve.CategoryAddName(builder, name)
		eve.CategoryAddPublished(builder, entry.Published)
		offsets = append(offsets, eve.CategoryEnd(builder))
	}

	return builder.CreateVectorOfSortedTables(offsets, eve.CategoryKeyCompare)
}

func writeAttributes(builder *flatbuffers.Builder, data *sde.Data) flatbuffers.UOffsetT {
	offsets := make([]flatbuffers.UOffsetT, 0, len(data.DogmaAttributes))

	for _, key := range sortedKeys(data.DogmaAttributes) {
		entry := data.DogmaAttributes[key]
		name := builder.CreateString(entry.Name)
		displayName := builder.CreateString(entry.DisplayName.En)

		eve.DogmaAttributeStart(builder)
		eve.DogmaAttributeAddId(builder, entry.Key)
		eve.DogmaAttributeAddName(builder, name)
		eve.DogmaAttributeAddDisplayName(builder, displayName)
		eve.DogmaAttributeAddDefaultValue(builder, float32(entry.DefaultValue))
		eve.DogmaAttributeAddHighIsGood(builder, entry.HighIsGood)
		eve.DogmaAttributeAddStackable(builder, entry.Stackable)
		eve.DogmaAttributeAddPublished(builder, entry.Published)
		eve.DogmaAttributeAddUnitId(builder, entry.UnitID)
		eve.DogmaAttributeAddMinAttributeId(builder, entry.MinAttributeID)
		eve.DogmaAttributeAddMaxAttributeId(builder, entry.MaxAttributeID)
		offsets = append(offsets, eve.DogmaAttributeEnd(builder))
	}

	return builder.CreateVectorOfSortedTables(offsets, eve.DogmaAttributeKeyCompare)
}

func writeEffects(builder *flatbuffers.Builder, data *sde.Data) flatbuffers.UOffsetT {
	offsets := make([]flatbuffers.UOffsetT, 0, len(data.DogmaEffects))

	for _, key := range sortedKeys(data.DogmaEffects) {
		entry := data.DogmaEffects[key]
		name := builder.CreateString(entry.Name)
		displayName := builder.CreateString(entry.DisplayName.En)

		eve.DogmaEffectStartModifiersVector(builder, len(entry.Modifiers))
		for i := len(entry.Modifiers) - 1; i >= 0; i-- {
			modifier := entry.Modifiers[i]
			eve.CreateModifier(builder,
				modifier.Domain,
				modifier.Func,
				modifier.Operation,
				modifier.ModifiedAttributeID,
				modifier.ModifyingAttributeID,
				modifier.GroupID,
				modifier.SkillTypeID,
			)
		}
		modifiers := builder.EndVector(len(entry.Modifiers))

		eve.DogmaEffectStart(builder)
		eve.DogmaEffectAddId(builder, entry.Key)
		eve.DogmaEffectAddName(builder, name)
		eve.DogmaEffectAddDisplayName(builder, displayName)
		eve.DogmaEffectAddEffectCategory(builder, eve.EffectCategory(entry.EffectCategoryID))
		eve.DogmaEffectAddPublished(builder, entry.Published)
		eve.DogmaEffectAddElectronicChance(builder, entry.ElectronicChance)
		eve.DogmaEffectAddIsAssistance(builder, entry.IsAssistance)
		eve.DogmaEffectAddIsOffensive(builder, entry.IsOffensive)
		eve.DogmaEffectAddIsWarpSafe(builder, entry.IsWarpSafe)
		eve.DogmaEffectAddPropulsionChance(builder, entry.PropulsionChance)
		eve.DogmaEffectAddRangeChance(builder, entry.RangeChance)
		eve.DogmaEffectAddDisallowAutoRepeat(builder, entry.DisallowAutoRepeat)
		eve.DogmaEffectAddDischargeAttributeId(builder, entry.DischargeAttributeID)
		eve.DogmaEffectAddDurationAttributeId(builder, entry.DurationAttributeID)
		eve.DogmaEffectAddFalloffAttributeId(builder, entry.FalloffAttributeID)
		eve.DogmaEffectAddFittingUsageChanceAttributeId(builder, entry.FittingUsageChanceAttributeID)
		eve.DogmaEffectAddRangeAttributeId(builder, entry.RangeAttributeID)
		eve.DogmaEffectAddResistanceAttributeId(builder, entry.ResistanceAttributeID)
		eve.DogmaEffectAddTrackingSpeedAttributeId(builder, entry.TrackingSpeedAttributeID)
		eve.DogmaEffectAddModifiers(builder, modifiers)
		offsets = append(offsets, eve.DogmaEffectEnd(builder))
	}

	return builder.CreateVectorOfSortedTables(offsets, eve.DogmaEffectKeyCompare)
}

func writeDbuffCollections(builder *flatbuffers.Builder, data *sde.Data) flatbuffers.UOffsetT {
	offsets := make([]flatbuffers.UOffsetT, 0, len(data.DbuffCollections))

	for _, key := range sortedKeys(data.DbuffCollections) {
		entry := data.DbuffCollections[key]
		displayName := builder.CreateString(entry.DisplayName.En)

		eve.DbuffCollectionStartModifiersVector(builder, len(entry.Modifiers))
		for i := len(entry.Modifiers) - 1; i >= 0; i-- {
			modifier := entry.Modifiers[i]
			eve.CreateDbuffModifier(builder,
				modifier.Func,
				modifier.ModifiedAttributeID,
				modifier.GroupID,
				modifier.SkillTypeID,
			)
		}
		modifiers := builder.EndVector(len(entry.Modifiers))

		eve.DbuffCollectionStart(builder)
		eve.DbuffCollectionAddId(builder, entry.Key)
		eve.DbuffCollectionAddDisplayName(builder, displayName)
		eve.DbuffCollectionAddAggregateMode(builder, entry.AggregateMode)
		eve.DbuffCollectionAddOperation(builder, entry.Operation)
		eve.DbuffCollectionAddDisplay(builder, entry.Display)
		eve.DbuffCollectionAddModifiers(builder, modifiers)
		offsets = append(offsets, eve.DbuffCollectionEnd(builder))
	}

	return builder.CreateVectorOfSortedTables(offsets, eve.DbuffCollectionKeyCompare)
}

func writeMutaplasmids(builder *flatbuffers.Builder, data *sde.Data) flatbuffers.UOffsetT {
	offsets := make([]flatbuffers.UOffsetT, 0, len(data.Mutaplasmids))

	for _, key := range sortedKeys(data.Mutaplasmids) {
		entry := data.Mutaplasmids[key]

		mappingOffsets := make([]flatbuffers.UOffsetT, len(entry.Mappings))
		for i, mapping := range entry.Mappings {
			applicable := slices.Sorted(slices.Values(mapping.ApplicableTypes))
			eve.MutaplasmidMappingStartApplicableTypeIdsVector(builder, len(applicable))
			for j := len(applicable) - 1; j >= 0; j-- {
				builder.PrependInt32(applicable[j])
			}
			applicableTypeIDs := builder.EndVector(len(applicable))

			eve.MutaplasmidMappingStart(builder)
			eve.MutaplasmidMappingAddApplicableTypeIds(builder, applicableTypeIDs)
			eve.MutaplasmidMappingAddResultingTypeId(builder, mapping.ResultingType)
			mappingOffsets[i] = eve.MutaplasmidMappingEnd(builder)
		}

		eve.MutaplasmidStartMappingsVector(builder, len(mappingOffsets))
		for i := len(mappingOffsets) - 1; i >= 0; i-- {
			builder.PrependUOffsetT(mappingOffsets[i])
		}
		mappings := builder.EndVector(len(mappingOffsets))

		eve.MutaplasmidStartAttributesVector(builder, len(entry.Attributes))
		for i := len(entry.Attributes) - 1; i >= 0; i-- {
			attribute := entry.Attributes[i]
			eve.CreateMutaplasmidAttribute(builder, attribute.AttributeID, float32(attribute.Min), float32(attribute.Max))
		}
		attributes := builder.EndVector(len(entry.Attributes))

		eve.MutaplasmidStart(builder)
		eve.MutaplasmidAddId(builder, entry.Key)
		eve.MutaplasmidAddAttributes(builder, attributes)
		eve.MutaplasmidAddMappings(builder, mappings)
		offsets = append(offsets, eve.MutaplasmidEnd(builder))
	}

	return builder.CreateVectorOfSortedTables(offsets, eve.MutaplasmidKeyCompare)
}
