package fbs

import (
	"strings"

	"github.com/EVEShipFit/sde-patched/internal/sde"
)

const (
	categoryShip       = 6
	groupShipModifiers = 1306
)

// shipModes gives the modes of each ship, lowest ID first.
//
// A mode is named after its ship, like "Confessor Defense Mode"; its ship is
// the longest start of that name that names a ship.
func shipModes(data *sde.Data) map[int32][]int32 {
	byName := map[string]*sde.Type{}
	for _, key := range sortedKeys(data.Types) {
		entry := data.Types[key]
		existing := byName[entry.Name.En]
		if existing == nil || (!existing.Published && entry.Published) {
			byName[entry.Name.En] = entry
		}
	}

	modes := map[int32][]int32{}
	for _, key := range sortedKeys(data.Types) {
		mode := data.Types[key]
		if mode.GroupID != groupShipModifiers {
			continue
		}

		words := strings.Split(mode.Name.En, " ")
		for count := len(words) - 1; count > 0; count-- {
			ship := byName[strings.Join(words[:count], " ")]
			if ship != nil && ship.CategoryID == categoryShip {
				modes[ship.Key] = append(modes[ship.Key], key)
				break
			}
		}
	}
	return modes
}
