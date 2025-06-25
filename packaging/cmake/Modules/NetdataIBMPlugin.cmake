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
    
    # Download IBM DB2 client libraries to build directory
    set(IBM_CLIDRIVER_DIR "${CMAKE_BINARY_DIR}/ibm-clidriver")
    set(IBM_CLIDRIVER_ARCHIVE "${CMAKE_BINARY_DIR}/linuxx64_odbc_cli.tar.gz")
    set(IBM_CLIDRIVER_URL "https://public.dhe.ibm.com/ibmdl/export/pub/software/data/db2/drivers/odbc_cli/v11.5.9/linuxx64_odbc_cli.tar.gz")
    
    if(NOT EXISTS "${IBM_CLIDRIVER_DIR}/include/sqlcli.h")
        message(STATUS "Downloading IBM DB2 client libraries for ibm.d.plugin build")
        file(MAKE_DIRECTORY "${CMAKE_BINARY_DIR}")
        
        if(CURL_EXECUTABLE)
            execute_process(
                COMMAND ${CURL_EXECUTABLE} -sL -o "${IBM_CLIDRIVER_ARCHIVE}" "${IBM_CLIDRIVER_URL}"
                WORKING_DIRECTORY "${CMAKE_BINARY_DIR}"
                RESULT_VARIABLE DOWNLOAD_RESULT
            )
        else()
            execute_process(
                COMMAND ${WGET_EXECUTABLE} -q -O "${IBM_CLIDRIVER_ARCHIVE}" "${IBM_CLIDRIVER_URL}"
                WORKING_DIRECTORY "${CMAKE_BINARY_DIR}"
                RESULT_VARIABLE DOWNLOAD_RESULT
            )
        endif()
        
        if(NOT DOWNLOAD_RESULT EQUAL 0)
            message(WARNING "Failed to download IBM DB2 client libraries - cannot build ibm.d.plugin")
            return()
        endif()
        
        # Extract full client driver
        execute_process(
            COMMAND ${CMAKE_COMMAND} -E tar xzf "${IBM_CLIDRIVER_ARCHIVE}"
            WORKING_DIRECTORY "${CMAKE_BINARY_DIR}"
            RESULT_VARIABLE EXTRACT_RESULT
        )
        
        if(NOT EXTRACT_RESULT EQUAL 0)
            message(WARNING "Failed to extract IBM DB2 client libraries - cannot build ibm.d.plugin")
            return()
        endif()
        
        # Move clidriver contents to ibm-clidriver directory
        execute_process(
            COMMAND ${CMAKE_COMMAND} -E rename "${CMAKE_BINARY_DIR}/clidriver" "${IBM_CLIDRIVER_DIR}"
            RESULT_VARIABLE RENAME_RESULT
        )
        
        if(NOT RENAME_RESULT EQUAL 0)
            message(WARNING "Failed to organize IBM DB2 client libraries - cannot build ibm.d.plugin")
            return()
        endif()
        
        # Remove archive
        file(REMOVE "${IBM_CLIDRIVER_ARCHIVE}")
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
            CGO_CFLAGS="-I${IBM_CLIDRIVER_DIR}/include"
            CGO_LDFLAGS="-L${IBM_CLIDRIVER_DIR}/lib -Wl,-rpath,'$ORIGIN/../../../lib/netdata/ibm-clidriver/lib' -Wl,-rpath,/opt/netdata/lib/netdata/ibm-clidriver/lib -Wl,-rpath,/usr/lib/netdata/ibm-clidriver/lib"
            IBM_DB_HOME="${IBM_CLIDRIVER_DIR}"
            LD_LIBRARY_PATH="${IBM_CLIDRIVER_DIR}/lib:$LD_LIBRARY_PATH"
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