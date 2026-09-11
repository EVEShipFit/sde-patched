package patches

import "github.com/EVEShipFit/sde-patched/internal/patch"

// Damage per second, with and without counting the time spent reloading.
var (
	damagePerSecondWithoutReload = patch.NewAttribute("damagePerSecondWithoutReload", calculated(0))
	damagePerSecondWithReload    = patch.NewAttribute("damagePerSecondWithReload", calculated(0))
	droneDamagePerSecond         = patch.NewAttribute("droneDamagePerSecond", calculated(0))
	speedOfReload                = patch.NewAttribute("speedOfReload", calculated(0))
	speedWithReload              = patch.NewAttribute("speedWithReload", calculated(0))
)

// Some modules keep their cycle time in "speed", others in "duration".
var dpsBasedOnSpeed = patch.NewEffect("damagePerSecondBasedOnSpeed", active(
	item(patch.PostDiv, damagePerSecondWithoutReload, attrSpeed),
	item(patch.PreAssign, speedWithReload, attrSpeed),
))

var dpsBasedOnDuration = patch.NewEffect("damagePerSecondBasedOnDuration", active(
	item(patch.PostDiv, damagePerSecondWithoutReload, attrDuration),
	item(patch.PreAssign, speedWithReload, attrDuration),
))

var dpsWithoutReload = patch.NewEffect("damagePerSecondWithoutReload", active(
	item(patch.PreAssign, damagePerSecondWithoutReload, thousand),
	item(patch.PostMul, damagePerSecondWithoutReload, damageVolley),
	ship(patch.ModAdd, damagePerSecondWithoutReload, damagePerSecondWithoutReload),
))

var dpsWithReload = patch.NewEffect("damagePerSecondWithReload", active(
	item(patch.PreAssign, speedOfReload, patch.Attribute("reloadTime")),
	item(patch.PostDiv, speedOfReload, chargeAmount),

	item(patch.ModAdd, speedWithReload, speedOfReload),

	item(patch.PreAssign, damagePerSecondWithReload, thousand),
	item(patch.PostMul, damagePerSecondWithReload, damageVolley),
	item(patch.PostDiv, damagePerSecondWithReload, speedWithReload),
	ship(patch.ModAdd, damagePerSecondWithReload, damagePerSecondWithReload),
))

// Drones never reload, so their damage without reload is their damage.
var dpsWithReloadDrone = patch.NewEffect("damagePerSecondWithReloadDrone", active(
	ship(patch.ModAdd, droneDamagePerSecond, damagePerSecondWithoutReload),
	ship(patch.ModAdd, damagePerSecondWithReload, damagePerSecondWithoutReload),
))

// Scan probe launchers have both, so prefer "speed": dividing by both would
// be wrong, even though their damage is meaningless anyway.
var (
	moduleCyclesOnSpeed    = patch.As("a damaging module that cycles on speed", patch.All(damagingModule, patch.HasAttribute(attrSpeed)))
	moduleCyclesOnDuration = patch.As("a damaging module that cycles on duration", patch.All(damagingModule, patch.HasAttribute(attrDuration), patch.Not(patch.HasAttribute(attrSpeed))))
	cyclingDamageDealer    = patch.As("anything that deals damage in cycles", patch.Any(moduleCyclesOnSpeed, moduleCyclesOnDuration, damagingDrone))
)

var _ = patch.ApplyEffect(dpsBasedOnSpeed).On(patch.Any(moduleCyclesOnSpeed, damagingDrone))
var _ = patch.ApplyEffect(dpsBasedOnDuration).On(moduleCyclesOnDuration)

var _ = patch.ApplyEffect(dpsWithoutReload).On(cyclingDamageDealer)
var _ = patch.ApplyEffect(dpsWithReload).On(patch.Any(moduleCyclesOnSpeed, moduleCyclesOnDuration))
var _ = patch.ApplyEffect(dpsWithReloadDrone).On(patch.Any(damagingDrone, moduleCyclesOnDuration))
