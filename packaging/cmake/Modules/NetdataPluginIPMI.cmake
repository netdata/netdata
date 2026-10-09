# SPDX-License-Identifier: GPL-3.0-or-later
# ipmi.plugin: the experimental Go IPMI plugin and its config tree.
#
# include()d from the root file, so paths resolve against the repository and
# build roots; nothing here may use CMAKE_CURRENT_LIST_DIR.
#
# The option's guard in NetdataOptions.cmake limits the plugin to Linux on
# 64-bit x86 or ARM, so the binary name carries no Windows .exe variant.

include_guard()

if(ENABLE_PLUGIN_IPMI)
    add_go_target(ipmi-go-plugin ipmi.plugin src/go cmd/ipmiplugin)

    install(PROGRAMS ${CMAKE_BINARY_DIR}/ipmi.plugin
            COMPONENT plugin-ipmi
            DESTINATION ${PLUGINS_DEST})

    install(FILES src/go/plugin/ipmi/config/ipmi.conf
            COMPONENT plugin-ipmi
            DESTINATION ${LIBCONFIG_DEST})

    install(DIRECTORY src/go/plugin/ipmi/config/ipmi
            COMPONENT plugin-ipmi
            DESTINATION ${LIBCONFIG_DEST}
            FILES_MATCHING PATTERN "*.conf")
endif()
