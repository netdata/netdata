# SPDX-License-Identifier: GPL-3.0-or-later

# Optional AS/400-DB2 plugin build (requires CGO)
if(ENABLE_PLUGIN_AS400_DB2)
    message(STATUS "Building as400-db2.plugin (requires IBM DB2 client libraries)")
    
    # Check for IBM DB2 environment variables
    if(NOT DEFINED ENV{IBM_DB_HOME})
        message(WARNING "IBM_DB_HOME not set - AS/400-DB2 plugin may fail to build")
    endif()
    
    set(AS400_DB2_PLUGIN_SOURCE_DIR ${CMAKE_CURRENT_SOURCE_DIR}/../../../..)
    set(AS400_DB2_PLUGIN_BINARY as400-db2.plugin)
    
    # Build with CGO enabled
    add_custom_command(
        OUTPUT ${AS400_DB2_PLUGIN_BINARY}
        COMMAND ${CMAKE_COMMAND} -E env 
            CGO_ENABLED=1
            GOPROXY=${GOPROXY}
            CGO_CFLAGS="-I$ENV{IBM_DB_HOME}/include"
            CGO_LDFLAGS="-L$ENV{IBM_DB_HOME}/lib"
            ${GO_EXECUTABLE} build
            -buildvcs=false
            -ldflags "${GO_LDFLAGS}"
            -o ${CMAKE_BINARY_DIR}/${AS400_DB2_PLUGIN_BINARY}
            ./cmd/as400db2plugin
        DEPENDS ${AS400_DB2_PLUGIN_FILES}
        COMMENT "Building as400-db2.plugin"
        WORKING_DIRECTORY ${AS400_DB2_PLUGIN_SOURCE_DIR}
    )
    
    add_custom_target(as400-db2.plugin ALL DEPENDS ${AS400_DB2_PLUGIN_BINARY})
    
    install(PROGRAMS ${CMAKE_BINARY_DIR}/${AS400_DB2_PLUGIN_BINARY}
            DESTINATION usr/libexec/netdata/plugins.d)
endif()