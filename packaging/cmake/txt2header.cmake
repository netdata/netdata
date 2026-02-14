# SPDX-License-Identifier: GPL-3.0
#
# CMake script to generate a C header file from a text file.
#
# The source (SRC), destination (DEST), and define name (NAME) must be
# specified using `-D` options.

# Get CMake to shut up about empty list elements (we want the new behavior anyway).
cmake_policy(SET CMP0007 NEW)

# Read the source file as a list of lines.
file(STRINGS ${SRC} lines)

# Escape everything that needs to be escaped in C strings.
# The replacement arguments below use bracket arguments instead of
# quoted arguments, because some varieties of CMake seem to choke on \"
# in a single-line quoted argument.
list(TRANSFORM lines REPLACE "\\\\" [[\\\\]])
list(TRANSFORM lines REPLACE "\\\"" [[\\"]])
list(TRANSFORM lines REPLACE "'" [[\\']])
list(TRANSFORM lines REPLACE "\\\n" [[\\n]])
list(TRANSFORM lines REPLACE "\\\r" [[\\r]])
list(TRANSFORM lines REPLACE "\\\t" [[\\t]])

# Prepare each line to be a string in a multi-line macro.
list(TRANSFORM lines PREPEND "\"")
list(TRANSFORM lines APPEND "\\n\" \\\n")

# Add the required header lines
list(INSERT lines 0
    "// DO NOT EDIT DIRECTLY\n"
    "// Generated at build time by CMake from ${SRC}\n"
    "\n"
    "#define ${NAME} \\\n"
)
list(APPEND lines "\n")

# Join the lines into one string.
list(JOIN lines "" content)

# Write the output file
file(WRITE ${DEST} "${content}")
