# SPDX-License-Identifier: GPL-3.0-or-later
# cups: CUPS printer collection.
#
# include()d from the root file, so paths resolve against the repository and
# build roots; nothing here may use CMAKE_CURRENT_LIST_DIR.
#
# Its source list, executable and install rule move together, so nothing here
# takes part in the NETDATA_FILES/LIBNETDATA_FILES composition.

include_guard()

if(ENABLE_PLUGIN_CUPS)
    pkg_check_modules(CUPS libcups)
    if(NOT CUPS_FOUND)
        pkg_check_modules(CUPS cups)
        if(NOT CUPS_FOUND)
                find_program(CUPS_CONFIG cups-config)
                if(CUPS_CONFIG)
                        execute_process(COMMAND ${CUPS_CONFIG} --api-version OUTPUT_VARIABLE CUPS_API_VERSION OUTPUT_STRIP_TRAILING_WHITESPACE)
                        if(CUPS_API_VERSION VERSION_LESS "1.7")
                                set(CUPS_FOUND False)
                        else()
                                execute_process(COMMAND ${CUPS_CONFIG} --cflags OUTPUT_VARIABLE CUPS_CFLAGS_OTHER OUTPUT_STRIP_TRAILING_WHITESPACE)
                                execute_process(COMMAND ${CUPS_CONFIG} --libs OUTPUT_VARIABLE CUPS_LIBRARIES OUTPUT_STRIP_TRAILING_WHITESPACE)
                                set(CUPS_FOUND True)
                        endif()
                endif()
        endif()
    endif()

    if(NOT CUPS_FOUND)
        message(WARNING "Could not find cups cflags and libs.")
    else()
        set(CUPS_PLUGIN_FILES src/collectors/cups.plugin/cups_plugin.c)
        add_executable(cups.plugin ${CUPS_PLUGIN_FILES})
        target_link_libraries (cups.plugin libnetdata ${CUPS_LIBRARIES})
        target_compile_options(cups.plugin PRIVATE ${CUPS_CFLAGS_OTHER})

        install(TARGETS cups.plugin
                COMPONENT plugin-cups
                DESTINATION ${PLUGINS_DEST})

        netdata_add_deb_copyright(plugin-cups netdata-plugin-cups)
    endif()
endif()
