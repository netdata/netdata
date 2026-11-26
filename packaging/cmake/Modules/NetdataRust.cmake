# SPDX-License-Identifier: GPL-3.0-or-later
# The Rust toolchain: the rustc preflight, Corrosion, crate imports and per-crate flags for every Rust-based feature.
#
# include()d from the root file, so paths resolve against the repository and
# build roots; nothing here may use CMAKE_CURRENT_LIST_DIR.

include_guard()

# Memory available to this build, in MiB: the host's RAM, or a smaller cgroup
# memory limit on this process (a container or a systemd scope), whichever is
# lower. Checks the process's own cgroup and every ancestor visible to it, in
# the cgroup v2 hierarchy and the v1 memory hierarchy, wherever /proc/self/mountinfo
# says they are mounted. Without a readable limit, the host's RAM is the answer.
function(_netdata_build_memory_mib out_mib out_source)
    cmake_host_system_information(RESULT _mib QUERY TOTAL_PHYSICAL_MEMORY)
    set(_source "host RAM")

    if(CMAKE_HOST_SYSTEM_NAME STREQUAL "Linux" AND EXISTS "/proc/self/cgroup" AND EXISTS "/proc/self/mountinfo")
        file(STRINGS "/proc/self/cgroup" _lines)
        foreach(_line IN LISTS _lines)
            if(_line MATCHES "^0::(/.*)$")
                set(_cgroup_v2 "${CMAKE_MATCH_1}")
            elseif(_line MATCHES "^[0-9]+:([^:]*,)?memory(,[^:]*)?:(/.*)$")
                set(_cgroup_v1 "${CMAKE_MATCH_3}")
            endif()
        endforeach()

        set(_files "")
        file(STRINGS "/proc/self/mountinfo" _mounts)
        foreach(_mount IN LISTS _mounts)
            # Fields 4 and 5: the cgroup mounted there and the mount point.
            if(DEFINED _cgroup_v2 AND _mount MATCHES "^[^ ]+ [^ ]+ [^ ]+ ([^ ]+) ([^ ]+) .* - cgroup2 ")
                set(_cgroup "${_cgroup_v2}")
                set(_leaf "memory.max")
            elseif(DEFINED _cgroup_v1 AND _mount MATCHES "^[^ ]+ [^ ]+ [^ ]+ ([^ ]+) ([^ ]+) .* - cgroup [^ ]+ (.*,)?memory(,.*)?$")
                set(_cgroup "${_cgroup_v1}")
                set(_leaf "memory.limit_in_bytes")
            else()
                continue()
            endif()
            set(_mount_root "${CMAKE_MATCH_1}")
            set(_mount_point "${CMAKE_MATCH_2}")

            # Only the part of the process's cgroup path below the mounted cgroup is visible here.
            if(_mount_root STREQUAL "/")
                set(_rel "${_cgroup}")
            else()
                string(FIND "${_cgroup}/" "${_mount_root}/" _pos)
                if(NOT _pos EQUAL 0)
                    continue()
                endif()
                string(LENGTH "${_mount_root}" _len)
                string(SUBSTRING "${_cgroup}" ${_len} -1 _rel)
            endif()

            while(NOT _rel STREQUAL "" AND NOT _rel STREQUAL "/")
                list(APPEND _files "${_mount_point}${_rel}/${_leaf}")
                get_filename_component(_rel "${_rel}" DIRECTORY)
            endwhile()
            list(APPEND _files "${_mount_point}/${_leaf}")
        endforeach()

        list(REMOVE_DUPLICATES _files)
        foreach(_file IN LISTS _files)
            if(EXISTS "${_file}")
                file(READ "${_file}" _limit)
                string(STRIP "${_limit}" _limit)
                # cgroup v2 writes "max" for no limit; v1 a huge byte count.
                if(_limit MATCHES "^[0-9]+$")
                    math(EXPR _limit_mib "${_limit} / 1048576")
                    if(_limit_mib LESS _mib)
                        set(_mib ${_limit_mib})
                        set(_source "the cgroup limit in ${_file}")
                    endif()
                endif()
            endif()
        endforeach()
    endif()

    set(${out_mib} ${_mib} PARENT_SCOPE)
    set(${out_source} "${_source}" PARENT_SCOPE)
endfunction()

if(ENABLE_NETDATA_JOURNAL_FILE_READER OR ENABLE_PLUGIN_OTEL OR ENABLE_PLUGIN_NETFLOW)
    # Check for the toolchain before fetching Corrosion: without this, a missing
    # rustc surfaces as an error deep inside Corrosion's FindRust with no hint of
    # which option to turn off (#23315). Corrosion's own Rust_COMPILER knob wins
    # when set, so the preflight can never reject a toolchain the build would
    # use. The rustup default location is searched explicitly because Corrosion's
    # FindRust searches it too, and PATH cannot be trusted to carry it: the
    # installer sources /etc/profile, which can reset PATH and hide a rustup
    # toolchain the build is about to use (the Docker build hits exactly this).
    if(Rust_COMPILER)
        set(RUSTC_EXECUTABLE "${Rust_COMPILER}")
    else()
        find_program(RUSTC_EXECUTABLE rustc HINTS "$ENV{CARGO_HOME}/bin" "$ENV{HOME}/.cargo/bin")
    endif()
    if(NOT RUSTC_EXECUTABLE)
        message(FATAL_ERROR "A Rust toolchain (rustc) is required by the enabled Rust-based features but was not found. Install one (e.g. via rustup), or pass -DENABLE_PLUGIN_OTEL=Off -DENABLE_PLUGIN_NETFLOW=Off -DENABLE_NETDATA_JOURNAL_FILE_READER=Off to build without them.")
    endif()

    file(STRINGS src/crates/Cargo.toml _nd_rust_version_line REGEX "^rust-version = \"")
    string(REGEX REPLACE "^rust-version = \"([0-9.]+)\".*$" "\\1" _nd_min_rust_version "${_nd_rust_version_line}")
    if(NOT _nd_min_rust_version MATCHES "^[0-9.]+$")
        # Fail closed: an unparsed manifest would otherwise compare against the
        # empty string, which VERSION_LESS treats as 0, silently dropping the floor.
        message(FATAL_ERROR "Could not read rust-version from src/crates/Cargo.toml; update the extraction in NetdataRust.cmake to match the manifest's current shape.")
    endif()
    execute_process(COMMAND "${RUSTC_EXECUTABLE}" --version
                    RESULT_VARIABLE _nd_rustc_result
                    OUTPUT_VARIABLE _nd_rustc_version
                    OUTPUT_STRIP_TRAILING_WHITESPACE)
    string(REGEX REPLACE "^rustc ([0-9.]+).*$" "\\1" _nd_rustc_version "${_nd_rustc_version}")
    if(_nd_rustc_result OR _nd_rustc_version VERSION_LESS _nd_min_rust_version)
        message(FATAL_ERROR "The enabled Rust-based features need rustc >= ${_nd_min_rust_version}, but ${RUSTC_EXECUTABLE} reports '${_nd_rustc_version}'. Update the toolchain, or pass -DENABLE_PLUGIN_OTEL=Off -DENABLE_PLUGIN_NETFLOW=Off -DENABLE_NETDATA_JOURNAL_FILE_READER=Off to build without them.")
    endif()
    unset(_nd_rust_version_line)
    unset(_nd_min_rust_version)
    unset(_nd_rustc_result)
    unset(_nd_rustc_version)

    include(FetchContent)
    FetchContent_Declare(
        Corrosion
        GIT_REPOSITORY https://github.com/netdata/corrosion.git
        GIT_TAG f23459775a9fe2157fa3a9100c2bcbaaf326d870 # stable/v0.5 + parallel-cargo-builds
    )
    FetchContent_MakeAvailable(Corrosion)

    # Corrosion places cargo build artifacts under ${CMAKE_BINARY_DIR}/cargo/
    # (build/, or build-<target>/ per target with ISOLATE_CARGO_TARGET_DIRS;
    # see Corrosion.cmake cargo_target_dir). Register it for cleanup so that
    # ninja clean removes it.  If a future Corrosion version changes this
    # path, this line must be updated to match.
    set_directory_properties(PROPERTIES ADDITIONAL_CLEAN_FILES "${CMAKE_BINARY_DIR}/cargo")

    if(ENABLE_NETDATA_JOURNAL_FILE_READER)
        corrosion_import_crate(MANIFEST_PATH src/crates/jf/Cargo.toml
                               CRATES journal_reader_ffi)
    endif()

    # Import crates from the main cargo workspace
    set(_WORKSPACE_CRATES "")
    if(ENABLE_PLUGIN_OTEL)
        list(APPEND _WORKSPACE_CRATES otel-plugin)
    endif()
    if(ENABLE_PLUGIN_NETFLOW)
        list(APPEND _WORKSPACE_CRATES netflow-plugin)
    endif()
    # otel-plugin and netflow-plugin build one after the other by default: each
    # cargo build runs in Ninja's console pool, and both share one cargo target
    # directory, which cargo locks for the whole build. Removing both lets them
    # build at the same time, at the cost of compiling the shared dependencies
    # twice and roughly doubling the peak memory of the Rust part of the build.
    set(_nd_rust_parallel_args "")
    if(ENABLE_PLUGIN_OTEL AND ENABLE_PLUGIN_NETFLOW)
        string(TOUPPER "${USE_PARALLEL_RUST_BUILDS}" _nd_rust_parallel_mode)
        if(_nd_rust_parallel_mode STREQUAL "AUTO")
            _netdata_build_memory_mib(_nd_build_memory_mib _nd_build_memory_source)
            # 15 GiB rather than 16: a 16 GB machine reports about 15.6 GiB.
            if(_nd_build_memory_mib GREATER_EQUAL 15360)
                set(_nd_rust_parallel ON)
            else()
                set(_nd_rust_parallel OFF)
            endif()
            set(_nd_rust_parallel_reason "AUTO: ${_nd_build_memory_mib} MiB of ${_nd_build_memory_source}, needs 15360")
            unset(_nd_build_memory_mib)
            unset(_nd_build_memory_source)
        elseif(USE_PARALLEL_RUST_BUILDS)
            set(_nd_rust_parallel ON)
            set(_nd_rust_parallel_reason "USE_PARALLEL_RUST_BUILDS=${USE_PARALLEL_RUST_BUILDS}")
        else()
            set(_nd_rust_parallel OFF)
            set(_nd_rust_parallel_reason "USE_PARALLEL_RUST_BUILDS=${USE_PARALLEL_RUST_BUILDS}")
        endif()

        if(_nd_rust_parallel)
            set(_nd_rust_parallel_args NO_USES_TERMINAL ISOLATE_CARGO_TARGET_DIRS)
            message(STATUS "Building the Rust plugins in parallel (${_nd_rust_parallel_reason})")
        else()
            message(STATUS "Building the Rust plugins one after the other (${_nd_rust_parallel_reason})")
        endif()
        unset(_nd_rust_parallel)
        unset(_nd_rust_parallel_mode)
        unset(_nd_rust_parallel_reason)
    endif()

    if(_WORKSPACE_CRATES)
        corrosion_import_crate(MANIFEST_PATH src/crates/Cargo.toml
                               CRATES ${_WORKSPACE_CRATES}
                               ${_nd_rust_parallel_args})
    endif()
    unset(_nd_rust_parallel_args)

    # Cargo links the Rust executables through the bare compiler driver, so CMAKE_EXE_LINKER_FLAGS never reach those
    # links. When the build asks for the Cortex-A53 erratum workarounds (static aarch64), have the linker apply them to
    # the Rust code too: 843419 and 835769 can both be fixed at link time, and rustc has no -mfix-cortex-a53-* flags.
    # 8-byte function alignment stops ld.bfd's 843419 fix stubs from shifting code it has already scanned.
    set(_nd_rust_link_flags "")
    if(CMAKE_EXE_LINKER_FLAGS MATCHES "--fix-cortex-a53-843419")
        list(APPEND _nd_rust_link_flags "-Clink-arg=-Wl,--fix-cortex-a53-835769" "-Clink-arg=-Wl,--fix-cortex-a53-843419")
        list(APPEND _nd_rust_link_flags "-Cllvm-args=-align-all-functions=3")
    endif()

    if(ENABLE_PLUGIN_OTEL)
      if(STATIC_BUILD)
        corrosion_add_target_rustflags(otel-plugin --cfg=tracing_unstable "-C" "target-feature=+crt-static" ${_nd_rust_link_flags})
      else()
        corrosion_add_target_rustflags(otel-plugin --cfg=tracing_unstable ${_nd_rust_link_flags})
      endif()

      # The workspace release profile propagates fat LTO, single-codegen-unit,
      # and debuginfo settings through the whole dependency graph. On 32-bit
      # targets that makes rustc-LLVM exhaust the ~3 GB process address space
      # during codegen ("rustc-LLVM ERROR: out of memory"). Override Cargo's
      # release profile so the otel-plugin build stays within budget.
      if(CMAKE_SIZEOF_VOID_P EQUAL 4 AND CMAKE_SYSTEM_NAME STREQUAL "Linux")
        corrosion_set_env_vars(
                otel-plugin
                "CARGO_PROFILE_RELEASE_DEBUG=0"
                "CARGO_PROFILE_RELEASE_LTO=off"
                "CARGO_PROFILE_RELEASE_CODEGEN_UNITS=8")
      endif()
    endif()

    if(ENABLE_PLUGIN_NETFLOW)
      set(NETFLOW_PLUGIN_RUSTFLAGS "")
      set(NETFLOW_PLUGIN_ENV_VARS
              "NETDATA_BUILD_CACHE_DIR=${CACHE_DIR}"
              "NETDATA_BUILD_LIB_DIR=${VARLIB_DIR}"
              "NETDATA_BUILD_STOCK_DATA_DIR=${STOCK_DATA_DIR}")

      if(CMAKE_SIZEOF_VOID_P EQUAL 4 AND CMAKE_SYSTEM_NAME STREQUAL "Linux")
        # The workspace release profile propagates fat LTO, single-codegen-unit,
        # and debuginfo settings through Rust dependencies on 32-bit builds. Override
        # Cargo's release profile so the entire dependency graph stays linkable.
        list(APPEND NETFLOW_PLUGIN_ENV_VARS
                "CARGO_PROFILE_RELEASE_DEBUG=0"
                "CARGO_PROFILE_RELEASE_LTO=off"
                "CARGO_PROFILE_RELEASE_CODEGEN_UNITS=8")
      endif()

      if(STATIC_BUILD)
        list(APPEND NETFLOW_PLUGIN_RUSTFLAGS "-C" "target-feature=+crt-static")
      endif()

      list(APPEND NETFLOW_PLUGIN_RUSTFLAGS ${_nd_rust_link_flags})

      if(NETFLOW_PLUGIN_RUSTFLAGS)
        corrosion_add_target_rustflags(netflow-plugin ${NETFLOW_PLUGIN_RUSTFLAGS})
      endif()

      corrosion_set_env_vars(netflow-plugin ${NETFLOW_PLUGIN_ENV_VARS})
    endif()

    unset(_nd_rust_link_flags)
endif()
