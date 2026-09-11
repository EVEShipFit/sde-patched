package patches

import "github.com/EVEShipFit/sde-patched/internal/patch"

// The SDE puts the "online" effect in the "active" category. The EVE client
// does some magic here; for us it should simply be in the "online" category.
var _ = patch.ChangeEffect(patch.Effect("online")).SetCategory(patch.Online)
