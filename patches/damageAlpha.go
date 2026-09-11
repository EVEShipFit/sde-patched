package patches

import "github.com/EVEShipFit/sde-patched/internal/patch"

// Alpha strike: the damage of every gun firing at once.
var damageAlpha = patch.NewAttribute("damageAlpha", calculated(0))

var damageAlphaEffect = patch.NewEffect("damageAlpha", active(
	ship(patch.ModAdd, damageAlpha, patch.Attribute("damageVolley")),
))

var _ = patch.ApplyEffect(damageAlphaEffect).On(damagingModule)
