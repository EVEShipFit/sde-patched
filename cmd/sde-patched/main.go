// Command sde-patched downloads the EVE SDE, patches its dogma data, and
// writes it out as a flatbuffer.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/EVEShipFit/sde-patched/internal/fbs"
	"github.com/EVEShipFit/sde-patched/internal/patch"
	"github.com/EVEShipFit/sde-patched/internal/sde"

	_ "github.com/EVEShipFit/sde-patched/patches"
)

var (
	sdeDir   = flag.String("sde-dir", "sde", "directory the SDE is downloaded to")
	out      = flag.String("out", filepath.Join("dist", "sde.dat"), "file to write")
	namesOut = flag.String("names-out", filepath.Join("dist", "names.dat"), "name lookup file to write")
	buildNum = flag.Int("build", 0, "SDE build to use; defaults to the latest")
	full     = flag.Bool("full", false, "explain: list every matched type")
	typeName = flag.String("type", "", "explain: show what touches this type")
)

func usage() {
	fmt.Fprint(os.Stderr, `usage: sde-patched [flags] <command>

commands:
  download   fetch the SDE
  build      patch the SDE and write the flatbuffer
  explain    show what a patch does; "explain <name>" or "explain --type <name>"
  patches    list all patches

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

	case "build":
		data, err := load()
		if err != nil {
			return err
		}
		if _, err := patch.Apply(data); err != nil {
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

	case "explain", "patches":
		data, err := load()
		if err != nil {
			return err
		}
		ctx, err := patch.Apply(data)
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

	default:
		usage()
		return fmt.Errorf("unknown command %q", command)
	}
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
