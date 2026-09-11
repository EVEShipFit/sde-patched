package patches

import "github.com/EVEShipFit/sde-patched/internal/patch"

// Some millisecond to second calculations need the value 1000. Dogma can only
// multiply by other attributes, so the constant has to be an attribute.
var thousand = patch.NewAttribute("thousand", patch.AttributeDef{
	DefaultValue: 1000,
	HighIsGood:   true,
	Stackable:    true,
	Published:    true,
})
