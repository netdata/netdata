# SPDX-License-Identifier: GPL-3.0-or-later
# Build options, the package-format validation they feed, and the per-format component remap.
#
# Relocated verbatim from the root CMakeLists.txt. include()d rather than
# add_subdirectory()d so CMAKE_CURRENT_SOURCE_DIR and CMAKE_CURRENT_BINARY_DIR
# keep pointing at the repository and build roots, which every relative path
# below depends on. Nothing here may use CMAKE_CURRENT_LIST_DIR.
#
# Kept as one unit: splitting the option() calls from the validation and the remap that consume them would manufacture modularity without providing any.

include(CMakeDependentOption)

# This is intended to make life easier for developers who are working on one
# specific feature.
#
# NOTE: DO NOT USE THIS OPTION FOR PRODUCTION BUILDS.
option(DEFAULT_FEATURE_STATE "Specify the default state for most optional features" True)
mark_as_advanced(DEFAULT_FEATURE_STATE)

# High-level features
option(ENABLE_ML "Enable machine learning features" ${DEFAULT_FEATURE_STATE})

if(ENABLE_ML)
  set(NETDATA_DLIB_SOURCE_DIR "" CACHE PATH "Path to local dlib sources for building ML code")
endif()

option(ENABLE_DBENGINE "Enable dbengine metrics storage" True)
option(ENABLE_DASHBOARD "Enable local dashboard" True)
mark_as_advanced(ENABLE_DASHBOARD)

# Data collection plugins
option(ENABLE_PLUGIN_GO "Enable metric collectors written in Go" ${DEFAULT_FEATURE_STATE})
cmake_dependent_option(ENABLE_ND_MCP "Build nd-mcp stdio-to-websocket bridge for MCP integration" ${DEFAULT_FEATURE_STATE} "ENABLE_PLUGIN_GO" False)
option(ENABLE_PLUGIN_SCRIPTS "Enable the experimental scripts plugin (Nagios compatibility module)" ON)
cmake_dependent_option(ENABLE_PLUGIN_OTEL "Enable collection of OpenTelemetry metrics and logs" ${DEFAULT_FEATURE_STATE} "OS_LINUX OR OS_MACOS" False)
cmake_dependent_option(ENABLE_PLUGIN_NETFLOW "Enable NetFlow/IPFIX/sFlow flow analysis plugin" False "NOT OS_WINDOWS" False)
option(ENABLE_PLUGIN_PYTHON "Enable metric collectors written in Python" ${DEFAULT_FEATURE_STATE})

cmake_dependent_option(ENABLE_PLUGIN_APPS "Enable per-process resource usage monitoring" ${DEFAULT_FEATURE_STATE} "OS_LINUX OR OS_FREEBSD OR OS_MACOS OR OS_WINDOWS" False)
cmake_dependent_option(ENABLE_PLUGIN_CHARTS "Enable metric collectors written in Bash" ${DEFAULT_FEATURE_STATE} "NOT OS_WINDOWS" False)
cmake_dependent_option(ENABLE_PLUGIN_CUPS "Enable CUPS monitoring" ${DEFAULT_FEATURE_STATE} "NOT OS_WINDOWS" False)

cmake_dependent_option(ENABLE_PLUGIN_FREEIPMI "Enable IPMI monitoring" ${DEFAULT_FEATURE_STATE} "OS_LINUX OR OS_FREEBSD" False)

cmake_dependent_option(ENABLE_PLUGIN_CGROUP_NETWORK "Enable Linux CGroup network usage monitoring" ${DEFAULT_FEATURE_STATE} "OS_LINUX" False)
cmake_dependent_option(ENABLE_CGROUPS_LOOKUP_SERVER "Enable cgroups.plugin CGROUPS_LOOKUP netipc server" True "OS_LINUX" False)
cmake_dependent_option(ENABLE_CGROUPS_LOOKUP_TEST_CLIENT "Build cgroups.plugin CGROUPS_LOOKUP test client" False "ENABLE_CGROUPS_LOOKUP_SERVER" False)
cmake_dependent_option(ENABLE_CGROUP_NAME "Build the cgroup-name helper" True "OS_LINUX;ENABLE_PLUGIN_GO" False)
cmake_dependent_option(ENABLE_PLUGIN_DEBUGFS "Enable Linux DebugFS metric collection" ${DEFAULT_FEATURE_STATE} "OS_LINUX" False)
cmake_dependent_option(ENABLE_PLUGIN_EBPF "Enable Linux eBPF metric collection" ${DEFAULT_FEATURE_STATE} "OS_LINUX" False)
cmake_dependent_option(ENABLE_LEGACY_EBPF_PROGRAMS "Enable eBPF programs for kernels without BTF support" True "ENABLE_PLUGIN_EBPF" False)
mark_as_advanced(ENABLE_LEGACY_EBPF_PROGRAMS)
cmake_dependent_option(ENABLE_PLUGIN_LOCAL_LISTENERS "Enable local listening socket tracking (including service auto-discovery support)" ${DEFAULT_FEATURE_STATE} "OS_LINUX" False)
cmake_dependent_option(ENABLE_PLUGIN_NETWORK_VIEWER "Enable network viewer functionality" ${DEFAULT_FEATURE_STATE} "OS_LINUX OR OS_WINDOWS OR OS_FREEBSD OR OS_MACOS" False)
cmake_dependent_option(ENABLE_PLUGIN_NFACCT "Enable Linux NFACCT metric collection" ${DEFAULT_FEATURE_STATE} "OS_LINUX" False)
cmake_dependent_option(ENABLE_PLUGIN_PERF "Enable Linux performance counter monitoring" ${DEFAULT_FEATURE_STATE} "OS_LINUX" False)
cmake_dependent_option(ENABLE_PLUGIN_SLABINFO "Enable Linux kernel SLAB allocator monitoring" ${DEFAULT_FEATURE_STATE} "OS_LINUX" False)
cmake_dependent_option(ENABLE_PLUGIN_SYSTEMD_JOURNAL "Enable systemd journal log collection" ${DEFAULT_FEATURE_STATE} "OS_LINUX" False)
cmake_dependent_option(ENABLE_PLUGIN_SYSTEMD_UNITS "Enable systemd units information collection" ${DEFAULT_FEATURE_STATE} "OS_LINUX" False)
cmake_dependent_option(ENABLE_PLUGIN_XENSTAT "Enable Xen domain monitoring" ${DEFAULT_FEATURE_STATE} "OS_LINUX" False)

cmake_dependent_option(ENABLE_PLUGIN_IBM "Enable IBM ecosystem collectors (requires CGO)" False "OS_LINUX AND CPU_X86_64" False)

# Metrics exporters
option(ENABLE_EXPORTER_PROMETHEUS_REMOTE_WRITE "Enable exporting to Prometheus via remote write API" ${DEFAULT_FEATURE_STATE})
option(ENABLE_EXPORTER_MONGODB "Enable exporting to MongoDB" ${DEFAULT_FEATURE_STATE})

# Vendoring
option(ENABLE_BUNDLED_JSONC "Force use of a vendored copy of JSON-C" False)
option(ENABLE_BUNDLED_YAML "Force use of a vendored copy of libyaml" False)
option(ENABLE_BUNDLED_PROTOBUF "Use a vendored copy of protobuf" False)

# Experimental features
option(ENABLE_WEBRTC "Enable WebRTC dashboard communications (experimental)" False)
mark_as_advanced(ENABLE_WEBRTC)

# Other optional functionality
option(ENABLE_SENTRY "Build with Sentry Native crash reporting" False)
mark_as_advanced(ENABLE_SENTRY)

option(BUILD_FOR_PACKAGING "Include component files for native packages" False)
mark_as_advanced(BUILD_FOR_PACKAGING)

# Selects which native package format the staged tree is shaped for. The empty
# default preserves the historical (DEB-shaped) layout so existing builds are
# unaffected; packaging/build-package.sh passes it explicitly.
set(NETDATA_PACKAGING_FORMAT "" CACHE STRING "Native package format being built (deb, rpm, or empty)")
mark_as_advanced(NETDATA_PACKAGING_FORMAT)
if(NOT NETDATA_PACKAGING_FORMAT STREQUAL "" AND
   NOT NETDATA_PACKAGING_FORMAT STREQUAL "deb" AND
   NOT NETDATA_PACKAGING_FORMAT STREQUAL "rpm")
        message(FATAL_ERROR "Invalid NETDATA_PACKAGING_FORMAT '${NETDATA_PACKAGING_FORMAT}' (expected deb, rpm, or empty)")
endif()

# CPack only emits Recommends: from CMake 4.1 on; older versions ignore the
# variables silently, which would strip the weak dependencies from the RPMs
# without any build failure. Checked here rather than in Packaging.cmake so
# the configure fails fast.
if(NETDATA_PACKAGING_FORMAT STREQUAL "rpm" AND CMAKE_VERSION VERSION_LESS 4.1)
        message(FATAL_ERROR "RPM packaging requires CMake >= 4.1 (CPack weak-dependency support); this is ${CMAKE_VERSION}")
endif()

# A few payloads live in different packages per format: RPM keeps the
# sensors3/otel stock configs and nd-mcp in the main package and the swagger
# files in the dashboard package, while the DEB layout groups them with their
# plugins (and swagger with the main package).
if(NETDATA_PACKAGING_FORMAT STREQUAL "rpm")
        set(NETDATA_SENSORS3_COMPONENT netdata)
        set(NETDATA_ND_MCP_COMPONENT netdata)
        set(NETDATA_OTEL_CONF_COMPONENT netdata)
        set(NETDATA_SWAGGER_COMPONENT dashboard)
else()
        set(NETDATA_SENSORS3_COMPONENT plugin-debugfs)
        set(NETDATA_ND_MCP_COMPONENT plugin-go)
        set(NETDATA_OTEL_CONF_COMPONENT plugin-otel)
        set(NETDATA_SWAGGER_COMPONENT netdata)
endif()
