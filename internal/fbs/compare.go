package fbs

import (
	"bytes"
	"fmt"
	"os"

	"github.com/EVEShipFit/sde-patched/internal/fbs/eve"
)

// Equal reports whether two files written by Write or WriteNames hold the
// same data. The SDE build number is ignored, as a new SDE build often changes
// nothing a dogma-engine uses.
func Equal(a, b string) (bool, error) {
	rawA, err := withoutBuildNumber(a)
	if err != nil {
		return false, err
	}
	rawB, err := withoutBuildNumber(b)
	if err != nil {
		return false, err
	}
	return bytes.Equal(rawA, rawB), nil
}

func withoutBuildNumber(filename string) ([]byte, error) {
	raw, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	switch {
	case eve.SdeBufferHasIdentifier(raw):
		eve.GetRootAsSde(raw, 0).MutateBuildNumber(0)
	case eve.NamesBufferHasIdentifier(raw):
		eve.GetRootAsNames(raw, 0).MutateBuildNumber(0)
	default:
		return nil, fmt.Errorf("%s: not an SDE export", filename)
	}
	return raw, nil
}
