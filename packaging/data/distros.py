'''Python module for handling of distros.yaml data.'''

from collections import Counter
from dataclasses import dataclass
from enum import StrEnum, unique
from pathlib import Path
from typing import Annotated, Final

from pydantic import AfterValidator, BaseModel, ConfigDict, Field
from ruamel.yaml import YAML

yaml: Final = YAML(typ='safe')
DATA_PATH: Final = Path(__file__).parent / 'distros.yaml'
PYDANTIC_CONFIG: Final = ConfigDict(
    extra='allow',
    frozen=True,
    allow_inf_nan=False,
    str_strip_whitespace=True,
)


@unique
class PackageType(StrEnum):
    RPM = 'rpm'
    DEB = 'deb'


@unique
class Arch(StrEnum):
    AMD64 = 'amd64'
    X86_64 = 'x86_64'
    I386 = 'i386'
    ARMHF = 'armhf'
    ARMHFP = 'armhfp'
    ARMV6L = 'armv6l'
    ARMV7L = 'armv7l'
    ARM64 = 'arm64'
    AARCH64 = 'aarch64'


@unique
class DockerArch(StrEnum):
    AMD64 = 'linux/amd64'
    ARM64V8 = 'linux/arm64/v8'
    ARMV7 = 'linux/arm/v7'
    ARMv6 = 'linux/arm/v6'
    I386 = 'linux/386'


@dataclass(kw_only=True, frozen=True, slots=True)
class ArchDataEntry:
    qemu: bool
    runner: Annotated[str, Field(min_length=1)]


def check_unique_list[T](value: list[T]) -> list[T]:
    c = Counter(value)
    if s := {k for k, v in c.items() if v > 1}:
        ValueError(f'Found duplicate values in list: {s}')

    return value


def check_arch_list(value: list[Arch]) -> list[Arch]:
    s1 = set(value)
    s2 = set(Arch)

    if s3 := s2 - s1:
        ValueError(f'Missing values from arch_order list: {s3}')

    return value


def check_arch_data(value: dict[Arch, ArchDataEntry]) -> dict[Arch, ArchDataEntry]:
    s1 = set(value.keys())
    s2 = set(Arch)

    if s3 := s2 - s1:
        ValueError(f'Missing keys from arch_data mapping: {s3}')

    return value


@dataclass(kw_only=True, frozen=True, slots=True)
class PackagingInfo:
    __pydantic_config__ = PYDANTIC_CONFIG

    type: PackageType
    repo_distro: Annotated[str, Field(min_length=1)]
    builder_rev: Annotated[str, Field(min_length=1)]
    arches: Annotated[list[Arch], Field(min_length=1)]
    alt_links: Annotated[list[Annotated[str, Field(min_length=1)]], Field(default_factory=list)]


@dataclass(kw_only=True, frozen=True, slots=True)
class BasicPackagingInfo:
    __pydantic_config__ = PYDANTIC_CONFIG

    arches: Annotated[list[Arch | DockerArch], Field(min_length=1)]


@dataclass(kw_only=True, frozen=True, slots=True)
class TestConfig:
    __pydantic_config__ = PYDANTIC_CONFIG

    ebpf_core: Annotated[bool, Field(alias='ebpf-core')] = False
    skip_local_build: Annotated[bool, Field(alias='skip-local-build')] = False


@dataclass(kw_only=True, frozen=True, slots=True)
class FullDistroEntry:
    __pydantic_config__ = PYDANTIC_CONFIG

    distro: Annotated[str, Field(min_length=1)]
    version: Annotated[str, Field(min_length=1)]
    support_type: Annotated[str, Field(min_length=1)]
    notes: str
    bundle_sentry: dict[Arch, bool]
    eol_check: str | bool = False
    eol_lts: bool = False
    base_image: Annotated[str, Field(default_factory=lambda data: f'{data.get('distro')}:{data.get('version')}')]
    env_prep: Annotated[str, Field(min_length=1)] | None = None
    jsonc_removal: Annotated[str, Field(min_length=1)] | None = None
    packages: PackagingInfo | None = None
    test: TestConfig = TestConfig()


@dataclass(kw_only=True, frozen=True, slots=True)
class LegacyDistroEntry:
    __pydantic_config__ = PYDANTIC_CONFIG

    distro: Annotated[str, Field(min_length=1)]
    version: Annotated[str, Field(min_length=1)]
    support_type: Annotated[str, Field(min_length=1)]
    notes: str
    bundle_sentry: dict[Arch, bool] | None = None
    eol_check: str | bool = False
    eol_lts: bool = False
    base_image: Annotated[str, Field(default_factory=lambda data: f'{data.get('distro')}:{data.get('version')}')]
    env_prep: Annotated[str, Field(min_length=1)] | None = None
    jsonc_removal: Annotated[str, Field(min_length=1)] | None = None
    packages: PackagingInfo | None = None
    test: TestConfig = TestConfig()


@dataclass(kw_only=True, frozen=True, slots=True)
class BasicDistroEntry:
    __pydantic_config__ = PYDANTIC_CONFIG

    distro: Annotated[str, Field(min_length=1)]
    version: Annotated[str, Field(min_length=1)]
    support_type: Annotated[str, Field(min_length=1)]
    notes: str
    packages: BasicPackagingInfo | None = None


class DistroData(BaseModel):
    model_config = PYDANTIC_CONFIG

    platform_map: dict[Arch, DockerArch]
    arch_order: Annotated[list[Arch], AfterValidator(check_arch_list)]
    arch_data: Annotated[dict[Arch, ArchDataEntry], AfterValidator(check_arch_data)]
    static_arches: Annotated[list[Arch], AfterValidator(check_unique_list)]
    docker_arches: Annotated[list[Arch], AfterValidator(check_unique_list)]
    include: Annotated[list[FullDistroEntry], Field(min_length=1)]
    legacy: list[LegacyDistroEntry]
    no_include: list[BasicDistroEntry]


def load_distro_data() -> DistroData:
    with open(DATA_PATH) as f:
        d = yaml.load(f)

    return DistroData.model_validate(d)
