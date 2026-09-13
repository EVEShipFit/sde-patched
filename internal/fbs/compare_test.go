package fbs

import (
	"path/filepath"
	"testing"

	"github.com/EVEShipFit/sde-patched/internal/sde"
)

func TestEqual(t *testing.T) {
	writers := map[string]func(*sde.Data, string) error{
		"sde.dat":   Write,
		"names.dat": WriteNames,
	}

	for name, write := range writers {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			written := func(file string, change func(*sde.Data)) string {
				data := testData()
				change(data)
				filename := filepath.Join(dir, file)
				if err := write(data, filename); err != nil {
					t.Fatal(err)
				}
				return filename
			}

			original := written("original", func(*sde.Data) {})
			rebuilt := written("rebuilt", func(data *sde.Data) { data.BuildNumber = 43 })
			renamed := written("renamed", func(data *sde.Data) { data.Types[2456].Name.En = "Hobgoblin III" })

			if equal, err := Equal(original, rebuilt); err != nil || !equal {
				t.Errorf("only the build number changed: equal = %v, err = %v", equal, err)
			}
			if equal, err := Equal(original, renamed); err != nil || equal {
				t.Errorf("a name changed: equal = %v, err = %v", equal, err)
			}
		})
	}
}
