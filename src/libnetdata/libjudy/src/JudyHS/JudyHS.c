// @(#) $Revision: 4.1 $ $Source: /judy/src/JudyHS/JudyHS.c
//=======================================================================
//   Author Douglas L. Baskins, Dec 2003.
//   Permission to use this code is freely granted, provided that this
//   statement is retained.
//   email - doug@sourcejudy.com -or- dougbaskins@yahoo.com
//=======================================================================

#include <string.h>                     // for memcmp(), memcpy()

#include <Judy.h>                       // for JudyL* routines/macros

/*
   This routine is a very fast "string" version of an ADT that stores
   (JudyHSIns()), retrieves (JudyHSGet()), deletes (JudyHSDel()) and
   frees the entire ADT (JudyHSFreeArray()) strings.  It uses the "Judy
   arrays" JudyL() API as the main workhorse.  The length of the string
   is included in the calling parameters so that strings with embedded
   \0s can be used.  The string lengths can be from 0 bytes to whatever
   malloc() can handle (~2GB).

   Compile:
  
      cc -O JudyHS.c -c         needs to link with -lJudy (libJudy.a)
  
      Note: in gcc version 3.3.1, -O2 generates faster code than -O
      Note: in gcc version 3.3.2, -O3 generates faster code than -O2

   NOTES:

1) There may be some performance issues with 64 bit machines, because I
   have not characterized that it yet.

2) It appears that a modern CPU (>2Ghz) that the instruction times are
   much faster that a RAM access, so building up a word from bytes takes
   no longer that a whole word access.  I am taking advantage of this to
   make this code endian neutral.  A side effect of this is strings do
   not need to be aligned, nor tested to be on to a word boundry.  In
   older and in slow (RISC) machines, this may be a performance issue.
   I have given up trying to optimize for machines that have very slow
   mpy, mod, variable shifts and call returns.

3) JudyHS is very scalable from 1 string to billions (with enough RAM).
   The memory usage is also scales with population.  I have attempted to
   combine the best characteristics of JudyL arrays with Hashing methods
   and well designed modern processors (such as the 1.3Ghz Intel
   Centrino this is being written on).

   HOW JudyHS WORKS: ( 4[8] means 4 bytes in 32 bit machine and 8 in 64)

   A) A JudyL array is used to separate strings of equal lengths into
   their own structures (a different hash table is used for each length
   of string).  The additional time overhead is very near zero because
   of the CPU cache.  The space efficiency is improved because the
   length need not be stored with the string (ls_t).  The "JLHash" ADT
   in the test program "StringCompare" is verification of both these
   assumptions.

   B) A 32 bit hash value is produced from the string.  Many thanks to
   the Internet and the author (Bob Jenkins) for coming up with a very
   good and fast universal string hash.  Next the 32 bit hash number is
   used as an Index to another JudyL array.  Notice that one (1) JudyL
   array is used as a hash table per each string length.  If there are
   no hash collisions (normally) then the string is copied to a
   structure (ls_t) along with room for storing a Value.  A flag is
   added to the pointer to note it is pointing to a ls_t structure.
   Since the lengths of the strings are the same, there is no need to
   stored length of string in the ls_t structure.  This saves about a
   word per string of memory.

   C) When there is a hashing collision (very rare), a JudyL array is
   used to decode the next 4[8] bytes of the string.  That is, the next
   4[8] bytes of the string are used as the Index.  This process is
   repeated until the remaining string is unique.  The remaining string
   (if any) is stored in a (now smaller) ls_t structure.  If the
   remaining string is less or equal to 4[8] bytes, then the ls_t
   structure is not needed and the Value area in the JudyL array is
   used.  A compile option -DDONOTUSEHASH is available to test this
   structure without using hashing (only the JudyL tree is used).  This
   is equivalent to having all strings hashed to the same bucket.  The
   speed is still better than all other tree based ADTs I have tested.
   An added benefit of this is a very fast "hash collision" resolving.
   It could foil hackers that exploit the slow synonym (linked-list)
   collision handling property used with most hashing algorithms.  If
   this is not a necessary property, then a simpler ADT "JLHash" that is
   documented the the test program "StringCompare.c" may be used with a
   little loss of memory efficiency (because it includes the string
   length with the ls_t structure).  JudyHS was written to be the
   fastest, very scalable, memory efficient, general purpose string ADT
   possible.  (However, I would like to eat those words someday). (dlb)

*/

#ifdef EXAMPLE_CODE
#include <stdio.h>
#include <unistd.h>
#include <string.h>

#include <Judy.h>

//#include "JudyHS.h"                   // for Judy.h without JudyHS*()

// By Doug Baskins Apr 2004 - for JudyHS man page

#define MAXLINE 1000000                 /* max length of line */
char      Index[MAXLINE];               // string to check

int     // Usage:  CheckDupLines < file
main()
{
    Pvoid_t   PJArray = (PWord_t)NULL;  // Judy array.
    PWord_t   PValue;                   // ^ Judy array element.
    Word_t    Bytes;                    // size of JudyHS array.
    Word_t    LineNumb = 0;             // current line number
    Word_t    Dups = 0;                 // number of duplicate lines

    while (fgets(Index, MAXLINE, stdin) != (char *)NULL)
    {
        LineNumb++;                     // line number

//      store string into array
        JHSI(PValue, PJArray, Index, strlen(Index)); 
        if (*PValue)                    // check if duplicate
        {
            Dups++;                     // count duplicates
            printf("Duplicate lines %lu:%lu:%s", *PValue, LineNumb, Index);
        }
        else
        {
            *PValue = LineNumb;         // store Line number
        }
    }
    printf("%lu Duplicates, free JudyHS array of %lu Lines\n", 
                    Dups, LineNumb - Dups);
    JHSFA(Bytes, PJArray);              // free array
    printf("The JudyHS array allocated %lu bytes of memory\n", Bytes);
    return (0);
}
#endif // EXAMPLE_CODE

// Note:  Use JLAP_INVALID, which is non-zero, to mark pointers to a ls_t
// This makes it compatable with previous versions of JudyL()

#define IS_PLS(PLS)     (((Word_t) (PLS)) & JLAP_INVALID)
#define CLEAR_PLS(PLS)  (((Word_t) (PLS)) & (~JLAP_INVALID))
#define SET_PLS(PLS)    (((Word_t) (PLS)) | JLAP_INVALID)

#define WORDSIZE     (sizeof(Word_t))

// this is the struct used for "leaf" strings.  Note that
// the Value is followed by a "variable" length ls_String array.
//
typedef struct L_EAFSTRING
{
    Word_t    ls_Value;                 // Value area (cannot change size)
    uint8_t   ls_String[WORDSIZE];      // to fill out to a Word_t size
} ls_t     , *Pls_t;

#define LS_STRUCTOVD     (sizeof(ls_t) - WORDSIZE)

// Calculate size of ls_t including the string of length of LEN.
//
#define LS_WORDLEN(LEN)  (((LEN) + LS_STRUCTOVD + WORDSIZE - 1) / WORDSIZE)

// Copy from 0..4[8] bytes from string to a Word_t
// NOTE: the copy in in little-endian order to take advantage of improved 
// memory efficiency of JudyLIns() with smaller numbers
//
#define        COPYSTRING4toWORD(WORD,STR,LEN)          \
{                                                       \
    WORD = 0;                                           \
    switch(LEN)                                         \
    {                                                   \
    default:    /* four and greater */                  \
    case 4:                                             \
        WORD += (Word_t)(((uint8_t *)(STR))[3] << 24);  \
    case 3:                                             \
        WORD += (Word_t)(((uint8_t *)(STR))[2] << 16);  \
    case 2:                                             \
        WORD += (Word_t)(((uint8_t *)(STR))[1] <<  8);  \
    case 1:                                             \
        WORD += (Word_t)(((uint8_t *)(STR))[0]);        \
    case 0: break;                                      \
    }                                                   \
}

#ifdef JU_64BIT

// copy from 0..8 bytes from string to Word_t
//
#define        COPYSTRING8toWORD(WORD,STR,LEN)          \
{                                                       \
    WORD = 0UL;                                         \
    switch(LEN)                                         \
    {                                                   \
    default:    /* eight and greater */                 \
    case 8:                                             \
        WORD += ((Word_t)((uint8_t *)(STR))[7] << 56);  \
    case 7:                                             \
        WORD += ((Word_t)((uint8_t *)(STR))[6] << 48);  \
    case 6:                                             \
        WORD += ((Word_t)((uint8_t *)(STR))[5] << 40);  \
    case 5:                                             \
        WORD += ((Word_t)((uint8_t *)(STR))[4] << 32);  \
    case 4:                                             \
        WORD += ((Word_t)((uint8_t *)(STR))[3] << 24);  \
    case 3:                                             \
        WORD += ((Word_t)((uint8_t *)(STR))[2] << 16);  \
    case 2:                                             \
        WORD += ((Word_t)((uint8_t *)(STR))[1] <<  8);  \
    case 1:                                             \
        WORD += ((Word_t)((uint8_t *)(STR))[0]);        \
    case 0: break;                                      \
    }                                                   \
}

#define COPYSTRINGtoWORD COPYSTRING8toWORD

#else  // JU_32BIT

#define COPYSTRINGtoWORD COPYSTRING4toWORD

#endif // JU_32BIT

// set JError_t locally

#define JU_SET_ERRNO(PJERROR, JERRNO)           \
{                                               \
    if (PJERROR != (PJError_t) NULL)            \
    {                                           \
        if (JERRNO)                             \
            JU_ERRNO(PJError) = (JERRNO);       \
        JU_ERRID(PJERROR) = __LINE__;           \
    }                                           \
}

//=======================================================================
// This routine must hash string to 24..32 bits.  The "goodness" of
// the hash is not as important as its speed.
//=======================================================================

// hash to no more than 32 bits

// extern Word_t gHmask; for hash bits experiments

#define JUDYHASHSTR(HVALUE,STRING,LENGTH)       \
{                                               \
    uint8_t *p_ = (uint8_t *)(STRING);          \
    uint8_t *q_ = p_ + (LENGTH);                \
    uint32_t c_ = 0;                            \
    for (; p_ != q_; ++p_)                      \
    {                                           \
        c_ = (c_ * 31) + *p_;                   \
    }                                           \
/*  c_ &= gHmask;   see above   */              \
    (HVALUE) = c_;                              \
}

// Find String of Len in JudyHS structure, return pointer to associated Value

PPvoid_t
JudyHSGet(Pcvoid_t PArray,              // pointer (^) to structure
           void * Str,                  // pointer to string
           Word_t Len                   // length of string
    )
{
    uint8_t  *String = (uint8_t *)Str;
    PPvoid_t  PPValue;                  // pointer to Value
    Word_t    Index;                    // 4[8] bytes of String

    JLG(PPValue, PArray, Len);          // find hash table for strings of Len
    if (PPValue == (PPvoid_t) NULL)
        return ((PPvoid_t) NULL);       // no strings of this Len

//  check for caller error (null pointer)
//
    if ((String == (void *) NULL) && (Len != 0))
        return ((PPvoid_t) NULL);       // avoid null-pointer dereference

#ifndef DONOTUSEHASH
    if (Len > WORDSIZE)                 // Hash table not necessary with short
    {
        uint32_t  HValue;               // hash of input string
        JUDYHASHSTR(HValue, String, Len);       // hash to no more than 32 bits
        JLG(PPValue, *PPValue, (Word_t)HValue); // get ^ to hash bucket
        if (PPValue == (PPvoid_t) NULL)
            return ((PPvoid_t) NULL);   // no entry in Hash table
    }
#endif // DONOTUSEHASH

/*
  Each JudyL array decodes 4[8] bytes of the string.  Since the hash
  collisions occur very infrequently, the performance is not important.
  However, even if the Hash code is not used this method still is
  significantly faster than common tree methods (AVL, Red-Black, Splay,
  b-tree, etc..).  You can compare it yourself with #define DONOTUSEHASH
  1 or putting -DDONOTUSEHASH in the cc line.  Use the "StringCompare.c"
  code to compare (9Dec2003 dlb).
*/
    while (Len > WORDSIZE)              // traverse tree of JudyL arrays
    {
        if (IS_PLS(*PPValue))           // ^ to JudyL array or ls_t struct?
        {
            Pls_t     Pls;              // ls_t struct, termination of tree
            Pls = (Pls_t) CLEAR_PLS(*PPValue);  // remove flag from ^

//          if remaining string matches, return ^ to Value, else NULL

            if (memcmp(String, Pls->ls_String, Len) == 0)
                return ((PPvoid_t) (&(Pls->ls_Value)));
            else
                return ((PPvoid_t) NULL);       // string does not match
        }
        else
        {
            COPYSTRINGtoWORD(Index, String, WORDSIZE);

            JLG(PPValue, *PPValue, Index);      // decode next 4[8] bytes
            if (PPValue == (PPvoid_t) NULL)     // if NULL array, bail out
                return ((PPvoid_t) NULL);       // string does not match

            String += WORDSIZE;                 // advance
            Len -= WORDSIZE;
        }
    }

//  Get remaining 1..4[8] bytes left in string

    COPYSTRINGtoWORD(Index, String, Len);
    JLG(PPValue, *PPValue, Index);      // decode last 1-4[8] bytes
    return (PPValue);
}

// Add string to a tree of JudyL arrays (all lengths must be same)

static PPvoid_t
insStrJudyLTree(uint8_t * String,      // string to add to tree of JudyL arrays
                 Word_t Len,            // length of string
                 PPvoid_t PPValue,      // pointer to root pointer
                 PJError_t PJError      // for returning error info
    )
{
    Word_t    Index;                    // next 4[8] bytes of String

    while (Len > WORDSIZE)              // add to JudyL tree
    {
//      CASE 1, pointer is to a NULL, make a new ls_t leaf

        if (*PPValue == (Pvoid_t)NULL)
        {
            Pls_t     Pls;              // memory for a ls_t
            Pls = (Pls_t) JudyMalloc(LS_WORDLEN(Len));
            if (Pls == NULL)
            {
                JU_SET_ERRNO(PJError, JU_ERRNO_NOMEM);
                return (PPJERR);
            }
            Pls->ls_Value = 0;                          // clear Value word
            memcpy(Pls->ls_String, String, Len);        // copy to new struct
            *PPValue = (Pvoid_t)SET_PLS(Pls);           // mark pointer
            return ((PPvoid_t) (&Pls->ls_Value));       // return ^ to Value
        }                                               // no exit here
//      CASE 2: is a ls_t, free (and shorten), then decode into JudyL tree

        if (IS_PLS(*PPValue))                   // pointer to a ls_t? (leaf)
        {
            Pls_t     Pls;                      // ^ to ls_t
            uint8_t  *String0;                  // ^ to string in ls_t
            Word_t    Index0;                   // 4[8] bytes in string
            Word_t    FreeLen;                  // length of ls_t
            PPvoid_t  PPsplit;

            FreeLen = LS_WORDLEN(Len);          // length of ls_t

            Pls = (Pls_t) CLEAR_PLS(*PPValue);  // demangle ^ to ls_t
            String0 = Pls->ls_String;
            if (memcmp(String, String0, Len) == 0)      // check if match?
            {
                return ((PPvoid_t) (&Pls->ls_Value));   // yes, duplicate
            }

            *PPValue = NULL;            // clear ^ to ls_t and make JudyL

//          This do loop is technically not required, saves multiple JudyFree()
//          when storing already sorted strings into structure

            do                          // decode next 4[8] bytes of string
            {                           // with a JudyL array
//              Note: string0 is always aligned

                COPYSTRINGtoWORD(Index0, String0, WORDSIZE);
                String0 += WORDSIZE;
                COPYSTRINGtoWORD(Index, String, WORDSIZE);
                String += WORDSIZE;
                Len -= WORDSIZE;
                PPsplit = PPValue;      // save for split below
                PPValue = JudyLIns(PPValue, Index0, PJError);
                if (PPValue == PPJERR)
                {
                    JU_SET_ERRNO(PJError, 0);
                    return (PPJERR);
                }

            } while ((Index0 == Index) && (Len > WORDSIZE));

//          finish storing remainder of string that was in the ls_t

            PPValue = insStrJudyLTree(String0, Len, PPValue, PJError);
            if (PPValue == PPJERR)
            {
                return (PPJERR);
            }
//          copy old Value to Value in new struct

            *(PWord_t)PPValue = Pls->ls_Value;

//          free the string buffer (ls_t)

            JudyFree((Pvoid_t)Pls, FreeLen);
            PPValue = JudyLIns(PPsplit, Index, PJError);
            if (PPValue == PPJERR)
            {
                JU_SET_ERRNO(PJError, 0);
                return (PPValue);
            }

//          finish remainder of newly inserted string

            PPValue = insStrJudyLTree(String, Len, PPValue, PJError);
            return (PPValue);
        }                               // no exit here
//      CASE 3, more JudyL arrays, decode to next tree

        COPYSTRINGtoWORD(Index, String, WORDSIZE);
        Len -= WORDSIZE;
        String += WORDSIZE;

        PPValue = JudyLIns(PPValue, Index, PJError);    // next 4[8] bytes
        if (PPValue == PPJERR)
        {
            JU_SET_ERRNO(PJError, 0);
            return (PPValue);
        }
    }
//  this is done outside of loop so "Len" can be an unsigned number

    COPYSTRINGtoWORD(Index, String, Len);
    PPValue = JudyLIns(PPValue, Index, PJError);    // remaining 4[8] bytes

    return (PPValue);
}


// Insert string to JudyHS structure, return pointer to associated Value

PPvoid_t
JudyHSIns(PPvoid_t PPArray,             // ^ to JudyHashArray name
           void * Str,                  // pointer to string
           Word_t Len,                  // length of string
           PJError_t PJError            // optional, for returning error info
    )
{
    uint8_t * String = (uint8_t *)Str;
    PPvoid_t  PPValue;

//  string can only be NULL if Len is 0.

    if ((String == (uint8_t *) NULL) && (Len != 0UL))
    {
        JU_SET_ERRNO(PJError, JU_ERRNO_NULLPINDEX);
        return (PPJERR);
    }
    JLG(PPValue, *PPArray, Len);        // JudyL hash table for strings of Len
    if (PPValue == (PPvoid_t) NULL)     // make new if missing, (very rare)
    {
        PPValue = JudyLIns(PPArray, Len, PJError);
        if (PPValue == PPJERR)
        {
            JU_SET_ERRNO(PJError, 0);
            return (PPJERR);
        }
    }
#ifndef DONOTUSEHASH
    if (Len > WORDSIZE)
    {
        uint32_t  HValue;                       // hash of input string
        JUDYHASHSTR(HValue, String, Len);       // hash to no more than 32 bits
        PPValue = JudyLIns(PPValue, (Word_t)HValue, PJError);
        if (PPValue == PPJERR)
        {
            JU_SET_ERRNO(PJError, 0);
            return (PPJERR);
        }
    }
#endif // DONOTUSEHASH

    PPValue = insStrJudyLTree(String, Len, PPValue, PJError); // add string 
    return (PPValue);                   //  ^  to Value
}

// Delete string from tree of JudyL arrays (all Lens must be same)

static int
delStrJudyLTree(uint8_t * String,      // delete from tree of JudyL arrays
                 Word_t Len,            // length of string
                 PPvoid_t PPValue,      // ^ to hash bucket
                 PJError_t PJError      // for returning error info
    )
{
    PPvoid_t  PPValueN;                 // next pointer
    Word_t    Index;
    int       Ret;                      // -1=failed, 1=success, 2=quit del

    if (IS_PLS(*PPValue))               // is pointer to ls_t?
    {
        Pls_t     Pls;
        Pls = (Pls_t) CLEAR_PLS(*PPValue);      // demangle pointer
        JudyFree((Pvoid_t)Pls, LS_WORDLEN(Len));        // free the ls_t

        *PPValue = (Pvoid_t)NULL;       // clean pointer
        return (1);                     // successfully deleted
    }

    if (Len > WORDSIZE)                 // delete from JudyL tree, not leaf
    {
        COPYSTRINGtoWORD(Index, String, WORDSIZE);      // get Index
        JLG(PPValueN, *PPValue, Index); // get pointer to next JudyL array

        String += WORDSIZE;             // advance to next 4[8] bytes
        Len -= WORDSIZE;

        Ret = delStrJudyLTree(String, Len, PPValueN, PJError);
        if (Ret != 1) return(Ret);

        if (*PPValueN == (PPvoid_t) NULL)
        {
//          delete JudyL element from tree

            Ret = JudyLDel(PPValue, Index, PJError);
        }
    }
    else
    {
        COPYSTRINGtoWORD(Index, String, Len);   // get leaf element

//      delete last 1-4[8] bytes from leaf element

        Ret = JudyLDel(PPValue, Index, PJError); 
    }
    return (Ret);
}

// Delete string from JHS structure

int
JudyHSDel(PPvoid_t PPArray,             // ^ to JudyHashArray struct
           void * Str,                  // pointer to string
           Word_t Len,                  // length of string
           PJError_t PJError            // optional, for returning error info
    )
{
    uint8_t * String = (uint8_t *)Str;
    PPvoid_t  PPBucket, PPHtble;
    int       Ret;                      // return bool from Delete routine
#ifndef DONOTUSEHASH
    uint32_t  HValue = 0;               // hash value of input string
#endif // DONOTUSEHASH

    if (PPArray == NULL)
        return (0);                     // no pointer, return not found

//  This is a little slower than optimum method, but not much in new CPU
//  Verify that string is in the structure -- simplifies future assumptions

    if (JudyHSGet(*PPArray, String, Len) == (PPvoid_t) NULL)
        return (0);                     // string not found, return

//  string is in structure, so testing for absence is not necessary

    JLG(PPHtble, *PPArray, Len);        // JudyL hash table for strings of Len

#ifdef DONOTUSEHASH
    PPBucket = PPHtble;                 // simulate below code
#else  // USEHASH
    if (Len > WORDSIZE)
    {
        JUDYHASHSTR(HValue, String, Len);       // hash to no more than 32 bits

//  get pointer to hash bucket

        JLG(PPBucket, *PPHtble, (Word_t)HValue);
    }
    else
    {
        PPBucket = PPHtble;             // no bucket to JLGet
    }
#endif // USEHASH

// delete from JudyL tree
//
    Ret = delStrJudyLTree(String, Len, PPBucket, PJError);
    if (Ret != 1)
    {
        JU_SET_ERRNO(PJError, 0);
        return(-1);
    }
//  handle case of missing JudyL array from hash table and length table

    if (*PPBucket == (Pvoid_t)NULL)     // if JudyL tree gone
    {
#ifndef DONOTUSEHASH
        if (Len > WORDSIZE)
        {
//          delete entry in Hash table

            Ret = JudyLDel(PPHtble, (Word_t)HValue, PJError); 
            if (Ret != 1)
            {
                JU_SET_ERRNO(PJError, 0);
                return(-1);
            }
        }
#endif // USEHASH
        if (*PPHtble == (PPvoid_t) NULL)        // if Hash table gone
        {
//          delete entry from the String length table

            Ret = JudyLDel(PPArray, Len, PJError); 
            if (Ret != 1)
            {
                JU_SET_ERRNO(PJError, 0);
                return(-1);
            }
        }
    }
    return (1);                         // success
}

static Word_t
delJudyLTree(PPvoid_t PPValue,                 // ^ to JudyL root pointer
              Word_t Len,                       // length of string
              PJError_t PJError)                // for returning error info
{
    Word_t    bytes_freed = 0;                  // bytes freed at point
    Word_t    bytes_total = 0;                  // accumulated bytes freed
    PPvoid_t  PPValueN;

//  Pointer is to another tree of JudyL arrays or ls_t struct

    if (Len > WORDSIZE)                         // more depth to tree
    {
        Word_t NEntry;

//      Pointer is to a ls_t struct

        if (IS_PLS(*PPValue)) 
        {
            Pls_t   Pls;
            Word_t  freewords;

            freewords = LS_WORDLEN(Len);        // calculate length
            Pls = (Pls_t)CLEAR_PLS(*PPValue);   // demangle pointer

//        *PPValue = (Pvoid_t)NULL;               // clean pointer
           JudyFree((Pvoid_t)Pls, freewords);   // free the ls_t

            return(freewords * WORDSIZE);
        }
//      else
//      Walk all the entrys in the JudyL array

        NEntry = 0;                             // start at beginning
        for (PPValueN = JudyLFirst(*PPValue, &NEntry, PJError);
            (PPValueN != (PPvoid_t) NULL) && (PPValueN != PPJERR);
             PPValueN = JudyLNext(*PPValue, &NEntry, PJError))
        {
//          recurse to the next level in the tree of arrays

            bytes_freed = delJudyLTree(PPValueN, Len - WORDSIZE, PJError);
            if (bytes_freed == JERR) return(JERR);
            bytes_total += bytes_freed;
        }
        if (PPValueN == PPJERR) return(JERR);

//      now free this JudyL array

        bytes_freed = JudyLFreeArray(PPValue, PJError);
        if (bytes_freed == JERR) return(JERR);
        bytes_total += bytes_freed;

        return(bytes_total);  // return amount freed
    }
//  else

//  Pointer to simple JudyL array

    bytes_freed = JudyLFreeArray(PPValue, PJError);

    return(bytes_freed);
}


Word_t                                  // bytes freed
JudyHSFreeArray(PPvoid_t PPArray,       // ^ to JudyHashArray struct
           PJError_t PJError            // optional, for returning error info
    )
{
    Word_t    Len;                      // start at beginning
    Word_t    bytes_freed;              // bytes freed at this level.
    Word_t    bytes_total;              // bytes total at all levels.
    PPvoid_t  PPHtble;

    if (PPArray == NULL) 
        return (0);                     // no pointer, return none

//  Walk the string length table for subsidary hash structs
//  NOTE: This is necessary to determine the depth of the tree

    bytes_freed = 0; 
    bytes_total = 0;
    Len = 0;                            // walk to length table

    for (PPHtble  = JudyLFirst(*PPArray, &Len, PJError);
        (PPHtble != (PPvoid_t) NULL) && (PPHtble != PPJERR);
         PPHtble  = JudyLNext(*PPArray, &Len, PJError))
    {
        PPvoid_t PPValueH;

#ifndef DONOTUSEHASH
        if (Len > WORDSIZE)
        {
            Word_t HEntry = 0;              // walk the hash tables

            for (PPValueH  = JudyLFirst(*PPHtble, &HEntry, PJError);
                (PPValueH != (PPvoid_t) NULL) && (PPValueH != PPJERR);
                 PPValueH  = JudyLNext(*PPHtble, &HEntry, PJError))
            {
                bytes_freed = delJudyLTree(PPValueH, Len, PJError);
                if (bytes_freed == JERR) return(JERR);
                bytes_total += bytes_freed;
            }

            if (PPValueH == PPJERR) return(JERR);

//          free the Hash table for this length of string

            bytes_freed = JudyLFreeArray(PPHtble, PJError);
            if (bytes_freed == JERR) return(JERR);
            bytes_total += bytes_freed;
        }
        else
#endif // DONOTUSEHASH
        {
                PPValueH = PPHtble;     // simulate hash table

                bytes_freed = delJudyLTree(PPValueH, Len, PJError);
                if (bytes_freed == JERR) return(JERR);
                bytes_total += bytes_freed;
        }
    }
    if (PPHtble == PPJERR) return(JERR);

//  free the length table

    bytes_freed = JudyLFreeArray(PPArray, PJError);
    if (bytes_freed == JERR) return(JERR);

    bytes_total += bytes_freed;

    return(bytes_total);                // return bytes freed
}
