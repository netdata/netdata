# SPDX-License-Identifier: GPL-3.0-or-later
# freeipmi: IPMI sensor collection.
#
# Relocated verbatim from the root CMakeLists.txt. include()d rather than
# add_subdirectory()d so CMAKE_CURRENT_SOURCE_DIR and CMAKE_CURRENT_BINARY_DIR
# keep pointing at the repository and build roots, which every relative path
# below depends on. Nothing here may use CMAKE_CURRENT_LIST_DIR.
#
# Its source list, executable and install rule move together, so nothing here
# takes part in the NETDATA_FILES/LIBNETDATA_FILES composition.

if(ENABLE_PLUGIN_FREEIPMI)
    if(NOT IPMI_FOUND)
        message(FATAL_ERROR "The freeipmi plugin needs libipmimonitoring, but it could not be found. Pass -DENABLE_PLUGIN_FREEIPMI=Off to build without it.")
    endif()

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
