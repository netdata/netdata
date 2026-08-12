# Build the Netdata Agent yourself

Build the Netdata Agent yourself when you need to develop or debug the Agent, package it for a distribution, integrate it
with another build system, or test a change before an official package is available. Building is different from installing
an official Netdata package: you are responsible for selecting dependencies, build options, installation paths, and the
update strategy for the resulting installation.

## Choose a build workflow

Use the workflow that matches what you intend to produce:

- **Compile and install from source:** Follow [Compile from source code](/packaging/installer/methods/source.md) for the
  supported dependency setup, CMake configuration, compilation, installation, and post-install checks. This is the normal
  route for local development and for operators who deliberately maintain a source-built Agent.
- **Maintain distribution packages:** Read the [package maintainer guide](/packaging/maintainers/README.md) before creating
  packages. It documents the build requirements and packaging concerns that differ from an interactive source install.
- **Integrate an external build system:** Use the [external build-system reference](/build_external/README.md) when another
  project controls dependency discovery, compilation, or installation rather than Netdata's normal build workflow.
- **Test native DEB or RPM packaging:** Follow
  [Build native packages locally](/packaging/building-native-packages-locally.md) to reproduce the repository's package
  build locally and inspect the resulting artifacts before submitting packaging changes.

## Before building

Start from a clean, current source checkout and read the selected guide completely. Install the documented build
dependencies for your operating system instead of disabling features merely to get configuration to pass. Decide whether
the build is temporary, a development installation, or a package intended for other systems; that decision controls the
appropriate prefix, enabled features, debug settings, and ownership of installed files.

Keep the configuration summary produced by the build. It records which collectors, exporters, libraries, and optional
features were detected. A successful compiler exit does not prove that the intended feature was included.

## Validate the result

Run the tests required by the guide and by the components you changed. After installation, verify that the Agent starts,
that its API responds, and that the expected collectors and features are present. For a local Agent, a basic reachability
check is:

```bash
curl http://localhost:19999/api/v1/info
```

Package maintainers should additionally install, upgrade, and remove the package in a clean target environment. Confirm
that service files, permissions, configuration preservation, dependencies, and uninstall behavior match the platform's
packaging conventions.

## Plan updates and removal

A manually built Agent does not automatically become an official package-manager installation. Preserve the original
checkout and build options so you can reproduce upgrades. Use the documented [update](/packaging/installer/UPDATE.md) and
[uninstall](/packaging/installer/UNINSTALL.md) guidance for the installation type you produced; mixing procedures from a
different installation method can leave files unmanaged or remove the wrong paths.
