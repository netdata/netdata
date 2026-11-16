# SPDX-License-Identifier: GPL-3.0-or-later
# nfacct: netfilter accounting collection.
#
# include()d from the root file, so paths resolve against the repository and
# build roots; nothing here may use CMAKE_CURRENT_LIST_DIR.
#
# Its source list, executable and install rule move together, so nothing here
# takes part in the NETDATA_FILES/LIBNETDATA_FILES composition.

include_guard()

if(ENABLE_PLUGIN_NFACCT)
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
