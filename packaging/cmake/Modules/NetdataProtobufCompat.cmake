# Handles normalizing protobuf variables for Netdata builds.
#
# This is needed because of the absolute insanity that is trying to
# cleanly handle Protobuf in CMake.

macro(set_netdata_protobuf_vars)
        if(TARGET protobuf::libprotobuf)
                if(NOT Protobuf_PROTOC_EXECUTABLE AND TARGET protobuf::protoc)
                        get_target_property(Protobuf_PROTOC_EXECUTABLE protobuf::protoc
                                IMPORTED_LOCATION_RELEASE)
                        if(NOT EXISTS "${Protobuf_PROTOC_EXECUTABLE}")
                                get_target_property(Protobuf_PROTOC_EXECUTABLE protobuf::protoc
                                        IMPORTED_LOCATION_RELWITHDEBINFO)
                        endif()
                        if(NOT EXISTS "${Protobuf_PROTOC_EXECUTABLE}")
                                get_target_property(Protobuf_PROTOC_EXECUTABLE protobuf::protoc
                                        IMPORTED_LOCATION_MINSIZEREL)
                        endif()
                        if(NOT EXISTS "${Protobuf_PROTOC_EXECUTABLE}")
                                get_target_property(Protobuf_PROTOC_EXECUTABLE protobuf::protoc
                                        IMPORTED_LOCATION_DEBUG)
                        endif()
                        if(NOT EXISTS "${Protobuf_PROTOC_EXECUTABLE}")
                                get_target_property(Protobuf_PROTOC_EXECUTABLE protobuf::protoc
                                        IMPORTED_LOCATION_NOCONFIG)
                        endif()
                        if(NOT Protobuf_PROTOC_EXECUTABLE)
                                set(Protobuf_PROTOC_EXECUTABLE protobuf::protoc)
                        endif()
                endif()

                # It is technically possible that this may still not
                # be set by this point, so we need to check it and
                # fail noisily if it isn't because the build won't
                # work without it.
                if(NOT Protobuf_PROTOC_EXECUTABLE)
                        message(FATAL_ERROR "Could not determine the location of the protobuf compiler for the detected version of protobuf.")
                endif()

                set(NETDATA_PROTOBUF_PROTOC_EXECUTABLE ${Protobuf_PROTOC_EXECUTABLE})
                set(NETDATA_PROTOBUF_LIBS protobuf::libprotobuf)
                get_target_property(NETDATA_PROTOBUF_CFLAGS_OTHER
                                        protobuf::libprotobuf
                                        INTERFACE_COMPILE_DEFINITIONS)
                get_target_property(NETDATA_PROTOBUF_INCLUDE_DIRS
                                        protobuf::libprotobuf
                                        INTERFACE_INCLUDE_DIRECTORIES)
        else()
                set(NETDATA_PROTOBUF_PROTOC_EXECUTABLE ${PROTOBUF_PROTOC_EXECUTABLE})
                set(NETDATA_PROTOBUF_CFLAGS_OTHER ${PROTOBUF_CFLAGS_OTHER})
                set(NETDATA_PROTOBUF_INCLUDE_DIRS ${PROTOBUF_INCLUDE_DIRS})
                set(NETDATA_PROTOBUF_LIBS ${PROTOBUF_LIBRARIES})
        endif()
endmacro()
