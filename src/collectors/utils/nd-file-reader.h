// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ND_FILE_READER_H
#define NETDATA_ND_FILE_READER_H

#include <stdbool.h>
#include <stdint.h>

// Private one-shot nd-run interface:
//   --file-reader read regular|stream <byte limit, 0 = unlimited> <path>
//   --file-reader stat <path>
// Read stdout is raw file data; stat stdout is signed Unix seconds and nanoseconds.
// Stderr terminates with exactly "NDFILE <result> <errno>\n" for a file result.
// Accept data only after exit 0 and result OK/0. Discard buffered bytes on failure;
// streaming consumers must report a failed terminal result instead of clean EOF.
// Usage, privilege, signal and transport failures may have no valid result line.
enum nd_file_result {
    ND_FILE_OK = 0,
    ND_FILE_OPEN_ERROR = 1,
    ND_FILE_STAT_ERROR = 2,
    ND_FILE_READ_ERROR = 3,
    ND_FILE_CLOSE_ERROR = 4,
    ND_FILE_NOT_REGULAR = 5,
    ND_FILE_TOO_LARGE = 6,
};

struct nd_file_request {
    const char *path;
    uint64_t limit;
    bool stat_only;
    bool regular;
};

bool nd_file_reader_parse(int argc, char **argv, struct nd_file_request *request);
// nd-run must complete privilege reduction before calling this function.
int nd_file_reader_run(const struct nd_file_request *request);

#endif // NETDATA_ND_FILE_READER_H
