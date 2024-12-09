#ifndef _JUDY_PRIVATE_BRANCH_INCLUDED
#define _JUDY_PRIVATE_BRANCH_INCLUDED
// _________________
//
// Copyright (C) 2000 - 2002 Hewlett-Packard Company
//
// This program is free software; you can redistribute it and/or modify it
// under the term of the GNU Lesser General Public License as published by the
// Free Software Foundation; either version 2 of the License, or (at your
// option) any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE.  See the GNU Lesser General Public License
// for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with this program; if not, write to the Free Software Foundation,
// Inc., 59 Temple Place, Suite 330, Boston, MA  02111-1307  USA
// _________________

// @(#) $Revision: 1.2 $ $Source: /home/doug/judy-1.0.5_min/test/../src/JudyCommon/RCS/JudyPrivateBranch.h,v $
//
// Header file for all Judy sources, for global but private (non-exported)
// declarations specific to branch support.
//
// See also the "Judy Shop Manual" (try judy/doc/int/JudyShopManual.*).


// ****************************************************************************
// JUDY POINTER (JP) SUPPORT
// ****************************************************************************
//
// This "rich pointer" object is pivotal to Judy execution.
//
// JP CONTAINING OTHER THAN IMMEDIATE INDEXES:
//
// If the JP points to a linear or bitmap leaf, jp_DcdPopO contains the
// Population-1 in LSbs and Decode (Dcd) bytes in the MSBs.  (In practice the
// Decode bits are masked off while accessing the Pop0 bits.)
//
// The Decode Size, the number of Dcd bytes available, is encoded in jpo_Type.
// It can also be thought of as the number of states "skipped" in the SM, where
// each state decodes 8 bits = 1 byte.
//
// TBD:  Dont need two structures, except possibly to force jp_Type to highest
// address!
//
// Note:  The jpo_u union is not required by HP-UX or Linux but Win32 because
// the cl.exe compiler otherwise refuses to pack a bitfield (DcdPopO) with
// anything else, even with the -Zp option.  This is pretty ugly, but
// fortunately portable, and its all hide-able by macros (see below).

typedef struct J_UDY_POINTER_OTHERS      // JPO.
        {
            Word_t      j_po_Addr;       // first word:  Pjp_t, Word_t, etc.
            union {
                Word_t  j_po_Addr1;
                uint8_t j_po_DcdP0[sizeof(Word_t) - 1];
                uint8_t j_po_Bytes[sizeof(Word_t)];     // last byte = jp_Type.
            } jpo_u;
        } jpo_t;


// JP CONTAINING IMMEDIATE INDEXES:
//
// j_pi_1Index[] plus j_pi_LIndex[] together hold as many N-byte (1..3-byte
// [1..7-byte]) Indexes as will fit in sizeof(jpi_t) less 1 byte for j_pi_Type
// (that is, 7..1 [15..1] Indexes).
//
// For Judy1, j_pi_1Index[] is used and j_pi_LIndex[] is not used.
// For JudyL, j_pi_LIndex[] is used and j_pi_1Index[] is not used.
//
// Note:  Actually when Pop1 = 1, jpi_t is not used, and the least bytes of the
// single Index are stored in j_po_DcdPopO, for both Judy1 and JudyL, so for
// JudyL the j_po_Addr field can hold the target value.
//
// TBD:  Revise this structure to not overload j_po_DcdPopO this way?  The
// current arrangement works, its just confusing.

typedef struct _JUDY_POINTER_IMMEDL  
        {
            Word_t  j_pL_Addr;
            uint8_t j_pL_LIndex[sizeof(Word_t) - 1];    // see above.
            uint8_t j_pL_Type;
        } jpL_t;

typedef struct _JUDY_POINTER_IMMED1   
        {
            uint8_t j_p1_1Index[(2 * sizeof(Word_t)) - 1];
            uint8_t j_p1_Type;
        } jp1_t;

// UNION OF JP TYPES:
//
// A branch is an array of cJU_BRANCHUNUMJPS (256) of this object, or an
// alternate data type such as:  A linear branch which is a list of 2..7 JPs,
// or a bitmap branch which contains 8 lists of 0..32 JPs.  JPs reside only in
// branches of a Judy SM.

typedef union J_UDY_POINTER             // JP.
        {
            jpo_t j_po;                 // other than immediate indexes.
            jpL_t j_pL;                 // immediate indexes.
            jp1_t j_p1;                 // immediate indexes.
        } jp_t, *Pjp_t;

// For coding convenience:
//
// Note, jp_Type has the same bits in jpo_t jpL_t and jp1_t.

#define jp_1Index  j_p1.j_p1_1Index     // for storing Indexes in first  word.
#define jp_LIndex  j_pL.j_pL_LIndex     // for storing Indexes in second word.
#define jp_Addr    j_po.j_po_Addr
#define jp_Addr1   j_po.jpo_u.j_po_Addr1
//#define       jp_DcdPop0 j_po.jpo_u.j_po_DcdPop0
#define jp_Addr1   j_po.jpo_u.j_po_Addr1
//#define jp_Type    j_po.jpo_u.j_po_Bytes[sizeof(Word_t) - 1]
#define jp_Type    j_p1.j_p1_Type
#define jp_DcdP0   j_po.jpo_u.j_po_DcdP0


// ****************************************************************************
// JUDY POINTER (JP) -- RELATED MACROS AND CONSTANTS
// ****************************************************************************

// EXTRACT VALUES FROM JP:
//
// Masks for the bytes in the Dcd and Pop0 parts of jp_DcdPopO:
//
// cJU_DCDMASK() consists of a mask that excludes the (LSb) Pop0 bytes and
// also, just to be safe, the top byte of the word, since jp_DcdPopO is 1 byte
// less than a full word.
//
// Note:  These are constant macros (cJU) because cPopBytes should be a
// constant.  Also note cPopBytes == state in the SM.

#define cJU_POP0MASK(cPopBytes) JU_LEASTBYTESMASK(cPopBytes)

#define cJU_DCDMASK(cPopBytes) \
        ((cJU_ALLONES >> cJU_BITSPERBYTE) & (~cJU_POP0MASK(cPopBytes)))

// Mask off the high byte from INDEX to it can be compared to DcdPopO:

#define JU_TRIMTODCDSIZE(INDEX) ((cJU_ALLONES >> cJU_BITSPERBYTE) & (INDEX))

// Get from jp_DcdPopO the Pop0 for various branch JP Types:
//
// Note:  There are no simple macros for cJU_BRANCH* Types because their
// populations must be added up and dont reside in an already-calculated
// place.

#define JU_JPBRANCH_POP0(PJP,cPopBytes) \
        (JU_JPDCDPOP0(PJP) & cJU_POP0MASK(cPopBytes))

// METHOD FOR DETERMINING IF OBJECTS HAVE ROOM TO GROW:
//
// J__U_GROWCK() is a generic method to determine if an object can grow in
// place, based on whether the next population size (one more) would use the
// same space.

#define J__U_GROWCK(POP1,MAXPOP1,POPTOWORDS) \
        (((POP1) != (MAXPOP1)) && (POPTOWORDS[POP1] == POPTOWORDS[(POP1) + 1]))

#define JU_BRANCHBJPGROWINPLACE(NumJPs) \
        J__U_GROWCK(NumJPs, cJU_BITSPERSUBEXPB, j__U_BranchBJPPopToWords)


// DETERMINE IF AN INDEX IS (NOT) IN A JPS EXPANSE:

#define JU_DCDNOTMATCHINDEX(INDEX,PJP,POP0BYTES) \
        (((INDEX) ^ JU_JPDCDPOP0(PJP)) & cJU_DCDMASK(POP0BYTES))


// NUMBER OF JPs IN AN UNCOMPRESSED BRANCH:
//
// An uncompressed branch is simply an array of 256 Judy Pointers (JPs).  It is
// a minimum cacheline fill object.  Define it here before its first needed.

#define cJU_BRANCHUNUMJPS  cJU_SUBEXPPERSTATE


// ****************************************************************************
// JUDY BRANCH LINEAR (JBL) SUPPORT
// ****************************************************************************
//
// A linear branch is a way of compressing empty expanses (null JPs) out of an
// uncompressed 256-way branch, when the number of populated expanses is so
// small that even a bitmap branch is excessive.
//
// The maximum number of JPs in a Judy linear branch:
//
// Note:  This number results in a 1-cacheline sized structure.  Previous
// versions had a larger struct so a linear branch didnt become a bitmap
// branch until the memory consumed was even, but for speed, its better to
// switch "sooner" and keep a linear branch fast.

#define cJU_BRANCHLMAXJPS 7


// LINEAR BRANCH STRUCT:
//
// 1-byte count, followed by array of byte-sized expanses, followed by JPs.

typedef struct J__UDY_BRANCH_LINEAR
        {
            uint8_t jbl_NumJPs;                     // num of JPs (Pjp_t), 1..N.
            uint8_t jbl_Expanse[cJU_BRANCHLMAXJPS]; // 1..7 MSbs of pop exps.
            jp_t    jbl_jp     [cJU_BRANCHLMAXJPS]; // JPs for populated exps.
        } jbl_t, * Pjbl_t;


// ****************************************************************************
// JUDY BRANCH BITMAP (JBB) SUPPORT
// ****************************************************************************
//
// A bitmap branch is a way of compressing empty expanses (null JPs) out of
// uncompressed 256-way branch.  This costs 1 additional cache line fill, but
// can save a lot of memory when it matters most, near the leaves, and
// typically there will be only one at most in the path to any Index (leaf).
//
// The bitmap indicates which of the cJU_BRANCHUNUMJPS (256) JPs in the branch
// are NOT null, that is, their expanses are populated.  The jbb_t also
// contains N pointers to "mini" Judy branches ("subexpanses") of up to M JPs
// each (see BITMAP_BRANCHMxN, for example, BITMAP_BRANCH32x8), where M x N =
// cJU_BRANCHUNUMJPS.  These are dynamically allocated and never contain
// cJ*_JPNULL* jp_Types.  An empty subexpanse is represented by no bit sets in
// the corresponding subexpanse bitmap, in which case the corresponding
// jbbs_Pjp pointers value is unused.
//
// Note that the number of valid JPs in each 1-of-N subexpanses is determined
// by POPULATION rather than by EXPANSE -- the desired outcome to save memory
// when near the leaves.  Note that the memory required for 185 JPs is about as
// much as an uncompressed 256-way branch, therefore 184 is set as the maximum.
// However, it is expected that a conversion to an uncompressed 256-way branch
// will normally take place before this limit is reached for other reasons,
// such as improving performance when the "wasted" memory is well amortized by
// the population under the branch, preserving an acceptable overall
// bytes/Index in the Judy array.
//
// The number of pointers to arrays of JPs in the Judy bitmap branch:
//
// Note:  The numbers below are the same in both 32 and 64 bit systems.

#define cJU_BRANCHBMAXJPS  184          // maximum JPs for bitmap branches.

// Convenience wrappers for referencing BranchB bitmaps or JP subarray
// pointers:
//
// Note:  JU_JBB_PJP produces a "raw" memory address that must pass through
// P_JP before use, except when freeing memory:

#define JU_JBB_BITMAP(Pjbb, SubExp)  ((Pjbb)->jbb_jbbs[SubExp].jbbs_Bitmap)
#define JU_JBB_PJP(   Pjbb, SubExp)  ((Pjbb)->jbb_jbbs[SubExp].jbbs_Pjp)

#define JU_SUBEXPB(Digit) (((Digit) / cJU_BITSPERSUBEXPB) & (cJU_NUMSUBEXPB-1))

#define JU_BITMAPTESTB(Pjbb, Index) \
        (JU_JBB_BITMAP(Pjbb, JU_SUBEXPB(Index)) &  JU_BITPOSMASKB(Index))

#define JU_BITMAPSETB(Pjbb, Index)  \
        (JU_JBB_BITMAP(Pjbb, JU_SUBEXPB(Index)) |= JU_BITPOSMASKB(Index))

// Note:  JU_BITMAPCLEARB is not defined because the code does it a faster way.

typedef struct J__UDY_BRANCH_BITMAP_SUBEXPANSE
        {
            BITMAPB_t jbbs_Bitmap;
            Pjp_t     jbbs_Pjp;

        } jbbs_t;

typedef struct J__UDY_BRANCH_BITMAP
        {
            jbbs_t jbb_jbbs   [cJU_NUMSUBEXPB];
#ifdef SUBEXPCOUNTS
            Word_t jbb_subPop1[cJU_NUMSUBEXPB];
#endif
        } jbb_t, * Pjbb_t;

#define JU_BRANCHJP_NUMJPSTOWORDS(NumJPs) (j__U_BranchBJPPopToWords[NumJPs])

#ifdef SUBEXPCOUNTS
#define cJU_NUMSUBEXPU  16      // number of subexpanse counts.
#endif


// ****************************************************************************
// JUDY BRANCH UNCOMPRESSED (JBU) SUPPORT
// ****************************************************************************

// Convenience wrapper for referencing BranchU JPs:
//
// Note:  This produces a non-"raw" address already passed through P_JBU().

#define JU_JBU_PJP(Pjp,Index,Level) \
        (&((P_JBU((Pjp)->jp_Addr))->jbu_jp[JU_DIGITATSTATE(Index, Level)]))
#define JU_JBU_PJP0(Pjp) \
        (&((P_JBU((Pjp)->jp_Addr))->jbu_jp[0]))

typedef struct J__UDY_BRANCH_UNCOMPRESSED
        {
            jp_t   jbu_jp     [cJU_BRANCHUNUMJPS];  // JPs for populated exp.
#ifdef SUBEXPCOUNTS
            Word_t jbu_subPop1[cJU_NUMSUBEXPU];
#endif
        } jbu_t, * Pjbu_t;


// ****************************************************************************
// OTHER SUPPORT FOR JUDY STATE MACHINES (SMs)
// ****************************************************************************

// OBJECT SIZES IN WORDS:
//
// Word_ts per various JudyL structures that have constant sizes.
// cJU_WORDSPERJP should always be 2; this is fundamental to the Judy
// structures.

#define cJU_WORDSPERJP (sizeof(jp_t)   / cJU_BYTESPERWORD)
#define cJU_WORDSPERCL (cJU_BYTESPERCL / cJU_BYTESPERWORD)


// OPPORTUNISTIC UNCOMPRESSION:
//
// Define populations at which a BranchL or BranchB must convert to BranchU.
// Earlier conversion is possible with good memory efficiency -- see below.

#ifndef NO_BRANCHU

// Max population below BranchL, then convert to BranchU:

#define JU_BRANCHL_MAX_POP      1000

// Minimum global population increment before next conversion of a BranchB to a
// BranchU:
//
// This is was done to allow malloc() to coalesce memory before the next big
// (~512 words) allocation.

#define JU_BTOU_POP_INCREMENT    300

// Min/max population below BranchB, then convert to BranchU:

#define JU_BRANCHB_MIN_POP       135
#define JU_BRANCHB_MAX_POP       750

#else // NO_BRANCHU

// These are set up to have conservative conversion schedules to BranchU:

#define JU_BRANCHL_MAX_POP      (-1UL)
#define JU_BTOU_POP_INCREMENT      300
#define JU_BRANCHB_MIN_POP        1000
#define JU_BRANCHB_MAX_POP      (-1UL)

#endif // NO_BRANCHU


// MISCELLANEOUS MACROS:

// Get N most significant bits from the shifted Index word:
//
// As Index words are decoded, they are shifted left so only relevant,
// undecoded Index bits remain.

#define JU_BITSFROMSFTIDX(SFTIDX, N)  ((SFTIDX) >> (cJU_BITSPERWORD - (N)))

// TBD:  I have my doubts about the necessity of these macros (dlb):

// Produce 1-digit mask at specified state:

#define cJU_MASKATSTATE(State)  (0xffL << (((State) - 1) * cJU_BITSPERBYTE))

// Get byte (digit) from Index at the specified state, right justified:
//
// Note:  State must be 1..cJU_ROOTSTATE, and Digits must be 1..(cJU_ROOTSTATE
// - 1), but theres no way to assert these within an expression.

#define JU_DIGITATSTATE(Index,cState) \
         ((uint8_t)((Index) >> (((cState) - 1) * cJU_BITSPERBYTE)))

// Similarly, place byte (digit) at correct position for the specified state:
//
// Note:  Cast digit to a Word_t first so there are no complaints or problems
// about shifting it more than 32 bits on a 64-bit system, say, when it is a
// uint8_t from jbl_Expanse[].  (Believe it or not, the C standard says to
// promote an unsigned char to a signed int; -Ac does not do this, but -Ae
// does.)
//
// Also, to make lint happy, cast the whole result again because apparently
// shifting a Word_t does not result in a Word_t!

#define JU_DIGITTOSTATE(Digit,cState) \
        ((Word_t) (((Word_t) (Digit)) << (((cState) - 1) * cJU_BITSPERBYTE)))

#endif // ! _JUDY_PRIVATE_BRANCH_INCLUDED


#ifdef TEST_INSDEL

// ****************************************************************************
// TEST CODE FOR INSERT/DELETE MACROS
// ****************************************************************************
//
// To use this, compile a temporary *.c file containing:
//
//      #define DEBUG
//      #define JUDY_ASSERT
//      #define TEST_INSDEL
//      #include "JudyPrivate.h"
//      #include "JudyPrivateBranch.h"
//
// Use a command like this:  cc -Ae +DD64 -I. -I JudyCommon -o t t.c
// For best results, include +DD64 on a 64-bit system.
//
// This test code exercises some tricky macros, but the output must be studied
// manually to verify it.  Assume that for even-index testing, whole words
// (Word_t) suffices.

#include <stdio.h>

#define INDEXES 3               // in each array.


// ****************************************************************************
// I N I T
//
// Set up variables for next test.  See usage.

FUNCTION void Init (
        int       base,
        PWord_t   PeIndex,
        PWord_t   PoIndex,
        PWord_t   Peleaf,       // always whole words.
#ifndef JU_64BIT
        uint8_t * Poleaf3)
#else
        uint8_t * Poleaf3,
        uint8_t * Poleaf5,
        uint8_t * Poleaf6,
        uint8_t * Poleaf7)
#endif
{
        int offset;

        *PeIndex = 99;

        for (offset = 0; offset <= INDEXES; ++offset)
            Peleaf[offset] = base + offset;

        for (offset = 0; offset < (INDEXES + 1) * 3; ++offset)
            Poleaf3[offset] = base + offset;

#ifndef JU_64BIT
        *PoIndex = (91 << 24) | (92 << 16) | (93 << 8) | 94;
#else

        *PoIndex = (91L << 56) | (92L << 48) | (93L << 40) | (94L << 32)
                 | (95L << 24) | (96L << 16) | (97L <<  8) |  98L;

        for (offset = 0; offset < (INDEXES + 1) * 5; ++offset)
            Poleaf5[offset] = base + offset;

        for (offset = 0; offset < (INDEXES + 1) * 6; ++offset)
            Poleaf6[offset] = base + offset;

        for (offset = 0; offset < (INDEXES + 1) * 7; ++offset)
            Poleaf7[offset] = base + offset;
#endif

} // Init()


// ****************************************************************************
// P R I N T   L E A F
//
// Print the byte values in a leaf.

FUNCTION void PrintLeaf (
        char *    Label,        // for output.
        int       IOffset,      // insertion offset in array.
        int       Indsize,      // index size in bytes.
        uint8_t * PLeaf)        // array of Index bytes.
{
        int       offset;       // in PLeaf.
        int       byte;         // in one word.

        (void) printf("%s %u: ", Label, IOffset);

        for (offset = 0; offset <= INDEXES; ++offset)
        {
            for (byte = 0; byte < Indsize; ++byte)
                (void) printf("%2d", PLeaf[(offset * Indsize) + byte]);

            (void) printf(" ");
        }

        (void) printf("\n");

} // PrintLeaf()


// ****************************************************************************
// M A I N
//
// Test program.

FUNCTION main()
{
        Word_t  eIndex;                         // even, to insert.
        Word_t  oIndex;                         // odd,  to insert.
        Word_t  eleaf [ INDEXES + 1];           // even leaf, index size 4.
        uint8_t oleaf3[(INDEXES + 1) * 3];      // odd leaf,  index size 3.
#ifdef JU_64BIT
        uint8_t oleaf5[(INDEXES + 1) * 5];      // odd leaf,  index size 5.
        uint8_t oleaf6[(INDEXES + 1) * 6];      // odd leaf,  index size 6.
        uint8_t oleaf7[(INDEXES + 1) * 7];      // odd leaf,  index size 7.
#endif
        Word_t  eleaf_2 [ INDEXES + 1];         // same, but second arrays:
        uint8_t oleaf3_2[(INDEXES + 1) * 3];
#ifdef JU_64BIT
        uint8_t oleaf5_2[(INDEXES + 1) * 5];
        uint8_t oleaf6_2[(INDEXES + 1) * 6];
        uint8_t oleaf7_2[(INDEXES + 1) * 7];
#endif
        int     ioffset;                // index insertion offset.

#ifndef JU_64BIT
#define INIT        Init( 0, & eIndex, & oIndex, eleaf,   oleaf3)
#define INIT2 INIT; Init(50, & eIndex, & oIndex, eleaf_2, oleaf3_2)
#else
#define INIT        Init( 0, & eIndex, & oIndex, eleaf,   oleaf3, \
                         oleaf5,   oleaf6,   oleaf7)
#define INIT2 INIT; Init(50, & eIndex, & oIndex, eleaf_2, oleaf3_2, \
                         oleaf5_2, oleaf6_2, oleaf7_2)
#endif

#define WSIZE sizeof (Word_t)           // shorthand.

#ifdef PRINTALL                 // to turn on "noisy" printouts.
#define PRINTLEAF(Label,IOffset,Indsize,PLeaf) \
        PrintLeaf(Label,IOffset,Indsize,PLeaf)
#else
#define PRINTLEAF(Label,IOffset,Indsize,PLeaf)  \
        if (ioffset == 0)                       \
        PrintLeaf(Label,IOffset,Indsize,PLeaf)
#endif

        (void) printf(
"In each case, tests operate on an initial array of %d indexes.  Even-index\n"
"tests set index values to 0,1,2...; odd-index tests set byte values to\n"
"0,1,2...  Inserted indexes have a value of 99 or else byte values 91,92,...\n",
                        INDEXES);

        (void) puts("\nJU_INSERTINPLACE():");

        for (ioffset = 0; ioffset <= INDEXES; ++ioffset)
        {
            INIT;
            PRINTLEAF("Before", ioffset, WSIZE, (uint8_t *) eleaf);
            JU_INSERTINPLACE(eleaf, INDEXES, ioffset, eIndex);
            PrintLeaf("After ", ioffset, WSIZE, (uint8_t *) eleaf);
        }

        (void) puts("\nJU_INSERTINPLACE3():");

        for (ioffset = 0; ioffset <= INDEXES; ++ioffset)
        {
            INIT;
            PRINTLEAF("Before", ioffset, 3, oleaf3);
            JU_INSERTINPLACE3(oleaf3, INDEXES, ioffset, oIndex);
            PrintLeaf("After ", ioffset, 3, oleaf3);
        }

#ifdef JU_64BIT
        (void) puts("\nJU_INSERTINPLACE5():");

        for (ioffset = 0; ioffset <= INDEXES; ++ioffset)
        {
            INIT;
            PRINTLEAF("Before", ioffset, 5, oleaf5);
            JU_INSERTINPLACE5(oleaf5, INDEXES, ioffset, oIndex);
            PrintLeaf("After ", ioffset, 5, oleaf5);
        }

        (void) puts("\nJU_INSERTINPLACE6():");

        for (ioffset = 0; ioffset <= INDEXES; ++ioffset)
        {
            INIT;
            PRINTLEAF("Before", ioffset, 6, oleaf6);
            JU_INSERTINPLACE6(oleaf6, INDEXES, ioffset, oIndex);
            PrintLeaf("After ", ioffset, 6, oleaf6);
        }

        (void) puts("\nJU_INSERTINPLACE7():");

        for (ioffset = 0; ioffset <= INDEXES; ++ioffset)
        {
            INIT;
            PRINTLEAF("Before", ioffset, 7, oleaf7);
            JU_INSERTINPLACE7(oleaf7, INDEXES, ioffset, oIndex);
            PrintLeaf("After ", ioffset, 7, oleaf7);
        }
#endif // JU_64BIT

        (void) puts("\nJU_DELETEINPLACE():");

        for (ioffset = 0; ioffset < INDEXES; ++ioffset)
        {
            INIT;
            PRINTLEAF("Before", ioffset, WSIZE, (uint8_t *) eleaf);
            JU_DELETEINPLACE(eleaf, INDEXES, ioffset);
            PrintLeaf("After ", ioffset, WSIZE, (uint8_t *) eleaf);
        }

        (void) puts("\nJU_DELETEINPLACE_ODD(3):");

        for (ioffset = 0; ioffset < INDEXES; ++ioffset)
        {
            INIT;
            PRINTLEAF("Before", ioffset, 3, oleaf3);
            JU_DELETEINPLACE_ODD(oleaf3, INDEXES, ioffset, 3);
            PrintLeaf("After ", ioffset, 3, oleaf3);
        }

#ifdef JU_64BIT
        (void) puts("\nJU_DELETEINPLACE_ODD(5):");

        for (ioffset = 0; ioffset < INDEXES; ++ioffset)
        {
            INIT;
            PRINTLEAF("Before", ioffset, 5, oleaf5);
            JU_DELETEINPLACE_ODD(oleaf5, INDEXES, ioffset, 5);
            PrintLeaf("After ", ioffset, 5, oleaf5);
        }

        (void) puts("\nJU_DELETEINPLACE_ODD(6):");

        for (ioffset = 0; ioffset < INDEXES; ++ioffset)
        {
            INIT;
            PRINTLEAF("Before", ioffset, 6, oleaf6);
            JU_DELETEINPLACE_ODD(oleaf6, INDEXES, ioffset, 6);
            PrintLeaf("After ", ioffset, 6, oleaf6);
        }

        (void) puts("\nJU_DELETEINPLACE_ODD(7):");

        for (ioffset = 0; ioffset < INDEXES; ++ioffset)
        {
            INIT;
            PRINTLEAF("Before", ioffset, 7, oleaf7);
            JU_DELETEINPLACE_ODD(oleaf7, INDEXES, ioffset, 7);
            PrintLeaf("After ", ioffset, 7, oleaf7);
        }
#endif // JU_64BIT

        (void) puts("\nJU_INSERTCOPY():");

        for (ioffset = 0; ioffset <= INDEXES; ++ioffset)
        {
            INIT2;
            PRINTLEAF("Before, src ", ioffset, WSIZE, (uint8_t *) eleaf);
            PRINTLEAF("Before, dest", ioffset, WSIZE, (uint8_t *) eleaf_2);
            JU_INSERTCOPY(eleaf_2, eleaf, INDEXES, ioffset, eIndex);
            PRINTLEAF("After,  src ", ioffset, WSIZE, (uint8_t *) eleaf);
            PrintLeaf("After,  dest", ioffset, WSIZE, (uint8_t *) eleaf_2);
        }

        (void) puts("\nJU_INSERTCOPY3():");

        for (ioffset = 0; ioffset <= INDEXES; ++ioffset)
        {
            INIT2;
            PRINTLEAF("Before, src ", ioffset, 3, oleaf3);
            PRINTLEAF("Before, dest", ioffset, 3, oleaf3_2);
            JU_INSERTCOPY3(oleaf3_2, oleaf3, INDEXES, ioffset, oIndex);
            PRINTLEAF("After,  src ", ioffset, 3, oleaf3);
            PrintLeaf("After,  dest", ioffset, 3, oleaf3_2);
        }

#ifdef JU_64BIT
        (void) puts("\nJU_INSERTCOPY5():");

        for (ioffset = 0; ioffset <= INDEXES; ++ioffset)
        {
            INIT2;
            PRINTLEAF("Before, src ", ioffset, 5, oleaf5);
            PRINTLEAF("Before, dest", ioffset, 5, oleaf5_2);
            JU_INSERTCOPY5(oleaf5_2, oleaf5, INDEXES, ioffset, oIndex);
            PRINTLEAF("After,  src ", ioffset, 5, oleaf5);
            PrintLeaf("After,  dest", ioffset, 5, oleaf5_2);
        }

        (void) puts("\nJU_INSERTCOPY6():");

        for (ioffset = 0; ioffset <= INDEXES; ++ioffset)
        {
            INIT2;
            PRINTLEAF("Before, src ", ioffset, 6, oleaf6);
            PRINTLEAF("Before, dest", ioffset, 6, oleaf6_2);
            JU_INSERTCOPY6(oleaf6_2, oleaf6, INDEXES, ioffset, oIndex);
            PRINTLEAF("After,  src ", ioffset, 6, oleaf6);
            PrintLeaf("After,  dest", ioffset, 6, oleaf6_2);
        }

        (void) puts("\nJU_INSERTCOPY7():");

        for (ioffset = 0; ioffset <= INDEXES; ++ioffset)
        {
            INIT2;
            PRINTLEAF("Before, src ", ioffset, 7, oleaf7);
            PRINTLEAF("Before, dest", ioffset, 7, oleaf7_2);
            JU_INSERTCOPY7(oleaf7_2, oleaf7, INDEXES, ioffset, oIndex);
            PRINTLEAF("After,  src ", ioffset, 7, oleaf7);
            PrintLeaf("After,  dest", ioffset, 7, oleaf7_2);
        }
#endif // JU_64BIT

        (void) puts("\nJU_DELETECOPY():");

        for (ioffset = 0; ioffset < INDEXES; ++ioffset)
        {
            INIT2;
            PRINTLEAF("Before, src ", ioffset, WSIZE, (uint8_t *) eleaf);
            PRINTLEAF("Before, dest", ioffset, WSIZE, (uint8_t *) eleaf_2);
            JU_DELETECOPY(eleaf_2, eleaf, INDEXES, ioffset, ignore);
            PRINTLEAF("After,  src ", ioffset, WSIZE, (uint8_t *) eleaf);
            PrintLeaf("After,  dest", ioffset, WSIZE, (uint8_t *) eleaf_2);
        }

        (void) puts("\nJU_DELETECOPY_ODD(3):");

        for (ioffset = 0; ioffset < INDEXES; ++ioffset)
        {
            INIT2;
            PRINTLEAF("Before, src ", ioffset, 3, oleaf3);
            PRINTLEAF("Before, dest", ioffset, 3, oleaf3_2);
            JU_DELETECOPY_ODD(oleaf3_2, oleaf3, INDEXES, ioffset, 3);
            PRINTLEAF("After,  src ", ioffset, 3, oleaf3);
            PrintLeaf("After,  dest", ioffset, 3, oleaf3_2);
        }

#ifdef JU_64BIT
        (void) puts("\nJU_DELETECOPY_ODD(5):");

        for (ioffset = 0; ioffset < INDEXES; ++ioffset)
        {
            INIT2;
            PRINTLEAF("Before, src ", ioffset, 5, oleaf5);
            PRINTLEAF("Before, dest", ioffset, 5, oleaf5_2);
            JU_DELETECOPY_ODD(oleaf5_2, oleaf5, INDEXES, ioffset, 5);
            PRINTLEAF("After,  src ", ioffset, 5, oleaf5);
            PrintLeaf("After,  dest", ioffset, 5, oleaf5_2);
        }

        (void) puts("\nJU_DELETECOPY_ODD(6):");

        for (ioffset = 0; ioffset < INDEXES; ++ioffset)
        {
            INIT2;
            PRINTLEAF("Before, src ", ioffset, 6, oleaf6);
            PRINTLEAF("Before, dest", ioffset, 6, oleaf6_2);
            JU_DELETECOPY_ODD(oleaf6_2, oleaf6, INDEXES, ioffset, 6);
            PRINTLEAF("After,  src ", ioffset, 6, oleaf6);
            PrintLeaf("After,  dest", ioffset, 6, oleaf6_2);
        }

        (void) puts("\nJU_DELETECOPY_ODD(7):");

        for (ioffset = 0; ioffset < INDEXES; ++ioffset)
        {
            INIT2;
            PRINTLEAF("Before, src ", ioffset, 7, oleaf7);
            PRINTLEAF("Before, dest", ioffset, 7, oleaf7_2);
            JU_DELETECOPY_ODD(oleaf7_2, oleaf7, INDEXES, ioffset, 7);
            PRINTLEAF("After,  src ", ioffset, 7, oleaf7);
            PrintLeaf("After,  dest", ioffset, 7, oleaf7_2);
        }
#endif // JU_64BIT

        return(0);

} // main()

#endif // TEST_INSDEL
