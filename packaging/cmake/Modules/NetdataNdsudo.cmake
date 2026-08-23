# SPDX-License-Identifier: GPL-3.0-or-later
# ndsudo: the setuid helper that runs privileged collector commands.
#
# include()d from the root file, so paths resolve against the repository and
# build roots; nothing here may use CMAKE_CURRENT_LIST_DIR.
#
# Its source list, executable and install rule move together, so nothing here
# takes part in the NETDATA_FILES/LIBNETDATA_FILES composition.

include_guard()

# Wanted by the Go-based collectors, which shell out to it for privileged
# commands, and on macOS. Derived here because this is its only consumer.
if(OS_MACOS OR ENABLE_PLUGIN_GO OR ENABLE_PLUGIN_SCRIPTS)
    set(NDSUDO_FILES src/collectors/utils/ndsudo.c)

    add_executable(ndsudo ${NDSUDO_FILES})

    install(TARGETS ndsudo
            COMPONENT netdata
            DESTINATION ${PLUGINS_DEST})
    set_source_files_properties(${NDSUDO_FILES} PROPERTIES COMPILE_OPTIONS "-Wno-unused-result")
endif()
