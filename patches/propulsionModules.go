package patches

import "github.com/EVEShipFit/sde-patched/internal/patch"

// Afterburners and microwarpdrives have no active effect in the dogma data;
// the EVE client handles them outside of dogma. Adding the effect here saves
// the dogma-engine from special-casing them.
//
// Note: the "mass" attribute is not right in the SDE. The "mass" field of the
// type is. This patch assumes the attribute holds the mass of the ship.
var velocityBoost = patch.NewAttribute("velocityBoost", calculated(0))

var velocityBoostEffect = patch.NewEffect("velocityBoost", passive(
	item(patch.PostDiv, velocityBoost, patch.Attribute("mass")),
	item(patch.PostPercent, patch.Attribute("maxVelocity"), velocityBoost),
))

var _ = patch.ChangeEffect(patch.Effect("moduleBonusMicrowarpdrive")).AddModifiers(
	ship(patch.PostPercent, patch.Attribute("signatureRadius"), patch.Attribute("signatureRadiusBonus")),
)

var _ = patch.ChangeEffect(patch.Effect("microJumpDrive")).AddModifiers(
	ship(patch.PostPercent, patch.Attribute("signatureRadius"), patch.Attribute("signatureRadiusBonusPercent")),
)

var propulsionModifiers = []patch.Modifier{
	ship(patch.ModAdd, patch.Attribute("mass"), patch.Attribute("massAddition")),
	ship(patch.ModAdd, velocityBoost, patch.Attribute("speedBoostFactor")),
	ship(patch.PostMul, velocityBoost, patch.Attribute("speedFactor")),
}

var _ = patch.ChangeEffect(patch.Effect("moduleBonusAfterburner")).AddModifiers(propulsionModifiers...)
var _ = patch.ChangeEffect(patch.Effect("moduleBonusMicrowarpdrive")).AddModifiers(propulsionModifiers...)

var _ = patch.ApplyEffect(velocityBoostEffect).On(isShip)
