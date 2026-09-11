package patches

import "github.com/EVEShipFit/sde-patched/internal/patch"

// Repair and boost rates, for the modules that do the repairing and for the
// ship that receives it. The effective rate also takes resistances into
// account, so it can be compared with incoming damage.
//
// The three layers work the same way; they differ in which attributes the SDE
// keeps their numbers in.
type repairLayer struct {
	rate          *patch.AttributeRef
	effectiveRate *patch.AttributeRef
}

func newRepairLayer(rateName, effectiveName string, amount *patch.AttributeRef, layer ehpLayer, module patch.Selector) repairLayer {
	rate := patch.NewAttribute(rateName, calculated(0))
	effectiveRate := patch.NewAttribute(effectiveName, calculated(0))

	rateEffect := patch.NewEffect(rateName, active(
		item(patch.PreAssign, rate, thousand),
		item(patch.PostMul, rate, amount),
		item(patch.PostDiv, rate, attrDuration),
		ship(patch.ModAdd, rate, rate),
	))

	effectiveEffect := patch.NewEffect(effectiveName, passive(
		item(patch.ModAdd, effectiveRate, rate),
		item(patch.PostDiv, effectiveRate, layer.effectiveResonance),
	))

	patch.ApplyEffect(rateEffect).On(module)
	patch.ApplyEffect(effectiveEffect).On(isShip)

	return repairLayer{rate: rate, effectiveRate: effectiveRate}
}

var armorRepair = newRepairLayer("armorRepairRate", "armorEffectiveRepairRate",
	patch.Attribute("armorDamageAmount"), armor,
	patch.All(isModule,
		patch.HasAttribute(patch.Attribute("armorDamageAmount")),
		patch.HasAttribute(attrDuration),
		patch.HasEffect(patch.Effect("armorRepair")),
	))

var hullRepair = newRepairLayer("hullRepairRate", "hullEffectiveRepairRate",
	patch.Attribute("structureDamageAmount"), hull,
	patch.All(isModule,
		patch.HasAttribute(patch.Attribute("structureDamageAmount")),
		patch.HasAttribute(attrDuration),
		patch.HasEffect(patch.Effect("structureRepair")),
	))

var shieldBoost = newRepairLayer("shieldBoostRate", "shieldEffectiveBoostRate",
	patch.Attribute("shieldBonus"), shield,
	patch.All(isModule,
		patch.HasAttribute(patch.Attribute("shieldBonus")),
		patch.HasEffect(patch.Effect("shieldBoosting")),
	))
