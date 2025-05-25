// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MCP_TIME_UTILS_H
#define NETDATA_MCP_TIME_UTILS_H

#include "mcp.h"

/**
 * Extract a time parameter from JSON object
 * 
 * This function supports multiple time formats:
 * - Integer epoch seconds (positive for absolute time)
 * - Negative integers for relative time (e.g., -3600 for "1 hour ago")
 * - RFC3339 formatted strings (e.g., "2024-01-15T10:30:00Z")
 * - String representations of integers
 * 
 * @param params The JSON object containing parameters
 * @param name The parameter name to extract
 * @param default_value The default value if parameter is missing or invalid
 * @return The parsed time_t value or default_value
 */
time_t mcp_extract_time_param(struct json_object *params, const char *name, time_t default_value);

/**
 * Convert a time_t to RFC3339 string for MCP output
 * 
 * @param buffer Buffer to write the RFC3339 string to
 * @param len Size of the buffer
 * @param timestamp The timestamp to convert
 * @param utc Whether to format in UTC (true) or local time (false)
 * @return Number of characters written (excluding null terminator)
 */
size_t mcp_time_to_rfc3339(char *buffer, size_t len, time_t timestamp, bool utc);

/**
 * Convert a usec_t to RFC3339 string for MCP output
 * 
 * @param buffer Buffer to write the RFC3339 string to
 * @param len Size of the buffer
 * @param timestamp_ut The timestamp in microseconds
 * @param fractional_digits Number of fractional digits (0-6)
 * @param utc Whether to format in UTC (true) or local time (false)
 * @return Number of characters written (excluding null terminator)
 */
size_t mcp_time_ut_to_rfc3339(char *buffer, size_t len, usec_t timestamp_ut, size_t fractional_digits, bool utc);

#endif //NETDATA_MCP_TIME_UTILS_H