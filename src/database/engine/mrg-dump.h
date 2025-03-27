// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MRG_DUMP_H
#define NETDATA_MRG_DUMP_H

#include "mrg-internals.h"
#include "rrdengineapi.h"
#include <zstd.h>

// File layout constants
#define MRG_FILE_HEADER_SIZE 4096
#define MRG_FILE_PAGE_HEADER_SIZE 64
#define MRG_FILE_PAGE_SIZE (1024 * 1024)  // 1 MiB uncompressed page size
#define MRG_FILE_EXTENSION ".mrg"
#define MRG_FILE_NAME "metrics" MRG_FILE_EXTENSION
#define MRG_FILE_TMP_NAME "metrics.tmp" MRG_FILE_EXTENSION

// Page types
typedef enum {
    MRG_PAGE_TYPE_METRIC = 1,
    MRG_PAGE_TYPE_FILE = 2
} mrg_page_type_t;

// File header structure
typedef struct {
    char magic[8];                   // Magic identifier "NETDMRG\0"
    uint32_t version;                // File format version
    uint64_t base_time;              // Base timestamp for relative time values
    uint32_t tiers_count;            // Number of tiers
    uint32_t metrics_count;          // Total number of metrics
    uint32_t files_count;            // Total number of files
    uint32_t compression_level;      // ZSTD compression level used

    struct {
        uint64_t last_offset;        // Offset of the last metrics page
        uint32_t page_count;         // Number of metrics pages
    } metric_pages;

    struct {
        uint64_t last_offset;        // Offset of the last file page
        uint32_t page_count;         // Number of file pages
    } file_pages;

    uint8_t reserved[4048];          // Reserved space to maintain 4KB header
} mrg_file_header_t;

// Page header structure
typedef struct {
    char magic[4];                   // Magic identifier "MRGP"
    uint32_t type;                   // Page type (metric, file)
    uint64_t prev_offset;            // Offset to previous page of same type
    uint32_t compressed_size;        // Size of the compressed data
    uint32_t uncompressed_size;      // Size of the uncompressed data
    uint32_t entries_count;          // Number of entries in this page
    uint8_t reserved[40];            // Reserved space to maintain 64-byte header
} mrg_page_header_t;

// Metric entry structure
typedef struct {
    ND_UUID uuid;                    // Metric UUID
    uint32_t tier;                   // Tier number this metric belongs to
    uint32_t first_time;             // First timestamp relative to base_time
    uint32_t last_time;              // Last timestamp relative to base_time
    uint32_t update_every;           // Update frequency
} mrg_file_metric_t;

// File entry structure
typedef struct {
    uint32_t tier;                   // Tier this file belongs to
    uint32_t fileno;                 // File number in tier
    uint64_t size;                   // File size
    uint64_t mtime;                  // File modification time
} mrg_file_entry_t;

// Context for file writing
typedef struct {
    int fd;                          // File descriptor
    uint64_t file_size;              // Current file size
    mrg_file_header_t header;        // File header

    struct {
        void *buffer;                // Uncompressed buffer for metrics
        uint32_t size;               // Current size used in buffer
        uint32_t entries;            // Number of entries in buffer
    } metric_pages;

    struct {
        void *buffer;                // Uncompressed buffer for files
        uint32_t size;               // Current size used in buffer
        uint32_t entries;            // Number of entries in buffer
    } file_pages;

    void *compressed_buffer;         // Buffer for compressed data
} mrg_file_ctx_t;

// Function prototypes
bool mrg_dump_save(MRG *mrg);
bool mrg_dump_load(MRG *mrg);

#endif //NETDATA_MRG_DUMP_H
