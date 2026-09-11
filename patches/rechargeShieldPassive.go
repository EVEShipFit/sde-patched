package patches

import "github.com/EVEShipFit/sde-patched/internal/patch"

// Shields recharge on their own; this is how fast.
var (
	passiveShieldRechargeRate          = patch.NewAttribute("passiveShieldRechargeRate", calculated(2500))
	passiveShieldEffectiveRechargeRate = patch.NewAttribute("passiveShieldEffectiveRechargeRate", calculated(2500))
)

var passiveShieldRechargeRateEffect = patch.NewEffect("passiveShieldRechargeRate", passive(
	item(patch.PostDiv, passiveShieldRechargeRate, patch.Attribute("shieldRechargeRate")),
	item(patch.PostMul, passiveShieldRechargeRate, patch.Attribute("shieldCapacity")),

	item(patch.PostDiv, passiveShieldEffectiveRechargeRate, patch.Attribute("shieldRechargeRate")),
	item(patch.PostMul, passiveShieldEffectiveRechargeRate, patch.Attribute("shieldCapacity")),
	item(patch.PostDiv, passiveShieldEffectiveRechargeRate, shield.effectiveResonance),
))

var _ = patch.ApplyEffect(passiveShieldRechargeRateEffect).On(isShip)
