package patches

import "github.com/EVEShipFit/sde-patched/internal/patch"

// Statistics show a single scan strength, but the SDE has one attribute per
// race. Three of the four are zero, so adding them up gives the one number a
// user interface wants.
var scanStrength = patch.NewAttribute("scanStrength", calculated(0))

var scanStrengthEffect = patch.NewEffect("scanStrength", passive(
	item(patch.ModAdd, scanStrength, patch.Attribute("scanRadarStrength")),
	item(patch.ModAdd, scanStrength, patch.Attribute("scanLadarStrength")),
	item(patch.ModAdd, scanStrength, patch.Attribute("scanMagnetometricStrength")),
	item(patch.ModAdd, scanStrength, patch.Attribute("scanGravimetricStrength")),
))

var _ = patch.ApplyEffect(scanStrengthEffect).On(isShip)
