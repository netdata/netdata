# SPDX-License-Identifier: GPL-3.0-or-later
# Functions and macros for bundling Bash into the macOS package

include_guard()

# Handle bundling of Bash for the macOS package.
#
# macOS ships Bash 3.2 (the last GPLv2 release) and several shipped scripts
# require Bash 4+ - most importantly alarm-notify.sh, the alert-notification
# dispatcher. The Linux static installer already solves this by building
# Bash 5.3 into its payload; the macOS package does the same, using the same
# pin and checksum (packaging/makeself/bundled-packages.version), minus the
# static-link flag Apple's linker cannot honour.
#
# The payload's scripts are then rewritten to this interpreter at install
# time - see netdata_add_macos_shebang_rewrite() below.
function(netdata_bundle_bash)
        include(ExternalProject)
        include(ProcessorCount)

        message(STATUS "Preparing bundled Bash")

        set(version 5.3)
        set(sha256 0d5cd86965f869a26cf64f4b71be7b96f90a3ba8b3d74e27e8e9d9d5550f31ba)

        set(install_dir "${CMAKE_BINARY_DIR}/_deps/bash-install")

        set(configure_args
                --prefix=${install_dir}
                --without-bash-malloc
                --enable-net-redirections
                --enable-array-variables
                --disable-progcomp
                --disable-profiling
                --disable-nls
        )

        if(CMAKE_OSX_DEPLOYMENT_TARGET)
                list(APPEND configure_args "CFLAGS=-O2 -mmacosx-version-min=${CMAKE_OSX_DEPLOYMENT_TARGET}")
        endif()

        ProcessorCount(ncpu)
        if(ncpu EQUAL 0)
                set(ncpu 2)
        endif()

        # Not EXCLUDE_FROM_ALL: nothing links Bash, so it must be reachable
        # from the default target for the install rule below to have a file
        # to install.
        ExternalProject_Add(bundled-bash
                URL https://ftp.gnu.org/gnu/bash/bash-${version}.tar.gz
                URL_HASH SHA256=${sha256}
                CONFIGURE_COMMAND <SOURCE_DIR>/configure ${configure_args}
                BUILD_COMMAND make -j${ncpu}
                INSTALL_COMMAND make install
                INSTALL_DIR "${install_dir}"
                BUILD_BYPRODUCTS "${install_dir}/bin/bash"
        )

        install(PROGRAMS "${install_dir}/bin/bash"
                COMPONENT netdata
                DESTINATION bin)

        message(STATUS "Finished preparing bundled Bash")
endfunction()

# Rewrite the bash shebangs of every script in the staged payload to the
# bundled interpreter. Registered as the last install rule of the macOS
# package so it runs after every script has been staged; scripts then run
# correctly however they are invoked, with no reliance on PATH ordering.
function(netdata_add_macos_shebang_rewrite)
        install(CODE "
                execute_process(
                        COMMAND \"${CMAKE_SOURCE_DIR}/packaging/macos/rewrite-shebangs.sh\"
                                \"\$ENV{DESTDIR}${CMAKE_INSTALL_PREFIX}\"
                                \"${NETDATA_RUNTIME_PREFIX}/bin/bash\"
                        RESULT_VARIABLE _rewrite_rc
                )
                if(NOT _rewrite_rc EQUAL 0)
                        message(FATAL_ERROR \"Shebang rewrite failed with status \${_rewrite_rc}\")
                endif()
        " COMPONENT netdata)
endfunction()
