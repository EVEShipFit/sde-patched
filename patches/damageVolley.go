package patches

import "github.com/EVEShipFit/sde-patched/internal/patch"

// The damage of a single volley of a module.
var damageVolley = patch.NewAttribute("damageVolley", calculated(0))

// A turret or launcher takes its damage from the charge it holds.
var damageVolleyAmmo = patch.NewEffect("damageVolleyAmmo", passive(
	other(patch.ModAdd, damageVolley, attrEmDamage),
	other(patch.ModAdd, damageVolley, attrExplosiveDamage),
	other(patch.ModAdd, damageVolley, attrKineticDamage),
	other(patch.ModAdd, damageVolley, attrThermalDamage),
))

// Damage over time is shown per target, not what the module could manage in a
// hypothetical situation, so it overwrites the module damage per second.
var damageVolleyDot = patch.NewEffect("damageVolleyDot", passive(
	other(patch.ModAdd, damageVolley, patch.Attribute("dotMaxDamagePerTick")),
	other(patch.PostAssign, patch.Attribute("damagePerSecondWithReload"), patch.Attribute("dotMaxDamagePerTick")),
	other(patch.PostAssign, patch.Attribute("damagePerSecondWithoutReload"), patch.Attribute("dotMaxDamagePerTick")),
))

var damageVolleyEffect = patch.NewEffect("damageVolley", passive(
	item(patch.ModAdd, damageVolley, attrEmDamage),
	item(patch.ModAdd, damageVolley, attrExplosiveDamage),
	item(patch.ModAdd, damageVolley, attrKineticDamage),
	item(patch.ModAdd, damageVolley, attrThermalDamage),
))

var damageVolleyMultiplier = patch.NewEffect("damageVolleyMultiplier", passive(
	item(patch.PostMul, damageVolley, patch.Attribute("damageMultiplier")),
))

var _ = patch.ApplyEffect(damageVolleyDot).On(isCharge, patch.HasAttribute(patch.Attribute("dotMaxDamagePerTick")))

var _ = patch.ApplyEffect(damageVolleyAmmo).On(isCharge, hasDamageAttribute)

var _ = patch.ApplyEffect(damageVolleyEffect).On(patch.Any(damagingModule, damagingDrone))

var _ = patch.ApplyEffect(damageVolleyMultiplier).On(
	patch.HasAttribute(patch.Attribute("damageMultiplier")),
	patch.Any(damagingModule, damagingDrone),
)
