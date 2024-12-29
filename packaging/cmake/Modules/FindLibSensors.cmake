# FindLibSensors.cmake
# Locate libsensors library and headers

# Locate the libsensors header file
find_path(LibSensors_INCLUDE_DIRS
        NAMES sensors/sensors.h
        PATHS /usr/include /usr/local/include
)

# Locate the libsensors library
find_library(LibSensors_LIBRARIES
        NAMES sensors
        PATHS /usr/lib /usr/local/lib
)

# Check if both the library and header were found
include(FindPackageHandleStandardArgs)
find_package_handle_standard_args(LibSensors
        REQUIRED_VARS LibSensors_LIBRARIES LibSensors_INCLUDE_DIRS
)

# Mark the variables as advanced (not shown in the GUI by default)
mark_as_advanced(LibSensors_LIBRARIES LibSensors_INCLUDE_DIRS)
