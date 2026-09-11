package patches

import "github.com/EVEShipFit/sde-patched/internal/patch"

// Capacitor information the EVE client works out itself.
//
//   - capacitorPeakRecharge is the recharge in GJ/s at its peak.
//   - capacitorPeakLoad is the drain in GJ if every module runs at once.
//   - capacitorPeakDelta is the difference between the two.
var (
	// Peak recharge is 5.0 / 2.0 * capacitorCapacity / rechargeRate.
	capacitorPeakRecharge = patch.NewAttribute("capacitorPeakRecharge", calculated(2.5))
	cycleTime             = patch.NewAttribute("cycleTime", patch.AttributeDef{
		DefaultValue: 0,
		HighIsGood:   false,
		Stackable:    true,
		Published:    true,
	})
	capacitorPeakLoad            = patch.NewAttribute("capacitorPeakLoad", calculated(0))
	capacitorPeakDelta           = patch.NewAttribute("capacitorPeakDelta", calculated(0))
	capacitorPeakDeltaPercentage = patch.NewAttribute("capacitorPeakDeltaPercentage", calculated(100))

	// Not done with dogma, as it needs simulation. The dogma-engine fills it in.
	_ = patch.NewAttribute("capacitorDepletesIn", calculated(0))
)

var capacitorPeakRechargeEffect = patch.NewEffect("capacitorPeakRecharge", passive(
	item(patch.PostMul, capacitorPeakRecharge, patch.Attribute("capacitorCapacity")),
	item(patch.PostDiv, capacitorPeakRecharge, patch.Attribute("rechargeRate")),
	item(patch.PostMul, capacitorPeakRecharge, thousand),
))

// Modules keep their cycle time in one of four different attributes.
var (
	cycleTimeDuration = patch.NewEffect("cycleTimeDuration", active(
		item(patch.ModAdd, cycleTime, attrDuration),
	))
	cycleTimeDurationHighisGood = patch.NewEffect("cycleTimeDurationHighisGood", active(
		item(patch.ModAdd, cycleTime, patch.Attribute("durationHighisGood")),
	))
	cycleTimeSpeed = patch.NewEffect("cycleTimeSpeed", active(
		item(patch.ModAdd, cycleTime, attrSpeed),
	))
	cycleTimeReactivationDelay = patch.NewEffect("cycleTimeReactivationDelay", active(
		item(patch.ModAdd, cycleTime, patch.Attribute("moduleReactivationDelay")),
	))
)

var capacitorPeakLoadEffect = patch.NewEffect("capacitorPeakLoad", active(
	item(patch.PreAssign, capacitorPeakLoad, patch.Attribute("capacitorNeed")),
	item(patch.PostDiv, capacitorPeakLoad, cycleTime),
	item(patch.PostMul, capacitorPeakLoad, thousand),
	ship(patch.ModAdd, capacitorPeakLoad, capacitorPeakLoad),
))

var capacitorPeakDeltaEffect = patch.NewEffect("capacitorPeakDelta", passive(
	item(patch.PreAssign, capacitorPeakDelta, capacitorPeakRecharge),
	item(patch.ModSub, capacitorPeakDelta, capacitorPeakLoad),
	item(patch.PostMul, capacitorPeakDeltaPercentage, capacitorPeakDelta),
	item(patch.PostDiv, capacitorPeakDeltaPercentage, capacitorPeakRecharge),
))

var usesCapacitor = patch.All(isModule, patch.HasAttribute(patch.Attribute("capacitorNeed")))

var _ = patch.ApplyEffect(capacitorPeakRechargeEffect).On(isShip)
var _ = patch.ApplyEffect(capacitorPeakDeltaEffect).On(isShip)
var _ = patch.ApplyEffect(capacitorPeakLoadEffect).On(usesCapacitor)

var _ = patch.ApplyEffect(cycleTimeDuration).On(usesCapacitor, patch.HasAttribute(attrDuration))
var _ = patch.ApplyEffect(cycleTimeDurationHighisGood).On(usesCapacitor, patch.HasAttribute(patch.Attribute("durationHighisGood")))
var _ = patch.ApplyEffect(cycleTimeSpeed).On(usesCapacitor, patch.HasAttribute(attrSpeed))
var _ = patch.ApplyEffect(cycleTimeReactivationDelay).On(usesCapacitor, patch.HasAttribute(patch.Attribute("moduleReactivationDelay")))
