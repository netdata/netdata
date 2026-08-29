# SPDX-License-Identifier: GPL-3.0-or-later
# freeipmi: IPMI sensor collection.
#
# include()d from the root file, so paths resolve against the repository and
# build roots; nothing here may use CMAKE_CURRENT_LIST_DIR.
#
# Its source list, executable and install rule move together, so nothing here
# takes part in the NETDATA_FILES/LIBNETDATA_FILES composition.

include_guard()

if(ENABLE_PLUGIN_FREEIPMI)
    set(FREEIPMI_PLUGIN_FILES src/collectors/freeipmi.plugin/freeipmi_plugin.c)

    add_executable(freeipmi.plugin ${FREEIPMI_PLUGIN_FILES})
    target_link_libraries (freeipmi.plugin libnetdata ${IPMI_LIBRARIES})
    target_include_directories(freeipmi.plugin PRIVATE ${IPMI_INCLUDE_DIRS})
    target_compile_options(freeipmi.plugin PRIVATE ${IPMI_CFLAGS_OTHER})

    install(TARGETS freeipmi.plugin
            COMPONENT plugin-freeipmi
            DESTINATION ${PLUGINS_DEST})

    netdata_add_deb_copyright(plugin-freeipmi netdata-plugin-freeipmi)
endif()
