#!/usr/bin/env python3
'''Python module for handling of distros.yaml data.

   If run as a script on its own, this will dump the JSON schema for distros.yaml.'''

from __future__ import annotations

from collections import Counter
from dataclasses import dataclass
from enum import StrEnum, unique
from pathlib import Path
from typing import Annotated, Final

from pydantic import BaseModel, ConfigDict, Field, field_validator
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
    qemu: Annotated[bool, Field(
        title='Use QEMU',
        description='Whether or not to use QEMU userspace emulation for this architecture.',
    )]
    runner: Annotated[str, Field(
        title='GitHub Actions Runner',
        description='What GitHub Actions Runner tag to use for this architecture.',
        min_length=1,
    )]


@dataclass(kw_only=True, frozen=True, slots=True)
class PackagingInfo:
    __pydantic_config__ = PYDANTIC_CONFIG

    type: Annotated[PackageType, Field(
        title='Package Type',
        description='Specifies the type of packages to use for this platform.',
    )]
    repo_distro: Annotated[str, Field(
        title='Repository Prefix',
        description='Specifies the prefix within the repository path for this platform.',
        min_length=1,
    )]
    builder_rev: Annotated[str, Field(
        title='Builder Revision',
        description='Specifies the package builder revision to be used for this platform.',
        min_length=1,
    )]
    arches: Annotated[list[Arch], Field(
        title='Package Architectures',
        description='Specifies the list of architectures to build packages for this platform for.',
        min_length=1,
    )]
    alt_links: Annotated[list[Annotated[str, Field(min_length=1)]], Field(
        title='Repository Aliases',
        description='An optional list of aliases for the repository prefix.',
        default_factory=list,
    )]


@dataclass(kw_only=True, frozen=True, slots=True)
class BasicPackagingInfo:
    __pydantic_config__ = PYDANTIC_CONFIG

    arches: Annotated[list[Arch | DockerArch], Field(
        title='Package Architectures',
        description='Specifies the list of architectures to build packages for this platform for.',
        min_length=1,
    )]


@dataclass(kw_only=True, frozen=True, slots=True)
class TestConfig:
    __pydantic_config__ = PYDANTIC_CONFIG

    ebpf_core: Annotated[bool, Field(
        title='eBPF CO-RE testing',
        description='Whether or not to run eBPF CO-RE testing for this platform.',
        alias='ebpf-core',
    )] = False
    skip_local_build: Annotated[bool, Field(
        title='Skip local build checks',
        description='When true, skip this platform for local build checks in CI.',
        alias='skip-local-build',
    )] = False


@dataclass(kw_only=True, frozen=True, slots=True)
class FullDistroEntry:
    __pydantic_config__ = PYDANTIC_CONFIG

    distro: Annotated[str, Field(
        title='Platform Name',
        description='The name of the platform',
        min_length=1,
    )]
    version: Annotated[str, Field(
        title='Platform Version',
        description='The version of the platform',
        min_length=1,
    )]
    support_type: Annotated[str, Field(
        title='Support Tier',
        description='The support tier for this particular platform.',
        min_length=1,
    )]
    notes: Annotated[str, Field(
        title='Notes',
        description='Any supplementary notes about the platform.',
    )]
    bundle_sentry: Annotated[dict[Arch, bool], Field(
        title='Bundle Sentry',
        description='Per-architecture Sentry configuration. If a given architecture’s key evaluates to true, then Sentry will be bundled in official builds for that architecture.',
    )]
    eol_check: Annotated[str | bool, Field(
        title='End of Life Check',
        description='Configures automatic generation of issues tracking platform EOL based on https://endoflife.date/. If False, EOL checking will be disabled. If True, EOL checking will be enabled and will use the value of the distro key for the lookup. If a non-empty string is specified, that will be used for EOL checking instead of the value of the distro key.',
    )] = False
    eol_lts: Annotated[bool, Field(
        title='Check LTS for EOL checks',
        description='When True, use the LTS release instead of the regular one when checking for platform EOL.',
    )] = False
    base_image: Annotated[str, Field(
        title='Platform Base Image',
        description='Specifies a Docker image to use as a base for builds for this platform. A sane default that works for most platforms is computed from the distro and version keys.',
        default_factory=lambda data: f'{data.get("distro")}:{data.get("version")}',
    )]
    env_prep: Annotated[str, Field(
        title='Environment Preparation Command',
        description='Specifies any additional commands that need to be run before attempting builds in the environment.',
        min_length=1,
    )] | None = None
    packages: Annotated[PackagingInfo | None, Field(
        title='Packaging Configuration',
        description='Controls packaging for the platform. If absent, packages will not be published.',
    )] = None
    test: Annotated[TestConfig, Field(
        title='Test Configuration',
        description='Controls automated test configuration for the platform.'
    )] = TestConfig()


@dataclass(kw_only=True, frozen=True, slots=True)
class BasicDistroEntry:
    __pydantic_config__ = PYDANTIC_CONFIG

    distro: Annotated[str, Field(
        title='Platform Name',
        description='The name of the platform',
        min_length=1,
    )]
    version: Annotated[str, Field(
        title='Platform Version',
        description='The version of the platform',
        min_length=1,
    )]
    support_type: Annotated[str, Field(
        title='Support Tier',
        description='The support tier for this particular platform.',
        min_length=1,
    )]
    notes: Annotated[str, Field(
        title='Notes',
        description='Any supplementary notes about the platform.',
    )]
    packages: Annotated[BasicPackagingInfo | None, Field(
        title='Packaging Configuration',
        description='Describes published packages for the platform.',
    )] = None


class DistroData(BaseModel):
    model_config = PYDANTIC_CONFIG

    platform_map: Annotated[dict[Arch, DockerArch], Field(
        title='Docker Platform Mapping',
        description='Mapping of internal architecture names to Docker platform strings.',
    )]
    arch_order: Annotated[list[Arch], Field(
        title='Architecture Build Order',
        description='Specifies the sort order for build architectures in CI. Must contain each supported architecture value exactly once.',
    )]
    arch_data: Annotated[dict[Arch, ArchDataEntry], Field(
        title='Per-Architecture Build Environment Config',
        description='Specifies GitHub Actions runner configuration for each build architecture in CI. Must contain an entry for each supported architecture value exactly once.',
    )]
    static_arches: Annotated[list[Arch], Field(
        title='Static Build Architectures',
        description='Specifies the architectures to create static builds for. Must be a subset of supported architectures without any duplicates.',
    )]
    docker_arches: Annotated[list[Arch], Field(
        title='Docker Build Architectures',
        description='Specifies the architectures to create Docker images for. Must be a subset of supported architectures without any duplicates.',
    )]
    include: Annotated[list[FullDistroEntry], Field(
        title='Platforms Included in CI',
        description='Contains platform configuration for each platform we run CI jobs for.',
        min_length=1,
    )]
    legacy: Annotated[list[FullDistroEntry], Field(
        title='Platforms Previously Included in CI',
        description='Contains platform configurations for platforms we previously included in CI and published packages for.',
    )]
    no_include: Annotated[list[BasicDistroEntry], Field(
        title='Platforms Not Included in CI',
        description='Contains platform descriptions for platforms not included in CI',
    )]

    @field_validator('static_arches', 'docker_arches', mode='after')
    @classmethod
    def check_unique_list[T](cls: type[DistroData], value: list[T]) -> list[T]:
        c = Counter(value)

        if s := {k for k, v in c.items() if v > 1}:
            raise ValueError(f'Found duplicate values in list: {s}')

        return value

    @field_validator('arch_order', mode='after')
    @classmethod
    def check_arch_list(cls: type[DistroData], value: list[Arch]) -> list[Arch]:
        c = Counter(value)

        if s := {k for k, v in c.items() if v > 1}:
            raise ValueError(f'Found duplicate values in arch_order: {s}')

        s1 = set(value)
        s2 = set(Arch)

        if s3 := s2 - s1:
            raise ValueError(f'Missing values from arch_order list: {s3}')

        return value

    @field_validator('arch_data', mode='after')
    @classmethod
    def check_arch_data(cls: type[DistroData], value: dict[Arch, ArchDataEntry]) -> dict[Arch, ArchDataEntry]:
        c = Counter(value.keys())

        if s := {k for k, v in c.items() if v > 1}:
            raise ValueError(f'Found duplicate keys in arch_data mapping: {s}')

        s1 = set(value.keys())
        s2 = set(Arch)

        if s3 := s2 - s1:
            raise ValueError(f'Missing keys from arch_data mapping: {s3}')

        return value


def load_distro_data() -> DistroData:
    with open(DATA_PATH) as f:
        d = yaml.load(f)

    return DistroData.model_validate(d)


if __name__ == '__main__':
    import json
    import sys

    json.dump(
        DistroData.model_json_schema(
            by_alias=True,
            union_format='any_of',
        ),
        sys.stdout,
        indent=4,
    )
