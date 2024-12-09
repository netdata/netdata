#ifndef _JUDYL_INCLUDED
#define _JUDYL_INCLUDED
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

// @(#) $Revision: 4.41 $ $Source: /judy/src/JudyL/JudyL.h $

// ****************************************************************************
//          JUDYL -- SMALL/LARGE AND/OR CLUSTERED/SPARSE ARRAYS
//
//                                    -by-
//
//                             Douglas L. Baskins
//                             doug@sourcejudy.com
//
// Judy arrays are designed to be used instead of arrays.  The performance
// suggests the reason why Judy arrays are thought of as arrays, instead of
// trees.  They are remarkably memory efficient at all populations.
// Implemented as a hybrid digital tree (but really a state machine, see
// below), Judy arrays feature fast insert/retrievals, fast near neighbor
// searching, and contain a population tree for extremely fast ordinal related
// retrievals.
//
// CONVENTIONS:
//
// - The comments here refer to 32-bit [64-bit] systems.
//
// - BranchL, LeafL refer to linear branches and leaves (small populations),
//   except LeafL does not actually appear as such; rather, Leaf1..3 [Leaf1..7]
//   is used to represent leaf Index sizes, and LeafW refers to a Leaf with
//   full (Long) word Indexes, which is also a type of linear leaf.  Note that
//   root-level LeafW (Leaf4 [Leaf8]) leaves are called LEAFW.
//
// - BranchB, LeafB1 refer to bitmap branches and leaves (intermediate
//   populations).
//
// - BranchU refers to uncompressed branches.  An uncompressed branch has 256
//   JPs, some of which could be null.  Note:  All leaves are compressed (and
//   sorted), or else an expanse is full (FullPopu), so there is no LeafU
//   equivalent to BranchU.
//
// - "Popu" is short for "Population".
// - "Pop1" refers to actual population (base 1).
// - "Pop0" refers to Pop1 - 1 (base 0), the way populations are stored in data
//   structures.
//
// - Branches and Leaves are both named by the number of bytes in their Pop0
//   field.  In the case of Leaves, the same number applies to the Index sizes.
//
// - The representation of many numbers as hex is a relatively safe and
//   portable way to get desired bitpatterns as unsigned longs.
//
// - Some preprocessors cant handle single apostrophe characters within
//   #ifndef code, so here, delete all instead.


#include "JudyPrivate.h"        // includes Judy.h in turn.
#include "JudyPrivateBranch.h"  // support for branches.


// ****************************************************************************
// JUDYL ROOT POINTER (JRP) AND JUDYL POINTER (JP) TYPE FIELDS
// ****************************************************************************

typedef enum            // uint8_t -- but C does not support this type of enum.
{

// JP NULL TYPES:
//
// There is a series of cJL_JPNULL* Types because each one pre-records a
// different Index Size for when the first Index is inserted in the previously
// null JP.  They must start >= 8 (three bits).
//
// Note:  These Types must be in sequential order for doing relative
// calculations between them.

        cJL_JPNULL1 = 1,
                                // Index Size 1[1] byte  when 1 Index inserted.
        cJL_JPNULL2,            // Index Size 2[2] bytes when 1 Index inserted.
        cJL_JPNULL3,            // Index Size 3[3] bytes when 1 Index inserted.

#ifndef JU_64BIT
#define cJL_JPNULLMAX cJL_JPNULL3
#else
        cJL_JPNULL4,            // Index Size 4[4] bytes when 1 Index inserted.
        cJL_JPNULL5,            // Index Size 5[5] bytes when 1 Index inserted.
        cJL_JPNULL6,            // Index Size 6[6] bytes when 1 Index inserted.
        cJL_JPNULL7,            // Index Size 7[7] bytes when 1 Index inserted.
#define cJL_JPNULLMAX cJL_JPNULL7
#endif


// JP BRANCH TYPES:
//
// Note:  There are no state-1 branches; only leaves reside at state 1.

// Linear branches:
//
// Note:  These Types must be in sequential order for doing relative
// calculations between them.

        cJL_JPBRANCH_L2,        // 2[2] bytes Pop0, 1[5] bytes Dcd.
        cJL_JPBRANCH_L3,        // 3[3] bytes Pop0, 0[4] bytes Dcd.

#ifdef JU_64BIT
        cJL_JPBRANCH_L4,        //  [4] bytes Pop0,  [3] bytes Dcd.
        cJL_JPBRANCH_L5,        //  [5] bytes Pop0,  [2] bytes Dcd.
        cJL_JPBRANCH_L6,        //  [6] bytes Pop0,  [1] byte  Dcd.
        cJL_JPBRANCH_L7,        //  [7] bytes Pop0,  [0] bytes Dcd.
#endif

        cJL_JPBRANCH_L,         // note:  DcdPopO field not used.

// Bitmap branches:
//
// Note:  These Types must be in sequential order for doing relative
// calculations between them.

        cJL_JPBRANCH_B2,        // 2[2] bytes Pop0, 1[5] bytes Dcd.
        cJL_JPBRANCH_B3,        // 3[3] bytes Pop0, 0[4] bytes Dcd.

#ifdef JU_64BIT
        cJL_JPBRANCH_B4,        //  [4] bytes Pop0,  [3] bytes Dcd.
        cJL_JPBRANCH_B5,        //  [5] bytes Pop0,  [2] bytes Dcd.
        cJL_JPBRANCH_B6,        //  [6] bytes Pop0,  [1] byte  Dcd.
        cJL_JPBRANCH_B7,        //  [7] bytes Pop0,  [0] bytes Dcd.
#endif

        cJL_JPBRANCH_B,         // note:  DcdPopO field not used.

// Uncompressed branches:
//
// Note:  These Types must be in sequential order for doing relative
// calculations between them.

        cJL_JPBRANCH_U2,        // 2[2] bytes Pop0, 1[5] bytes Dcd.
        cJL_JPBRANCH_U3,        // 3[3] bytes Pop0, 0[4] bytes Dcd.

#ifdef JU_64BIT
        cJL_JPBRANCH_U4,        //  [4] bytes Pop0,  [3] bytes Dcd.
        cJL_JPBRANCH_U5,        //  [5] bytes Pop0,  [2] bytes Dcd.
        cJL_JPBRANCH_U6,        //  [6] bytes Pop0,  [1] byte  Dcd.
        cJL_JPBRANCH_U7,        //  [7] bytes Pop0,  [0] bytes Dcd.
#endif

        cJL_JPBRANCH_U,         // note:  DcdPopO field not used.


// JP LEAF TYPES:

// Linear leaves:
//
// Note:  These Types must be in sequential order for doing relative
// calculations between them.
//
// Note:  There is no full-word (4-byte [8-byte]) Index leaf under a JP because
// non-root-state leaves only occur under branches that decode at least one
// byte.  Full-word, root-state leaves are under a JRP, not a JP.  However, in
// the code a "fake" JP can be created temporarily above a root-state leaf.

        cJL_JPLEAF1,            // 1[1] byte  Pop0, 2    bytes Dcd.
        cJL_JPLEAF2,            // 2[2] bytes Pop0, 1[5] bytes Dcd.
        cJL_JPLEAF3,            // 3[3] bytes Pop0, 0[4] bytes Dcd.

#ifdef JU_64BIT
        cJL_JPLEAF4,            //  [4] bytes Pop0,  [3] bytes Dcd.
        cJL_JPLEAF5,            //  [5] bytes Pop0,  [2] bytes Dcd.
        cJL_JPLEAF6,            //  [6] bytes Pop0,  [1] byte  Dcd.
        cJL_JPLEAF7,            //  [7] bytes Pop0,  [0] bytes Dcd.
#endif

// Bitmap leaf; Index Size == 1:
//
// Note:  These are currently only supported at state 1.  At other states the
// bitmap would grow from 256 to 256^2, 256^3, ... bits, which would not be
// efficient..

        cJL_JPLEAF_B1,          // 1[1] byte Pop0, 2[6] bytes Dcd.

// Full population; Index Size == 1 virtual leaf:
//
// Note:  JudyL has no cJL_JPFULLPOPU1 equivalent to cJ1_JPFULLPOPU1, because
// in the JudyL case this could result in a values-only leaf of up to 256 words
// (value areas) that would be slow to insert/delete.


// JP IMMEDIATES; leaves (Indexes) stored inside a JP:
//
// The second numeric suffix is the Pop1 for each type.  As the Index Size
// increases, the maximum possible population decreases.
//
// Note:  These Types must be in sequential order in each group (Index Size),
// and the groups in correct order too, for doing relative calculations between
// them.  For example, since these Types enumerate the Pop1 values (unlike
// other JP Types where there is a Pop0 value in the JP), the maximum Pop1 for
// each Index Size is computable.
//
// All enums equal or above this point are cJL_JPIMMEDs.

        cJL_JPIMMED_1_01,       // Index Size = 1, Pop1 = 1.
        cJL_JPIMMED_2_01,       // Index Size = 2, Pop1 = 1.
        cJL_JPIMMED_3_01,       // Index Size = 3, Pop1 = 1.

#ifdef JU_64BIT
        cJL_JPIMMED_4_01,       // Index Size = 4, Pop1 = 1.
        cJL_JPIMMED_5_01,       // Index Size = 5, Pop1 = 1.
        cJL_JPIMMED_6_01,       // Index Size = 6, Pop1 = 1.
        cJL_JPIMMED_7_01,       // Index Size = 7, Pop1 = 1.
#endif

        cJL_JPIMMED_1_02,       // Index Size = 1, Pop1 = 2.
        cJL_JPIMMED_1_03,       // Index Size = 1, Pop1 = 3.

#ifdef JU_64BIT
        cJL_JPIMMED_1_04,       // Index Size = 1, Pop1 = 4.
        cJL_JPIMMED_1_05,       // Index Size = 1, Pop1 = 5.
        cJL_JPIMMED_1_06,       // Index Size = 1, Pop1 = 6.
        cJL_JPIMMED_1_07,       // Index Size = 1, Pop1 = 7.

        cJL_JPIMMED_2_02,       // Index Size = 2, Pop1 = 2.
        cJL_JPIMMED_2_03,       // Index Size = 2, Pop1 = 3.

        cJL_JPIMMED_3_02,       // Index Size = 3, Pop1 = 2.
#endif

// This special Type is merely a sentinel for doing relative calculations.
// This value should not be used in switch statements (to avoid allocating code
// for it), which is also why it appears at the end of the enum list.

        cJL_JPIMMED_CAP

} jpL_Type_t;


// RELATED VALUES:

// Index Size (state) for leaf JP, and JP type based on Index Size (state):

#define JL_LEAFINDEXSIZE(jpType) ((jpType)    - cJL_JPLEAF1 + 1)
#define JL_LEAFTYPE(IndexSize)   ((IndexSize) + cJL_JPLEAF1 - 1)


// MAXIMUM POPULATIONS OF LINEAR LEAVES:

#ifndef JU_64BIT // 32-bit

#define J_L_MAXB                (sizeof(Word_t) * 64)
#define ALLOCSIZES { 3, 5, 7, 11, 15, 23, 32, 47, 64, TERMINATOR } // in words.
#define cJL_LEAF1_MAXWORDS               (32)   // max Leaf1 size in words.

// Note:  cJL_LEAF1_MAXPOP1 is chosen such that the index portion is less than
// 32 bytes -- the number of bytes the index takes in a bitmap leaf.

#define cJL_LEAF1_MAXPOP1 \
   ((cJL_LEAF1_MAXWORDS * cJU_BYTESPERWORD)/(1 + cJU_BYTESPERWORD))
#define cJL_LEAF2_MAXPOP1       (J_L_MAXB / (2 + cJU_BYTESPERWORD))
#define cJL_LEAF3_MAXPOP1       (J_L_MAXB / (3 + cJU_BYTESPERWORD))
#define cJL_LEAFW_MAXPOP1 \
           ((J_L_MAXB - cJU_BYTESPERWORD) / (2 * cJU_BYTESPERWORD))

#else // 64-bit

#define J_L_MAXB                (sizeof(Word_t) * 64)
#define ALLOCSIZES { 3, 5, 7, 11, 15, 23, 32, 47, 64, TERMINATOR } // in words.
#define cJL_LEAF1_MAXWORDS       (15)   // max Leaf1 size in words.

#define cJL_LEAF1_MAXPOP1 \
   ((cJL_LEAF1_MAXWORDS * cJU_BYTESPERWORD)/(1 + cJU_BYTESPERWORD))
#define cJL_LEAF2_MAXPOP1       (J_L_MAXB / (2 + cJU_BYTESPERWORD))
#define cJL_LEAF3_MAXPOP1       (J_L_MAXB / (3 + cJU_BYTESPERWORD))
#define cJL_LEAF4_MAXPOP1       (J_L_MAXB / (4 + cJU_BYTESPERWORD))
#define cJL_LEAF5_MAXPOP1       (J_L_MAXB / (5 + cJU_BYTESPERWORD))
#define cJL_LEAF6_MAXPOP1       (J_L_MAXB / (6 + cJU_BYTESPERWORD))
#define cJL_LEAF7_MAXPOP1       (J_L_MAXB / (7 + cJU_BYTESPERWORD))
#define cJL_LEAFW_MAXPOP1 \
           ((J_L_MAXB - cJU_BYTESPERWORD) / (2 * cJU_BYTESPERWORD))

#endif // 64-bit


// MAXIMUM POPULATIONS OF IMMEDIATE JPs:
//
// These specify the maximum Population of immediate JPs with various Index
// Sizes (== sizes of remaining undecoded Index bits).  Since the JP Types enum
// already lists all the immediates in order by state and size, calculate these
// values from it to avoid redundancy.

#define cJL_IMMED1_MAXPOP1  ((cJU_BYTESPERWORD - 1) / 1)        // 3 [7].
#define cJL_IMMED2_MAXPOP1  ((cJU_BYTESPERWORD - 1) / 2)        // 1 [3].
#define cJL_IMMED3_MAXPOP1  ((cJU_BYTESPERWORD - 1) / 3)        // 1 [2].

#ifdef JU_64BIT
#define cJL_IMMED4_MAXPOP1  ((cJU_BYTESPERWORD - 1) / 4)        //   [1].
#define cJL_IMMED5_MAXPOP1  ((cJU_BYTESPERWORD - 1) / 5)        //   [1].
#define cJL_IMMED6_MAXPOP1  ((cJU_BYTESPERWORD - 1) / 6)        //   [1].
#define cJL_IMMED7_MAXPOP1  ((cJU_BYTESPERWORD - 1) / 7)        //   [1].
#endif


// ****************************************************************************
// JUDYL LEAF BITMAP (JLLB) SUPPORT
// ****************************************************************************
//
// Assemble bitmap leaves out of smaller units that put bitmap subexpanses
// close to their associated pointers.  Why not just use a bitmap followed by a
// series of pointers?  (See 4.27.)  Turns out this wastes a cache fill on
// systems with smaller cache lines than the assumed value cJU_WORDSPERCL.

#define JL_JLB_BITMAP(Pjlb, Subexp)  ((Pjlb)->jLlb_jLlbs[Subexp].jLlbs_Bitmap)
#define JL_JLB_PVALUE(Pjlb, Subexp)  ((Pjlb)->jLlb_jLlbs[Subexp].jLlbs_PValue)

typedef struct J__UDYL_LEAF_BITMAP_SUBEXPANSE
{
        BITMAPL_t jLlbs_Bitmap;
        Pjv_t     jLlbs_PValue;

} jLlbs_t;

typedef struct J__UDYL_LEAF_BITMAP
{
        jLlbs_t jLlb_jLlbs[cJU_NUMSUBEXPL];

} jLlb_t, * PjLlb_t;

// Words per bitmap leaf:

#define cJL_WORDSPERLEAFB1  (sizeof(jLlb_t) / cJU_BYTESPERWORD)


// ****************************************************************************
// MEMORY ALLOCATION SUPPORT
// ****************************************************************************

// ARRAY-GLOBAL INFORMATION:
//
// At the cost of an occasional additional cache fill, this object, which is
// pointed at by a JRP and in turn points to a JP_BRANCH*, carries array-global
// information about a JudyL array that has sufficient population to amortize
// the cost.  The jpm_Pop0 field prevents having to add up the total population
// for the array in insert, delete, and count code.  The jpm_JP field prevents
// having to build a fake JP for entry to a state machine; however, the
// jp_DcdPopO field in jpm_JP, being one byte too small, is not used.
//
// Note:  Struct fields are ordered to keep "hot" data in the first 8 words
// (see left-margin comments) for machines with 8-word cache lines, and to keep
// sub-word fields together for efficient packing.

typedef struct J_UDYL_POPULATION_AND_MEMORY
{
/* 1 */ Word_t     jpm_Pop0;            // total population-1 in array.
/* 2 */ jp_t       jpm_JP;              // JP to first branch; see above.
/* 4 */ Word_t     jpm_LastUPop0;       // last jpm_Pop0 when convert to BranchU
/* 7 */ Pjv_t      jpm_PValue;          // pointer to value to return.
// Note:  Field names match PJError_t for convenience in macros:
/* 8 */ char       je_Errno;            // one of the enums in Judy.h.
/* 8/9  */ int     je_ErrID;            // often an internal source line number.
/* 9/10 */ Word_t  jpm_TotalMemWords;   // words allocated in array.
} jLpm_t, *PjLpm_t;


// TABLES FOR DETERMINING IF LEAVES HAVE ROOM TO GROW:
//
// These tables indicate if a given memory chunk can support growth of a given
// object into wasted (rounded-up) memory in the chunk.  Note:  This violates
// the hiddenness of the JudyMalloc code.

extern const uint8_t j__L_Leaf1PopToWords[cJL_LEAF1_MAXPOP1 + 1];
extern const uint8_t j__L_Leaf2PopToWords[cJL_LEAF2_MAXPOP1 + 1];
extern const uint8_t j__L_Leaf3PopToWords[cJL_LEAF3_MAXPOP1 + 1];
#ifdef JU_64BIT
extern const uint8_t j__L_Leaf4PopToWords[cJL_LEAF4_MAXPOP1 + 1];
extern const uint8_t j__L_Leaf5PopToWords[cJL_LEAF5_MAXPOP1 + 1];
extern const uint8_t j__L_Leaf6PopToWords[cJL_LEAF6_MAXPOP1 + 1];
extern const uint8_t j__L_Leaf7PopToWords[cJL_LEAF7_MAXPOP1 + 1];
#endif
extern const uint8_t j__L_LeafWPopToWords[cJL_LEAFW_MAXPOP1 + 1];
extern const uint8_t j__L_LeafVPopToWords[];

// These tables indicate where value areas start:

extern const uint8_t j__L_Leaf1Offset    [cJL_LEAF1_MAXPOP1 + 1];
extern const uint8_t j__L_Leaf2Offset    [cJL_LEAF2_MAXPOP1 + 1];
extern const uint8_t j__L_Leaf3Offset    [cJL_LEAF3_MAXPOP1 + 1];
#ifdef JU_64BIT
extern const uint8_t j__L_Leaf4Offset    [cJL_LEAF4_MAXPOP1 + 1];
extern const uint8_t j__L_Leaf5Offset    [cJL_LEAF5_MAXPOP1 + 1];
extern const uint8_t j__L_Leaf6Offset    [cJL_LEAF6_MAXPOP1 + 1];
extern const uint8_t j__L_Leaf7Offset    [cJL_LEAF7_MAXPOP1 + 1];
#endif
extern const uint8_t j__L_LeafWOffset    [cJL_LEAFW_MAXPOP1 + 1];

// Also define macros to hide the details in the code using these tables.

#define JL_LEAF1GROWINPLACE(Pop1) \
        J__U_GROWCK(Pop1, cJL_LEAF1_MAXPOP1, j__L_Leaf1PopToWords)
#define JL_LEAF2GROWINPLACE(Pop1) \
        J__U_GROWCK(Pop1, cJL_LEAF2_MAXPOP1, j__L_Leaf2PopToWords)
#define JL_LEAF3GROWINPLACE(Pop1) \
        J__U_GROWCK(Pop1, cJL_LEAF3_MAXPOP1, j__L_Leaf3PopToWords)
#ifdef JU_64BIT
#define JL_LEAF4GROWINPLACE(Pop1) \
        J__U_GROWCK(Pop1, cJL_LEAF4_MAXPOP1, j__L_Leaf4PopToWords)
#define JL_LEAF5GROWINPLACE(Pop1) \
        J__U_GROWCK(Pop1, cJL_LEAF5_MAXPOP1, j__L_Leaf5PopToWords)
#define JL_LEAF6GROWINPLACE(Pop1) \
        J__U_GROWCK(Pop1, cJL_LEAF6_MAXPOP1, j__L_Leaf6PopToWords)
#define JL_LEAF7GROWINPLACE(Pop1) \
        J__U_GROWCK(Pop1, cJL_LEAF7_MAXPOP1, j__L_Leaf7PopToWords)
#endif
#define JL_LEAFWGROWINPLACE(Pop1) \
        J__U_GROWCK(Pop1, cJL_LEAFW_MAXPOP1, j__L_LeafWPopToWords)
#define JL_LEAFVGROWINPLACE(Pop1)  \
        J__U_GROWCK(Pop1, cJU_BITSPERSUBEXPL,  j__L_LeafVPopToWords)

#define JL_LEAF1VALUEAREA(Pjv,Pop1)  (((PWord_t)(Pjv)) + j__L_Leaf1Offset[Pop1])
#define JL_LEAF2VALUEAREA(Pjv,Pop1)  (((PWord_t)(Pjv)) + j__L_Leaf2Offset[Pop1])
#define JL_LEAF3VALUEAREA(Pjv,Pop1)  (((PWord_t)(Pjv)) + j__L_Leaf3Offset[Pop1])
#ifdef JU_64BIT
#define JL_LEAF4VALUEAREA(Pjv,Pop1)  (((PWord_t)(Pjv)) + j__L_Leaf4Offset[Pop1])
#define JL_LEAF5VALUEAREA(Pjv,Pop1)  (((PWord_t)(Pjv)) + j__L_Leaf5Offset[Pop1])
#define JL_LEAF6VALUEAREA(Pjv,Pop1)  (((PWord_t)(Pjv)) + j__L_Leaf6Offset[Pop1])
#define JL_LEAF7VALUEAREA(Pjv,Pop1)  (((PWord_t)(Pjv)) + j__L_Leaf7Offset[Pop1])
#endif
#define JL_LEAFWVALUEAREA(Pjv,Pop1)  (((PWord_t)(Pjv)) + j__L_LeafWOffset[Pop1])

#define JL_LEAF1POPTOWORDS(Pop1)        (j__L_Leaf1PopToWords[Pop1])
#define JL_LEAF2POPTOWORDS(Pop1)        (j__L_Leaf2PopToWords[Pop1])
#define JL_LEAF3POPTOWORDS(Pop1)        (j__L_Leaf3PopToWords[Pop1])
#ifdef JU_64BIT
#define JL_LEAF4POPTOWORDS(Pop1)        (j__L_Leaf4PopToWords[Pop1])
#define JL_LEAF5POPTOWORDS(Pop1)        (j__L_Leaf5PopToWords[Pop1])
#define JL_LEAF6POPTOWORDS(Pop1)        (j__L_Leaf6PopToWords[Pop1])
#define JL_LEAF7POPTOWORDS(Pop1)        (j__L_Leaf7PopToWords[Pop1])
#endif
#define JL_LEAFWPOPTOWORDS(Pop1)        (j__L_LeafWPopToWords[Pop1])
#define JL_LEAFVPOPTOWORDS(Pop1)        (j__L_LeafVPopToWords[Pop1])


// FUNCTIONS TO ALLOCATE OBJECTS:

PjLpm_t j__udyLAllocJLPM(void);                         // constant size.

Pjbl_t  j__udyLAllocJBL(          PjLpm_t);             // constant size.
Pjbb_t  j__udyLAllocJBB(          PjLpm_t);             // constant size.
Pjp_t   j__udyLAllocJBBJP(Word_t, PjLpm_t);
Pjbu_t  j__udyLAllocJBU(          PjLpm_t);             // constant size.

Pjll_t  j__udyLAllocJLL1( Word_t, PjLpm_t);
Pjll_t  j__udyLAllocJLL2( Word_t, PjLpm_t);
Pjll_t  j__udyLAllocJLL3( Word_t, PjLpm_t);

#ifdef JU_64BIT
Pjll_t  j__udyLAllocJLL4( Word_t, PjLpm_t);
Pjll_t  j__udyLAllocJLL5( Word_t, PjLpm_t);
Pjll_t  j__udyLAllocJLL6( Word_t, PjLpm_t);
Pjll_t  j__udyLAllocJLL7( Word_t, PjLpm_t);
#endif

Pjlw_t  j__udyLAllocJLW(  Word_t         );             // no PjLpm_t needed.
PjLlb_t j__udyLAllocJLB1(         PjLpm_t);             // constant size.
Pjv_t   j__udyLAllocJV(   Word_t, PjLpm_t);


// FUNCTIONS TO FREE OBJECTS:

void    j__udyLFreeJLPM( PjLpm_t,        PjLpm_t);      // constant size.

void    j__udyLFreeJBL(  Pjbl_t,         PjLpm_t);      // constant size.
void    j__udyLFreeJBB(  Pjbb_t,         PjLpm_t);      // constant size.
void    j__udyLFreeJBBJP(Pjp_t,  Word_t, PjLpm_t);
void    j__udyLFreeJBU(  Pjbu_t,         PjLpm_t);      // constant size.

void    j__udyLFreeJLL1( Pjll_t, Word_t, PjLpm_t);
void    j__udyLFreeJLL2( Pjll_t, Word_t, PjLpm_t);
void    j__udyLFreeJLL3( Pjll_t, Word_t, PjLpm_t);

#ifdef JU_64BIT
void    j__udyLFreeJLL4( Pjll_t, Word_t, PjLpm_t);
void    j__udyLFreeJLL5( Pjll_t, Word_t, PjLpm_t);
void    j__udyLFreeJLL6( Pjll_t, Word_t, PjLpm_t);
void    j__udyLFreeJLL7( Pjll_t, Word_t, PjLpm_t);
#endif

void    j__udyLFreeJLW(  Pjlw_t, Word_t, PjLpm_t);
void    j__udyLFreeJLB1( PjLlb_t,        PjLpm_t);      // constant size.
void    j__udyLFreeJV(   Pjv_t,  Word_t, PjLpm_t);
void    j__udyLFreeSM(   Pjp_t,          PjLpm_t);      // everything below Pjp.

#endif // ! _JUDYL_INCLUDED
