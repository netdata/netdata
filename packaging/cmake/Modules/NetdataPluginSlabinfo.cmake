# SPDX-License-Identifier: GPL-3.0-or-later
# slabinfo: kernel SLAB allocator collection.
#
# Relocated verbatim from the root CMakeLists.txt. include()d rather than
# add_subdirectory()d so CMAKE_CURRENT_SOURCE_DIR and CMAKE_CURRENT_BINARY_DIR
# keep pointing at the repository and build roots, which every relative path
# below depends on. Nothing here may use CMAKE_CURRENT_LIST_DIR.
#
# Its source list, executable and install rule move together, so nothing here
# takes part in the NETDATA_FILES/LIBNETDATA_FILES composition.

if(ENABLE_PLUGIN_SLABINFO)
    set(SLABINFO_PLUGIN_FILES src/collectors/slabinfo.plugin/slabinfo.c)

    add_executable(slabinfo.plugin ${SLABINFO_PLUGIN_FILES})
    target_link_libraries(slabinfo.plugin libnetdata)

    install(TARGETS slabinfo.plugin
            COMPONENT plugin-slabinfo
            DESTINATION ${PLUGINS_DEST})

    netdata_add_deb_copyright(plugin-slabinfo netdata-plugin-slabinfo)
endif()
