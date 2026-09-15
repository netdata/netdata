// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ND_FILE_READER_H
#define NETDATA_ND_FILE_READER_H

// Private nd-run / Go credentialfile protocol. Integers use network byte order.
// Hello: "NDFILE01", uint32 PATH_MAX (exclusive), uint32 maximum chunk length.
// Request: uint32 operation, uint32 flags, uint64 byte limit (0 = unlimited),
//          uint32 path length, uint32 reserved (0), followed by path bytes.
// Frame: uint32 kind, uint32 payload length, uint32 result, uint32 errno.
// DATA carries bytes; RESULT terminates a read. STAT carries signed int64 Unix
// seconds and nanoseconds and terminates a stat request. Failed operations use
// RESULT with no payload. Error frames and diagnostics never include file content.
enum nd_file_operation { ND_FILE_READ = 1, ND_FILE_STAT = 2 };
enum nd_file_flag { ND_FILE_REGULAR = 1 };
enum nd_file_frame { ND_FILE_DATA = 1, ND_FILE_RESULT = 2, ND_FILE_INFO = 3 };
enum nd_file_result {
    ND_FILE_OK = 0,
    ND_FILE_OPEN_ERROR = 1,
    ND_FILE_STAT_ERROR = 2,
    ND_FILE_READ_ERROR = 3,
    ND_FILE_CLOSE_ERROR = 4,
    ND_FILE_NOT_REGULAR = 5,
    ND_FILE_TOO_LARGE = 6,
};

#define ND_FILE_CHUNK_SIZE 32768

int nd_file_reader_main(void);

#endif // NETDATA_ND_FILE_READER_H
