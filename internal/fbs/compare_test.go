package fbs

import (
	"path/filepath"
	"testing"

	"github.com/EVEShipFit/sde-patched/internal/sde"
)

func TestEqual(t *testing.T) {
	renameType := func(data *sde.Data) { data.Types[2456].Name.En = "Hobgoblin III" }
	writers := map[string]struct {
		write  func(*sde.Data, string) error
		change func(*sde.Data)
	}{
		"sde.dat":   {Write, renameType},
		"names.dat": {WriteNames, renameType},
		"texts.dat": {WriteTexts, func(data *sde.Data) { data.DogmaAttributes[9].TooltipTitle.En = "Hull Hitpoints" }},
	}

	for name, writer := range writers {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			written := func(file string, change func(*sde.Data)) string {
				data := testData()
				change(data)
				filename := filepath.Join(dir, file)
				if err := writer.write(data, filename); err != nil {
					t.Fatal(err)
				}
				return filename
			}

			original := written("original", func(*sde.Data) {})
			rebuilt := written("rebuilt", func(data *sde.Data) {
				data.BuildNumber = 43
				data.ReleaseDate = data.ReleaseDate.AddDate(0, 0, 1)
			})
			changed := written("changed", writer.change)

			if equal, err := Equal(original, rebuilt); err != nil || !equal {
				t.Errorf("only the build changed: equal = %v, err = %v", equal, err)
			}
			if equal, err := Equal(original, changed); err != nil || equal {
				t.Errorf("the data changed: equal = %v, err = %v", equal, err)
			}
		})
	}
}
