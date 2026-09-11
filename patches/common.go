package patches

import "github.com/EVEShipFit/sde-patched/internal/patch"

// Attributes and selectors that more than one patch needs.
var (
	attrEmDamage        = patch.Attribute("emDamage")
	attrExplosiveDamage = patch.Attribute("explosiveDamage")
	attrKineticDamage   = patch.Attribute("kineticDamage")
	attrThermalDamage   = patch.Attribute("thermalDamage")
	attrDuration        = patch.Attribute("duration")
	attrSpeed           = patch.Attribute("speed")
)

// hasDamageAttribute matches items that carry a damage value themselves.
var hasDamageAttribute = patch.As("has a damage attribute", patch.Any(
	patch.HasAttribute(attrEmDamage),
	patch.HasAttribute(attrExplosiveDamage),
	patch.HasAttribute(attrKineticDamage),
	patch.HasAttribute(attrThermalDamage),
))

// dealsDamage also covers turrets and missile launchers, which hold no damage
// value of their own: their charge does.
var dealsDamage = patch.As("deals damage", patch.Any(
	hasDamageAttribute,
	patch.HasEffect(patch.Effect("useMissiles")),
	patch.HasEffect(patch.Effect("turretFitted")),
))

var (
	isShip   = patch.InCategory("Ship")
	isModule = patch.InCategory("Module")
	isDrone  = patch.InCategory("Drone")
	isCharge = patch.InCategory("Charge")
)

var (
	damagingModule = patch.As("a module that deals damage", patch.All(isModule, dealsDamage))
	damagingDrone  = patch.As("a drone that deals damage", patch.All(isDrone, hasDamageAttribute))
)

// passive and active build the effect definition for the two kinds of effect
// we add: one that always calculates, and one that only calculates while the
// module is running.
//
// These build a definition rather than the effect itself, so that every patch
// still declares its own effects and "explain" can name the right file.
func passive(modifiers ...patch.Modifier) patch.EffectDef {
	return patch.EffectDef{Category: patch.Passive, IsWarpSafe: true, Modifiers: modifiers}
}

func active(modifiers ...patch.Modifier) patch.EffectDef {
	return patch.EffectDef{Category: patch.Active, IsWarpSafe: true, Modifiers: modifiers}
}

func online(modifiers ...patch.Modifier) patch.EffectDef {
	return patch.EffectDef{Category: patch.Online, IsWarpSafe: true, Modifiers: modifiers}
}

// calculated is what nearly every attribute we add looks like: a number that
// only exists to hold the result of a calculation.
func calculated(defaultValue float64) patch.AttributeDef {
	return patch.AttributeDef{DefaultValue: defaultValue, HighIsGood: true, Stackable: true, Published: true}
}

// item, ship and other build a modifier for one domain; they keep the long
// lists of modifiers in the patches readable.
func item(operation patch.ModifierOperation, modified, modifying *patch.AttributeRef) patch.Modifier {
	return patch.Modifier{Domain: patch.ItemID, Func: patch.ItemModifier, Operation: operation, Modified: modified, Modifying: modifying}
}

func ship(operation patch.ModifierOperation, modified, modifying *patch.AttributeRef) patch.Modifier {
	return patch.Modifier{Domain: patch.ShipID, Func: patch.ItemModifier, Operation: operation, Modified: modified, Modifying: modifying}
}

func other(operation patch.ModifierOperation, modified, modifying *patch.AttributeRef) patch.Modifier {
	return patch.Modifier{Domain: patch.OtherID, Func: patch.ItemModifier, Operation: operation, Modified: modified, Modifying: modifying}
}

// ownerSkill applies to whoever has this skill as a required skill.
func ownerSkill(operation patch.ModifierOperation, modified, modifying *patch.AttributeRef) patch.Modifier {
	return patch.Modifier{Domain: patch.CharID, Func: patch.OwnerRequiredSkillModifier, Operation: operation, Modified: modified, Modifying: modifying, Skill: patch.AnySkill()}
}

func locationSkill(operation patch.ModifierOperation, modified, modifying *patch.AttributeRef) patch.Modifier {
	return patch.Modifier{Domain: patch.ShipID, Func: patch.LocationRequiredSkillModifier, Operation: operation, Modified: modified, Modifying: modifying, Skill: patch.AnySkill()}
}
