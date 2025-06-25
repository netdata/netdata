# SPDX-License-Identifier: GPL-3.0-or-later
# Build configuration for ibm.d.plugin which requires CGO

# add_ibm_plugin_target: Add target for building ibm.d.plugin with CGO
#
# This plugin requires CGO and IBM DB2 headers for compilation.
# The plugin will download IBM DB2 libraries on first run.
macro(add_ibm_plugin_target)
    # Check if we can download IBM headers for build
    find_program(CURL_EXECUTABLE curl)
    find_program(WGET_EXECUTABLE wget)
    
    if(NOT CURL_EXECUTABLE AND NOT WGET_EXECUTABLE)
        message(WARNING "Neither curl nor wget found - cannot build ibm.d.plugin")
        return()
    endif()
    
    # Download IBM DB2 headers
    set(IBM_HEADERS_DIR "${CMAKE_BINARY_DIR}/ibm-headers")
    set(IBM_HEADERS_ARCHIVE "${IBM_HEADERS_DIR}/linuxx64_odbc_cli.tar.gz")
    set(IBM_HEADERS_URL "https://public.dhe.ibm.com/ibmdl/export/pub/software/data/db2/drivers/odbc_cli/v11.5.9/linuxx64_odbc_cli.tar.gz")
    
    if(NOT EXISTS "${IBM_HEADERS_DIR}/clidriver/include/sqlcli.h")
        message(STATUS "Downloading IBM DB2 headers for ibm.d.plugin build")
        file(MAKE_DIRECTORY "${IBM_HEADERS_DIR}")
        
        if(CURL_EXECUTABLE)
            execute_process(
                COMMAND ${CURL_EXECUTABLE} -sL -o "${IBM_HEADERS_ARCHIVE}" "${IBM_HEADERS_URL}"
                WORKING_DIRECTORY "${IBM_HEADERS_DIR}"
                RESULT_VARIABLE DOWNLOAD_RESULT
            )
        else()
            execute_process(
                COMMAND ${WGET_EXECUTABLE} -q -O "${IBM_HEADERS_ARCHIVE}" "${IBM_HEADERS_URL}"
                WORKING_DIRECTORY "${IBM_HEADERS_DIR}"
                RESULT_VARIABLE DOWNLOAD_RESULT
            )
        endif()
        
        if(NOT DOWNLOAD_RESULT EQUAL 0)
            message(WARNING "Failed to download IBM DB2 headers - cannot build ibm.d.plugin")
            return()
        endif()
        
        # Extract only headers
        execute_process(
            COMMAND ${CMAKE_COMMAND} -E tar xzf "${IBM_HEADERS_ARCHIVE}" clidriver/include/
            WORKING_DIRECTORY "${IBM_HEADERS_DIR}"
            RESULT_VARIABLE EXTRACT_RESULT
        )
        
        if(NOT EXTRACT_RESULT EQUAL 0)
            message(WARNING "Failed to extract IBM DB2 headers - cannot build ibm.d.plugin")
            return()
        endif()
    endif()
    
    # Find all Go source files
    file(GLOB_RECURSE IBM_PLUGIN_DEPS CONFIGURE_DEPENDS "${CMAKE_SOURCE_DIR}/src/go/*.go")
    list(APPEND IBM_PLUGIN_DEPS
        "${CMAKE_SOURCE_DIR}/src/go/go.mod"
        "${CMAKE_SOURCE_DIR}/src/go/go.sum"
    )
    
    # Build with CGO enabled and multiple rpath entries for different installation methods
    # $ORIGIN/../../../lib/netdata/ibm-clidriver/lib - for relative to plugin location
    # /opt/netdata/lib/netdata/ibm-clidriver/lib - for static builds
    # /usr/lib/netdata/ibm-clidriver/lib - for system packages
    add_custom_command(
        OUTPUT ibm.d.plugin
        COMMAND "${CMAKE_COMMAND}" -E env 
            GOROOT=${GO_ROOT} 
            CGO_ENABLED=1 
            CGO_CFLAGS="-I${IBM_HEADERS_DIR}/clidriver/include"
            CGO_LDFLAGS="-L${IBM_HEADERS_DIR}/clidriver/lib -Wl,-rpath,'$ORIGIN/../../../lib/netdata/ibm-clidriver/lib' -Wl,-rpath,/opt/netdata/lib/netdata/ibm-clidriver/lib -Wl,-rpath,/usr/lib/netdata/ibm-clidriver/lib"
            IBM_DB_HOME="${IBM_HEADERS_DIR}/clidriver"
            GOPROXY=https://proxy.golang.org,direct 
            "${GO_EXECUTABLE}" build -buildvcs=false -ldflags "${GO_LDFLAGS}" -o "${CMAKE_BINARY_DIR}/ibm.d.plugin" "./cmd/ibmdplugin"
        DEPENDS ${IBM_PLUGIN_DEPS}
        COMMENT "Building ibm.d.plugin (with CGO)"
        WORKING_DIRECTORY "${CMAKE_SOURCE_DIR}/src/go"
        VERBATIM
    )
    
    add_custom_target(
        ibm-plugin ALL
        DEPENDS ibm.d.plugin
    )
endmacro()