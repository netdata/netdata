#include "common.h"

// ----------------------------------------------------------------------------
// PROMETHEUS Data Collection Backend
// This backend provides data in Prometheus format via the endpoint:
// /api/v1/allmetrics?format=prometheus
// ----------------------------------------------------------------------------

// Structure to keep track of the last access time for each Prometheus server
// that scrapes metrics. This is used to send incremental updates.
static struct prometheus_server {
    const char *server;          // The identifier of the Prometheus server (e.g., its URL or a name)
    uint32_t hash;               // Hash of the server identifier for faster lookups
    time_t last_access;          // Timestamp of the last time this server requested metrics
    struct prometheus_server *next; // Pointer to the next server in the linked list
} *prometheus_server_root = NULL; // Root of the linked list of Prometheus servers

/**
 * @brief Retrieves the last access time for a given server and updates it to 'now'.
 *
 * This function maintains a list of Prometheus servers and their last access times.
 * If a server is found, its last access time is updated, and the previous last access time is returned.
 * If the server is not found, a new entry is created, added to the list, and 0 is returned,
 * indicating that this is the first time this server is seen (or its state was lost).
 *
 * @param server The identifier of the Prometheus server.
 * @param now The current timestamp.
 * @return The previous last access time for the server, or 0 if the server is new.
 */
static inline time_t prometheus_server_last_access(const char *server, time_t now) {
    uint32_t hash = simple_hash(server); // Calculate hash for the server string for efficient lookup

    struct prometheus_server *ps;
    // Traverse the list of known Prometheus servers
    for(ps = prometheus_server_root; ps ;ps = ps->next) {
        // Compare hash first for speed, then the actual string if hashes match
        if (hash == ps->hash && !strcmp(server, ps->server)) {
            time_t last = ps->last_access; // Store the previous last access time
            ps->last_access = now;         // Update to the current time
            return last;                   // Return the previous time
        }
    }

    // Server not found, create a new entry
    ps = callocz(1, sizeof(struct prometheus_server)); // Allocate and zero-initialize memory
    ps->server = strdupz(server);                      // Duplicate the server string
    ps->hash = hash;                                   // Store the hash
    ps->last_access = now;                             // Set initial last access time
    ps->next = prometheus_server_root;                 // Link the new entry to the beginning of the list
    prometheus_server_root = ps;                       // Update the root to the new entry

    return 0; // Return 0 as this is a new server
}

/**
 * @brief Copies a source string 's' to destination 'd', converting non-alphanumeric characters to underscores.
 *
 * This function is used to sanitize strings for use in Prometheus metric names.
 * It iterates through the source string and copies alphanumeric characters directly.
 * Non-alphanumeric characters are replaced with '_'.
 * The copy stops if the source string ends or the usable destination buffer size is reached.
 * The destination string is always null-terminated.
 *
 * Optimization: Uses memcpy for contiguous blocks of alphanumeric characters.
 *
 * @param d Pointer to the destination buffer.
 * @param s Pointer to the source string.
 * @param usable Maximum number of bytes that can be written to 'd', including the null terminator.
 * @return The number of characters written to 'd', excluding the null terminator.
 */
static inline size_t prometheus_name_copy(char *d, const char *s, size_t usable) {
    size_t n = 0; // Number of characters written
    const char *start_alphanum = NULL; // Pointer to the start of a sequence of alphanumeric characters

    // Iterate while there are characters in source and space in destination (leaving space for null terminator)
    while (*s && n < usable -1) { // usable-1 to ensure space for null terminator at the end
        if (isalnum(*s)) { // Current character is alphanumeric
            if (start_alphanum == NULL) { // If not already in a block, mark the start
                start_alphanum = s;
            }
        } else { // Current character is non-alphanumeric
            if (start_alphanum != NULL) { // If we were in an alphanumeric block, copy it
                size_t len = s - start_alphanum; // Length of the alphanumeric block
                if (n + len > usable -1) { // Ensure not to overflow usable space (reserved for null term)
                    len = usable -1 - n;
                }
                memcpy(d, start_alphanum, len); // Copy the block
                d += len; // Advance destination pointer
                n += len; // Increment count of written characters
                start_alphanum = NULL; // Reset block start
            }
            // Add the underscore for the non-alphanumeric character
            if (n < usable -1) { // Check if there's space for the underscore (and null term)
                *d++ = '_';
                n++;
            } else {
                // No space for the underscore if we are at usable-1 already.
                // Break to ensure null termination at d.
                break;
            }
        }
        s++; // Advance source pointer
    }

    // If the string ends with an alphanumeric block, copy it
    if (start_alphanum != NULL && n < usable -1) {
        size_t len = s - start_alphanum;
        if (n + len > usable -1) {
            len = usable -1 - n;
        }
        memcpy(d, start_alphanum, len);
        d += len;
        n += len;
    }

    *d = '\0'; // Null-terminate the destination string
    return n;    // Return the number of characters written (excluding null terminator)
}

/**
 * @brief Copies a source string 's' to destination 'd', escaping characters for Prometheus labels.
 *
 * Prometheus label values require escaping for newline ('\n'), double quote ('"'), and backslash ('\').
 * This function iterates through the source string:
 * - If a character needs escaping, a backslash is prepended.
 * - Other characters are copied directly.
 * The copy stops if the source string ends or the usable destination buffer size is reached.
 * 'usable' is decremented initially to ensure space for at least one escaped character without overflow.
 * The destination string is always null-terminated.
 *
 * @param d Pointer to the destination buffer.
 * @param s Pointer to the source string.
 * @param usable Maximum number of bytes that can be written to 'd', including the null terminator.
 * @return The number of characters written to 'd', excluding the null terminator.
 */
static inline size_t prometheus_label_copy(char *d, const char *s, size_t usable) {
    size_t n; // Number of characters written

    // Make sure we can escape at least one character ('\c') without overflowing the buffer.
    // This means we need space for the escape char '\' and the char itself, plus null terminator.
    // If usable is 0, no space at all. If 1, only space for null terminator.
    if (usable == 0) return 0;
    usable--; // Reserve space for null terminator. Now 'usable' is max chars before null.

    for(n = 0; *s && n < usable ; d++, s++, n++) {
        register char c = *s;

        // Escape characters: ", \, \n
        // These are considered unlikely in typical label values, optimizing for the common case.
        if(unlikely(c == '"' || c == '\\' || c == '\n')) {
            *d++ = '\\'; // Prepend backslash
            n++;         // Increment count for the backslash
            // Check if there's still space after adding the backslash for the actual character
            if (n >= usable) { // If not, break. The backslash is written, but the char it escapes is not.
                               // This might lead to a truncated escape sequence if not handled carefully by caller.
                               // However, d is advanced, so null term will be after the written backslash.
                d--; // Point d back to where the char would have been, so null term is correct.
                break;
            }
        }
        *d = c; // Copy original character (or the character after backslash)
    }
    *d = '\0'; // Null-terminate the destination string

    return n; // Return the number of characters written (excluding null terminator)
}

/**
 * @brief Copies and transforms a units string 's' to a Prometheus-compatible format in 'd'.
 *
 * Prometheus metric names should not contain non-alphanumeric characters.
 * This function handles specific unit transformations:
 * - "%" becomes "_percent".
 * - Strings ending in "/s" (e.g., "bytes/s") become "_persec" (e.g., "netdata_bytes_persec").
 *   The part before "/s" is prepended with an underscore and sanitized.
 * For other strings:
 * - An underscore '_' is prepended.
 * - The string is copied, with non-alphanumeric characters replaced by '_'.
 * The copy stops if the source string ends or the usable destination buffer size is reached.
 * The destination string is always null-terminated.
 *
 * Optimization: Uses memcpy for contiguous blocks of alphanumeric characters in the default case.
 * Optimization: Caches strlen(s) to avoid repeated calls.
 *
 * @param d Pointer to the destination buffer.
 * @param s Pointer to the source units string.
 * @param usable Maximum number of bytes that can be written to 'd', including the null terminator.
 * @return Pointer to the beginning of the destination buffer 'd' (char *start_d).
 */
static inline char *prometheus_units_copy(char *d, const char *s, size_t usable) {
    char *start_d = d; // Keep track of the beginning of the destination buffer
    size_t n = 0;      // Number of characters written

    // Ensure there's enough space for at least a null terminator.
    // If usable is 1, only a null terminator can be written.
    if (usable < 1) return start_d; // Not enough space for anything.
    if (usable == 1) { *d = '\0'; return start_d; } // Only space for null terminator.
    // Now usable >= 2, meaning space for at least one character and a null terminator.
    
    // Handle special unit cases first
    size_t len_s = strlen(s); // Cache strlen for efficiency, used in "/s" check.
    if (strcmp(s, "%") == 0) { // strcmp is safe as s is non-NULL (implicit assumption from callers)
        strncpy(d, "_percent", usable - 1); // usable-1 ensures space for null terminator.
        d[usable - 1] = '\0';               // Ensure null termination, strncpy might not if src is too long.
        return start_d;
    } else if (len_s > 2 && strcmp(s + len_s - 2, "/s") == 0) { // Check for "/s" suffix.
        // Case: units like "requests/s" -> transform to "_requests_persec"
        size_t prefix_len = len_s - 2; // Length of the part before "/s".
        
        // Prepend an underscore if there's a prefix part (e.g., "requests" in "requests/s").
        if (prefix_len > 0 && n < usable - 1) { // usable-1 leaves space for char + null terminator.
            *d++ = '_';
            n++;
        }
        
        // Copy and sanitize the prefix part (e.g., "requests").
        const char *s_ptr = s;
        for (size_t i = 0; i < prefix_len && n < usable - 1; i++, s_ptr++, n++) {
            *d++ = isalnum(*s_ptr) ? *s_ptr : '_'; // Replace non-alphanum with '_'.
        }
        
        // Append the "_persec" suffix.
        const char *suffix = "_persec";
        while (*suffix && n < usable - 1) { // Check space for char + null terminator.
            *d++ = *suffix++;
            n++;
        }
        *d = '\0'; // Null-terminate.
        return start_d;
    }

    // Default behavior: prepend '_', then copy 's', replacing non-alphanum chars with '_'.
    if (n < usable - 1) { // Check space for '_' and null terminator.
        *d++ = '_';
        n++;
    }

    const char *s_ptr = s; // Use a new pointer to iterate over the source string 's'.
    const char *start_alphanum = NULL; // For optimized block copying.
    
    // Iterate while source has chars and there's space in destination (reserving one for null term).
    while(*s_ptr && n < usable - 1) {
        if (isalnum(*s_ptr)) { // Current char is alphanumeric.
            if (start_alphanum == NULL) { // Start of a new alphanumeric block.
                start_alphanum = s_ptr;
            }
        } else { // Current char is non-alphanumeric.
            if (start_alphanum != NULL) { // If we were in an alphanumeric block, copy it.
                size_t len = s_ptr - start_alphanum; // Length of the block.
                if (n + len > usable - 1) { // Ensure not to overflow (usable-1 for null term).
                    len = usable - 1 - n;
                }
                memcpy(d, start_alphanum, len); // Copy the block.
                d += len; // Advance destination pointer
                n += len; // Increment count of written characters
                start_alphanum = NULL; // Reset block start
            }
            // Add the underscore for the non-alphanumeric character.
            if (n < usable - 1) { // Check if there's space for '_' and null terminator.
                *d++ = '_';
                n++;
            } else { // Not enough space for '_' and null terminator.
                break; 
            }
        }
        s_ptr++; // Advance source pointer.
    }

    // If the string ends with an alphanumeric block, copy it.
    if (start_alphanum != NULL && n < usable - 1) {
        size_t len = s_ptr - start_alphanum;
        if (n + len > usable - 1) {
            len = usable - 1 - n;
        }
        memcpy(d, start_alphanum, len);
        d += len;
        // n is not strictly needed after this for the return value of this function,
        // but if it were, it should be: n += len;
    }
    
    *d = '\0'; // Null-terminate.
    return start_d; // Return pointer to the start of the destination buffer.
}


#define PROMETHEUS_ELEMENT_MAX 256  // Max length for individual metric name elements (like context, dimension)
#define PROMETHEUS_LABELS_MAX 1024   // Max length for concatenated label strings

/**
 * @brief Generates Prometheus metrics for a given RRDHOST.
 *
 * This is the core function for formatting Netdata metrics into Prometheus exposition format.
 * It iterates through all charts (RRDSET) and their dimensions (RRDDIM) for the given host,
 * applying Prometheus naming conventions and label formatting.
 *
 * Key Optimizations:
 * - Pre-computation of common metric name parts (e.g., `prefix_context`) to avoid redundant string
 *   concatenations within inner loops.
 * - `likely()`/`unlikely()` annotations for branch prediction hints.
 *
 * @param host The RRDHOST to collect metrics from.
 * @param wb The BUFFER to write the Prometheus formatted metrics into.
 * @param prefix A string prefix for all metric names (e.g., "netdata").
 * @param options Backend options controlling data source (as_collected, average, sum) and filtering.
 * @param after Timestamp indicating the start of the data collection window (for incremental updates).
 * @param before Timestamp indicating the end of the data collection window.
 * @param allhosts Integer flag (0 or 1); if 1, an 'instance' label with the hostname is added to metrics.
 *                 Also affects generation of `netdata_host_tags`.
 * @param help Integer flag; if 1, detailed HELP comments are generated for each metric.
 * @param types Integer flag; if 1, TYPE comments are generated for each metric.
 * @param names Integer flag; if 1, user-friendly names are preferred over IDs for charts/dimensions.
 */
static void rrd_stats_api_v1_charts_allmetrics_prometheus(RRDHOST *host, BUFFER *wb, const char *prefix, uint32_t options, time_t after, time_t before, int allhosts, int help, int types, int names) {
    rrdhost_rdlock(host); // Acquire read lock on the host data

    // Prepare hostname label (escaped for Prometheus)
    char hostname_label[PROMETHEUS_ELEMENT_MAX + 1];
    prometheus_label_copy(hostname_label, host->hostname, PROMETHEUS_ELEMENT_MAX);

    // Suffix for labels, typically includes the instance label if 'allhosts' is true.
    // This string will be appended to the labels of each metric.
    char common_labels_suffix[PROMETHEUS_LABELS_MAX + 1] = "";
    if(allhosts) { // If exporting for multiple hosts context, add instance tags
        if(host->tags && *(host->tags)) // Output host tags if they exist
            buffer_sprintf(wb, "netdata_host_tags{instance=\"%s\",%s} 1 %llu\n", hostname_label, host->tags, now_realtime_usec() / USEC_PER_MS);
        // Prepare the instance label for appending to all metrics from this host
        snprintfz(common_labels_suffix, PROMETHEUS_LABELS_MAX, ",instance=\"%s\"", hostname_label);
    }
    else { // Single host context
        if(host->tags && *(host->tags)) // Output host tags if they exist
            buffer_sprintf(wb, "netdata_host_tags{%s} 1 %llu\n", host->tags, now_realtime_usec() / USEC_PER_MS);
    }

    // Iterate over each chart (RRDSET) for the current host
    RRDSET *st;
    rrdset_foreach_read(st, host) {
        // Buffers for Prometheus-compatible chart, family, and context strings
        char chart_label_str[PROMETHEUS_ELEMENT_MAX + 1];    // For {chart="chart_name", ...}
        char family_label_str[PROMETHEUS_ELEMENT_MAX + 1];   // For {family="family_name", ...}
        char context_name_str[PROMETHEUS_ELEMENT_MAX + 1];   // For metric name part: prefix_CONTEXTNAME...
        // Buffer for processed units string, used if units are part of the metric name (e.g., for averages).
        // Initialized to empty; populated by prometheus_units_copy if needed.
        char processed_units_for_name[PROMETHEUS_ELEMENT_MAX + 1] = "";

        // Sanitize chart ID/name, family, and context for Prometheus labels/names
        prometheus_label_copy(chart_label_str, (names && st->name)?st->name:st->id, PROMETHEUS_ELEMENT_MAX);
        prometheus_label_copy(family_label_str, st->family, PROMETHEUS_ELEMENT_MAX);
        prometheus_name_copy(context_name_str, st->context, PROMETHEUS_ELEMENT_MAX);

        // Precompute `prefix_context` part of the metric name.
        // This is done once per chart to avoid repeated concatenations in the dimension loop.
        // Example: "netdata_system"
        char metric_name_prefix_context[2 * PROMETHEUS_ELEMENT_MAX + 2]; 
        snprintfz(metric_name_prefix_context, sizeof(metric_name_prefix_context), "%s_%s", prefix, context_name_str);
        
        // `metric_name_base` will be the fundamental name part before value suffixes (_total, _average).
        // It can be `prefix_context` or `prefix_context_units` depending on options.
        // Example: "netdata_system" or "netdata_system_ms"
        char metric_name_base[3 * PROMETHEUS_ELEMENT_MAX + 3]; 

        // Check if this RRDSET should be sent, based on backend options and chart properties.
        // This is typically true unless specific filtering is active.
        if(likely(backends_can_send_rrdset(options, st))) {
            rrdset_rdlock(st); // Acquire read lock on the chart (RRDSET)

            // Determine if data should be "as collected" or aggregated (average, sum).
            int as_collected = ((options & BACKEND_SOURCE_BITS) == BACKEND_SOURCE_DATA_AS_COLLECTED);
            // A chart is homogeneous if all its dimensions have the same algorithm, multiplier, and divisor.
            // This affects how metrics are named and labeled.
            int homogeneus = 1; // Assume dimensions are homogeneous by default.

            if(as_collected) {
                // If "as collected", explicitly check and update the homogeneous flag for the chart.
                if(rrdset_flag_check(st, RRDSET_FLAG_HOMEGENEOUS_CHECK))
                    rrdset_update_heterogeneous_flag(st); // This updates RRDSET_FLAG_HETEROGENEOUS if necessary.

                if(rrdset_flag_check(st, RRDSET_FLAG_HETEROGENEOUS))
                    homogeneus = 0; // Mark as heterogeneous if dimensions differ in properties.
                
                // For "as collected" data, the metric name base is typically `prefix_context`.
                // Units are not included in the name itself but would be part of HELP text.
                snprintfz(metric_name_base, sizeof(metric_name_base), "%s", metric_name_prefix_context);
            } else { // Not "as collected" (i.e., aggregated data like average or sum).
                if ((options & BACKEND_SOURCE_BITS) == BACKEND_SOURCE_DATA_AVERAGE) {
                    // If 'average' data is requested, process units (e.g., "ms" -> "_ms").
                    // The processed units are appended to `metric_name_prefix_context` to form `metric_name_base`.
                    // Example: "netdata_context_ms"
                    prometheus_units_copy(processed_units_for_name, st->units, PROMETHEUS_ELEMENT_MAX);
                    snprintfz(metric_name_base, sizeof(metric_name_base), "%s%s", metric_name_prefix_context, processed_units_for_name);
                } else {
                    // For 'sum' or other non-average aggregated data, units are not typically part of the metric name.
                    // So, `metric_name_base` is just `prefix_context`.
                    snprintfz(metric_name_base, sizeof(metric_name_base), "%s", metric_name_prefix_context);
                }
            }

            // Output HELP comments for the chart if requested.
            // This is unlikely for typical Prometheus scraping configurations.
            if(unlikely(help))
                buffer_sprintf(wb, "\n# COMMENT %s chart \"%s\", context \"%s\", family \"%s\", units \"%s\"\n"
                               , (homogeneus)?"homogeneus":"heterogeneous" // Indicates if chart dimensions are uniform
                               , (names && st->name) ? st->name : st->id // Use chart name if available, else ID
                               , st->context // Original context string for comments
                               , st->family
                               , st->units   // Original units string for comments
                );

            // Iterate over each dimension (RRDDIM) in the current chart
            RRDDIM *rd;
            rrddim_foreach_read(rd, st) {
                // Process dimension only if it has ever collected any data.
                if(rd->collections_counter) {
                    // Prepare dimension string for labels: {dimension="dim_name", ...}
                    char dim_label_str[PROMETHEUS_ELEMENT_MAX + 1];
                    prometheus_label_copy(dim_label_str, (names && rd->name) ? rd->name : rd->id, PROMETHEUS_ELEMENT_MAX);
                    
                    // Prepare dimension name part for heterogeneous metric names (e.g., ..._context_DIMENSION_total).
                    // This is only needed if data is "as collected" and chart is heterogeneous.
                    char dim_name_part_str[PROMETHEUS_ELEMENT_MAX + 1] = "";
                    if (as_collected && !homogeneus) {
                        prometheus_name_copy(dim_name_part_str, (names && rd->name) ? rd->name : rd->id, PROMETHEUS_ELEMENT_MAX);
                    }

                    // Suffix for the metric name (e.g., "_total", "_average", "_sum").
                    const char *value_suffix = "";
                    // Prometheus metric type ("gauge", "counter").
                    const char *prom_type = "gauge"; // Default to gauge.
                    // Verb used in HELP comments to describe the value (e.g., "gives", "delta gives").
                    const char *prom_help_verb = "gives";

                    if (as_collected) { // Handling for "as collected" data.
                        // Determine if it's a counter type based on RRD algorithm.
                        if(rd->algorithm == RRD_ALGORITHM_INCREMENTAL ||
                           rd->algorithm == RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL) {
                            prom_type = "counter";
                            prom_help_verb = "delta gives";
                            value_suffix = "_total"; // Counters in Prometheus usually have a _total suffix.
                        }

                        if(homogeneus) { // Homogeneous: all dimensions share same metric name base, differ by label.
                            // Output HELP text for this dimension if requested (unlikely path).
                            if(unlikely(help))
                                buffer_sprintf(wb
                                               , "# COMMENT %s%s: chart \"%s\", context \"%s\", family \"%s\", dimension \"%s\", value * " COLLECTED_NUMBER_FORMAT " / " COLLECTED_NUMBER_FORMAT " %s %s (%s)\n"
                                               , metric_name_base // Base is `prefix_context`
                                               , value_suffix
                                               , (names && st->name) ? st->name : st->id
                                               , st->context
                                               , st->family
                                               , (names && rd->name) ? rd->name : rd->id // Original dimension name for comment
                                               , rd->multiplier
                                               , rd->divisor
                                               , prom_help_verb
                                               , st->units
                                               , prom_type
                                );

                            // Output TYPE comment if requested (unlikely path).
                            if(unlikely(types))
                                buffer_sprintf(wb, "# COMMENT TYPE %s%s %s\n"
                                               , metric_name_base // Base is `prefix_context`
                                               , value_suffix
                                               , prom_type
                                );

                            // Output the actual metric data point.
                            // Format: metric_name_base<value_suffix>{chart="...",family="...",dimension="..."<common_labels_suffix>} value timestamp
                            buffer_sprintf(wb
                                           , "%s%s{chart=\"%s\",family=\"%s\",dimension=\"%s\"%s} " COLLECTED_NUMBER_FORMAT " %llu\n"
                                           , metric_name_base // Base is `prefix_context`
                                           , value_suffix
                                           , chart_label_str
                                           , family_label_str
                                           , dim_label_str // Dimension name as a label
                                           , common_labels_suffix // e.g., instance label
                                           , rd->last_collected_value
                                           , timeval_msec(&rd->last_collected_time)
                            );
                        }
                        else { // Heterogeneous: each dimension forms a distinct metric name.
                            // Output HELP text if requested (unlikely path).
                            if(unlikely(help))
                                buffer_sprintf(wb
                                               , "# COMMENT %s_%s%s: chart \"%s\", context \"%s\", family \"%s\", dimension \"%s\", value * " COLLECTED_NUMBER_FORMAT " / " COLLECTED_NUMBER_FORMAT " %s %s (%s)\n"
                                               , metric_name_prefix_context // Base is `prefix_context`
                                               , dim_name_part_str          // Dimension name is part of the metric name
                                               , value_suffix
                                               , (names && st->name) ? st->name : st->id
                                               , st->context
                                               , st->family
                                               , (names && rd->name) ? rd->name : rd->id
                                               , rd->multiplier
                                               , rd->divisor
                                               , prom_help_verb
                                               , st->units
                                               , prom_type
                                );

                            // Output TYPE comment if requested (unlikely path).
                            if(unlikely(types))
                                buffer_sprintf(wb, "# COMMENT TYPE %s_%s%s %s\n"
                                               , metric_name_prefix_context // Base is `prefix_context`
                                               , dim_name_part_str          // Dimension name is part of the metric name
                                               , value_suffix
                                               , prom_type
                                );

                            // Output the actual metric data point.
                            // Format: metric_name_prefix_context_dim_name_part_str<value_suffix>{chart="...",family="..."<common_labels_suffix>} value timestamp
                            // Note: No 'dimension' label as it's part of the metric name here.
                            buffer_sprintf(wb
                                           , "%s_%s%s{chart=\"%s\",family=\"%s\"%s} " COLLECTED_NUMBER_FORMAT " %llu\n"
                                           , metric_name_prefix_context // `prefix_context`
                                           , dim_name_part_str          // Dimension name part
                                           , value_suffix
                                           , chart_label_str
                                           , family_label_str
                                           , common_labels_suffix // No dimension label for heterogeneous
                                           , rd->last_collected_value
                                           , timeval_msec(&rd->last_collected_time)
                            );
                        }
                    }
                    else { // Handling for aggregated data (not "as collected", e.g., average or sum).
                        time_t first_t = after, last_t = before; // Time range for aggregation.
                        // Calculate the aggregated value from stored data in the RRD archive.
                        calculated_number value = backend_calculate_value_from_stored_data(st, rd, after, before, options, &first_t, &last_t);

                        // Process only if the value is valid (not NaN or Inf). This is the likely case.
                        if(likely(!isnan(value) && !isinf(value))) {
                            if((options & BACKEND_SOURCE_BITS) == BACKEND_SOURCE_DATA_AVERAGE)
                                value_suffix = "_average";
                            else if((options & BACKEND_SOURCE_BITS) == BACKEND_SOURCE_DATA_SUM)
                                value_suffix = "_sum";
                            // prom_type is already "gauge" by default for aggregated values.

                            // Output HELP text if requested (unlikely path).
                            // `metric_name_base` here is `prefix_context` (for sum) or `prefix_context_units` (for average).
                            if (unlikely(help))
                                buffer_sprintf(wb, "# COMMENT %s%s: dimension \"%s\", value is %s, gauge, dt %llu to %llu inclusive\n"
                                               , metric_name_base
                                               , value_suffix
                                               , (names && rd->name) ? rd->name : rd->id
                                               , st->units // Original units for comment clarity
                                               , (unsigned long long)first_t
                                               , (unsigned long long)last_t
                                );

                            // Output TYPE comment if requested (unlikely path).
                            if (unlikely(types))
                                buffer_sprintf(wb, "# COMMENT TYPE %s%s gauge\n"
                                               , metric_name_base
                                               , value_suffix
                                );

                            // Output the actual metric data point for aggregated data.
                            // Format: metric_name_base<value_suffix>{chart="...",family="...",dimension="..."<common_labels_suffix>} value timestamp
                            buffer_sprintf(wb, "%s%s{chart=\"%s\",family=\"%s\",dimension=\"%s\"%s} " CALCULATED_NUMBER_FORMAT " %llu\n"
                                           , metric_name_base // `prefix_context` or `prefix_context_units`
                                           , value_suffix
                                           , chart_label_str
                                           , family_label_str
                                           , dim_label_str // Dimension as a label
                                           , common_labels_suffix
                                           , value
                                           , last_t * MSEC_PER_SEC // Timestamp is the end of the aggregation period.
                            );
                        }
                    }
                }
            }
            rrdset_unlock(st); // Release read lock on the chart.
        }
    }
    rrdhost_unlock(host); // Release read lock on the host.
}

/**
 * @brief Prepares parameters for Prometheus data collection, primarily the 'after' timestamp.
 *
 * This function determines the 'after' timestamp, which signifies the start time
 * for querying metrics for a specific Prometheus server. It uses `prometheus_server_last_access`
 * to retrieve (and update) the last time the specified 'server' scraped data.
 * If it's the first time this server is seen (or its state was lost), 'after' is set
 * to `now - backend_update_every` to fetch a recent window of data.
 * It also handles potential clock skew issues (where 'after' might be in the future)
 * and generates introductory help comments in the output buffer if 'help' is enabled.
 *
 * @param host The RRDHOST instance (used for hostname in help comments).
 * @param wb The BUFFER to write help comments to (if 'help' is true).
 * @param options Backend options, used to describe the data source mode in help comments.
 * @param server Identifier of the Prometheus server making the request.
 * @param now Current timestamp.
 * @param help Flag indicating if help comments should be generated.
 * @return The 'after' timestamp, defining the start of the time window for metric collection.
 */
static inline time_t prometheus_preparation(RRDHOST *host, BUFFER *wb, uint32_t options, const char *server, time_t now, int help) {
    // If server identifier is NULL or empty, use "default".
    // This is unlikely if server ident is properly passed by the caller but provides a fallback.
    if(unlikely(!server || !*server)) server = "default";

    // Get the last access time for this server, also updating its last access to 'now'.
    time_t after  = prometheus_server_last_access(server, now);

    int first_seen = 0;
    // If 'after' is 0, it means this server hasn't been seen before or its state was lost.
    // `prometheus_server_last_access` returns 0 for new servers.
    if(!after) { 
        // Set 'after' to fetch data for the default backend update period (typically recent data).
        after = now - backend_update_every;
        first_seen = 1; // Mark that this is the first time data is being fetched for this server (or since state loss).
    }

    // Clock skew check: 'after' should not be in the future relative to 'now'.
    // This is an unlikely error condition, possibly due to system clock adjustments.
    if(unlikely(after > now)) {
        error("Prometheus backend: Clock skew detected for server '%s'. 'after' (%lu) is greater than 'now' (%lu). Resetting 'after'.", server, after, now);
        after = now - backend_update_every; // Reset 'after' to a safe recent interval.
    }

    // Generate introductory help comments if requested (unlikely for typical Prometheus scraping).
    if(unlikely(help)) {
        int show_range = 1; // Whether to show the time range in comments. Default true.
        char *mode;         // Description of the data source mode (e.g., "as collected", "average").

        // Determine the data source mode string based on options.
        if((options & BACKEND_SOURCE_BITS) == BACKEND_SOURCE_DATA_AS_COLLECTED) {
            mode = "as collected";
            show_range = 0; // Time range is less relevant for "as collected" raw data.
        }
        else if((options & BACKEND_SOURCE_BITS) == BACKEND_SOURCE_DATA_AVERAGE)
            mode = "average";
        else if((options & BACKEND_SOURCE_BITS) == BACKEND_SOURCE_DATA_SUM)
            mode = "sum";
        else
            mode = "unknown";

        // Output initial comment block with information about the scrape.
        buffer_sprintf(wb, "# COMMENT netdata \"%s\" to %sprometheus \"%s\", source \"%s\", last seen %lu %s"
                , host->hostname
                , (first_seen)?"FIRST SEEN ":"" // Indicate if this is the first scrape for this server.
                , server
                , mode
                , (unsigned long)((first_seen)?0:(now - after)) // Seconds since last seen.
                , (first_seen)?"never":"seconds ago"
        );

        if(show_range) // Append time range if applicable.
            buffer_sprintf(wb, ", time range %lu to %lu", (unsigned long)after, (unsigned long)now);

        buffer_strcat(wb, "\n\n"); // Add newlines for separation.
    }

    return after; // Return the calculated 'after' timestamp.
}

/**
 * @brief Generates Prometheus metrics for a single host.
 *
 * This function is a wrapper that sets up the time window ('after', 'before') using
 * `prometheus_preparation` and then calls the core metric generation function
 * `rrd_stats_api_v1_charts_allmetrics_prometheus` for the specified host.
 *
 * @param host The RRDHOST to generate metrics for.
 * @param wb The BUFFER to write metrics into.
 * @param server Identifier of the Prometheus server.
 * @param prefix Prefix for metric names.
 * @param options Backend options.
 * @param help Flag for HELP comments.
 * @param types Flag for TYPE comments.
 * @param names Flag for using names vs. IDs.
 */
void rrd_stats_api_v1_charts_allmetrics_prometheus_single_host(RRDHOST *host, BUFFER *wb, const char *server, const char *prefix, uint32_t options, int help, int types, int names) {
    time_t before = now_realtime_sec(); // Current time, acts as the end of the query window ('now').

    // Determine the 'after' timestamp. This also updates the server's last access time.
    // 'after' defines the start of the time window for which to fetch metrics.
    time_t after = prometheus_preparation(host, wb, options, server, before, help);

    // Call the main function to generate metrics for this host. 'allhosts' is 0 (false).
    rrd_stats_api_v1_charts_allmetrics_prometheus(host, wb, prefix, options, after, before, 0, help, types, names);
}

/**
 * @brief Generates Prometheus metrics for all known hosts.
 *
 * This function iterates over all hosts monitored by Netdata. For each host, it calls
 * `rrd_stats_api_v1_charts_allmetrics_prometheus` to generate its metrics.
 * The 'after' timestamp is determined once by `prometheus_preparation` for the entire scrape cycle.
 *
 * @param host Unused directly, but `prometheus_main_host` is used in `prometheus_preparation`
 *             for context if help comments are generated. Iteration happens over all hosts.
 * @param wb The BUFFER to write metrics into.
 * @param server Identifier of the Prometheus server.
 * @param prefix Prefix for metric names.
 * @param options Backend options.
 * @param help Flag for HELP comments.
 * @param types Flag for TYPE comments.
 * @param names Flag for using names vs. IDs.
 */
void rrd_stats_api_v1_charts_allmetrics_prometheus_all_hosts(RRDHOST *host, BUFFER *wb, const char *server, const char *prefix, uint32_t options, int help, int types, int names) {
    time_t before = now_realtime_sec(); // Current time, end of the query window ('now').
    (void)host; // The 'host' parameter is not directly used here for iteration;
                // iteration is done via rrdhost_foreach_read.
                // However, prometheus_main_host is used by prometheus_preparation for context if help is enabled.

    // Determine the 'after' timestamp for all hosts based on the server's last access.
    // prometheus_main_host is passed for context in help comments.
    time_t after = prometheus_preparation(prometheus_main_host, wb, options, server, before, help);

    rrd_rdlock(); // Acquire global read lock for accessing RRD data across hosts.
    RRDHOST *current_host; // Variable to hold the host being processed in the loop.
    rrdhost_foreach_read(current_host) { // Iterate over all available hosts.
        // Call the main metric generation function for each host.
        // 'allhosts' is 1 (true) to indicate multi-host context (e.g., for instance labels).
        rrd_stats_api_v1_charts_allmetrics_prometheus(current_host, wb, prefix, options, after, before, 1, help, types, names);
    }
    rrd_unlock(); // Release global read lock.
}
