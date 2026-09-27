package fbs

import (
	"bytes"
	"fmt"
	"os"

	"github.com/EVEShipFit/sde-patched/internal/fbs/eve"
)

// Equal reports whether two files written by Write, WriteNames or WriteTexts hold the
// same data. The SDE build number and release date are ignored, as a new SDE
// build often changes nothing a dogma-engine uses.
func Equal(a, b string) (bool, error) {
	rawA, err := withoutBuild(a)
	if err != nil {
		return false, err
	}
	rawB, err := withoutBuild(b)
	if err != nil {
		return false, err
	}
	return bytes.Equal(rawA, rawB), nil
}

func withoutBuild(filename string) ([]byte, error) {
	raw, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	switch {
	case eve.SdeBufferHasIdentifier(raw):
		root := eve.GetRootAsSde(raw, 0)
		root.MutateBuildNumber(0)
		clear(root.ReleaseDate())
	case eve.NamesBufferHasIdentifier(raw):
		root := eve.GetRootAsNames(raw, 0)
		root.MutateBuildNumber(0)
		clear(root.ReleaseDate())
	case eve.TextsBufferHasIdentifier(raw):
		root := eve.GetRootAsTexts(raw, 0)
		root.MutateBuildNumber(0)
		clear(root.ReleaseDate())
	default:
		return nil, fmt.Errorf("%s: not an SDE export", filename)
	}
	return raw, nil
}
