"""The static data EVEShip.fit's dogma engine calculates with.

This package holds nothing but the two data files, their schemas, and the
paths to them::

    from eveshipfit_sde import sde_path

    import esf_dogma_engine as dogma
    dogma.load_sde_from_file(sde_path())
"""

from importlib.resources import files
from importlib.metadata import version
from pathlib import Path

__all__ = ["build_number", "names_path", "sde_path", "specs_path"]


def sde_path() -> Path:
    """`sde.dat`: the types, dogma, groups, categories, buffs and mutaplasmids,
    with English names only. It is all a dogma engine needs."""
    return Path(str(files(__package__) / "sde.dat"))


def names_path() -> Path:
    """`names.dat`: a name in any of the eight languages EVE supports, mapped
    back to a type id. Only an EFT-fit importer needs it."""
    return Path(str(files(__package__) / "names.dat"))


def specs_path() -> Path:
    """The directory holding `eve.fbs` and `names.fbs`, the flatbuffer schemas
    that describe the two data files."""
    return Path(str(files(__package__) / "specs"))


def build_number() -> int:
    """The SDE build these files were made from.

    Releases are versioned `<major>.<SDE build>.<n>`.
    """
    return int(version("eveshipfit-sde").split(".")[1])
