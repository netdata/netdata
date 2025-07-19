# SPDX-License-Identifier: GPL-3.0-or-later

# Optional IBM.d plugin build (requires CGO)
if(ENABLE_PLUGIN_IBM_D)
    message(STATUS "Building ibm.d.plugin (requires IBM DB2 client libraries)")
    
    # Check for IBM DB2 environment variables
    if(NOT DEFINED ENV{IBM_DB_HOME})
        message(WARNING "IBM_DB_HOME not set - IBM.d plugin may fail to build")
    endif()
    
    set(IBM_D_PLUGIN_SOURCE_DIR ${CMAKE_CURRENT_SOURCE_DIR}/../..)
    set(IBM_D_PLUGIN_BINARY ibm.d.plugin)
    set(IBM_D_PLUGIN_FILES 
        ${CMAKE_CURRENT_SOURCE_DIR}/collector/as400/collector.go
        ${CMAKE_CURRENT_SOURCE_DIR}/collector/db2/collector.go)
    
    # Build with CGO enabled
    add_custom_command(
        OUTPUT ${IBM_D_PLUGIN_BINARY}
        COMMAND ${CMAKE_COMMAND} -E env 
            CGO_ENABLED=1
            GOPROXY=${GOPROXY}
            CGO_CFLAGS="-I$ENV{IBM_DB_HOME}/include"
            CGO_LDFLAGS="-L$ENV{IBM_DB_HOME}/lib"
            ${GO_EXECUTABLE} build
            -buildvcs=false
            -ldflags "${GO_LDFLAGS}"
            -o ${CMAKE_BINARY_DIR}/${IBM_D_PLUGIN_BINARY}
            ./cmd/ibmdplugin
        DEPENDS ${IBM_D_PLUGIN_FILES}
        COMMENT "Building ibm.d.plugin"
        WORKING_DIRECTORY ${IBM_D_PLUGIN_SOURCE_DIR}
    )
    
    add_custom_target(ibm.d.plugin ALL DEPENDS ${IBM_D_PLUGIN_BINARY})
    
    install(PROGRAMS ${CMAKE_BINARY_DIR}/${IBM_D_PLUGIN_BINARY}
            DESTINATION usr/libexec/netdata/plugins.d)
endif()