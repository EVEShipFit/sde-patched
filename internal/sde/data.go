package sde

import (
	"encoding/json"
	"fmt"

	"github.com/EVEShipFit/sde-patched/internal/fbs/eve"
)

// Localized is a translated string as the SDE stores it. Only "en" is
// guaranteed to be present.
type Localized struct {
	En string `json:"en"`
	De string `json:"de"`
	Es string `json:"es"`
	Fr string `json:"fr"`
	Ja string `json:"ja"`
	Ko string `json:"ko"`
	Ru string `json:"ru"`
	Zh string `json:"zh"`
}

type TypeDogmaAttribute struct {
	AttributeID int32   `json:"attributeID"`
	Value       float64 `json:"value"`
}

type TypeDogmaEffect struct {
	EffectID  int32 `json:"effectID"`
	IsDefault bool  `json:"isDefault"`
}

type TypeFighterAbility struct {
	Slot             int8
	AbilityID        int32
	CooldownSeconds  float64
	ChargeCount      int32
	RearmTimeSeconds float64
}

type Type struct {
	Key           int32     `json:"_key"`
	Name          Localized `json:"name"`
	GroupID       int32     `json:"groupID"`
	Published     bool      `json:"published"`
	FactionID     int32     `json:"factionID"`
	MarketGroupID int32     `json:"marketGroupID"`
	MetaGroupID   int32     `json:"metaGroupID"`
	RaceID        int32     `json:"raceID"`
	Capacity      *float64  `json:"capacity"`
	Mass          *float64  `json:"mass"`
	Radius        *float64  `json:"radius"`
	Volume        *float64  `json:"volume"`

	// Merged in from typeDogma.jsonl; patches treat this as part of the type.
	DogmaAttributes []TypeDogmaAttribute `json:"-"`
	DogmaEffects    []TypeDogmaEffect    `json:"-"`

	// Merged in from fighterAbilitiesByType.jsonl.
	FighterAbilities []TypeFighterAbility `json:"-"`

	// CategoryID is resolved via the group after loading.
	CategoryID int32 `json:"-"`
}

type Group struct {
	Key        int32     `json:"_key"`
	Name       Localized `json:"name"`
	CategoryID int32     `json:"categoryID"`
	Published  bool      `json:"published"`
}

type Category struct {
	Key       int32     `json:"_key"`
	Name      Localized `json:"name"`
	Published bool      `json:"published"`
}

type MarketGroup struct {
	Key           int32     `json:"_key"`
	Name          Localized `json:"name"`
	ParentGroupID int32     `json:"parentGroupID"`
}

type MetaGroup struct {
	Key  int32     `json:"_key"`
	Name Localized `json:"name"`
}

type DogmaAttribute struct {
	Key            int32     `json:"_key"`
	Name           string    `json:"name"`
	DisplayName    Localized `json:"displayName"`
	DefaultValue   float64   `json:"defaultValue"`
	HighIsGood     bool      `json:"highIsGood"`
	Stackable      bool      `json:"stackable"`
	Published      bool      `json:"published"`
	UnitID         int32     `json:"unitID"`
	MinAttributeID int32     `json:"minAttributeID"`
	MaxAttributeID int32     `json:"maxAttributeID"`
}

// Modifier is one rule of an effect. The SDE stores domain and func as
// strings and leaves operation out for effect-stoppers; both are normalised
// into enums while loading, so nothing downstream has to deal with that.
type Modifier struct {
	Domain               eve.ModifierDomain
	Func                 eve.ModifierFunc
	Operation            eve.ModifierOperation
	ModifiedAttributeID  int32
	ModifyingAttributeID int32
	GroupID              int32
	SkillTypeID          int32
}

var modifierDomains = map[string]eve.ModifierDomain{
	"itemID":      eve.ModifierDomainItemID,
	"shipID":      eve.ModifierDomainShipID,
	"charID":      eve.ModifierDomainCharID,
	"otherID":     eve.ModifierDomainOtherID,
	"structureID": eve.ModifierDomainStructureID,
	"target":      eve.ModifierDomainTarget,
	"targetID":    eve.ModifierDomainTargetID,
}

func (m *Modifier) UnmarshalJSON(data []byte) error {
	var raw struct {
		Domain               string `json:"domain"`
		Func                 string `json:"func"`
		Operation            *int32 `json:"operation"`
		ModifiedAttributeID  int32  `json:"modifiedAttributeID"`
		ModifyingAttributeID int32  `json:"modifyingAttributeID"`
		GroupID              int32  `json:"groupID"`
		SkillTypeID          int32  `json:"skillTypeID"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	domain, ok := modifierDomains[raw.Domain]
	if !ok {
		return fmt.Errorf("unknown modifier domain %q", raw.Domain)
	}
	function, ok := eve.EnumValuesModifierFunc[raw.Func]
	if !ok {
		return fmt.Errorf("unknown modifier func %q", raw.Func)
	}

	operation := eve.ModifierOperationUnset
	if raw.Operation != nil {
		operation = eve.ModifierOperation(*raw.Operation)
	}

	*m = Modifier{
		Domain:               domain,
		Func:                 function,
		Operation:            operation,
		ModifiedAttributeID:  raw.ModifiedAttributeID,
		ModifyingAttributeID: raw.ModifyingAttributeID,
		GroupID:              raw.GroupID,
		SkillTypeID:          raw.SkillTypeID,
	}
	return nil
}

type DogmaEffect struct {
	Key                           int32      `json:"_key"`
	Name                          string     `json:"name"`
	DisplayName                   Localized  `json:"displayName"`
	EffectCategoryID              int32      `json:"effectCategoryID"`
	Published                     bool       `json:"published"`
	ElectronicChance              bool       `json:"electronicChance"`
	IsAssistance                  bool       `json:"isAssistance"`
	IsOffensive                   bool       `json:"isOffensive"`
	IsWarpSafe                    bool       `json:"isWarpSafe"`
	PropulsionChance              bool       `json:"propulsionChance"`
	RangeChance                   bool       `json:"rangeChance"`
	DisallowAutoRepeat            bool       `json:"disallowAutoRepeat"`
	DischargeAttributeID          int32      `json:"dischargeAttributeID"`
	DurationAttributeID           int32      `json:"durationAttributeID"`
	FalloffAttributeID            int32      `json:"falloffAttributeID"`
	FittingUsageChanceAttributeID int32      `json:"fittingUsageChanceAttributeID"`
	RangeAttributeID              int32      `json:"rangeAttributeID"`
	ResistanceAttributeID         int32      `json:"resistanceAttributeID"`
	TrackingSpeedAttributeID      int32      `json:"trackingSpeedAttributeID"`
	Modifiers                     []Modifier `json:"modifierInfo"`
}

// DbuffModifier is one rule of a buff. Unlike an effect's modifier it names no
// modifying attribute: the value comes from whoever applies the buff.
type DbuffModifier struct {
	Func                eve.ModifierFunc
	ModifiedAttributeID int32
	GroupID             int32
	SkillTypeID         int32
}

// DbuffCollection is a buff another ship can put on this one, like a command
// burst boost. The SDE splits its modifiers over four lists, one per func, and
// spells two of the operations differently than it does for an effect; both are
// normalised while loading.
type DbuffCollection struct {
	Key           int32
	DisplayName   Localized
	AggregateMode eve.DbuffAggregateMode
	Operation     eve.ModifierOperation
	Display       eve.DbuffDisplay
	Modifiers     []DbuffModifier
}

var dbuffDisplays = map[string]eve.DbuffDisplay{
	"ShowNormal":   eve.DbuffDisplayNormal,
	"ShowInverted": eve.DbuffDisplayInverted,
	"Hide":         eve.DbuffDisplayHidden,
}

var dbuffOperations = map[string]eve.ModifierOperation{
	"PreAssignment":  eve.ModifierOperationPreAssign,
	"PostAssignment": eve.ModifierOperationPostAssign,
}

func (d *DbuffCollection) UnmarshalJSON(data []byte) error {
	type modifier struct {
		DogmaAttributeID int32 `json:"dogmaAttributeID"`
		GroupID          int32 `json:"groupID"`
		SkillID          int32 `json:"skillID"`
	}
	var raw struct {
		Key                            int32      `json:"_key"`
		DisplayName                    Localized  `json:"displayName"`
		AggregateMode                  string     `json:"aggregateMode"`
		OperationName                  string     `json:"operationName"`
		ShowOutputValueInUI            string     `json:"showOutputValueInUI"`
		ItemModifiers                  []modifier `json:"itemModifiers"`
		LocationModifiers              []modifier `json:"locationModifiers"`
		LocationGroupModifiers         []modifier `json:"locationGroupModifiers"`
		LocationRequiredSkillModifiers []modifier `json:"locationRequiredSkillModifiers"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	aggregateMode, ok := eve.EnumValuesDbuffAggregateMode[raw.AggregateMode]
	if !ok {
		return fmt.Errorf("unknown buff aggregate mode %q", raw.AggregateMode)
	}
	operation, ok := dbuffOperations[raw.OperationName]
	if !ok {
		if operation, ok = eve.EnumValuesModifierOperation[raw.OperationName]; !ok {
			return fmt.Errorf("unknown buff operation %q", raw.OperationName)
		}
	}

	display, ok := dbuffDisplays[raw.ShowOutputValueInUI]
	if !ok {
		return fmt.Errorf("unknown buff display %q", raw.ShowOutputValueInUI)
	}

	*d = DbuffCollection{
		Key:           raw.Key,
		DisplayName:   raw.DisplayName,
		AggregateMode: aggregateMode,
		Operation:     operation,
		Display:       display,
	}

	for _, entry := range raw.ItemModifiers {
		d.Modifiers = append(d.Modifiers, DbuffModifier{
			Func:                eve.ModifierFuncItemModifier,
			ModifiedAttributeID: entry.DogmaAttributeID,
		})
	}
	for _, entry := range raw.LocationModifiers {
		d.Modifiers = append(d.Modifiers, DbuffModifier{
			Func:                eve.ModifierFuncLocationModifier,
			ModifiedAttributeID: entry.DogmaAttributeID,
		})
	}
	for _, entry := range raw.LocationGroupModifiers {
		d.Modifiers = append(d.Modifiers, DbuffModifier{
			Func:                eve.ModifierFuncLocationGroupModifier,
			ModifiedAttributeID: entry.DogmaAttributeID,
			GroupID:             entry.GroupID,
		})
	}
	for _, entry := range raw.LocationRequiredSkillModifiers {
		d.Modifiers = append(d.Modifiers, DbuffModifier{
			Func:                eve.ModifierFuncLocationRequiredSkillModifier,
			ModifiedAttributeID: entry.DogmaAttributeID,
			SkillTypeID:         entry.SkillID,
		})
	}

	return nil
}

type MutaplasmidAttribute struct {
	AttributeID int32   `json:"_key"`
	Min         float64 `json:"min"`
	Max         float64 `json:"max"`
}

type MutaplasmidMapping struct {
	ApplicableTypes []int32 `json:"applicableTypes"`
	ResultingType   int32   `json:"resultingType"`
}

type Mutaplasmid struct {
	Key        int32                  `json:"_key"`
	Attributes []MutaplasmidAttribute `json:"attributeIDs"`
	Mappings   []MutaplasmidMapping   `json:"inputOutputMapping"`
}

// Data is the part of the SDE this tool uses.
type Data struct {
	BuildNumber      int32
	Types            map[int32]*Type
	Groups           map[int32]*Group
	Categories       map[int32]*Category
	MarketGroups     map[int32]*MarketGroup
	MetaGroups       map[int32]*MetaGroup
	DogmaAttributes  map[int32]*DogmaAttribute
	DogmaEffects     map[int32]*DogmaEffect
	DbuffCollections map[int32]*DbuffCollection
	Mutaplasmids     map[int32]*Mutaplasmid
}
