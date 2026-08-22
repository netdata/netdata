# SPDX-License-Identifier: GPL-3.0-or-later
# ndsudo: the setuid helper that runs privileged collector commands.
#
# Relocated verbatim from the root CMakeLists.txt. include()d rather than
# add_subdirectory()d so CMAKE_CURRENT_SOURCE_DIR and CMAKE_CURRENT_BINARY_DIR
# keep pointing at the repository and build roots, which every relative path
# below depends on. Nothing here may use CMAKE_CURRENT_LIST_DIR.
#
# Its source list, executable and install rule move together, so nothing here
# takes part in the NETDATA_FILES/LIBNETDATA_FILES composition.

if(NEED_NDSUDO)
    set(NDSUDO_FILES src/collectors/utils/ndsudo.c)

    add_executable(ndsudo ${NDSUDO_FILES})

    install(TARGETS ndsudo
            COMPONENT netdata
            DESTINATION ${PLUGINS_DEST})
    set_source_files_properties(${NDSUDO_FILES} PROPERTIES COMPILE_OPTIONS "-Wno-unused-result")
endif()
