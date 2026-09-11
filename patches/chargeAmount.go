package patches

import "github.com/EVEShipFit/sde-patched/internal/patch"

// How many charges fit in a module.
var chargeAmount = patch.NewAttribute("chargeAmount", calculated(1))

var chargeAmountEffect = patch.NewEffect("chargeAmount", passive(
	item(patch.PreAssign, chargeAmount, patch.Attribute("capacity")),
))

var chargeAmountAmmoEffect = patch.NewEffect("chargeAmountAmmo", passive(
	other(patch.PostDiv, chargeAmount, patch.Attribute("volume")),
))

// Only modules that can actually hold a charge.
var _ = patch.ApplyEffect(chargeAmountEffect).On(isModule, patch.Any(
	patch.HasAttribute(patch.Attribute("chargeGroup1")),
	patch.HasAttribute(patch.Attribute("chargeGroup2")),
	patch.HasAttribute(patch.Attribute("chargeGroup3")),
	patch.HasAttribute(patch.Attribute("chargeGroup4")),
	patch.HasAttribute(patch.Attribute("chargeGroup5")),
))

var _ = patch.ApplyEffect(chargeAmountAmmoEffect).On(isCharge)
