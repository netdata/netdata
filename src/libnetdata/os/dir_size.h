// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DIR_SIZE_H
#define NETDATA_DIR_SIZE_H

#include "libnetdata/libnetdata.h"
#include "libnetdata/simple_pattern/simple_pattern.h"

typedef struct {
    uint64_t bytes;        // Total size in bytes
    uint64_t files;        // Number of files
    uint64_t directories;  // Number of directories (including root directory)
    uint64_t depth;        // Maximum depth found
    uint64_t errors;       // Number of errors encountered during calculation
} DIR_SIZE;

#define DIR_SIZE_EMPTY (DIR_SIZE){ 0 }
#define DIR_SIZE_OK(ds) ((ds).bytes > 0 && (ds).errors == 0)

/**
 * Calculate the total size of a directory and its contents
 *
 * @param path Path to the directory
 * @param pattern Simple pattern to filter files (NULL to include all files)
 * @param max_depth Maximum recursion depth (0 for unlimited)
 * @return DIR_SIZE structure with the size information
 *
 * This function recursively traverses the directory structure starting from
 * the given path and calculates the total size in bytes, counting files and
 * directories. It safely handles symbolic links to avoid infinite loops.
 */
DIR_SIZE dir_size(const char *path, SIMPLE_PATTERN *pattern, int max_depth);

/**
 * Calculate multiple directory sizes in one pass
 *
 * @param paths Array of directory paths to calculate
 * @param num_paths Number of paths in the array
 * @param pattern Simple pattern to filter files (NULL to include all files)
 * @param max_depth Maximum recursion depth (0 for unlimited)
 * @return DIR_SIZE structure with the combined size information
 *
 * This function calculates sizes for multiple directories and returns their combined total.
 */
DIR_SIZE dir_size_multiple(const char **paths, int num_paths, SIMPLE_PATTERN *pattern, int max_depth);

#endif //NETDATA_DIR_SIZE_H