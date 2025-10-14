#include "bridge.h"
#include <sql.h>
#include <sqlext.h>
#include <stdlib.h>
#include <string.h>
#include <stdio.h>

// Connection structure with optimizations
typedef struct {
    SQLHENV env;
    SQLHDBC dbc;
    SQLHSTMT stmt;
    int connected;
    bool stmt_prepared;
    bool cursor_open;
    char last_error[SQL_MAX_MESSAGE_LENGTH];
    char last_sqlstate[6];
    
    // Optimization: reuse buffers
    SQLLEN* indicators;
    void** column_buffers;
    int column_count;
    size_t* buffer_sizes;
} odbc_connection;

// Helper function to extract ODBC errors
static void extract_error(const char* fn, SQLHANDLE handle, SQLSMALLINT type, 
                         char* error_buf, int buf_size, char* sqlstate) {
    SQLINTEGER i = 0;
    SQLINTEGER native;
    SQLCHAR state[7];
    SQLCHAR text[SQL_MAX_MESSAGE_LENGTH];
    SQLSMALLINT len;
    SQLRETURN ret;

    error_buf[0] = '\0';
    if (sqlstate) sqlstate[0] = '\0';

    snprintf(error_buf, buf_size, "%s: ", fn);
    int offset = strlen(error_buf);

    do {
        ret = SQLGetDiagRec(type, handle, ++i, state, &native, text,
                            sizeof(text), &len);
        if (SQL_SUCCEEDED(ret)) {
            if (i == 1 && sqlstate) {
                strcpy(sqlstate, (char*)state);
            }
            snprintf(error_buf + offset, buf_size - offset,
                     "%s:%ld:%ld:%s ", state, (long)i, (long)native, text);
            offset = strlen(error_buf);
            if (offset >= buf_size - 1) break;
        }
    } while (ret == SQL_SUCCESS);
}

// Convert SQL type to our type enum
static odbc_data_type_t sql_type_to_odbc_type(SQLSMALLINT sql_type) {
    switch (sql_type) {
        case SQL_SMALLINT:
        case SQL_INTEGER:
        case SQL_BIGINT:
        case SQL_TINYINT:
            return ODBC_TYPE_INT64;
            
        case SQL_FLOAT:
        case SQL_REAL:
        case SQL_DOUBLE:
        case SQL_DECIMAL:
        case SQL_NUMERIC:
            return ODBC_TYPE_DOUBLE;
            
        case SQL_CHAR:
        case SQL_VARCHAR:
        case SQL_LONGVARCHAR:
        case SQL_WCHAR:
        case SQL_WVARCHAR:
        case SQL_WLONGVARCHAR:
            return ODBC_TYPE_STRING;
            
        case SQL_BINARY:
        case SQL_VARBINARY:
        case SQL_LONGVARBINARY:
            return ODBC_TYPE_BINARY;
            
        default:
            return ODBC_TYPE_STRING; // Default to string
    }
}

// Connect to database
odbc_conn_t odbc_connect(const char* dsn, char* error_buf, int error_buf_size) {
    odbc_connection* conn = (odbc_connection*)calloc(1, sizeof(odbc_connection));
    if (!conn) {
        snprintf(error_buf, error_buf_size, "Memory allocation failed");
        return NULL;
    }

    SQLRETURN ret;

    // Allocate environment handle
    ret = SQLAllocHandle(SQL_HANDLE_ENV, SQL_NULL_HANDLE, &conn->env);
    if (!SQL_SUCCEEDED(ret)) {
        snprintf(error_buf, error_buf_size, "Failed to allocate environment handle");
        free(conn);
        return NULL;
    }

    // Set ODBC version
    ret = SQLSetEnvAttr(conn->env, SQL_ATTR_ODBC_VERSION, (void*)SQL_OV_ODBC3, 0);
    if (!SQL_SUCCEEDED(ret)) {
        extract_error("SQLSetEnvAttr", conn->env, SQL_HANDLE_ENV, error_buf, error_buf_size, NULL);
        SQLFreeHandle(SQL_HANDLE_ENV, conn->env);
        free(conn);
        return NULL;
    }

    // Allocate connection handle
    ret = SQLAllocHandle(SQL_HANDLE_DBC, conn->env, &conn->dbc);
    if (!SQL_SUCCEEDED(ret)) {
        extract_error("SQLAllocHandle(DBC)", conn->env, SQL_HANDLE_ENV, error_buf, error_buf_size, NULL);
        SQLFreeHandle(SQL_HANDLE_ENV, conn->env);
        free(conn);
        return NULL;
    }

    // Set connection attributes for better performance
    SQLSetConnectAttr(conn->dbc, SQL_ATTR_AUTOCOMMIT, (SQLPOINTER)SQL_AUTOCOMMIT_ON, 0);

    // Connect
    SQLCHAR outstr[1024];
    SQLSMALLINT outstrlen;
    ret = SQLDriverConnect(conn->dbc, NULL, (SQLCHAR*)dsn, SQL_NTS,
                           outstr, sizeof(outstr), &outstrlen,
                           SQL_DRIVER_NOPROMPT);
    
    if (!SQL_SUCCEEDED(ret)) {
        extract_error("SQLDriverConnect", conn->dbc, SQL_HANDLE_DBC, error_buf, error_buf_size, NULL);
        SQLFreeHandle(SQL_HANDLE_DBC, conn->dbc);
        SQLFreeHandle(SQL_HANDLE_ENV, conn->env);
        free(conn);
        return NULL;
    }

    // Pre-allocate statement handle for reuse
    ret = SQLAllocHandle(SQL_HANDLE_STMT, conn->dbc, &conn->stmt);
    if (!SQL_SUCCEEDED(ret)) {
        extract_error("SQLAllocHandle(STMT)", conn->dbc, SQL_HANDLE_DBC, error_buf, error_buf_size, NULL);
        SQLDisconnect(conn->dbc);
        SQLFreeHandle(SQL_HANDLE_DBC, conn->dbc);
        SQLFreeHandle(SQL_HANDLE_ENV, conn->env);
        free(conn);
        return NULL;
    }

    conn->connected = 1;
    return (odbc_conn_t)conn;
}

// Prepare a statement for execution
int odbc_prepare(odbc_conn_t conn_handle, const char* query, char* error_buf, int error_buf_size) {
    if (!conn_handle) return ODBC_ERROR_CONNECT;
    
    odbc_connection* conn = (odbc_connection*)conn_handle;
    
    // Reset statement if needed
    if (conn->stmt_prepared || conn->cursor_open) {
        odbc_reset_statement(conn_handle);
    }
    
    SQLRETURN ret = SQLPrepare(conn->stmt, (SQLCHAR*)query, SQL_NTS);
    if (!SQL_SUCCEEDED(ret)) {
        extract_error("SQLPrepare", conn->stmt, SQL_HANDLE_STMT, error_buf, error_buf_size, conn->last_sqlstate);
        return ODBC_ERROR_QUERY;
    }
    
    conn->stmt_prepared = true;
    return ODBC_SUCCESS;
}

// Execute a prepared statement
int odbc_execute(odbc_conn_t conn_handle, char* error_buf, int error_buf_size) {
    if (!conn_handle) return ODBC_ERROR_CONNECT;
    
    odbc_connection* conn = (odbc_connection*)conn_handle;
    
    if (!conn->stmt_prepared) {
        snprintf(error_buf, error_buf_size, "No statement prepared");
        return ODBC_ERROR_QUERY;
    }
    
    SQLRETURN ret = SQLExecute(conn->stmt);
    if (!SQL_SUCCEEDED(ret)) {
        extract_error("SQLExecute", conn->stmt, SQL_HANDLE_STMT, error_buf, error_buf_size, conn->last_sqlstate);
        // Reset on error to prevent SQL0519
        odbc_reset_statement(conn_handle);
        return ODBC_ERROR_QUERY;
    }
    
    conn->cursor_open = true;
    return ODBC_SUCCESS;
}

// Execute query directly (no prepare)
int odbc_execute_direct(odbc_conn_t conn_handle, const char* query, char* error_buf, int error_buf_size) {
    if (!conn_handle) return ODBC_ERROR_CONNECT;
    
    odbc_connection* conn = (odbc_connection*)conn_handle;
    
    // Reset statement if needed
    if (conn->stmt_prepared || conn->cursor_open) {
        odbc_reset_statement(conn_handle);
    }
    
    SQLRETURN ret = SQLExecDirect(conn->stmt, (SQLCHAR*)query, SQL_NTS);
    if (!SQL_SUCCEEDED(ret)) {
        extract_error("SQLExecDirect", conn->stmt, SQL_HANDLE_STMT, error_buf, error_buf_size, conn->last_sqlstate);
        // Reset on error to prevent SQL0519
        odbc_reset_statement(conn_handle);
        return ODBC_ERROR_QUERY;
    }
    
    conn->cursor_open = true;
    return ODBC_SUCCESS;
}

// Get row count - can be negative on AS400!
int64_t odbc_get_row_count(odbc_conn_t conn_handle) {
    if (!conn_handle) return 0;
    
    odbc_connection* conn = (odbc_connection*)conn_handle;
    SQLLEN row_count = 0;
    
    SQLRETURN ret = SQLRowCount(conn->stmt, &row_count);
    if (!SQL_SUCCEEDED(ret)) {
        return 0;
    }
    
    // AS400 can return negative row counts!
    return (int64_t)row_count;
}

// Get column count
int odbc_get_column_count(odbc_conn_t conn_handle) {
    if (!conn_handle) return 0;
    
    odbc_connection* conn = (odbc_connection*)conn_handle;
    SQLSMALLINT column_count = 0;
    
    SQLRETURN ret = SQLNumResultCols(conn->stmt, &column_count);
    if (!SQL_SUCCEEDED(ret)) {
        return 0;
    }
    
    // Allocate column buffers if needed
    if (column_count > 0 && column_count != conn->column_count) {
        // Free old buffers
        if (conn->column_buffers) {
            for (int i = 0; i < conn->column_count; i++) {
                free(conn->column_buffers[i]);
            }
            free(conn->column_buffers);
            free(conn->indicators);
            free(conn->buffer_sizes);
        }
        
        // Allocate new buffers
        conn->column_count = column_count;
        conn->column_buffers = calloc(column_count, sizeof(void*));
        conn->indicators = calloc(column_count, sizeof(SQLLEN));
        conn->buffer_sizes = calloc(column_count, sizeof(size_t));
    }
    
    return column_count;
}

// Get column metadata
int odbc_get_column_info(odbc_conn_t conn_handle, int column_index, odbc_column_info_t* info) {
    if (!conn_handle || !info) return ODBC_ERROR;
    
    odbc_connection* conn = (odbc_connection*)conn_handle;
    
    SQLSMALLINT name_length;
    SQLSMALLINT data_type;
    SQLULEN column_size;
    SQLSMALLINT decimal_digits;
    SQLSMALLINT nullable;
    
    SQLRETURN ret = SQLDescribeCol(conn->stmt, column_index + 1,
                                    (SQLCHAR*)info->name, sizeof(info->name),
                                    &name_length, &data_type, &column_size,
                                    &decimal_digits, &nullable);
    
    if (!SQL_SUCCEEDED(ret)) {
        return ODBC_ERROR;
    }
    
    info->sql_type = data_type;
    info->type = sql_type_to_odbc_type(data_type);
    info->size = column_size;
    info->scale = decimal_digits;
    info->nullable = (nullable == SQL_NULLABLE);
    
    // For numeric types, get precision
    if (info->type == ODBC_TYPE_DOUBLE) {
        info->precision = column_size;
    }
    
    return ODBC_SUCCESS;
}

// Fetch next row
int odbc_fetch_row(odbc_conn_t conn_handle) {
    if (!conn_handle) return ODBC_ERROR;
    
    odbc_connection* conn = (odbc_connection*)conn_handle;
    if (!conn->cursor_open) return ODBC_ERROR;
    
    SQLRETURN ret = SQLFetch(conn->stmt);
    if (ret == SQL_NO_DATA) {
        return ODBC_NO_DATA;
    } else if (SQL_SUCCEEDED(ret)) {
        return ODBC_SUCCESS;
    } else {
        extract_error("SQLFetch", conn->stmt, SQL_HANDLE_STMT, 
                      conn->last_error, sizeof(conn->last_error), NULL);
        return ODBC_ERROR_FETCH;
    }
}

// Get value with proper type handling
int odbc_get_value(odbc_conn_t conn_handle, int column_index, odbc_value_t* value) {
    if (!conn_handle || !value) return ODBC_ERROR;
    
    odbc_connection* conn = (odbc_connection*)conn_handle;
    
    // First get column info to know the type
    odbc_column_info_t info;
    if (odbc_get_column_info(conn_handle, column_index, &info) != ODBC_SUCCESS) {
        return ODBC_ERROR;
    }
    
    SQLLEN indicator;
    SQLRETURN ret;
    
    // Handle different data types
    switch (info.type) {
        case ODBC_TYPE_INT64: {
            SQLBIGINT int_val = 0;
            ret = SQLGetData(conn->stmt, column_index + 1, SQL_C_SBIGINT,
                             &int_val, sizeof(int_val), &indicator);
            if (SQL_SUCCEEDED(ret)) {
                if (indicator == SQL_NULL_DATA) {
                    value->is_null = true;
                } else {
                    value->type = ODBC_TYPE_INT64;
                    value->is_null = false;
                    value->data.int_val = int_val;
                }
                return ODBC_SUCCESS;
            }
            break;
        }
        
        case ODBC_TYPE_DOUBLE: {
            double double_val = 0.0;
            ret = SQLGetData(conn->stmt, column_index + 1, SQL_C_DOUBLE,
                             &double_val, sizeof(double_val), &indicator);
            if (SQL_SUCCEEDED(ret)) {
                if (indicator == SQL_NULL_DATA) {
                    value->is_null = true;
                } else {
                    value->type = ODBC_TYPE_DOUBLE;
                    value->is_null = false;
                    value->data.double_val = double_val;
                }
                return ODBC_SUCCESS;
            }
            break;
        }
        
        case ODBC_TYPE_STRING:
        default: {
            // First call to get the required buffer size
            char dummy[1];
            SQLLEN str_len_or_ind;
            
            ret = SQLGetData(conn->stmt, column_index + 1, SQL_C_CHAR,
                             dummy, 0, &str_len_or_ind);
            
            if (str_len_or_ind == SQL_NULL_DATA) {
                value->is_null = true;
                return ODBC_SUCCESS;
            }
            
            // Allocate buffer and get the actual data
            size_t buffer_size = (str_len_or_ind > 0) ? str_len_or_ind + 1 : 4096;
            char* buffer = malloc(buffer_size);
            
            ret = SQLGetData(conn->stmt, column_index + 1, SQL_C_CHAR,
                             buffer, buffer_size, &indicator);
            
            if (SQL_SUCCEEDED(ret)) {
                if (indicator == SQL_NULL_DATA) {
                    value->is_null = true;
                    free(buffer);
                } else {
                    value->type = ODBC_TYPE_STRING;
                    value->is_null = false;
                    value->data.string_val = buffer;
                }
                return ODBC_SUCCESS;
            } else {
                free(buffer);
            }
            break;
        }
    }
    
    return ODBC_ERROR;
}

// Free allocated value
void odbc_free_value(odbc_value_t* value) {
    if (!value) return;
    
    if (!value->is_null) {
        switch (value->type) {
            case ODBC_TYPE_STRING:
                if (value->data.string_val) {
                    free(value->data.string_val);
                    value->data.string_val = NULL;
                }
                break;
            case ODBC_TYPE_BINARY:
                if (value->data.binary_val.data) {
                    free(value->data.binary_val.data);
                    value->data.binary_val.data = NULL;
                }
                break;
            default:
                break;
        }
    }
}

// Reset statement for reuse
int odbc_reset_statement(odbc_conn_t conn_handle) {
    if (!conn_handle) return ODBC_ERROR;
    
    odbc_connection* conn = (odbc_connection*)conn_handle;
    
    // Close cursor if open
    if (conn->cursor_open) {
        SQLCloseCursor(conn->stmt);
        conn->cursor_open = false;
    }
    
    // Reset parameters if prepared
    if (conn->stmt_prepared) {
        SQLFreeStmt(conn->stmt, SQL_RESET_PARAMS);
        SQLFreeStmt(conn->stmt, SQL_UNBIND);
        conn->stmt_prepared = false;
    }
    
    return ODBC_SUCCESS;
}

// Close cursor only
int odbc_close_cursor(odbc_conn_t conn_handle) {
    if (!conn_handle) return ODBC_ERROR;
    
    odbc_connection* conn = (odbc_connection*)conn_handle;
    
    if (conn->cursor_open) {
        SQLRETURN ret = SQLCloseCursor(conn->stmt);
        if (SQL_SUCCEEDED(ret)) {
            conn->cursor_open = false;
            return ODBC_SUCCESS;
        }
    }
    
    return ODBC_SUCCESS;
}

// Free statement completely
int odbc_free_statement(odbc_conn_t conn_handle) {
    if (!conn_handle) return ODBC_ERROR;
    
    odbc_connection* conn = (odbc_connection*)conn_handle;
    
    if (conn->stmt) {
        SQLFreeHandle(SQL_HANDLE_STMT, conn->stmt);
        conn->stmt = NULL;
        conn->stmt_prepared = false;
        conn->cursor_open = false;
        
        // Allocate new statement for next use
        SQLRETURN ret = SQLAllocHandle(SQL_HANDLE_STMT, conn->dbc, &conn->stmt);
        if (!SQL_SUCCEEDED(ret)) {
            return ODBC_ERROR;
        }
    }
    
    return ODBC_SUCCESS;
}

// Disconnect and cleanup
void odbc_disconnect(odbc_conn_t conn_handle) {
    if (!conn_handle) return;
    
    odbc_connection* conn = (odbc_connection*)conn_handle;
    
    // Free column buffers
    if (conn->column_buffers) {
        for (int i = 0; i < conn->column_count; i++) {
            free(conn->column_buffers[i]);
        }
        free(conn->column_buffers);
        free(conn->indicators);
        free(conn->buffer_sizes);
    }
    
    if (conn->stmt) {
        SQLFreeHandle(SQL_HANDLE_STMT, conn->stmt);
    }
    
    if (conn->connected && conn->dbc) {
        SQLDisconnect(conn->dbc);
    }
    
    if (conn->dbc) {
        SQLFreeHandle(SQL_HANDLE_DBC, conn->dbc);
    }
    
    if (conn->env) {
        SQLFreeHandle(SQL_HANDLE_ENV, conn->env);
    }
    
    free(conn);
}

int64_t odbc_value_get_int64(const odbc_value_t* value) {
    if (!value) {
        return 0;
    }
    return value->data.int_val;
}

double odbc_value_get_double(const odbc_value_t* value) {
    if (!value) {
        return 0.0;
    }
    return value->data.double_val;
}

const char* odbc_value_get_string(const odbc_value_t* value) {
    if (!value) {
        return NULL;
    }
    return value->data.string_val;
}

// Check connection status
int odbc_is_connected(odbc_conn_t conn_handle) {
    if (!conn_handle) return 0;
    odbc_connection* conn = (odbc_connection*)conn_handle;
    return conn->connected;
}

// Get last error
const char* odbc_get_last_error(odbc_conn_t conn_handle) {
    if (!conn_handle) return "Invalid connection handle";
    odbc_connection* conn = (odbc_connection*)conn_handle;
    return conn->last_error;
}

// Get SQLSTATE
int odbc_get_sqlstate(odbc_conn_t conn_handle, char* state, size_t state_size) {
    if (!conn_handle || !state) return ODBC_ERROR;
    
    odbc_connection* conn = (odbc_connection*)conn_handle;
    strncpy(state, conn->last_sqlstate, state_size - 1);
    state[state_size - 1] = '\0';
    
    return ODBC_SUCCESS;
}
