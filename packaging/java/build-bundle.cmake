# SPDX-License-Identifier: GPL-3.0-or-later
# Called only by the opt-in java-bundle build target. All tools and caches are
# private to the build directory; no host Java or Maven installation is used.
cmake_minimum_required(VERSION 3.18)
foreach(required SOURCE_DIR BUNDLE_DIR JAVA_ARCH)
    if(NOT DEFINED ${required} OR "${${required}}" STREQUAL "")
        message(FATAL_ERROR "Missing ${required}")
    endif()
endforeach()
if(JAVA_ARCH STREQUAL "x64")
    set(jdk_sha256 ce79869e1307ed8ee1e2baa86a412b1eb5b75d10a01006d788a6f968bcfaee94)
elseif(JAVA_ARCH STREQUAL "aarch64")
    set(jdk_sha256 23e37e026f12f3e706f18938ff611db3032d075b09d0879a25d06718c773e223)
else()
    message(FATAL_ERROR "Unsupported Java bundle architecture: ${JAVA_ARCH}")
endif()
file(MAKE_DIRECTORY "${BUNDLE_DIR}/downloads" "${BUNDLE_DIR}/tools" "${BUNDLE_DIR}/install/helper")

function(download_fixed url name algorithm digest)
    set(destination "${BUNDLE_DIR}/downloads/${name}")
    if(EXISTS "${destination}")
        file(${algorithm} "${destination}" actual)
        if(actual STREQUAL digest)
            return()
        endif()
    endif()
    # All URLs below are fixed public artifact URLs, without credentials.
    message("Java bundle: download ${url}")
    file(DOWNLOAD "${url}" "${destination}" EXPECTED_HASH "${algorithm}=${digest}"
         TLS_VERIFY ON STATUS result)
    list(GET result 0 status)
    if(NOT status EQUAL 0)
        message(FATAL_ERROR "Java bundle download failed in ${BUNDLE_DIR}: ${result}")
    endif()
endfunction()

set(jdk_name "OpenJDK21U-jdk_${JAVA_ARCH}_linux_hotspot_21.0.12.1_1.tar.gz")
download_fixed("https://github.com/adoptium/temurin21-binaries/releases/download/jdk-21.0.12.1%2B1/${jdk_name}"
               "${jdk_name}" SHA256 "${jdk_sha256}")
download_fixed("https://archive.apache.org/dist/maven/maven-3/3.9.11/binaries/apache-maven-3.9.11-bin.tar.gz"
               maven.tar.gz SHA512 bcfe4fe305c962ace56ac7b5fc7a08b87d5abd8b7e89027ab251069faebee516b0ded8961445d6d91ec1985dfe30f8153268843c89aa392733d1a3ec956c9978)
download_fixed("https://github.com/open-telemetry/opentelemetry-java-instrumentation/releases/download/v2.32.0/opentelemetry-javaagent.jar"
               otel.jar SHA256 f787eb6c7f3d18e69a431e108a15278d25ee37f83d68b678f621e063f3988f82)

file(ARCHIVE_EXTRACT INPUT "${BUNDLE_DIR}/downloads/${jdk_name}" DESTINATION "${BUNDLE_DIR}/tools")
file(ARCHIVE_EXTRACT INPUT "${BUNDLE_DIR}/downloads/maven.tar.gz" DESTINATION "${BUNDLE_DIR}/tools")
set(jdk "${BUNDLE_DIR}/tools/jdk-21.0.12.1+1")
set(maven "${BUNDLE_DIR}/tools/apache-maven-3.9.11")
message("Java bundle: compile NetdataAttach.java with private javac (--release 17 --add-modules jdk.attach)")
execute_process(COMMAND "${jdk}/bin/javac" --release 17 --add-modules jdk.attach
                -d "${BUNDLE_DIR}/install/helper" "${SOURCE_DIR}/src/collectors/java.plugin/NetdataAttach.java"
                WORKING_DIRECTORY "${BUNDLE_DIR}" RESULT_VARIABLE status)
if(NOT status EQUAL 0)
    message(FATAL_ERROR "Java helper compilation failed in ${BUNDLE_DIR}, status ${status}")
endif()
message("Java bundle: package extension using private Maven, strict checksums, private repository and output directory")
execute_process(COMMAND "${CMAKE_COMMAND}" -E env "JAVA_HOME=${jdk}" "MAVEN_SKIP_RC=1" "MAVEN_OPTS="
                "${maven}/bin/mvn" --batch-mode --strict-checksums --no-transfer-progress
                "-Dmaven.repo.local=${BUNDLE_DIR}/maven-repository"
                "-Dnetdata.build.directory=${BUNDLE_DIR}/extension-build"
                -f "${SOURCE_DIR}/src/collectors/java.plugin/extension/pom.xml" package
                WORKING_DIRECTORY "${BUNDLE_DIR}" RESULT_VARIABLE status)
if(NOT status EQUAL 0)
    message(FATAL_ERROR "Java extension build failed in ${BUNDLE_DIR}, status ${status}")
endif()
# Keep the complete runtime, including its legal/ tree and release metadata.
file(COPY "${jdk}/" DESTINATION "${BUNDLE_DIR}/install/runtime" USE_SOURCE_PERMISSIONS)
configure_file("${BUNDLE_DIR}/downloads/otel.jar" "${BUNDLE_DIR}/install/otel.jar" COPYONLY)
configure_file("${BUNDLE_DIR}/extension-build/hikari-extension-1.0.jar"
               "${BUNDLE_DIR}/install/hikari-extension.jar" COPYONLY)
file(WRITE "${BUNDLE_DIR}/install/VERSIONS" "Temurin JDK 21.0.12.1+1\nOpenTelemetry Java agent 2.32.0\nNetdata Hikari extension 1.0\n")
file(WRITE "${BUNDLE_DIR}/complete" "complete\n")
