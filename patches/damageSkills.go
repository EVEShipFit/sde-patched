package patches

import "github.com/EVEShipFit/sde-patched/internal/patch"

// Some damage skills have effects the EVE client applies itself, as they would
// be awkward to write in dogma data. Each of these applies to whatever item
// lists the skill as a required skill.
var _ = patch.ChangeEffect(patch.Effect("missileEMDmgBonus")).
	AddModifiers(ownerSkill(patch.PostPercent, attrEmDamage, patch.Attribute("damageMultiplierBonus")))

var _ = patch.ChangeEffect(patch.Effect("missileExplosiveDmgBonus")).
	AddModifiers(ownerSkill(patch.PostPercent, attrExplosiveDamage, patch.Attribute("damageMultiplierBonus")))

var _ = patch.ChangeEffect(patch.Effect("missileKineticDmgBonus2")).
	AddModifiers(ownerSkill(patch.PostPercent, attrKineticDamage, patch.Attribute("damageMultiplierBonus")))

var _ = patch.ChangeEffect(patch.Effect("missileThermalDmgBonus")).
	AddModifiers(ownerSkill(patch.PostPercent, attrThermalDamage, patch.Attribute("damageMultiplierBonus")))

var _ = patch.ChangeEffect(patch.Effect("selfRof")).
	AddModifiers(locationSkill(patch.PostPercent, attrSpeed, patch.Attribute("rofBonus")))

var _ = patch.ChangeEffect(patch.Effect("droneDmgBonus")).
	AddModifiers(ownerSkill(patch.PostPercent, patch.Attribute("damageMultiplier"), patch.Attribute("damageMultiplierBonus")))
