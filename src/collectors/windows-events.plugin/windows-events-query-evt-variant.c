// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows-events.h"
#include <sddl.h>  // For SID string conversion

// Function to append the separator if the buffer is not empty
static inline void append_separator_if_needed(BUFFER *b, const char *separator) {
    if (buffer_strlen(b) > 0 && separator != NULL)
        buffer_strcat(b, separator);
}

// Helper function to convert UTF16 strings to UTF8 and append to the buffer
static inline void append_utf16(BUFFER *b, LPCWSTR utf16Str, const char *separator) {
    if (!utf16Str || !*utf16Str) return;

    append_separator_if_needed(b, separator);

    size_t remaining = b->size - b->len;
    if(remaining < 128) {
        buffer_need_bytes(b, 128);
        remaining = b->size - b->len;
    }

    bool truncated = false;
    size_t used = utf16_to_utf8(&b->buffer[b->len], remaining, utf16Str, -1, &truncated);
    if(truncated) {
        // we need to resize
        size_t needed = utf16_to_utf8(NULL, 0, utf16Str, -1, NULL); // find the size needed
        buffer_need_bytes(b, needed);
        remaining = b->size - b->len;
        used = utf16_to_utf8(&b->buffer[b->len], remaining, utf16Str, -1, NULL);
    }

    if(used) {
        b->len += used - 1;

        internal_fatal(buffer_strlen(b) != strlen(buffer_tostring(b)),
                       "Buffer length mismatch.");
    }
}

// Function to append binary data to the buffer
static inline void append_binary(BUFFER *b, PBYTE data, DWORD size, const char *separator) {
    if (data == NULL || size == 0) return;

    append_separator_if_needed(b, separator);

    buffer_need_bytes(b, size * 4);
    for (DWORD i = 0; i < size; i++) {
        uint8_t value = data[i];
        b->buffer[b->len++] = hex_digits[(value & 0xf0) >> 4];
        b->buffer[b->len++] = hex_digits[(value & 0x0f)];
    }
}

// Function to append size_t to the buffer
static inline void append_size_t(BUFFER *b, size_t size, const char *separator) {
    append_separator_if_needed(b, separator);
    buffer_print_uint64(b, size);
}

// Function to append HexInt32 in hexadecimal format
static inline void append_uint32_hex(BUFFER *b, UINT32 n, const char *separator) {
    append_separator_if_needed(b, separator);
    buffer_print_uint64_hex(b, n);
}

// Function to append HexInt64 in hexadecimal format
static inline void append_uint64_hex(BUFFER *b, UINT64 n, const char *separator) {
    append_separator_if_needed(b, separator);
    buffer_print_uint64_hex(b, n);
}

// Function to append various data types to the buffer
static inline void append_uint64(BUFFER *b, UINT64 n, const char *separator) {
    append_separator_if_needed(b, separator);
    buffer_print_uint64(b, n);
}

static inline void append_int64(BUFFER *b, INT64 n, const char *separator) {
    append_separator_if_needed(b, separator);
    buffer_print_int64(b, n);
}

static inline void append_double(BUFFER *b, double n, const char *separator) {
    append_separator_if_needed(b, separator);
    buffer_print_netdata_double(b, n);
}

static inline void append_guid(BUFFER *b, GUID *guid, const char *separator) {
    fatal_assert(sizeof(GUID) == sizeof(nd_uuid_t));

    append_separator_if_needed(b, separator);

    ND_UUID *uuid = (ND_UUID *)guid;
    buffer_need_bytes(b, UUID_STR_LEN);
    uuid_unparse_lower(uuid->uuid, &b->buffer[b->len]);
    b->len += UUID_STR_LEN - 1;

    internal_fatal(buffer_strlen(b) != strlen(buffer_tostring(b)),
                   "Buffer length mismatch.");
}

static inline void append_systime(BUFFER *b, SYSTEMTIME *st, const char *separator) {
    append_separator_if_needed(b, separator);
    buffer_sprintf(b, "%04d-%02d-%02d %02d:%02d:%02d",
                   st->wYear, st->wMonth, st->wDay, st->wHour, st->wMinute, st->wSecond);
}

static inline void append_filetime(BUFFER *b, FILETIME *ft, const char *separator) {
    SYSTEMTIME st;
    if (FileTimeToSystemTime(ft, &st))
        append_systime(b, &st, separator);
}

static inline void append_sid(BUFFER *b, PSID sid, const char *separator) {
    cached_sid_to_buffer_append(sid, b, separator);
}

static inline void append_sbyte(BUFFER *b, INT8 n, const char *separator) {
    append_separator_if_needed(b, separator);
    buffer_print_int64(b, n);
}

static inline void append_byte(BUFFER *b, UINT8 n, const char *separator) {
    append_separator_if_needed(b, separator);
    buffer_print_uint64(b, n);
}

static inline void append_int16(BUFFER *b, INT16 n, const char *separator) {
    append_separator_if_needed(b, separator);
    buffer_print_int64(b, n);
}

static inline void append_uint16(BUFFER *b, UINT16 n, const char *separator) {
    append_separator_if_needed(b, separator);
    buffer_print_uint64(b, n);
}

static inline void append_int32(BUFFER *b, INT32 n, const char *separator) {
    append_separator_if_needed(b, separator);
    buffer_print_int64(b, n);
}

static inline void append_uint32(BUFFER *b, UINT32 n, const char *separator) {
    append_separator_if_needed(b, separator);
    buffer_print_uint64(b, n);
}

// Function to append EVT_HANDLE to the buffer
static inline void append_evt_handle(BUFFER *b, EVT_HANDLE h, const char *separator) {
    append_separator_if_needed(b, separator);
    buffer_print_uint64_hex(b, (uintptr_t)h);
}

// Function to append XML data (UTF-16) to the buffer
static inline void append_evt_xml(BUFFER *b, LPCWSTR xmlData, const char *separator) {
    append_utf16(b, xmlData, separator);  // XML data is essentially UTF-16 string
}

void evt_variant_to_buffer(BUFFER *b, EVT_VARIANT *ev, const char *separator) {
    if(ev->Type == EvtVarTypeNull) return;

    if (ev->Type & EVT_VARIANT_TYPE_ARRAY) {
        for (DWORD i = 0; i < ev->Count; i++) {
            switch (ev->Type & EVT_VARIANT_TYPE_MASK) {
                case EvtVarTypeString:
                    append_utf16(b, ev->StringArr[i], separator);
                    break;

                case EvtVarTypeAnsiString:
                    if (ev->AnsiStringArr[i] != NULL) {
                        append_utf16(b, (LPCWSTR)ev->AnsiStringArr[i], separator);
                    }
                    break;

                case EvtVarTypeSByte:
                    append_sbyte(b, ev->SByteArr[i], separator);
                    break;

                case EvtVarTypeByte:
                    append_byte(b, ev->ByteArr[i], separator);
                    break;

                case EvtVarTypeInt16:
                    append_int16(b, ev->Int16Arr[i], separator);
                    break;

                case EvtVarTypeUInt16:
                    append_uint16(b, ev->UInt16Arr[i], separator);
                    break;

                case EvtVarTypeInt32:
                    append_int32(b, ev->Int32Arr[i], separator);
                    break;

                case EvtVarTypeUInt32:
                    append_uint32(b, ev->UInt32Arr[i], separator);
                    break;

                case EvtVarTypeInt64:
                    append_int64(b, ev->Int64Arr[i], separator);
                    break;

                case EvtVarTypeUInt64:
                    append_uint64(b, ev->UInt64Arr[i], separator);
                    break;

                case EvtVarTypeSingle:
                    append_double(b, ev->SingleArr[i], separator);
                    break;

                case EvtVarTypeDouble:
                    append_double(b, ev->DoubleArr[i], separator);
                    break;

                case EvtVarTypeGuid:
                    append_guid(b, &ev->GuidArr[i], separator);
                    break;

                case EvtVarTypeFileTime:
                    append_filetime(b, &ev->FileTimeArr[i], separator);
                    break;

                case EvtVarTypeSysTime:
                    append_systime(b, &ev->SysTimeArr[i], separator);
                    break;

                case EvtVarTypeSid:
                    append_sid(b, ev->SidArr[i], separator);
                    break;

                case EvtVarTypeBinary:
                    append_binary(b, ev->BinaryVal, ev->Count, separator);
                    break;

                case EvtVarTypeSizeT:
                    append_size_t(b, ev->SizeTArr[i], separator);
                    break;

                case EvtVarTypeHexInt32:
                    append_uint32_hex(b, ev->UInt32Arr[i], separator);
                    break;

                case EvtVarTypeHexInt64:
                    append_uint64_hex(b, ev->UInt64Arr[i], separator);
                    break;

                case EvtVarTypeEvtHandle:
                    append_evt_handle(b, ev->EvtHandleVal, separator);
                    break;

                case EvtVarTypeEvtXml:
                    append_evt_xml(b, ev->XmlValArr[i], separator);
                    break;

                default:
                    // Skip unknown array types
                    break;
            }
        }
    } else {
        switch (ev->Type & EVT_VARIANT_TYPE_MASK) {
            case EvtVarTypeNull:
                // Do nothing for null types
                break;

            case EvtVarTypeString:
                append_utf16(b, ev->StringVal, separator);
                break;

            case EvtVarTypeAnsiString:
                append_utf16(b, (LPCWSTR)ev->AnsiStringVal, separator);
                break;

            case EvtVarTypeSByte:
                append_sbyte(b, ev->SByteVal, separator);
                break;

            case EvtVarTypeByte:
                append_byte(b, ev->ByteVal, separator);
                break;

            case EvtVarTypeInt16:
                append_int16(b, ev->Int16Val, separator);
                break;

            case EvtVarTypeUInt16:
                append_uint16(b, ev->UInt16Val, separator);
                break;

            case EvtVarTypeInt32:
                append_int32(b, ev->Int32Val, separator);
                break;

            case EvtVarTypeUInt32:
                append_uint32(b, ev->UInt32Val, separator);
                break;

            case EvtVarTypeInt64:
                append_int64(b, ev->Int64Val, separator);
                break;

            case EvtVarTypeUInt64:
                append_uint64(b, ev->UInt64Val, separator);
                break;

            case EvtVarTypeSingle:
                append_double(b, ev->SingleVal, separator);
                break;

            case EvtVarTypeDouble:
                append_double(b, ev->DoubleVal, separator);
                break;

            case EvtVarTypeBoolean:
                append_separator_if_needed(b, separator);
                buffer_strcat(b, ev->BooleanVal ? "true" : "false");
                break;

            case EvtVarTypeGuid:
                append_guid(b, ev->GuidVal, separator);
                break;

            case EvtVarTypeBinary:
                append_binary(b, ev->BinaryVal, ev->Count, separator);
                break;

            case EvtVarTypeSizeT:
                append_size_t(b, ev->SizeTVal, separator);
                break;

            case EvtVarTypeHexInt32:
                append_uint32_hex(b, ev->UInt32Val, separator);
                break;

            case EvtVarTypeHexInt64:
                append_uint64_hex(b, ev->UInt64Val, separator);
                break;

            case EvtVarTypeEvtHandle:
                append_evt_handle(b, ev->EvtHandleVal, separator);
                break;

            case EvtVarTypeEvtXml:
                append_evt_xml(b, ev->XmlVal, separator);
                break;

            default:
                // Skip unknown types
                break;
        }
    }
}
