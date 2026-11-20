# SPDX-License-Identifier: GPL-3.0-or-later
# The Rust toolchain: the rustc preflight, Corrosion, crate imports and per-crate flags for every Rust-based feature.
#
# include()d from the root file, so paths resolve against the repository and
# build roots; nothing here may use CMAKE_CURRENT_LIST_DIR.

include_guard()

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
        GIT_TAG f3b91559efca32c6b54837866ef35ba98ff5b2ca # stable/v0.5
    )
    FetchContent_MakeAvailable(Corrosion)

    # Corrosion places cargo build artifacts under ${CMAKE_BINARY_DIR}/cargo/
    # (see Corrosion.cmake cargo_target_dir). Register it for cleanup so that
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
    if(_WORKSPACE_CRATES)
        corrosion_import_crate(MANIFEST_PATH src/crates/Cargo.toml
                               CRATES ${_WORKSPACE_CRATES})
    endif()

    # Cargo links the Rust executables through the bare compiler driver, so CMAKE_EXE_LINKER_FLAGS never reach those
    # links. When the build asks for the Cortex-A53 erratum workarounds (static aarch64), have the linker apply them to
    # the Rust code too: 843419 and 835769 can both be fixed at link time, and rustc has no -mfix-cortex-a53-* flags.
    # 8-byte function alignment stops ld.bfd's 843419 fix stubs from shifting code it has already scanned.
    set(_nd_rust_link_flags "")
    if(CMAKE_EXE_LINKER_FLAGS MATCHES "--fix-cortex-a53-843419")
        list(APPEND _nd_rust_link_flags "-Clink-arg=-Wl,--fix-cortex-a53-835769" "-Clink-arg=-Wl,--fix-cortex-a53-843419")
        list(APPEND _nd_rust_link_flags "-Cllvm-args=-align-all-functions=3")
    endif()

    # Cargo release-profile overrides, applied to every Rust plugin below. The
    # workspace release profile (src/crates/Cargo.toml) uses fat LTO with one
    # codegen unit.
    set(_nd_rust_32bit_linux OFF)
    set(_nd_rust_release_env "")
    if(CMAKE_SIZEOF_VOID_P EQUAL 4 AND CMAKE_SYSTEM_NAME STREQUAL "Linux")
      # On 32-bit targets that profile makes rustc-LLVM exhaust the ~3 GB process
      # address space during codegen ("rustc-LLVM ERROR: out of memory"), so the
      # whole dependency graph is built without LTO and debuginfo.
      set(_nd_rust_32bit_linux ON)
      set(_nd_rust_release_env
              "CARGO_PROFILE_RELEASE_DEBUG=0"
              "CARGO_PROFILE_RELEASE_LTO=off"
              "CARGO_PROFILE_RELEASE_CODEGEN_UNITS=8")
    elseif(NOT USE_LTO AND NETDATA_PACKAGE_KIND STREQUAL "bundle")
      # A build that asked for no LTO builds otel-plugin and netflow-plugin
      # without it too: fat LTO is a several-minute single-core step on every
      # clean build and Rust rebuild. src/crates/jf keeps its own profile.
      # Native packages keep the workspace profile even with USE_LTO off
      # (openSUSE RPMs disable C/C++ LTO only).
      set(_nd_rust_release_env
              "CARGO_PROFILE_RELEASE_LTO=off"
              "CARGO_PROFILE_RELEASE_CODEGEN_UNITS=16")
    endif()

    if(ENABLE_PLUGIN_OTEL)
      if(STATIC_BUILD)
        corrosion_add_target_rustflags(otel-plugin --cfg=tracing_unstable "-C" "target-feature=+crt-static" ${_nd_rust_link_flags})
      else()
        corrosion_add_target_rustflags(otel-plugin --cfg=tracing_unstable ${_nd_rust_link_flags})
      endif()

      if(_nd_rust_32bit_linux)
        # The `io-uring` crate (a transitive dependency) provides no prebuilt
        # bindings for 32-bit arches. The dependency is compile-time only -
        # foyer's io-uring engine is never used - so its arch check is safe
        # to skip.
        corrosion_add_target_rustflags(otel-plugin --cfg=io_uring_skip_arch_check)
      endif()

      if(_nd_rust_release_env)
        corrosion_set_env_vars(otel-plugin ${_nd_rust_release_env})
      endif()
    endif()

    if(ENABLE_PLUGIN_NETFLOW)
      set(NETFLOW_PLUGIN_RUSTFLAGS "")
      set(NETFLOW_PLUGIN_ENV_VARS
              "NETDATA_BUILD_CACHE_DIR=${CACHE_DIR}"
              "NETDATA_BUILD_LIB_DIR=${VARLIB_DIR}"
              "NETDATA_BUILD_STOCK_DATA_DIR=${STOCK_DATA_DIR}"
              ${_nd_rust_release_env})

      # The io-uring situation from the otel-plugin block above applies here
      # too: compile-time-only dependency, no prebuilt 32-bit bindings.
      if(_nd_rust_32bit_linux)
        list(APPEND NETFLOW_PLUGIN_RUSTFLAGS --cfg=io_uring_skip_arch_check)
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

    unset(_nd_rust_32bit_linux)
    unset(_nd_rust_release_env)
    unset(_nd_rust_link_flags)
endif()
