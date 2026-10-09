# SPDX-License-Identifier: GPL-3.0-or-later
# Explicitly opt-in native Java monitoring and its private, fixed Java bundle.
include_guard()

if(ENABLE_PLUGIN_JAVA)
    add_go_target(java-plugin java.plugin src/go cmd/javaplugin)
    add_go_target(java-helper-go java-helper src/go cmd/javahelper)

    if(CPU_X86_64)
        set(JAVA_BUNDLE_ARCH x64)
    else()
        set(JAVA_BUNDLE_ARCH aarch64)
    endif()
    set(JAVA_BUNDLE_DIR "${CMAKE_BINARY_DIR}/java-bundle")
    file(GLOB_RECURSE JAVA_EXTENSION_SOURCES CONFIGURE_DEPENDS
         "${CMAKE_SOURCE_DIR}/src/collectors/java.plugin/extension/src/*")
    add_custom_command(
        OUTPUT "${JAVA_BUNDLE_DIR}/complete"
        COMMAND "${CMAKE_COMMAND}"
                "-DSOURCE_DIR=${CMAKE_SOURCE_DIR}"
                "-DBUNDLE_DIR=${JAVA_BUNDLE_DIR}"
                "-DJAVA_ARCH=${JAVA_BUNDLE_ARCH}"
                -P "${CMAKE_SOURCE_DIR}/packaging/java/build-bundle.cmake"
        DEPENDS packaging/java/build-bundle.cmake
                src/collectors/java.plugin/NetdataAttach.java
                src/collectors/java.plugin/extension/pom.xml
                ${JAVA_EXTENSION_SOURCES}
        COMMENT "Building the private Java monitoring bundle"
        VERBATIM)
    add_custom_target(java-bundle ALL DEPENDS "${JAVA_BUNDLE_DIR}/complete")
    add_dependencies(java-plugin java-helper-go java-bundle)

    install(PROGRAMS "${CMAKE_BINARY_DIR}/java.plugin" "${CMAKE_BINARY_DIR}/java-helper"
            COMPONENT plugin-java DESTINATION ${PLUGINS_DEST})
    install(FILES src/go/plugin/java/config/java.conf
            COMPONENT plugin-java DESTINATION ${LIBCONFIG_DEST})
    install(DIRECTORY src/go/plugin/java/config/java
            COMPONENT plugin-java DESTINATION ${LIBCONFIG_DEST}
            FILES_MATCHING PATTERN "*.conf")
    install(DIRECTORY "${JAVA_BUNDLE_DIR}/install/"
            COMPONENT plugin-java DESTINATION ${STOCK_DATA_DEST}/java
            FILE_PERMISSIONS OWNER_READ OWNER_WRITE GROUP_READ WORLD_READ
            DIRECTORY_PERMISSIONS OWNER_READ OWNER_WRITE OWNER_EXECUTE GROUP_READ GROUP_EXECUTE WORLD_READ WORLD_EXECUTE
            PATTERN "runtime" EXCLUDE)
    # Preserve the pinned JDK executable modes, not the build user's umask for
    # generated bundle files. The privileged helper rejects writable artifacts.
    install(DIRECTORY "${JAVA_BUNDLE_DIR}/install/runtime"
            COMPONENT plugin-java DESTINATION ${STOCK_DATA_DEST}/java
            USE_SOURCE_PERMISSIONS
            DIRECTORY_PERMISSIONS OWNER_READ OWNER_WRITE OWNER_EXECUTE GROUP_READ GROUP_EXECUTE WORLD_READ WORLD_EXECUTE)
endif()
