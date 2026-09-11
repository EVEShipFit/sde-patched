package patches

import "github.com/EVEShipFit/sde-patched/internal/patch"

// CPU and powergrid load is not part of the dogma data.
//
// It reuses the existing "cpuLoad" and "powerLoad": looking at their IDs,
// those are most likely meant to hold exactly this. "cpuFree" and "powerFree"
// show what is left.
var (
	cpuFree   = patch.NewAttribute("cpuFree", calculated(0))
	powerFree = patch.NewAttribute("powerFree", calculated(0))
)

var cpuPowerLoad = patch.NewEffect("cpuPowerLoad", online(
	ship(patch.ModAdd, patch.Attribute("cpuLoad"), patch.Attribute("cpu")),
	ship(patch.ModAdd, patch.Attribute("powerLoad"), patch.Attribute("power")),
))

var cpuPowerFree = patch.NewEffect("cpuPowerFree", passive(
	item(patch.PreAssign, cpuFree, patch.Attribute("cpuOutput")),
	item(patch.ModSub, cpuFree, patch.Attribute("cpuLoad")),
	item(patch.PreAssign, powerFree, patch.Attribute("powerOutput")),
	item(patch.ModSub, powerFree, patch.Attribute("powerLoad")),
))

var _ = patch.ApplyEffect(cpuPowerLoad).On(isModule)
var _ = patch.ApplyEffect(cpuPowerFree).On(isShip)
