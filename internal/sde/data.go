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

type DogmaAttribute struct {
	Key          int32     `json:"_key"`
	Name         string    `json:"name"`
	DisplayName  Localized `json:"displayName"`
	DefaultValue float64   `json:"defaultValue"`
	HighIsGood   bool      `json:"highIsGood"`
	Stackable    bool      `json:"stackable"`
	Published    bool      `json:"published"`
	UnitID       int32     `json:"unitID"`
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

// Data is the part of the SDE this tool uses.
type Data struct {
	BuildNumber     int32
	Types           map[int32]*Type
	Groups          map[int32]*Group
	Categories      map[int32]*Category
	DogmaAttributes map[int32]*DogmaAttribute
	DogmaEffects    map[int32]*DogmaEffect
}
