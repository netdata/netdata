#ifndef ODBC_BRIDGE_H
#define ODBC_BRIDGE_H

#include <stdint.h>
#include <stdbool.h>
#include <stddef.h>

// Error codes
#define ODBC_SUCCESS             0
#define ODBC_ERROR              -1
#define ODBC_NO_DATA           100
#define ODBC_ERROR_CONNECT     -10
#define ODBC_ERROR_QUERY       -20
#define ODBC_ERROR_STMT_RESET  -30
#define ODBC_ERROR_FETCH       -40

// Connection handle
typedef void* odbc_conn_t;

// Data type indicators
typedef enum {
    ODBC_TYPE_NULL = 0,
    ODBC_TYPE_INT64 = 1,
    ODBC_TYPE_DOUBLE = 2,
    ODBC_TYPE_STRING = 3,
    ODBC_TYPE_BINARY = 4
} odbc_data_type_t;

// Column metadata
typedef struct {
    char name[256];
    odbc_data_type_t type;
    int sql_type;
    size_t size;
    int precision;
    int scale;
    bool nullable;
} odbc_column_info_t;

// Value union for different data types
typedef struct {
    odbc_data_type_t type;
    bool is_null;
    union {
        int64_t int_val;
        double double_val;
        char* string_val;
        struct {
            void* data;
            size_t len;
        } binary_val;
    } data;
} odbc_value_t;

// Connection management
odbc_conn_t odbc_connect(const char* dsn, char* error_buf, int error_buf_size);
void odbc_disconnect(odbc_conn_t conn);
int odbc_is_connected(odbc_conn_t conn);

// Query execution modes
int odbc_prepare(odbc_conn_t conn, const char* query, char* error_buf, int error_buf_size);
int odbc_execute(odbc_conn_t conn, char* error_buf, int error_buf_size);
int odbc_execute_direct(odbc_conn_t conn, const char* query, char* error_buf, int error_buf_size);

// Statement management
int odbc_reset_statement(odbc_conn_t conn);
int odbc_close_cursor(odbc_conn_t conn);
int odbc_free_statement(odbc_conn_t conn);

// Result metadata
int odbc_get_column_count(odbc_conn_t conn);
int odbc_get_column_info(odbc_conn_t conn, int column_index, odbc_column_info_t* info);
int64_t odbc_get_row_count(odbc_conn_t conn); // Can return negative on AS400!

// Result fetching
int odbc_fetch_row(odbc_conn_t conn);
int odbc_get_value(odbc_conn_t conn, int column_index, odbc_value_t* value);
void odbc_free_value(odbc_value_t* value);

// Optimized bulk operations
int odbc_set_array_size(odbc_conn_t conn, int size);
int odbc_bind_column(odbc_conn_t conn, int column_index, void* buffer, size_t buffer_size);

// Error handling
const char* odbc_get_last_error(odbc_conn_t conn);
int odbc_get_sqlstate(odbc_conn_t conn, char* state, size_t state_size);

#endif // ODBC_BRIDGE_H