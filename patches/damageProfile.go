package patches

import "github.com/EVEShipFit/sde-patched/internal/patch"

// Incoming damage needs an assumed damage profile. It is uniform by default:
// equal damage on all four resistances. The four should always add up to
// exactly one, or effective hitpoints come out wrong.
var (
	damageProfileEm        = patch.NewAttribute("damageProfileEm", calculated(0.25))
	damageProfileExplosive = patch.NewAttribute("damageProfileExplosive", calculated(0.25))
	damageProfileKinetic   = patch.NewAttribute("damageProfileKinetic", calculated(0.25))
	damageProfileThermal   = patch.NewAttribute("damageProfileThermal", calculated(0.25))
)
