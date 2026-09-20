# SDE Patched

[![npm](https://img.shields.io/npm/v/%40eveshipfit%2Fsde.svg)](https://www.npmjs.com/package/@eveshipfit/sde)
[![PyPI](https://img.shields.io/pypi/v/eveshipfit-sde.svg)](https://pypi.org/project/eveshipfit-sde/)
[![CI](https://github.com/EVEShipFit/sde-patched/actions/workflows/testing.yml/badge.svg)](https://github.com/EVEShipFit/sde-patched/actions/workflows/testing.yml)

[![Discord](https://img.shields.io/badge/Discord-Join-5865F2?logo=discord&logoColor=white)](https://discord.gg/S5V5BkvNf7)

A patched [EVE Online SDE](https://developers.eveonline.com/static-data) to work better with EVEShipFit's dogma-engine.

The main goal is to produce a small data-file, with everything included for dogma-engine to calculate fit statistics.

## Prerequisite

- Go 1.27 or later
- flatc

## Usage

```bash
go run ./cmd/sde-patched download        # fetch the latest SDE into sde/
go run ./cmd/sde-patched build           # write dist/sde.dat and dist/names.dat
go run ./cmd/sde-patched compare old/    # tell whether dist/ differs from old/, ignoring the SDE build
go run ./cmd/sde-patched patches         # list all patches
go run ./cmd/sde-patched ids             # give an ID to anything added without one
go run ./cmd/sde-patched explain alignTime
go run ./cmd/sde-patched explain --type "Rifter"
go run ./cmd/sde-patched serve           # edit the patches in a browser
```

`build` downloads the SDE when there is none yet, and uses the one on disk otherwise.
Use `--build <number>` to pin a version.

## Editing

The patches are best edited via the web-editor (via `serve`).
It allows for full customization of all existing and new attributes.

This editor is fully vibe-coded, and it is very likely no human actually understands how it works.
It is also not meant as "production-ready" software, but much more as "an easier way to digest patches".

## Output

`build` writes two flatbuffers, described in [specs/](https://github.com/EVEShipFit/sde-patched/tree/main/specs):
- `dist/sde.dat` holds the types, dogma, groups, categories, buffs and mutaplasmids, with English
names only. It is all a dogma-engine needs.
- `dist/names.dat` maps a name in any of the eight languages EVE supports back to
a type ID. Only an EFT-fit importer needs it, and only ever to search.

## npm

```sh
npm install @eveshipfit/sde
```

[`@eveshipfit/sde`](https://www.npmjs.com/package/@eveshipfit/sde) ships both files and the schemas, under `dist/` and `specs/`.

## Python

```sh
pip install eveshipfit-sde
```

[`eveshipfit-sde`](https://pypi.org/project/eveshipfit-sde/) ships both files and the schemas, and the paths to them.

```python
from eveshipfit_sde import build_number, names_path, sde_path, specs_path

dogma.load_sde_from_file(sde_path())
```

`build_number()` reads the SDE build straight off the version.

## Releasing

Every day at 12:00 UTC, `main` is built against the latest SDE.
When the result differs from the latest release, the SDE build number aside, a new release is made and published on npm and PyPI.
A change to the patches is released this way too.

## Patches

The SDE describes what the EVE client needs, not what a fitting tool needs.
Align time, for example, is not an attribute; the client works it out.
Patches add the missing attributes and effects, so a dogma-engine can work them out instead.

## License

All EVE Online data belongs to CCP, and is subject to [their license agreement](https://developers.eveonline.com/license-agreement).
Everything EVEShip.fit-specific is MIT.
