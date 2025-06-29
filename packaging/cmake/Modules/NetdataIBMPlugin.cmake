# SPDX-License-Identifier: GPL-3.0-or-later
# Build configuration for ibm.d.plugin which requires CGO

# add_ibm_plugin_target: Add target for building ibm.d.plugin with CGO
#
# This plugin requires CGO and IBM DB2/MQ headers for compilation.
# The plugin will download IBM libraries on first run.
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
    set(IBM_CLIDRIVER_URL "https://public.dhe.ibm.com/ibmdl/export/pub/software/data/db2/drivers/odbc_cli/v12.1.0/linuxx64_odbc_cli.tar.gz")
    
    # Download IBM MQ client libraries to build directory
    set(IBM_MQ_DIR "${CMAKE_BINARY_DIR}/ibm-mqclient")
    set(IBM_MQ_ARCHIVE "${CMAKE_BINARY_DIR}/IBM-MQC-Redist-LinuxX64.tar.gz")
    set(IBM_MQ_URL "https://public.dhe.ibm.com/ibmdl/export/pub/software/websphere/messaging/mqdev/redist/9.4.1.0-IBM-MQC-Redist-LinuxX64.tar.gz")
    
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
    
    # Download IBM MQ client libraries if not present
    if(NOT EXISTS "${IBM_MQ_DIR}/inc/cmqc.h")
        message(STATUS "Downloading IBM MQ client libraries for ibm.d.plugin build")
        
        if(CURL_EXECUTABLE)
            execute_process(
                COMMAND ${CURL_EXECUTABLE} -sL -o "${IBM_MQ_ARCHIVE}" "${IBM_MQ_URL}"
                WORKING_DIRECTORY "${CMAKE_BINARY_DIR}"
                RESULT_VARIABLE MQ_DOWNLOAD_RESULT
            )
        else()
            execute_process(
                COMMAND ${WGET_EXECUTABLE} -q -O "${IBM_MQ_ARCHIVE}" "${IBM_MQ_URL}"
                WORKING_DIRECTORY "${CMAKE_BINARY_DIR}"
                RESULT_VARIABLE MQ_DOWNLOAD_RESULT
            )
        endif()
        
        if(NOT MQ_DOWNLOAD_RESULT EQUAL 0)
            message(WARNING "Failed to download IBM MQ client libraries - ibm_mq collector will be disabled")
            # Don't return here, we can still build with just DB2 support
        else()
            # Extract MQ client
            file(MAKE_DIRECTORY "${IBM_MQ_DIR}")
            execute_process(
                COMMAND ${CMAKE_COMMAND} -E tar xzf "${IBM_MQ_ARCHIVE}"
                WORKING_DIRECTORY "${IBM_MQ_DIR}"
                RESULT_VARIABLE MQ_EXTRACT_RESULT
            )
            
            if(NOT MQ_EXTRACT_RESULT EQUAL 0)
                message(WARNING "Failed to extract IBM MQ client libraries - ibm_mq collector will be disabled")
            endif()
            
            # Remove archive
            file(REMOVE "${IBM_MQ_ARCHIVE}")
        endif()
    endif()
    
    # Find all Go source files
    file(GLOB_RECURSE IBM_PLUGIN_DEPS CONFIGURE_DEPENDS "${CMAKE_SOURCE_DIR}/src/go/*.go")
    list(APPEND IBM_PLUGIN_DEPS
        "${CMAKE_SOURCE_DIR}/src/go/go.mod"
        "${CMAKE_SOURCE_DIR}/src/go/go.sum"
    )
    
    # Set up build paths based on what libraries are available
    set(IBM_CGO_CFLAGS "-I${IBM_CLIDRIVER_DIR}/include")
    set(IBM_CGO_LDFLAGS "-L${IBM_CLIDRIVER_DIR}/lib")
    set(IBM_RPATH_FLAGS "-Wl,-rpath,\$ORIGIN/../../../lib/netdata/ibm-clidriver/lib -Wl,-rpath,${NETDATA_RUNTIME_PREFIX}/usr/lib/netdata/ibm-clidriver/lib -Wl,-rpath,/usr/lib/netdata/ibm-clidriver/lib -Wl,-rpath,/opt/netdata/lib/netdata/ibm-clidriver/lib")
    
    # Add MQ paths if available
    if(EXISTS "${IBM_MQ_DIR}/inc/cmqc.h")
        set(IBM_CGO_CFLAGS "${IBM_CGO_CFLAGS} -I${IBM_MQ_DIR}/inc")
        set(IBM_CGO_LDFLAGS "${IBM_CGO_LDFLAGS} -L${IBM_MQ_DIR}/lib64 -lmqm")
        set(IBM_RPATH_FLAGS "${IBM_RPATH_FLAGS} -Wl,-rpath,\$ORIGIN/../../../lib/netdata/ibm-mqclient/lib64 -Wl,-rpath,${NETDATA_RUNTIME_PREFIX}/usr/lib/netdata/ibm-mqclient/lib64 -Wl,-rpath,/usr/lib/netdata/ibm-mqclient/lib64 -Wl,-rpath,/opt/netdata/lib/netdata/ibm-mqclient/lib64")
        set(MQ_INSTALLATION_PATH "${IBM_MQ_DIR}")
    else()
        # If MQ libraries aren't downloaded, try standard paths
        set(MQ_INSTALLATION_PATH "/opt/mqm")
    endif()
    
    # Build with CGO enabled and multiple rpath entries for different installation methods
    add_custom_command(
        OUTPUT ibm.d.plugin
        COMMAND "${CMAKE_COMMAND}" -E env 
            GOROOT=${GO_ROOT} 
            CGO_ENABLED=1 
            CGO_CFLAGS=${IBM_CGO_CFLAGS}
            CGO_LDFLAGS=${IBM_CGO_LDFLAGS}
            IBM_DB_HOME=${IBM_CLIDRIVER_DIR}
            LD_LIBRARY_PATH=${IBM_CLIDRIVER_DIR}/lib:${IBM_MQ_DIR}/lib64:$LD_LIBRARY_PATH
            MQ_INSTALLATION_PATH=${MQ_INSTALLATION_PATH}
            GOPROXY=https://proxy.golang.org,direct 
            "${GO_EXECUTABLE}" build -buildvcs=false -ldflags "${GO_LDFLAGS} -extldflags '${IBM_RPATH_FLAGS}'" -o "${CMAKE_BINARY_DIR}/ibm.d.plugin" "./cmd/ibmdplugin"
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