// Package patch declares changes to the dogma data of the SDE.
//
// Patches are Go files in the "patches" package. They declare what they want
// at package-level, which means Go itself works out the order: an effect that
// uses an attribute is always built after that attribute.
//
// Nothing is looked up while declaring; the SDE is not loaded yet at that
// point. A declaration only writes down the intent, and Apply resolves it.
package patch

import "github.com/EVEShipFit/sde-patched/internal/fbs/eve"

type (
	EffectCategory    = eve.EffectCategory
	ModifierDomain    = eve.ModifierDomain
	ModifierFunc      = eve.ModifierFunc
	ModifierOperation = eve.ModifierOperation
)

const (
	Passive  = eve.EffectCategoryPassive
	Active   = eve.EffectCategoryActive
	Target   = eve.EffectCategoryTarget
	Area     = eve.EffectCategoryArea
	Online   = eve.EffectCategoryOnline
	Overload = eve.EffectCategoryOverload
	Dungeon  = eve.EffectCategoryDungeon
	System   = eve.EffectCategorySystem
)

const (
	ItemID      = eve.ModifierDomainItemID
	ShipID      = eve.ModifierDomainShipID
	CharID      = eve.ModifierDomainCharID
	OtherID     = eve.ModifierDomainOtherID
	StructureID = eve.ModifierDomainStructureID
	TargetID    = eve.ModifierDomainTargetID
	// DomainTarget, not Target, as Target is already an effect category.
	DomainTarget = eve.ModifierDomainTarget
)

const (
	ItemModifier                  = eve.ModifierFuncItemModifier
	LocationGroupModifier         = eve.ModifierFuncLocationGroupModifier
	LocationModifier              = eve.ModifierFuncLocationModifier
	LocationRequiredSkillModifier = eve.ModifierFuncLocationRequiredSkillModifier
	OwnerRequiredSkillModifier    = eve.ModifierFuncOwnerRequiredSkillModifier
	EffectStopper                 = eve.ModifierFuncEffectStopper
)

const (
	PreAssign   = eve.ModifierOperationPreAssign
	PreMul      = eve.ModifierOperationPreMul
	PreDiv      = eve.ModifierOperationPreDiv
	ModAdd      = eve.ModifierOperationModAdd
	ModSub      = eve.ModifierOperationModSub
	PostMul     = eve.ModifierOperationPostMul
	PostDiv     = eve.ModifierOperationPostDiv
	PostPercent = eve.ModifierOperationPostPercent
	PostAssign  = eve.ModifierOperationPostAssign
)

// AttributeDef describes a dogma attribute that does not exist in the SDE.
type AttributeDef struct {
	// ID pins the ID of the attribute. Leave it zero to get one assigned; all
	// assigned IDs are negative, so they stand out as not being CCP's.
	ID int32

	DisplayName  string
	DefaultValue float64
	HighIsGood   bool
	Stackable    bool
	Published    bool
	UnitID       int32
}

// EffectDef describes a dogma effect that does not exist in the SDE.
type EffectDef struct {
	ID int32

	DisplayName      string
	Category         EffectCategory
	Published        bool
	ElectronicChance bool
	IsAssistance     bool
	IsOffensive      bool
	IsWarpSafe       bool
	PropulsionChance bool
	RangeChance      bool

	DischargeAttribute          *AttributeRef
	DurationAttribute           *AttributeRef
	FalloffAttribute            *AttributeRef
	FittingUsageChanceAttribute *AttributeRef
	RangeAttribute              *AttributeRef
	ResistanceAttribute         *AttributeRef
	TrackingSpeedAttribute      *AttributeRef

	Modifiers []Modifier
}

// Modifier is one rule of an effect: what it changes, and with what.
type Modifier struct {
	Domain    ModifierDomain
	Func      ModifierFunc
	Operation ModifierOperation

	Modified  *AttributeRef
	Modifying *AttributeRef

	// Group is only used by LocationGroupModifier, Skill only by the two
	// required-skill modifiers.
	Group *GroupRef
	Skill *TypeRef
}
