# Netdata Agent Versioning Policy (DRAFT)

This document outlines how versions are handled for the Netdata Agent. This policy applies to version 2.0.0 of
the Netdata Agent and newer versions.

## Stable Releases

Versions for stable releases of the Netdata Agent consist of three parts, a major version, a minor version, and
a patch version, presented like `<major>.<minor>.<patch>`. For example, a version of `1.42.3` has a major version
of 1, a minor version of 42, and a patch version of 3.

The patch version is incremented when a new stable release is made that only contains bug fixes that do not alter
the public API in a backwards incompatible manner. Special exceptions may be made for critical security bugs,
but such exceptions will be prominently noted in the release notes for the versions for which they are made.

The minor version is incremented when a new stable release is made that contains new features and functionality
that do not alter the strictly defined parts of public API in a backwards incompatible manner. A new minor version
may have changes to the loosely defined parts of the public API that are not backwards compatible, but unless
they are critical security fixes they will be announced ahead of time in the release notes for the previous minor
version. Once a new minor version is published, no new patch releases will be published for previous minor versions
unless they fix serious bugs.

The major version is incremented when a new stable release is made that alters the strictly defined public API in
some backwards incompatible manner. Any backwards incompatible changes that will be included in a new major version
will be announced ahead of time in the release notes for the previous minor version. Once a given major version
is published, no new minor releases will be published for any prior major version, though new patch releases _may_
be published for the latest minor release of any prior major version to fix serious bugs.

In most cases, just prior to a new major version being published, a final stable minor release will be published
for the previous major version, including all non-breaking changes that will be in the new major version. This is
intended to ensure that users who choose to remain on the previous major version for an extended period of time
will be as up-to-date as possible.

## Nightly Builds

Versions for nightly builds of the Netdata Agent consist of four parts, a major version, a minor version, a revision
number, and an optional commit ID, presented like `<major>.<minor>.0-<revision>-<commit>`. For example, a version
of `1.43.0-11-gb15437502` has a major version of 1, a minor version of 43, a revision of 11, and a commit ID of
`gb15437502`. A commit ID consists of a lowercase letter `g`, followed by the short commit hash for the corresponding
commit. If the commit ID is not included, it may be replaced by the word ‘nightly’.

The major and minor version numbers for a nightly build correspond exactly to an associated stable release. A
given major version of a nightly build has the same compatibility guarantees as it would for a stable release. A
given minor version of a nightly build will generally include any backwards-incompatible changes to the loosely
defined public API that will be in the _next_ minor version of the associated stable release.

The revision number indicates the number of commits on the main branch of the Netdata Agent git repository since
the associated stable release, and the commit ID, if included, should indicate the exact commit hash used for the
nightly build.

Due to how our release process works, nightly version numbers do not track stable patch releases. For example, if the
latest stable release is `1.42.4`, the latest nightly version will still show something like `1.42.0-209-nightly`. The
first nightly build version published after an associated stable release will include all relevant fixes that were
in that stable release. In addition, in most cases, the last nightly build version published before an associated
stable patch release will include all relevant fixes that are in that patch release.

Nightly builds are only published on days when changes have actually been committed to the main branch of the
Netdata Agent git repository.

## Public API

The remainder of the document outlines the public API of the Netdata Agent.

We define two categories of components within the public API:

- Strictly defined components are guaranteed not to change in a backwards incompatible manner without an associated
  major version bump, and will have impending changes announced in the release notes at least one minor release
  before they are changed.
- Loosely defined components are guaranteed not to change in a backwards incompatible manner without an associated
  minor version bump, and will have impending changes announced in the release notes at least one minor release
  before they are changed.

There are also a few things we handle specially, which will be noted later in the document.

### Strictly Defined Public API Components

The following aspects of the public API are strictly defined, and are guaranteed not to change in a backwards
incompatible manner without an associated major version increase, and such changes will be announced in the release
notes at least one minor release prior to being merged:

- All mandatory build dependencies which are not vendored in the Netdata Agent code. This includes, but is not
  limited to:
  - The underlying build system (such as autotools or CMake).
  - Primary library dependencies (such as libuv).
  - Any external tooling that is required at build time.
- The REST API provided by the Netdata Agent’s internal web server, accessible via the `/api` endpoint. This
  does not extend to the charts, labels, or other system-specific data returned by some API endpoints.
- The protocol used for streaming and replicating data between Netdata Agents.
- The protocol used for communicating with external data collection plugins.
- The APIs provided by the `python.d.plugin` and `charts.d.plugin` data collection frameworks.
- The set of optional features supported by the Agent which are provided by default in our pre-built packages. If
  support for an optional feature is being completely removed from the Agent, that is instead covered by what
  component that feature is part of.

### Loosely Defined Public API Components

The following aspects of the public API are loosely defined. They are guaranteed to not change in a backwards
incompatible manner without an associated minor version increase, and such changes will be announced in the release
notes at least one minor release prior to being merged:

- Configuration options in any configuration file normally located under `/etc/netdata` on a typical install,
  as well as their default values.
- Environment variables that are interpreted by the Netdata Agent, or by the startup code in our official OCI
  container images.
- The exact set of charts provided, including chart families, chart names, and provided metrics.
- The exact set of supported data collection sources and data export targets.
- The exact set of system service managers we officially support running the Netdata Agent under.
- The exact set of alert delivery mechanisms supported by the Netdata Agent.
- The high-level implementation of the Netdata Agent’s integrated web server.
- The v0 and v1 dashboard UIs provided through the Netdata Agent’s internal web server.

All loosely defined API components may also change in a backwards incompatible manner if the major version is
increased. Large scale changes to these components may also warrant a major version increase even if there are no
backwards incompatible changes to strictly defined public API components.

### Special Cases

The following special exceptions to the public API exist:

- When an internal on-disk file format (such as the dbengine data file format) is changed, the old format is
  guaranteed to be supported for in-place updates for at least two minor versions after the change happens. The
  new format is not guaranteed to be backwards compatible.
- The list of supported platforms is functionally a part of the public API, but our existing [platform support
  policy](/packaging/PLATFORM_SUPPORT.md) dictates when and how
  support for specific platforms is added or removed.
- The list of components provided as separate packages in our official native packages is considered part of our
  strictly defined public API, but changes to our packaging that do not alter the functionality of existing installs
  are considered to be backwards compatible. This means that we may choose to split a plugin out to it’s own
  package at any time, but it will remain as a mandatory dependency until at least the next major release.
- Options and environment variables used by the `kickstart.sh` install script and the `netdata-updater.sh` script
  are handled separately from regular Netdata Agent versioning. Backwards compatible changes may happen at any
  time for these, while backwards incompatible changes will have a deprecation period during which the old behavior
  will be preserved but will issue a warning about the impending change.

### Things Not Covered By The Public API

Any components which are not explicitly listed above as being part of the public API are not part of the public
API. This includes, but is not limited to:

- Any mandatory build components which are vendored as part of the Netdata sources, such as SQLite3 or libJudy. This
  extends to both the presence or absence of such components, as well as the exact version being bundled.
- The exact installation mechanism that will be used on any given system when using our `kickstart.sh` installation
  script.
- The exact underlying implementation of any data collection plugin.
- The exact underlying implementation of any data export mechanism.
