// SPDX-License-Identifier: GPL-3.0-or-later

/*
** 2016-06-29
**
** The author disclaims copyright to this source code.  In place of
** a legal notice, here is a blessing:
**
**    May you do good and not evil.
**    May you find forgiveness for yourself and forgive others.
**    May you share freely, never taking more than you give.
**
*************************************************************************
**
** This file demonstrates how to create a table-valued-function that
** returns the values in a C-language array.
** Examples:
**
**      SELECT * FROM carray($ptr,5)
**
** The query above returns 5 integers contained in a C-language array
** at the address $ptr.  $ptr is a pointer to the array of integers.
** The pointer value must be assigned to $ptr using the
** sqlite3_bind_pointer() interface with a pointer type of "carray".
** For example:
**
**    static int aX[] = { 53, 9, 17, 2231, 4, 99 };
**    int i = sqlite3_bind_parameter_index(pStmt, "$ptr");
**    sqlite3_bind_pointer(pStmt, i, aX, "carray", 0);
**
** There is an optional third parameter to determine the datatype of
** the C-language array.  Allowed values of the third parameter are
** 'int32', 'int64', 'double', 'char*'.  Example:
**
**      SELECT * FROM carray($ptr,10,'char*');
**
** The default value of the third parameter is 'int32'.
**
** HOW IT WORKS
**
** The carray "function" is really a virtual table with the
** following schema:
**
**     CREATE TABLE carray(
**       value,
**       pointer HIDDEN,
**       count HIDDEN,
**       ctype TEXT HIDDEN
**     );
**
** If the hidden columns "pointer" and "count" are unconstrained, then
** the virtual table has no rows.  Otherwise, the virtual table interprets
** the integer value of "pointer" as a pointer to the array and "count"
** as the number of elements in the array.  The virtual table steps through
** the array, element by element.
*/

/*
#include "sqlite3ext.h"
SQLITE_EXTENSION_INIT1
#include <assert.h>
#include <string.h>

#ifndef SQLITE_OMIT_VIRTUALTABLE


** Allowed datatypes
*/
#define CARRAY_INT32    0
#define CARRAY_INT64    1
#define CARRAY_DOUBLE   2
#define CARRAY_TEXT     3

///*
//** Names of types
//*/
//static const char *azType[] = { "int32", "int64", "double", "char*" };
//
//
///* carray_cursor is a subclass of sqlite3_vtab_cursor which will
//** serve as the underlying representation of a cursor that scans
//** over rows of the result
//*/
//typedef struct carray_cursor carray_cursor;
//struct carray_cursor {
//    sqlite3_vtab_cursor base;  /* Base class - must be first */
//    sqlite3_int64 iRowid;      /* The rowid */
//    void *pPtr;                /* Pointer to the array of values */
//    sqlite3_int64 iCnt;        /* Number of integers in the array */
//    unsigned char eType;       /* One of the CARRAY_type values */
//};
//
///*
//** The carrayConnect() method is invoked to create a new
//** carray_vtab that describes the carray virtual table.
//**
//** Think of this routine as the constructor for carray_vtab objects.
//**
//** All this routine needs to do is:
//**
//**    (1) Allocate the carray_vtab object and initialize all fields.
//**
//**    (2) Tell SQLite (via the sqlite3_declare_vtab() interface) what the
//**        result set of queries against carray will look like.
//*/
//static int carrayConnect(
//    sqlite3 *db,
//    void *pAux,
//    int argc, const char *const*argv,
//    sqlite3_vtab **ppVtab,
//    char **pzErr
//){
//    sqlite3_vtab *pNew;
//    int rc;
//
///* Column numbers */
//#define CARRAY_COLUMN_VALUE   0
//#define CARRAY_COLUMN_POINTER 1
//#define CARRAY_COLUMN_COUNT   2
//#define CARRAY_COLUMN_CTYPE   3
//
//    rc = sqlite3_declare_vtab(db,
//                              "CREATE TABLE x(value,pointer hidden,count hidden,ctype hidden)");
//    if( rc==SQLITE_OK ){
//        pNew = *ppVtab = sqlite3_malloc( sizeof(*pNew) );
//        if( pNew==0 ) return SQLITE_NOMEM;
//        memset(pNew, 0, sizeof(*pNew));
//    }
//    return rc;
//}
//
///*
//** This method is the destructor for carray_cursor objects.
//*/
//static int carrayDisconnect(sqlite3_vtab *pVtab){
//    sqlite3_free(pVtab);
//    return SQLITE_OK;
//}
//
///*
//** Constructor for a new carray_cursor object.
//*/
//static int carrayOpen(sqlite3_vtab *p, sqlite3_vtab_cursor **ppCursor){
//    carray_cursor *pCur;
//    pCur = sqlite3_malloc( sizeof(*pCur) );
//    if( pCur==0 ) return SQLITE_NOMEM;
//    memset(pCur, 0, sizeof(*pCur));
//    *ppCursor = &pCur->base;
//    return SQLITE_OK;
//}
//
///*
//** Destructor for a carray_cursor.
//*/
//static int carrayClose(sqlite3_vtab_cursor *cur){
//    sqlite3_free(cur);
//    return SQLITE_OK;
//}
//
//
///*
//** Advance a carray_cursor to its next row of output.
//*/
//static int carrayNext(sqlite3_vtab_cursor *cur){
//    carray_cursor *pCur = (carray_cursor*)cur;
//    pCur->iRowid++;
//    return SQLITE_OK;
//}
//
///*
//** Return values of columns for the row at which the carray_cursor
//** is currently pointing.
//*/
//static int carrayColumn(
//    sqlite3_vtab_cursor *cur,   /* The cursor */
//    sqlite3_context *ctx,       /* First argument to sqlite3_result_...() */
//    int i                       /* Which column to return */
//){
//    carray_cursor *pCur = (carray_cursor*)cur;
//    sqlite3_int64 x = 0;
//    switch( i ){
//        case CARRAY_COLUMN_POINTER:   return SQLITE_OK;
//        case CARRAY_COLUMN_COUNT:     x = pCur->iCnt;   break;
//        case CARRAY_COLUMN_CTYPE: {
//            sqlite3_result_text(ctx, azType[pCur->eType], -1, SQLITE_STATIC);
//            return SQLITE_OK;
//        }
//        default: {
//            switch( pCur->eType ){
//                case CARRAY_INT32: {
//                    int *p = (int*)pCur->pPtr;
//                    sqlite3_result_int(ctx, p[pCur->iRowid-1]);
//                    return SQLITE_OK;
//                }
//                case CARRAY_INT64: {
//                    sqlite3_int64 *p = (sqlite3_int64*)pCur->pPtr;
//                    sqlite3_result_int64(ctx, p[pCur->iRowid-1]);
//                    return SQLITE_OK;
//                }
//                case CARRAY_DOUBLE: {
//                    double *p = (double*)pCur->pPtr;
//                    sqlite3_result_double(ctx, p[pCur->iRowid-1]);
//                    return SQLITE_OK;
//                }
//                case CARRAY_TEXT: {
//                    const char **p = (const char**)pCur->pPtr;
//                    sqlite3_result_text(ctx, p[pCur->iRowid-1], -1, SQLITE_TRANSIENT);
//                    return SQLITE_OK;
//                }
//            }
//        }
//    }
//    sqlite3_result_int64(ctx, x);
//    return SQLITE_OK;
//}
//
///*
//** Return the rowid for the current row.  In this implementation, the
//** rowid is the same as the output value.
//*/
//static int carrayRowid(sqlite3_vtab_cursor *cur, sqlite_int64 *pRowid){
//    carray_cursor *pCur = (carray_cursor*)cur;
//    *pRowid = pCur->iRowid;
//    return SQLITE_OK;
//}
//
///*
//** Return TRUE if the cursor has been moved off of the last
//** row of output.
//*/
//static int carrayEof(sqlite3_vtab_cursor *cur){
//    carray_cursor *pCur = (carray_cursor*)cur;
//    return pCur->iRowid>pCur->iCnt;
//}
//
///*
//** This method is called to "rewind" the carray_cursor object back
//** to the first row of output.
//*/
//static int carrayFilter(
//    sqlite3_vtab_cursor *pVtabCursor,
//    int idxNum, const char *idxStr,
//    int argc, sqlite3_value **argv
//){
//    carray_cursor *pCur = (carray_cursor *)pVtabCursor;
//    if( idxNum ){
//        pCur->pPtr = sqlite3_value_pointer(argv[0], "carray");
//        pCur->iCnt = pCur->pPtr ? sqlite3_value_int64(argv[1]) : 0;
//        if( idxNum<3 ){
//            pCur->eType = CARRAY_INT32;
//        }else{
//            unsigned char i;
//            const char *zType = (const char*)sqlite3_value_text(argv[2]);
//            for(i=0; i<sizeof(azType)/sizeof(azType[0]); i++){
//                if( sqlite3_stricmp(zType, azType[i])==0 ) break;
//            }
//            if( i>=sizeof(azType)/sizeof(azType[0]) ){
//                pVtabCursor->pVtab->zErrMsg = sqlite3_mprintf(
//                    "unknown datatype: %Q", zType);
//                return SQLITE_ERROR;
//            }else{
//                pCur->eType = i;
//            }
//        }
//    }else{
//        pCur->pPtr = 0;
//        pCur->iCnt = 0;
//    }
//    pCur->iRowid = 1;
//    return SQLITE_OK;
//}
//
///*
//** SQLite will invoke this method one or more times while planning a query
//** that uses the carray virtual table.  This routine needs to create
//** a query plan for each invocation and compute an estimated cost for that
//** plan.
//**
//** In this implementation idxNum is used to represent the
//** query plan.  idxStr is unused.
//**
//** idxNum is 2 if the pointer= and count= constraints exist,
//** 3 if the ctype= constraint also exists, and is 0 otherwise.
//** If idxNum is 0, then carray becomes an empty table.
//*/
//static int carrayBestIndex(
//    sqlite3_vtab *tab,
//    sqlite3_index_info *pIdxInfo
//){
//    int i;                 /* Loop over constraints */
//    int ptrIdx = -1;       /* Index of the pointer= constraint, or -1 if none */
//    int cntIdx = -1;       /* Index of the count= constraint, or -1 if none */
//    int ctypeIdx = -1;     /* Index of the ctype= constraint, or -1 if none */
//
//    const struct sqlite3_index_constraint *pConstraint;
//    pConstraint = pIdxInfo->aConstraint;
//    for(i=0; i<pIdxInfo->nConstraint; i++, pConstraint++){
//        if( pConstraint->usable==0 ) continue;
//        if( pConstraint->op!=SQLITE_INDEX_CONSTRAINT_EQ ) continue;
//        switch( pConstraint->iColumn ){
//            case CARRAY_COLUMN_POINTER:
//                ptrIdx = i;
//                break;
//            case CARRAY_COLUMN_COUNT:
//                cntIdx = i;
//                break;
//            case CARRAY_COLUMN_CTYPE:
//                ctypeIdx = i;
//                break;
//        }
//    }
//    if( ptrIdx>=0 && cntIdx>=0 ){
//        pIdxInfo->aConstraintUsage[ptrIdx].argvIndex = 1;
//        pIdxInfo->aConstraintUsage[ptrIdx].omit = 1;
//        pIdxInfo->aConstraintUsage[cntIdx].argvIndex = 2;
//        pIdxInfo->aConstraintUsage[cntIdx].omit = 1;
//        pIdxInfo->estimatedCost = (double)1;
//        pIdxInfo->estimatedRows = 100;
//        pIdxInfo->idxNum = 2;
//        if( ctypeIdx>=0 ){
//            pIdxInfo->aConstraintUsage[ctypeIdx].argvIndex = 3;
//            pIdxInfo->aConstraintUsage[ctypeIdx].omit = 1;
//            pIdxInfo->idxNum = 3;
//        }
//    }else{
//        pIdxInfo->estimatedCost = (double)2147483647;
//        pIdxInfo->estimatedRows = 2147483647;
//        pIdxInfo->idxNum = 0;
//    }
//    return SQLITE_OK;
//}
//
///*
//** This following structure defines all the methods for the
//** carray virtual table.
//*/
//static sqlite3_module carrayModule = {
//    0,                         /* iVersion */
//    0,                         /* xCreate */
//    carrayConnect,             /* xConnect */
//    carrayBestIndex,           /* xBestIndex */
//    carrayDisconnect,          /* xDisconnect */
//    0,                         /* xDestroy */
//    carrayOpen,                /* xOpen - open a cursor */
//    carrayClose,               /* xClose - close a cursor */
//    carrayFilter,              /* xFilter - configure scan constraints */
//    carrayNext,                /* xNext - advance a cursor */
//    carrayEof,                 /* xEof - check for end of scan */
//    carrayColumn,              /* xColumn - read data */
//    carrayRowid,               /* xRowid - read data */
//    0,                         /* xUpdate */
//    0,                         /* xBegin */
//    0,                         /* xSync */
//    0,                         /* xCommit */
//    0,                         /* xRollback */
//    0,                         /* xFindMethod */
//    0,                         /* xRename */
//};
//
///*
//** For testing purpose in the TCL test harness, we need a method for
//** setting the pointer value.  The inttoptr(X) SQL function accomplishes
//** this.  Tcl script will bind an integer to X and the inttoptr() SQL
//** function will use sqlite3_result_pointer() to convert that integer into
//** a pointer.
//**
//** This is for testing on TCL only.
//*/
//#ifdef SQLITE_TEST
//static void inttoptrFunc(
//  sqlite3_context *context,
//  int argc,
//  sqlite3_value **argv
//){
//  void *p;
//  sqlite3_int64 i64;
//  i64 = sqlite3_value_int64(argv[0]);
//  if( sizeof(i64)==sizeof(p) ){
//    memcpy(&p, &i64, sizeof(p));
//  }else{
//    int i32 = i64 & 0xffffffff;
//    memcpy(&p, &i32, sizeof(p));
//  }
//  sqlite3_result_pointer(context, p, "carray", 0);
//}
//#endif /* SQLITE_TEST */
//
//#endif /* SQLITE_OMIT_VIRTUALTABLE */
//
//#ifdef _WIN32
//__declspec(dllexport)
//#endif
//int sqlite3_carray_init(
//    sqlite3 *db,
//    char **pzErrMsg,
//    const sqlite3_api_routines *pApi
//){
//    int rc = SQLITE_OK;
//    SQLITE_EXTENSION_INIT2(pApi);
//#ifndef SQLITE_OMIT_VIRTUALTABLE
//    rc = sqlite3_create_module(db, "carray", &carrayModule, 0);
//#ifdef SQLITE_TEST
//    if( rc==SQLITE_OK ){
//    rc = sqlite3_create_function(db, "inttoptr", 1, SQLITE_UTF8, 0,
//                                 inttoptrFunc, 0, 0);
//  }
//#endif /* SQLITE_TEST */
//#endif /* SQLITE_OMIT_VIRTUALTABLE */
//    return rc;
//}
