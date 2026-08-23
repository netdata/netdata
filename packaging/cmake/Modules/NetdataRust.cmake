# SPDX-License-Identifier: GPL-3.0-or-later
# The Rust toolchain: the rustc preflight, Corrosion, crate imports and per-crate flags for every Rust-based feature.
#
# Relocated verbatim from the root CMakeLists.txt. include()d rather than
# add_subdirectory()d so CMAKE_CURRENT_SOURCE_DIR and CMAKE_CURRENT_BINARY_DIR
# keep pointing at the repository and build roots, which every relative path
# below depends on. Nothing here may use CMAKE_CURRENT_LIST_DIR.

# Setup Rust/Corrosion for plugins that need it
if(ENABLE_NETDATA_JOURNAL_FILE_READER OR ENABLE_PLUGIN_OTEL OR ENABLE_PLUGIN_NETFLOW)
    # Check for the toolchain before fetching Corrosion: without this, a missing
    # rustc surfaces as an error deep inside Corrosion's FindRust with no hint of
    # which option to turn off (#23315). The floor is the workspace's declared
    # rust-version, read from the manifest so the two cannot drift apart.
    find_program(RUSTC_EXECUTABLE rustc)
    if(NOT RUSTC_EXECUTABLE)
        message(FATAL_ERROR "A Rust toolchain (rustc) is required by the enabled Rust-based features but was not found. Install one (e.g. via rustup), or pass -DENABLE_PLUGIN_OTEL=Off -DENABLE_PLUGIN_NETFLOW=Off -DENABLE_NETDATA_JOURNAL_FILE_READER=Off to build without them.")
    endif()

    file(STRINGS src/crates/Cargo.toml _nd_rust_version_line REGEX "^rust-version = \"")
    string(REGEX REPLACE "^rust-version = \"([0-9.]+)\".*$" "\\1" _nd_min_rust_version "${_nd_rust_version_line}")
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
    # `ninja clean` removes it.  If a future Corrosion version changes this
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

    if(ENABLE_PLUGIN_OTEL)
      if(STATIC_BUILD)
        corrosion_add_target_rustflags(otel-plugin --cfg=tracing_unstable "-C" "target-feature=+crt-static")
      else()
        corrosion_add_target_rustflags(otel-plugin --cfg=tracing_unstable)
      endif()

      # The workspace release profile propagates fat LTO, single-codegen-unit,
      # and debuginfo settings through the whole dependency graph. On 32-bit
      # targets that makes rustc-LLVM exhaust the ~3 GB process address space
      # during codegen ("rustc-LLVM ERROR: out of memory"). Override Cargo's
      # release profile so the otel-plugin build stays within budget.
      if(CMAKE_SIZEOF_VOID_P EQUAL 4 AND CMAKE_SYSTEM_NAME STREQUAL "Linux")
        # We depend (transitively) on the `io-uring` crate which doesn't provide
        # prebuilt bindings for 32-bit arches. Considering this is a compile-time
        # and not a runtime dependency (because we don't use foyer's io-uring
        # engine), we can simply skip the check.
        corrosion_add_target_rustflags(otel-plugin --cfg=io_uring_skip_arch_check)
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

      # We depend (transitively) on the `io-uring` crate which doesn't provide
      # prebuilt bindings for 32-bit arches. Considering this is a compile-time
      # and not a runtime dependency (because we don't use foyer's io-uring
      # engine), we can simply skip the check.
      if(CMAKE_SIZEOF_VOID_P EQUAL 4 AND CMAKE_SYSTEM_NAME STREQUAL "Linux")
        list(APPEND NETFLOW_PLUGIN_RUSTFLAGS --cfg=io_uring_skip_arch_check)
        # The workspace release profile still propagates fat LTO, single-codegen-unit,
        # and debuginfo settings through Rust dependencies on 32-bit builds. Override
        # Cargo's release profile too so the entire dependency graph stays linkable.
        list(APPEND NETFLOW_PLUGIN_ENV_VARS
                "CARGO_PROFILE_RELEASE_DEBUG=0"
                "CARGO_PROFILE_RELEASE_LTO=off"
                "CARGO_PROFILE_RELEASE_CODEGEN_UNITS=8")
      endif()

      if(STATIC_BUILD)
        list(APPEND NETFLOW_PLUGIN_RUSTFLAGS "-C" "target-feature=+crt-static")
      endif()

      if(NETFLOW_PLUGIN_RUSTFLAGS)
        corrosion_add_target_rustflags(netflow-plugin ${NETFLOW_PLUGIN_RUSTFLAGS})
      endif()

      corrosion_set_env_vars(netflow-plugin ${NETFLOW_PLUGIN_ENV_VARS})
    endif()
endif()
