# SDE Patched

A patched [EVE Online SDE](https://developers.eveonline.com/static-data) to work better with EVEShipFit's dogma-engine.

The main goal is to produce a small data-file, with everything included for dogma-engine to calculate fit statistics.

## prerequisite

- Go 1.27 or later
- flatc

## Usage

```bash
go run ./cmd/sde-patched download        # fetch the latest SDE into sde/
go run ./cmd/sde-patched build           # write dist/sde.dat and dist/names.dat
go run ./cmd/sde-patched patches         # list all patches
go run ./cmd/sde-patched explain alignTime
go run ./cmd/sde-patched explain --type "Rifter"
```

`build` downloads the SDE when there is none yet, and uses the one on disk
otherwise. Use `--build <number>` to pin a version.

## Output

`build` writes two flatbuffers, described in [specs/](specs/).

`dist/sde.dat` holds the types, dogma, groups and categories, with English
names only. It is all a dogma-engine needs.

`dist/names.dat` maps a name in any of the eight languages EVE supports back to
a type ID. Only an EFT-fit importer needs it, and only ever to search, so it is
a separate download; it is larger than the SDE itself. Match its `build_number`
against the one in `dist/sde.dat` before trusting the type IDs.

## Patches

The SDE describes what the EVE client needs, not what a fitting tool needs.
Align time, for example, is not an attribute; the client works it out. Patches
add the missing attributes and effects, so a dogma-engine can work them out
instead.

A patch is a Go file in [patches/](patches/). It declares what it wants at
package level, and Go works out the order: an effect that uses an attribute is
always built after that attribute.

New attributes and effects always get a negative ID, so it is obvious they are
not in EVE.

### Picking types

`On` takes selectors. All of them have to hold; use `Any` for "one of these"
and `Not` to invert.

| Selector | Matches |
| --- | --- |
| `InCategory(names...)` | types in one of these categories |
| `InGroup(names...)` | types in one of these groups |
| `Named(names...)` | types with one of these English names |
| `HasAttribute(ref)` | types that already carry the attribute |
| `HasEffect(ref)` | types that already carry the effect |
| `IsPublished()` | published types |

### Other actions

| Action | Does |
| --- | --- |
| `ApplyEffect(effect).On(...)` | give types an effect; `.AsDefault()` makes it run on its own |
| `RemoveEffect(effect).From(...)` | take an effect away |
| `SetAttribute(attribute, value).On(...)` | set an attribute value on types |
| `ChangeEffect(effect).SetCategory(...)` | change the effect itself |
| `ChangeAttribute(attribute).SetDefaultValue(...)` | change the attribute itself |

Wrap a selector in `As("a module that deals damage", ...)` to give it a name;
"explain" then shows that name instead of the whole nest of conditions.
