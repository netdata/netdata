# SPDX-License-Identifier: GPL-3.0-or-later
# The netdata daemon: source aggregation, the mqtt library, the ACLK protobuf schemas, the exporters, and the netdata target itself.
#
# Relocated verbatim from the root CMakeLists.txt. include()d rather than
# add_subdirectory()d so CMAKE_CURRENT_SOURCE_DIR and CMAKE_CURRENT_BINARY_DIR
# keep pointing at the repository and build roots, which every relative path
# below depends on. Nothing here may use CMAKE_CURRENT_LIST_DIR.

include_guard()

# source file inventory for everything defined in this file; every operation on
# those lists - the conditional list(APPEND ...) calls and the NETDATA_FILES
# aggregation - stays here. A component that owns its own build definition
# keeps its inventory beside it instead: a plugin in its
# NetdataPlugin*.cmake module, libnetdata in src/libnetdata/SourceLists.cmake.
include(NetdataSourceLists)

if(ENABLE_SYSTEMD_DBUS)
        list(APPEND DAEMON_FILES ${DAEMON_SYSTEMD_WATCHER_FILES})
endif()

if(ENABLE_ML)
        set(ML_FILES
                ${ML_ENABLED_FILES}
        )

        list(APPEND ML_FILES ${ML_MEMORY_FILES})
        set_source_files_properties(${ML_FILES} PROPERTIES COMPILE_OPTIONS "-Wno-attributes")
else()
        set(ML_FILES
                ${ML_DISABLED_FILES}
        )
endif()

if(ENABLE_DBENGINE)
    list(APPEND RRD_PLUGIN_FILES
            ${RRD_DBENGINE_FILES}
    )
endif()

set(NETDATA_FILES
        ${COLLECTORS_ALL_FILES}
        ${DAEMON_FILES}
        ${API_PLUGIN_FILES}
        ${EXPORTING_ENGINE_FILES}
        ${HEALTH_PLUGIN_FILES}
        ${IDLEJITTER_PLUGIN_FILES}
        ${ML_FILES}
        ${PLUGINSD_PLUGIN_FILES}
        ${RRD_PLUGIN_FILES}
        ${REGISTRY_PLUGIN_FILES}
        ${STATSD_PLUGIN_FILES}
        ${STREAMING_PLUGIN_FILES}
        ${WEB_PLUGIN_FILES}
        ${CLAIM_PLUGIN_FILES}
        ${ACLK_ALWAYS_BUILD}
        ${PROFILE_PLUGIN_FILES}
)

if(OS_LINUX)
        list(APPEND NETDATA_FILES
                ${DAEMON_LINUX_FILES}
                ${CGROUPS_PLUGIN_FILES}
                ${DISKSPACE_PLUGIN_FILES}
                ${PROC_PLUGIN_FILES}
                ${TC_PLUGIN_FILES}
                ${TIMEX_PLUGIN_FILES}
                ${INTERNAL_COLLECTORS_FILES}
        )

        if(ENABLE_SENTRY)
            list(APPEND NETDATA_FILES
                    ${DAEMON_SENTRY_FILES})
        endif()
elseif(OS_MACOS)
        list(APPEND NETDATA_FILES
                ${DAEMON_MACOS_FILES}
                ${MACOS_PLUGIN_FILES}
                ${TIMEX_PLUGIN_FILES}
                ${INTERNAL_COLLECTORS_FILES}
        )
elseif(OS_FREEBSD)
        list(APPEND NETDATA_FILES
                ${DAEMON_FREEBSD_FILES}
                ${FREEBSD_PLUGIN_FILES}
                ${TIMEX_PLUGIN_FILES}
                ${INTERNAL_COLLECTORS_FILES}
        )
elseif(OS_WINDOWS)
        list(APPEND NETDATA_FILES
                ${DAEMON_WINDOWS_FILES}
                ${WINDOWS_PLUGIN_FILES}
                ${INTERNAL_COLLECTORS_FILES}
        )
endif()

set_source_files_properties(src/aclk/schema-wrappers/proto_2_json.cc PROPERTIES COMPILE_OPTIONS "-Wno-unused-result")

#
# mqtt library
#
set(ENABLE_MQTTWEBSOCKETS True)
if(ENABLE_MQTTWEBSOCKETS)
        add_library(mqttwebsockets STATIC ${MQTT_WEBSOCKETS_FILES})

        target_compile_options(mqttwebsockets PUBLIC -DMQTT_WSS_CUSTOM_ALLOC
                                                     -DRBUF_CUSTOM_MALLOC
                                                     -DMQTT_WSS_CPUSTATS)

        target_include_directories(mqttwebsockets PUBLIC ${CMAKE_SOURCE_DIR}/aclk/helpers)

        target_link_libraries(mqttwebsockets PRIVATE libnetdata)

endif()

#
# proto definitions
#
# aclk-schemas moved to Buf v2 (PR netdata/aclk-schemas#57): the module root is
# now "proto/" and internal imports are relative to it (e.g. "aclk/v1/lib.proto"),
# so protoc's include root must be the proto/ directory. Generated headers are
# therefore included without the "proto/" prefix (e.g. "aclk/v1/lib.pb.h").
netdata_protoc_generate_cpp("${CMAKE_SOURCE_DIR}/src/aclk/aclk-schemas/proto"
                            "${CMAKE_BINARY_DIR}/src/aclk/aclk-schemas"
                            ACLK_PROTO_BUILT_SRCS
                            ACLK_PROTO_BUILT_HDRS
                            ${ACLK_PROTO_DEFS})

list(APPEND ACLK_FILES ${ACLK_PROTO_BUILT_SRCS}
                       ${ACLK_PROTO_BUILT_HDRS})

#
# exporters
#

if(ENABLE_EXPORTER_MONGODB)
        if(MONGOC_FOUND)
                SET(HAVE_MONGOC True)
        else()
                SET(ENABLE_EXPORTER_MONGODB False)
        endif()
endif()

if(ENABLE_EXPORTER_PROMETHEUS_REMOTE_WRITE)
        if (NOT SNAPPY_FOUND)
                include(CheckLibraryExists)
                check_library_exists(snappy snappy_compress "" HAVE_SNAPPY_LIB)

                if(HAVE_SNAPPY_LIB)
                        set(SNAPPY_INCLUDE_DIRS "")
                        set(SNAPPY_CFLAGS_OTHER "")
                        set(SNAPPY_LIBRARIES "-lsnappy")
                else()
                        message(FATAL_ERROR "Could not find snappy libraries with pkg-config or internal cmake checks.")
                endif()
        endif()

        netdata_protoc_generate_cpp("${CMAKE_SOURCE_DIR}/src/exporting/prometheus/remote_write"
                                    "${CMAKE_BINARY_DIR}/src/exporting/prometheus/remote_write"
                                    PROMETHEUS_REMOTE_WRITE_BUILT_SRCS
                                    PROMETHEUS_REMOTE_WRITE_BUILT_HDRS
                                    "src/exporting/prometheus/remote_write/remote_write.proto")

        list(APPEND PROMETHEUS_REMOTE_WRITE_EXPORTING_FILES
                    ${PROMETHEUS_REMOTE_WRITE_BUILT_SRCS}
                    ${PROMETHEUS_REMOTE_WRITE_BUILT_HDRS})

        set(ENABLE_PROMETHEUS_REMOTE_WRITE True)
endif()

#
# build netdata (only Linux ATM)
#

# The manifest is referenced by netdata.rc at compile time; the configured copy
# lands in the source tree because that is where the .rc resolves it.
if(OS_WINDOWS)
        configure_file(packaging/windows/resources/netdata.manifest.in ${CMAKE_SOURCE_DIR}/packaging/windows/resources/netdata.manifest @ONLY)
endif()

add_executable(netdata
        ${NETDATA_FILES}
        "${ACLK_FILES}"
        "$<$<BOOL:${ENABLE_EXPORTER_MONGODB}>:${MONGODB_EXPORTING_FILES}>"
        "$<$<BOOL:${ENABLE_EXPORTER_PROMETHEUS_REMOTE_WRITE}>:${PROMETHEUS_REMOTE_WRITE_EXPORTING_FILES}>"
        "$<$<BOOL:${OS_WINDOWS}>:${NETDATA_RES_FILES}>"
)

if(ENABLE_ML)
  netdata_add_dlib_to_target(netdata)
endif()

target_compile_options(netdata PRIVATE
        "$<$<BOOL:${ENABLE_EXPORTER_MONGODB}>:${MONGOC_CFLAGS_OTHER}>"
        "$<$<BOOL:${ENABLE_EXPORTER_PROMETHEUS_REMOTE_WRITE}>:${SNAPPY_CFLAGS_OTHER}>"
)

target_include_directories(netdata PRIVATE
        "${CMAKE_BINARY_DIR}/src/aclk/aclk-schemas"
        "$<$<BOOL:${ENABLE_EXPORTER_MONGODB}>:${MONGOC_INCLUDE_DIRS}>"
        "$<$<BOOL:${ENABLE_EXPORTER_PROMETHEUS_REMOTE_WRITE}>:${SNAPPY_INCLUDE_DIRS}>"
)

target_link_libraries(netdata PRIVATE
        m
        libnetdata
        "$<$<BOOL:${HAVE_LIBRT}>:rt>"
        "$<$<BOOL:${ENABLE_MQTTWEBSOCKETS}>:mqttwebsockets>"
        "$<$<BOOL:${ENABLE_EXPORTER_MONGODB}>:${MONGOC_LIBRARIES}>"
        "$<$<BOOL:${ENABLE_EXPORTER_PROMETHEUS_REMOTE_WRITE}>:${SNAPPY_LIBRARIES}>"
        "$<$<BOOL:${OS_MACOS}>:${IOKIT};${FOUNDATION}>"
        "$<$<BOOL:${ENABLE_SENTRY}>:sentry>"
        "$<$<BOOL:${ENABLE_WEBRTC}>:LibDataChannel::LibDataChannelStatic>"
        PkgConfig::CURL
        "$<$<BOOL:${OS_WINDOWS}>:odbc32;setupapi>"
)

netdata_add_protobuf(netdata)
