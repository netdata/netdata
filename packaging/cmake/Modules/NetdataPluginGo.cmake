# SPDX-License-Identifier: GPL-3.0-or-later
# go.d.plugin, the snmp trap profile generator, and the nd-mcp bridge: every Go binary gated on ENABLE_PLUGIN_GO.
#
# Relocated verbatim from the root CMakeLists.txt. include()d rather than
# add_subdirectory()d so CMAKE_CURRENT_SOURCE_DIR and CMAKE_CURRENT_BINARY_DIR
# keep pointing at the repository and build roots, which every relative path
# below depends on. Nothing here may use CMAKE_CURRENT_LIST_DIR.

if(ENABLE_PLUGIN_GO)
    if (OS_WINDOWS)
        set(GO_PLUGIN_BIN go.d.plugin.exe)
    else()
        set(GO_PLUGIN_BIN go.d.plugin)
    endif()

    add_go_target(go-plugin ${GO_PLUGIN_BIN} src/go cmd/godplugin)

    install(PROGRAMS ${CMAKE_BINARY_DIR}/${GO_PLUGIN_BIN}
            COMPONENT plugin-go
            DESTINATION ${PLUGINS_DEST})

    if (OS_WINDOWS)
        set(SNMP_TRAP_PROFILE_GEN_BIN snmp-trap-profile-gen.exe)
    else()
        set(SNMP_TRAP_PROFILE_GEN_BIN snmp-trap-profile-gen)
    endif()

    add_go_target(snmp_trap_profile_gen ${SNMP_TRAP_PROFILE_GEN_BIN} src/go cmd/snmptrapprofilegen)

    install(PROGRAMS ${CMAKE_BINARY_DIR}/${SNMP_TRAP_PROFILE_GEN_BIN}
            COMPONENT plugin-go
            DESTINATION ${PLUGINS_DEST})

    # Build and install nd-mcp (stdio-golang bridge) exactly like go.d.plugin
    if(ENABLE_ND_MCP)
        if (OS_WINDOWS)
            set(ND_MCP_NAME nd-mcp.exe)
        else()
            set(ND_MCP_NAME nd-mcp)
        endif()

        add_go_target(nd-mcp-target ${ND_MCP_NAME} src/web/mcp/bridges/stdio-golang .)
        install(PROGRAMS
                ${CMAKE_BINARY_DIR}/${ND_MCP_NAME}
                COMPONENT ${NETDATA_ND_MCP_COMPONENT}
                DESTINATION "${BINDIR}")
    endif()
endif()
