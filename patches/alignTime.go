package patches

import "github.com/EVEShipFit/sde-patched/internal/patch"

// Align time in seconds. The EVE client works this out itself, so the SDE has
// no attribute for it.
var alignTime = patch.NewAttribute("alignTime", patch.AttributeDef{
	DefaultValue: 1.3862943611198906, // -ln(0.25)
	HighIsGood:   false,
	Stackable:    true,
	Published:    true,
})

var alignTimeEffect = patch.NewEffect("alignTime", passive(
	item(patch.PostMul, alignTime, patch.Attribute("agility")),
	item(patch.PostMul, alignTime, patch.Attribute("mass")),
	// Twice: once for the mass in kilogrammes, once for the milliseconds.
	item(patch.PostDiv, alignTime, thousand),
	item(patch.PostDiv, alignTime, thousand),
))

var _ = patch.ApplyEffect(alignTimeEffect).On(isShip)
