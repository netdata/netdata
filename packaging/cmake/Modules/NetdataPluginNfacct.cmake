# SPDX-License-Identifier: GPL-3.0-or-later
# nfacct: netfilter accounting collection.
#
# Relocated verbatim from the root CMakeLists.txt. include()d rather than
# add_subdirectory()d so CMAKE_CURRENT_SOURCE_DIR and CMAKE_CURRENT_BINARY_DIR
# keep pointing at the repository and build roots, which every relative path
# below depends on. Nothing here may use CMAKE_CURRENT_LIST_DIR.
#
# Its source list, executable and install rule move together; that is the only form of source-list extraction that does not risk the NETDATA_FILES/LIBNETDATA_FILES composition.

if(ENABLE_PLUGIN_NFACCT)
    if (NOT MNL_FOUND)
        message(FATAL_ERROR "Can not build nfacct.plugin because MNL library could not be found.")
    endif()

    pkg_check_modules(NFACCT REQUIRED libnetfilter_acct)

    set(NFACCT_PLUGIN_FILES src/collectors/nfacct.plugin/plugin_nfacct.c)

    add_executable(nfacct.plugin ${NFACCT_PLUGIN_FILES})
    target_link_libraries (nfacct.plugin libnetdata ${MNL_LIBRARIES} ${NFACCT_LIBRARIES})
    target_include_directories(nfacct.plugin PRIVATE ${MNL_INCLUDE_DIRS} ${NFACCT_INCLUDE_DIRS})
    target_compile_options(nfacct.plugin PRIVATE ${MNL_CFLAGS_OTHER} ${NFACCT_CFLAGS_OTHER})

    install(TARGETS nfacct.plugin
            COMPONENT plugin-nfacct
            DESTINATION ${PLUGINS_DEST})

    netdata_add_deb_copyright(plugin-nfacct netdata-plugin-nfacct)
endif()
