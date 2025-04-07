// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "dir_size.h"

#include <dirent.h>
#include <sys/stat.h>
#include <unistd.h>
#include <string.h>
#include <errno.h>

// Hash table to keep track of visited inodes to avoid cycles
typedef struct {
    ino_t inode;    // Inode number
    dev_t device;   // Device ID
} INODE_DEVICE_PAIR;

// Internal function to recursively calculate directory size
static void calc_dir_size_recursive(const char *base_path, const char *rel_path, 
                                   SIMPLE_PATTERN *pattern, size_t max_depth, size_t current_depth,
                                   DIR_SIZE *result, DICTIONARY *visited_inodes) {
    
    char path[FILENAME_MAX + 1];
    struct stat statbuf;
    struct dirent *entry;
    DIR *dir;

    // Check max depth
    if (max_depth > 0 && current_depth > max_depth)
        return;

    // Update max depth found
    if (current_depth > result->depth)
        result->depth = current_depth;

    // Construct full path (avoid double slashes)
    if (rel_path && *rel_path) {
        if (base_path[strlen(base_path) - 1] == '/')
            snprintfz(path, FILENAME_MAX, "%s%s", base_path, rel_path);
        else
            snprintfz(path, FILENAME_MAX, "%s/%s", base_path, rel_path);
    } else {
        snprintfz(path, FILENAME_MAX, "%s", base_path);
    }

    // Get file/directory stats
    if (lstat(path, &statbuf) != 0) {
        result->errors++;
        return;
    }

    // Create inode-device pair to detect loops
    INODE_DEVICE_PAIR id_pair = {
        .inode = statbuf.st_ino,
        .device = statbuf.st_dev
    };

    // Use string representation as the dictionary name
    char name[sizeof(INODE_DEVICE_PAIR) * 2 + 1];
    snprintfz(name, sizeof(name), "%lu_%lu", (unsigned long)id_pair.inode, (unsigned long)id_pair.device);

    // Check if we've seen this inode-device pair before (for symlink loop detection)
    if (dictionary_get(visited_inodes, name))
        return;
    
    // Add to visited inodes
    dictionary_set(visited_inodes, name, NULL, sizeof(void *));

    // Handle different file types
    if (S_ISDIR(statbuf.st_mode)) {
        result->directories++;
        
        // Open directory
        dir = opendir(path);
        if (!dir) {
            result->errors++;
            return;
        }

        // Iterate through directory entries
        while ((entry = readdir(dir)) != NULL) {
            // Skip "." and ".."
            if (strcmp(entry->d_name, ".") == 0 || strcmp(entry->d_name, "..") == 0)
                continue;

            // Build relative path (this is the path relative to base_path)
            char next_rel_path[FILENAME_MAX + 1];
            if (rel_path[0] == '\0')
                snprintfz(next_rel_path, FILENAME_MAX, "%s", entry->d_name);
            else
                snprintfz(next_rel_path, FILENAME_MAX, "%s/%s", rel_path, entry->d_name);

            // Build full path to check file type (avoid double slashes)
            char full_path[FILENAME_MAX + 1];
            if (path[strlen(path) - 1] == '/')
                snprintfz(full_path, FILENAME_MAX, "%s%s", path, entry->d_name);
            else
                snprintfz(full_path, FILENAME_MAX, "%s/%s", path, entry->d_name);
            
            struct stat entry_stat;
            if (lstat(full_path, &entry_stat) != 0) {
                result->errors++;
                continue;
            }
            
            if (S_ISDIR(entry_stat.st_mode)) {
                // Always recurse on directories regardless of pattern
                calc_dir_size_recursive(base_path, next_rel_path, pattern, max_depth, 
                                       current_depth + 1, result, visited_inodes);
            } 
            else if (S_ISREG(entry_stat.st_mode)) {
                // For files, apply pattern filtering if specified
                if (pattern && !simple_pattern_matches(pattern, next_rel_path))
                    continue;
                    
                // Count the file
                result->files++;
                result->bytes += entry_stat.st_size;
            }
            // Other file types (symlinks, etc.) are not counted
        }

        closedir(dir);
    } 
    else if (S_ISREG(statbuf.st_mode)) {
        // For individual files (when dir_size is called directly on a file)
        // Apply pattern filtering if specified
        if (pattern && !simple_pattern_matches(pattern, rel_path))
            return;
            
        // Count the file
        result->files++;
        result->bytes += statbuf.st_size;
    }
    // Other file types (symlinks, etc.) are not counted in size calculation
}

DIR_SIZE dir_size(const char *path, SIMPLE_PATTERN *pattern, size_t max_depth) {
    DIR_SIZE result = DIR_SIZE_EMPTY;
    
    if (!path || !*path)
        return result;

    // Create dictionary to track visited inodes
    DICTIONARY *visited_inodes = dictionary_create(DICT_OPTION_SINGLE_THREADED);
    
    // Check if path exists and get initial stats
    struct stat statbuf;
    
    if (stat(path, &statbuf) != 0) {
        result.errors++;
        dictionary_destroy(visited_inodes);
        return result;
    }

    if (S_ISDIR(statbuf.st_mode)) {
        // Start recursion from the base path for directories
        calc_dir_size_recursive(path, "", pattern, max_depth, 0, &result, visited_inodes);
    } else if (S_ISREG(statbuf.st_mode)) {
        // Single file case
        // Extract the filename for pattern matching
        const char *filename = strrchr(path, '/');
        filename = filename ? filename + 1 : path;
        
        // Apply pattern filtering if specified
        if (pattern && !simple_pattern_matches(pattern, filename))
            return result;
            
        result.files = 1;
        result.bytes = statbuf.st_size;
    }

    dictionary_destroy(visited_inodes);
    return result;
}

DIR_SIZE dir_size_multiple(const char **paths, int num_paths, SIMPLE_PATTERN *pattern, size_t max_depth) {
    DIR_SIZE result = DIR_SIZE_EMPTY;
    
    if (!paths || num_paths <= 0)
        return result;

    // Calculate size for each path and combine results
    for (int i = 0; i < num_paths; i++) {
        if (!paths[i] || !*paths[i])
            continue;
            
        DIR_SIZE path_result = dir_size(paths[i], pattern, max_depth);
        
        // Combine results
        result.bytes += path_result.bytes;
        result.files += path_result.files;
        result.directories += path_result.directories;
        result.errors += path_result.errors;
        
        // Take the maximum depth found
        if (path_result.depth > result.depth)
            result.depth = path_result.depth;
    }
    
    return result;
}