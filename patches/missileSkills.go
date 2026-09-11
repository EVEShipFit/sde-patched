package patches

import "github.com/EVEShipFit/sde-patched/internal/patch"

// Some missile skills have no effect in the dogma data; the EVE client applies
// them outside of dogma. Adding the effect here saves the dogma-engine from
// special-casing them.
var missileDamage = patch.NewEffect("missileDamage", patch.EffectDef{
	Category:    patch.Passive,
	IsOffensive: true,
	IsWarpSafe:  true,
	Modifiers: []patch.Modifier{
		missileLauncherOperation(attrEmDamage),
		missileLauncherOperation(attrExplosiveDamage),
		missileLauncherOperation(attrKineticDamage),
		missileLauncherOperation(attrThermalDamage),
	},
})

func missileLauncherOperation(damage *patch.AttributeRef) patch.Modifier {
	return patch.Modifier{
		Domain:    patch.CharID,
		Func:      patch.OwnerRequiredSkillModifier,
		Operation: patch.PostMul,
		Modified:  damage,
		Modifying: patch.Attribute("missileDamageMultiplier"),
		Skill:     patch.Skill("Missile Launcher Operation"),
	}
}

var _ = patch.ApplyEffect(missileDamage).On(patch.Named("CharacterType"))
