// Command sde-patched downloads the EVE SDE, patches its dogma data, and
// writes it out as a flatbuffer.
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/EVEShipFit/sde-patched/internal/fbs"
	"github.com/EVEShipFit/sde-patched/internal/patch"
	"github.com/EVEShipFit/sde-patched/internal/sde"
	"github.com/EVEShipFit/sde-patched/internal/web"
)

var (
	sdeDir     = flag.String("sde-dir", "sde", "directory the SDE is downloaded to")
	patchesDir = flag.String("patches-dir", "patches", "directory the patches are read from")
	out        = flag.String("out", filepath.Join("dist", "sde.dat"), "file to write")
	namesOut   = flag.String("names-out", filepath.Join("dist", "names.dat"), "name lookup file to write")
	buildNum   = flag.Int("build", 0, "SDE build to use; defaults to the latest")
	full       = flag.Bool("full", false, "explain: list every matched type")
	typeName   = flag.String("type", "", "explain: show what touches this type")
	addr       = flag.String("addr", "localhost:8080", "serve: address to listen on")
)

func usage() {
	fmt.Fprint(os.Stderr, `usage: sde-patched [flags] <command>

commands:
  download   fetch the SDE
  build      patch the SDE and write the flatbuffer
  compare    tell whether the build differs from the one in a directory; "compare <dir>"
  explain    show what a patch does; "explain <name>" or "explain --type <name>"
  patches    list all patches
  ids        give an ID to anything the patches added without one
  serve      open an editor for the patches in a browser

flags:
`)
	flag.PrintDefaults()
}

func main() {
	flag.Usage = usage

	// The command comes first, so that flags can be written after it.
	args := os.Args[1:]
	command := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		command, args = args[0], args[1:]
	}
	if err := flag.CommandLine.Parse(args); err != nil {
		os.Exit(2)
	}

	if err := run(command, flag.Args()); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(command string, args []string) error {
	switch command {
	case "download":
		_, _, err := download()
		return err

	case "ids":
		spec, err := loadPatches()
		if err != nil {
			return err
		}
		handed := spec.IDs.Record(spec)
		if handed == 0 {
			fmt.Println("every attribute and effect already has an ID")
			return nil
		}
		if err := spec.IDs.Save(); err != nil {
			return err
		}
		fmt.Printf("gave out %d new ID(s), written to patches/%s\n", handed, patch.IDsFile)
		return nil

	case "build":
		spec, err := loadPatches()
		if err != nil {
			return err
		}
		if err := spec.IDs.Missing(spec); err != nil {
			return err
		}
		data, err := load()
		if err != nil {
			return err
		}
		if _, err := patch.Apply(spec, data); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
			return err
		}
		if err := fbs.Write(data, *out); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(*namesOut), 0o755); err != nil {
			return err
		}
		if err := fbs.WriteNames(data, *namesOut); err != nil {
			return err
		}

		for _, filename := range []string{*out, *namesOut} {
			info, err := os.Stat(filename)
			if err != nil {
				return err
			}
			fmt.Printf("wrote %s (%.1f MiB)\n", filename, float64(info.Size())/1024/1024)
		}
		fmt.Printf("%d types\n", len(data.Types))
		return nil

	case "compare":
		if len(args) == 0 {
			return fmt.Errorf("compare needs the directory of an earlier build")
		}
		for _, filename := range []string{*out, *namesOut} {
			equal, err := fbs.Equal(filepath.Join(args[0], filepath.Base(filename)), filename)
			if err != nil {
				return err
			}
			if !equal {
				fmt.Println("changed")
				return nil
			}
		}
		fmt.Println("unchanged")
		return nil

	case "explain", "patches":
		spec, err := loadPatches()
		if err != nil {
			return err
		}
		data, err := load()
		if err != nil {
			return err
		}
		ctx, err := patch.Apply(spec, data)
		if err != nil {
			return err
		}

		if command == "patches" {
			for _, name := range ctx.Patches() {
				fmt.Println(name)
			}
			return nil
		}
		if *typeName != "" {
			return ctx.ExplainType(os.Stdout, *typeName)
		}
		if len(args) == 0 {
			return fmt.Errorf("explain needs a patch name; try 'patches' for the list")
		}
		return ctx.Explain(os.Stdout, args[0], *full)

	case "serve":
		// Patches are re-read on every request; only the SDE is loaded up front.
		data, err := load()
		if err != nil {
			return err
		}
		server := web.New(data, *patchesDir)

		fmt.Printf("editing %s on http://%s\n", *patchesDir, *addr)
		return http.ListenAndServe(*addr, server.Handler())

	default:
		usage()
		return fmt.Errorf("unknown command %q", command)
	}
}

// loadPatches reads the patches and checks everything that can be checked
// without the SDE, so a typo is reported before the long load.
func loadPatches() (*patch.Spec, error) {
	spec, err := patch.Load(*patchesDir)
	if err != nil {
		return nil, err
	}
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	return spec, nil
}

func download() (string, int32, error) {
	build := int32(*buildNum)
	if build == 0 {
		latest, err := sde.LatestBuild()
		if err != nil {
			return "", 0, err
		}
		build = latest
	}

	filename, err := sde.Download(*sdeDir, build)
	if err != nil {
		return "", 0, err
	}

	fmt.Fprintf(os.Stderr, "using SDE build %d\n", build)
	return filename, build, nil
}

// load uses the SDE already on disk when there is one, so that building does
// not need the network.
func load() (*sde.Data, error) {
	build := int32(*buildNum)
	if build == 0 {
		build = sde.LocalBuild(*sdeDir)
	}

	if build == 0 {
		filename, downloaded, err := download()
		if err != nil {
			return nil, err
		}
		return sde.Load(filename, downloaded)
	}

	filename, err := sde.Download(*sdeDir, build)
	if err != nil {
		return nil, err
	}
	return sde.Load(filename, build)
}
