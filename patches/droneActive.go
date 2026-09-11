package patches

import "github.com/EVEShipFit/sde-patched/internal/patch"

// How many drones are active, and how much bandwidth they use.
var (
	droneActive = patch.NewAttribute("droneActive", calculated(0))
	droneUsage  = patch.NewAttribute("droneUsage", calculated(1))
)

var droneActiveEffect = patch.NewEffect("droneActive", active(
	ship(patch.ModAdd, droneActive, droneUsage),
	ship(patch.ModAdd, patch.Attribute("droneBandwidthLoad"), patch.Attribute("droneBandwidthUsed")),
))

var _ = patch.ApplyEffect(droneActiveEffect).On(isDrone)
