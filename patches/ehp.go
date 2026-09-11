package patches

import "github.com/EVEShipFit/sde-patched/internal/patch"

// Shield, armor and hull are hitpoints in the dogma data. What a user wants to
// see is effective hitpoints, which take resistances into account.
//
// The three layers work the same way, they only differ in which attributes the
// SDE holds their resistances in.
type ehpLayer struct {
	// effectiveResonance is the resistance left after the damage profile is
	// applied; the repair patches need it too.
	effectiveResonance *patch.AttributeRef
	ehp                *patch.AttributeRef
	effect             *patch.EffectRef
}

func newEhpLayer(layer string, resonances [4]*patch.AttributeRef, hitpoints *patch.AttributeRef) ehpLayer {
	damageTypes := [4]string{"Em", "Explosive", "Kinetic", "Thermal"}
	profiles := [4]*patch.AttributeRef{damageProfileEm, damageProfileExplosive, damageProfileKinetic, damageProfileThermal}

	effectiveResonance := patch.NewAttribute(layer+"DamageEffectiveResonance", calculated(0))
	ehp := patch.NewAttribute(layer+"Ehp", calculated(0))

	var modifiers []patch.Modifier
	for i, damageType := range damageTypes {
		perType := patch.NewAttribute(layer+damageType+"DamageEffectiveResonance", calculated(0))
		modifiers = append(modifiers,
			item(patch.PreAssign, perType, profiles[i]),
			item(patch.PostMul, perType, resonances[i]),
			item(patch.ModAdd, effectiveResonance, perType),
		)
	}
	modifiers = append(modifiers,
		item(patch.PreAssign, ehp, hitpoints),
		item(patch.PostDiv, ehp, effectiveResonance),
	)

	return ehpLayer{
		effectiveResonance: effectiveResonance,
		ehp:                ehp,
		effect:             patch.NewEffect(layer+"Ehp", passive(modifiers...)),
	}
}

var armor = newEhpLayer("armor", [4]*patch.AttributeRef{
	patch.Attribute("armorEmDamageResonance"),
	patch.Attribute("armorExplosiveDamageResonance"),
	patch.Attribute("armorKineticDamageResonance"),
	patch.Attribute("armorThermalDamageResonance"),
}, patch.Attribute("armorHP"))

var hull = newEhpLayer("hull", [4]*patch.AttributeRef{
	patch.Attribute("emDamageResonance"),
	patch.Attribute("explosiveDamageResonance"),
	patch.Attribute("kineticDamageResonance"),
	patch.Attribute("thermalDamageResonance"),
}, patch.Attribute("hp"))

var shield = newEhpLayer("shield", [4]*patch.AttributeRef{
	patch.Attribute("shieldEmDamageResonance"),
	patch.Attribute("shieldExplosiveDamageResonance"),
	patch.Attribute("shieldKineticDamageResonance"),
	patch.Attribute("shieldThermalDamageResonance"),
}, patch.Attribute("shieldCapacity"))

var ehpAttr = patch.NewAttribute("ehp", calculated(0))

var ehpEffect = patch.NewEffect("ehp", passive(
	item(patch.ModAdd, ehpAttr, shield.ehp),
	item(patch.ModAdd, ehpAttr, armor.ehp),
	item(patch.ModAdd, ehpAttr, hull.ehp),
))

var _ = patch.ApplyEffect(armor.effect).On(isShip)
var _ = patch.ApplyEffect(hull.effect).On(isShip)
var _ = patch.ApplyEffect(shield.effect).On(isShip)
var _ = patch.ApplyEffect(ehpEffect).On(isShip)
