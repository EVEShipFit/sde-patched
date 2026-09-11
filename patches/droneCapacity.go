package patches

import "github.com/EVEShipFit/sde-patched/internal/patch"

// How much of the drone bay is in use.
var droneCapacityLoad = patch.NewAttribute("droneCapacityLoad", calculated(0))

var droneLoad = patch.NewEffect("droneLoad", passive(
	ship(patch.ModAdd, droneCapacityLoad, patch.Attribute("volume")),
))

var _ = patch.ApplyEffect(droneLoad).On(isDrone)
