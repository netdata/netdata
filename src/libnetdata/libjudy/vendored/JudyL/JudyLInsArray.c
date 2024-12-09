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

// TBD:  It would probably be faster for the caller if the JudyL version took
// PIndex as an interleaved array of indexes and values rather than just
// indexes with a separate values array (PValue), especially considering
// indexes and values are copied here with for-loops anyway and not the
// equivalent of memcpy().  All code could be revised to simply count by two
// words for JudyL?  Supports "streaming" the data to/from disk better later?
// In which case get rid of JU_ERRNO_NULLPVALUE, no longer needed, and simplify
// the API to this code.
// _________________

// @(#) $Revision: 4.21 $ $Source: /judy/src/JudyCommon/JudyInsArray.c $
//
// Judy1SetArray() and JudyLInsArray() functions for Judy1 and JudyL.
// Compile with one of -DJUDY1 or -DJUDYL.

#if (! (defined(JUDY1) || defined(JUDYL)))
#error:  One of -DJUDY1 or -DJUDYL must be specified.
#endif

#ifdef JUDY1
#include "Judy1.h"
#else
#include "JudyL.h"
#endif

#include "JudyPrivate1L.h"

DBGCODE(extern void JudyCheckPop(Pvoid_t PArray);)


// IMMED AND LEAF SIZE AND BRANCH TYPE ARRAYS:
//
// These support fast and easy lookup by level.

static uint8_t immed_maxpop1[] = {
    0,
    cJU_IMMED1_MAXPOP1,
    cJU_IMMED2_MAXPOP1,
    cJU_IMMED3_MAXPOP1,
#ifdef JU_64BIT
    cJU_IMMED4_MAXPOP1,
    cJU_IMMED5_MAXPOP1,
    cJU_IMMED6_MAXPOP1,
    cJU_IMMED7_MAXPOP1,
#endif
    // note:  There are no IMMEDs for whole words.
};

static uint8_t leaf_maxpop1[] = {
    0,
#if (defined(JUDYL) || (! defined(JU_64BIT)))
    cJU_LEAF1_MAXPOP1,
#else
    0,                                  // 64-bit Judy1 has no Leaf1.
#endif
    cJU_LEAF2_MAXPOP1,
    cJU_LEAF3_MAXPOP1,
#ifdef JU_64BIT
    cJU_LEAF4_MAXPOP1,
    cJU_LEAF5_MAXPOP1,
    cJU_LEAF6_MAXPOP1,
    cJU_LEAF7_MAXPOP1,
#endif
    // note:  Root-level leaves are handled differently.
};

static uint8_t branchL_JPtype[] = {
    0,
    0,
    cJU_JPBRANCH_L2,
    cJU_JPBRANCH_L3,
#ifdef JU_64BIT
    cJU_JPBRANCH_L4,
    cJU_JPBRANCH_L5,
    cJU_JPBRANCH_L6,
    cJU_JPBRANCH_L7,
#endif
    cJU_JPBRANCH_L,
};

static uint8_t branchB_JPtype[] = {
    0,
    0,
    cJU_JPBRANCH_B2,
    cJU_JPBRANCH_B3,
#ifdef JU_64BIT
    cJU_JPBRANCH_B4,
    cJU_JPBRANCH_B5,
    cJU_JPBRANCH_B6,
    cJU_JPBRANCH_B7,
#endif
    cJU_JPBRANCH_B,
};

static uint8_t branchU_JPtype[] = {
    0,
    0,
    cJU_JPBRANCH_U2,
    cJU_JPBRANCH_U3,
#ifdef JU_64BIT
    cJU_JPBRANCH_U4,
    cJU_JPBRANCH_U5,
    cJU_JPBRANCH_U6,
    cJU_JPBRANCH_U7,
#endif
    cJU_JPBRANCH_U,
};

// Subexpanse masks are similer to JU_DCDMASK() but without the need to clear
// the first digits bits.  Avoid doing variable shifts by precomputing a
// lookup array.

static Word_t subexp_mask[] = {
    0,
    ~cJU_POP0MASK(1),
    ~cJU_POP0MASK(2),
    ~cJU_POP0MASK(3),
#ifdef JU_64BIT
    ~cJU_POP0MASK(4),
    ~cJU_POP0MASK(5),
    ~cJU_POP0MASK(6),
    ~cJU_POP0MASK(7),
#endif
};


// FUNCTION PROTOTYPES:

static bool_t j__udyInsArray(Pjp_t PjpParent, int Level, PWord_t PPop1,
                             PWord_t PIndex,
#ifdef JUDYL
                             Pjv_t   PValue,
#endif
                             Pjpm_t  Pjpm);


// ****************************************************************************
// J U D Y   1   S E T   A R R A Y
// J U D Y   L   I N S   A R R A Y
//
// Main entry point.  See the manual entry for external overview.
//
// TBD:  Until thats written, note that the function returns 1 for success or
// JERRI for serious error, including insufficient memory to build whole array;
// use Judy*Count() to see how many were stored, the first N of the total
// Count.  Also, since it takes Count == Pop1, it cannot handle a full array.
// Also, "sorted" means ascending without duplicates, otherwise you get the
// "unsorted" error.
//
// The purpose of these functions is to allow rapid construction of a large
// Judy array given a sorted list of indexes (and for JudyL, corresponding
// values).  At least one customer saw this as useful, and probably it would
// also be useful as a sufficient workaround for fast(er) unload/reload to/from
// disk.
//
// This code is written recursively for simplicity, until/unless someone
// decides to make it faster and more complex.  Hopefully recursion is fast
// enough simply because the function is so much faster than a series of
// Set/Ins calls.

#ifdef JUDY1
FUNCTION int Judy1SetArray
#else
FUNCTION int JudyLInsArray
#endif
        (
        PPvoid_t  PPArray,      // in which to insert, initially empty.
        Word_t    Count,        // number of indexes (and values) to insert.
const   Word_t *  const PIndex, // list of indexes to insert.
#ifdef JUDYL
const   Word_t *  const PValue, // list of corresponding values.
#endif
        PJError_t PJError       // optional, for returning error info.
        )
{
        Pjlw_t    Pjlw;         // new root-level leaf.
        Pjlw_t    Pjlwindex;    // first index in root-level leaf.
        int       offset;       // in PIndex.


// CHECK FOR NULL OR NON-NULL POINTER (error by caller):

        if (PPArray == (PPvoid_t) NULL)
        { JU_SET_ERRNO(PJError, JU_ERRNO_NULLPPARRAY);   return(JERRI); }

        if (*PPArray != (Pvoid_t) NULL)
        { JU_SET_ERRNO(PJError, JU_ERRNO_NONNULLPARRAY); return(JERRI); }

        if (PIndex == (PWord_t) NULL)
        { JU_SET_ERRNO(PJError, JU_ERRNO_NULLPINDEX);    return(JERRI); }

#ifdef JUDYL
        if (PValue == (PWord_t) NULL)
        { JU_SET_ERRNO(PJError, JU_ERRNO_NULLPVALUE);    return(JERRI); }
#endif


// HANDLE LARGE COUNT (= POP1) (typical case):
//
// Allocate and initialize a JPM, set the root pointer to point to it, and then
// build the tree underneath it.

// Common code for unusual error handling when no JPM available:

        if (Count > cJU_LEAFW_MAXPOP1)  // too big for root-level leaf.
        {
            Pjpm_t Pjpm;                        // new, to allocate.

// Allocate JPM:

            Pjpm = j__udyAllocJPM();
            JU_CHECKALLOC(Pjpm_t, Pjpm, JERRI);
            *PPArray = (Pvoid_t) Pjpm;

// Set some JPM fields:

            (Pjpm->jpm_Pop0) = Count - 1;
            // note: (Pjpm->jpm_TotalMemWords) is now initialized.

// Build Judy tree:
//
// In case of error save the final Count, possibly modified, unless modified to
// 0, in which case free the JPM itself:

            if (! j__udyInsArray(&(Pjpm->jpm_JP), cJU_ROOTSTATE, &Count,
                                 (PWord_t) PIndex,
#ifdef JUDYL
                                 (Pjv_t) PValue,
#endif
                                 Pjpm))
            {
                JU_COPY_ERRNO(PJError, Pjpm);

                if (Count)              // partial success, adjust pop0:
                {
                    (Pjpm->jpm_Pop0) = Count - 1;
                }
                else                    // total failure, free JPM:
                {
                    j__udyFreeJPM(Pjpm, (Pjpm_t) NULL);
                    *PPArray = (Pvoid_t) NULL;
                }

                DBGCODE(JudyCheckPop(*PPArray);)
                return(JERRI);
            }

            DBGCODE(JudyCheckPop(*PPArray);)
            return(1);

        } // large count


// HANDLE SMALL COUNT (= POP1):
//
// First ensure indexes are in sorted order:

        for (offset = 1; offset < Count; ++offset)
        {
            if (PIndex[offset - 1] >= PIndex[offset])
            { JU_SET_ERRNO(PJError, JU_ERRNO_UNSORTED); return(JERRI); }
        }

        if (Count == 0) return(1);              // *PPArray remains null.

        {
            Pjlw      = j__udyAllocJLW(Count + 1);
                        JU_CHECKALLOC(Pjlw_t, Pjlw, JERRI);
            *PPArray  = (Pvoid_t) Pjlw;
            Pjlw[0]   = Count - 1;              // set pop0.
            Pjlwindex = Pjlw + 1;
        }

// Copy whole-word indexes (and values) to the root-level leaf:

          JU_COPYMEM(Pjlwindex,                      PIndex, Count);
JUDYLCODE(JU_COPYMEM(JL_LEAFWVALUEAREA(Pjlw, Count), PValue, Count));

        DBGCODE(JudyCheckPop(*PPArray);)
        return(1);

} // Judy1SetArray() / JudyLInsArray()


// ****************************************************************************
// __ J U D Y   I N S   A R R A Y
//
// Given:
//
// - a pointer to a JP
//
// - the JPs level in the tree, that is, the number of digits left to decode
//   in the indexes under the JP (one less than the level of the JPM or branch
//   in which the JP resides); cJU_ROOTSTATE on first entry (when JP is the one
//   in the JPM), down to 1 for a Leaf1, LeafB1, or FullPop
//
// - a pointer to the number of indexes (and corresponding values) to store in
//   this subtree, to modify in case of partial success
//
// - a list of indexes (and for JudyL, corresponding values) to store in this
//   subtree
//
// - a JPM for tracking memory usage and returning errors
//
// Recursively build a subtree (immediate indexes, leaf, or branch with
// subtrees) and modify the JP accordingly.  On the way down, build a BranchU
// (only) for any expanse with *PPop1 too high for a leaf; on the way out,
// convert the BranchU to a BranchL or BranchB if appropriate.  Keep memory
// statistics in the JPM.
//
// Return TRUE for success, or FALSE with error information set in the JPM in
// case of error, in which case leave a partially constructed but healthy tree,
// and modify parent population counts on the way out.
//
// Note:  Each call of this function makes all modifications to the PjpParent
// it receives; neither the parent nor child calls do this.

FUNCTION static bool_t j__udyInsArray(
        Pjp_t   PjpParent,              // parent JP in/under which to store.
        int     Level,                  // initial digits remaining to decode.
        PWord_t PPop1,                  // number of indexes to store.
        PWord_t PIndex,                 // list of indexes to store.
#ifdef JUDYL
        Pjv_t   PValue,                 // list of corresponding values.
#endif
        Pjpm_t  Pjpm)                   // for memory and errors.
{
        Pjp_t   Pjp;                    // lower-level JP.
        Word_t  Pjbany;                 // any type of branch.
        int     levelsub;               // actual, of Pjps node, <= Level.
        Word_t  pop1 = *PPop1;          // fast local value.
        Word_t  pop1sub;                // population of one subexpanse.
        uint8_t JPtype;                 // current JP type.
        uint8_t JPtype_null;            // precomputed value for new branch.
        jp_t    JPnull;                 // precomputed for speed.
        Pjbu_t  PjbuRaw;                // constructed BranchU.
        Pjbu_t  Pjbu;
        int     digit;                  // in BranchU.
        Word_t  digitmask;              // for a digit in a BranchU.
        Word_t  digitshifted;           // shifted to correct offset.
        Word_t  digitshincr;            // increment for digitshifted.
        int     offset;                 // in PIndex, or a bitmap subexpanse.
        int     numJPs;                 // number non-null in a BranchU.
        bool_t  retval;                 // to return from this func.
JUDYLCODE(Pjv_t PjvRaw);                // destination value area.
JUDYLCODE(Pjv_t Pjv);


// MACROS FOR COMMON CODE:
//
// Note:  These use function and local parameters from the context.
// Note:  Assume newly allocated memory is zeroed.

// Indicate whether a sorted list of indexes in PIndex, based on the first and
// last indexes in the list using pop1, are in the same subexpanse between
// Level and L_evel:
//
// This can be confusing!  Note that SAMESUBEXP(L) == TRUE means the indexes
// are the same through level L + 1, and it says nothing about level L and
// lower; they might be the same or they might differ.
//
// Note:  In principle SAMESUBEXP needs a mask for the digits from Level,
// inclusive, to L_evel, exclusive.  But in practice, since the indexes are all
// known to be identical above Level, it just uses a mask for the digits
// through L_evel + 1; see subexp_mask[].

#define SAMESUBEXP(L_evel) \
        (! ((PIndex[0] ^ PIndex[pop1 - 1]) & subexp_mask[L_evel]))

// Set PjpParent to a null JP appropriate for the level of the node to which it
// points, which is 1 less than the level of the node in which the JP resides,
// which is by definition Level:
//
// Note:  This can set the JPMs JP to an invalid jp_Type, but it doesnt
// matter because the JPM is deleted by the caller.

#define SETJPNULL_PARENT \
            JU_JPSETADT(PjpParent, 0, 0, cJU_JPNULL1 + Level - 1);

// Variation to set a specified JP (in a branch being built) to a precomputed
// null JP:

#define SETJPNULL(Pjp) *(Pjp) = JPnull

// Handle complete (as opposed to partial) memory allocation failure:  Set the
// parent JP to an appropriate null type (to leave a consistent tree), zero the
// callers population count, and return FALSE:
//
// Note:  At Level == cJU_ROOTSTATE this sets the JPMs JPs jp_Type to a bogus
// value, but it doesnt matter because the JPM should be deleted by the
// caller.

#define NOMEM { SETJPNULL_PARENT; *PPop1 = 0; return(FALSE); }

// Allocate a Leaf1-N and save the address in Pjll; in case of failure, NOMEM:

#define ALLOCLEAF(AllocLeaf) \
        if ((PjllRaw = AllocLeaf(pop1, Pjpm)) == (Pjll_t) NULL) NOMEM; \
        Pjll = P_JLL(PjllRaw);

// Copy indexes smaller than words (and values which are whole words) from
// given arrays to immediate indexes or a leaf:
//
// TBD:  These macros overlap with some of the code in JudyCascade.c; do some
// merging?  That file has functions while these are macros.

#define COPYTOLEAF_EVEN_SUB(Pjll,LeafType)              \
        {                                               \
            LeafType * P_leaf  = (LeafType *) (Pjll);   \
            Word_t     p_op1   = pop1;                  \
            PWord_t    P_Index = PIndex;                \
                                                        \
            assert(pop1 > 0);                           \
                                                        \
            do { *P_leaf++ = *P_Index++; /* truncates */\
            } while (--(p_op1));                        \
        }

#define COPYTOLEAF_ODD_SUB(cLevel,Pjll,Copy)            \
        {                                               \
            uint8_t * P_leaf  = (uint8_t *) (Pjll);     \
            Word_t    p_op1   = pop1;                   \
            PWord_t   P_Index = PIndex;                 \
                                                        \
            assert(pop1 > 0);                           \
                                                        \
            do {                                        \
                Copy(P_leaf, *P_Index);                 \
                P_leaf += (cLevel); ++P_Index;          \
            } while (--(p_op1));                        \
        }

#ifdef JUDY1

#define COPYTOLEAF_EVEN(Pjll,LeafType)   COPYTOLEAF_EVEN_SUB(Pjll,LeafType)
#define COPYTOLEAF_ODD(cLevel,Pjll,Copy) COPYTOLEAF_ODD_SUB(cLevel,Pjll,Copy)

#else // JUDYL adds copying of values:

#define COPYTOLEAF_EVEN(Pjll,LeafType)                  \
        {                                               \
            COPYTOLEAF_EVEN_SUB(Pjll,LeafType)          \
            JU_COPYMEM(Pjv, PValue, pop1);              \
        }

#define COPYTOLEAF_ODD(cLevel,Pjll,Copy)                \
        {                                               \
            COPYTOLEAF_ODD_SUB( cLevel,Pjll,Copy)       \
            JU_COPYMEM(Pjv, PValue, pop1);              \
        }

#endif

// Set the JP type for an immediate index, where BaseJPType is JPIMMED_*_02:

#define SETIMMTYPE(BaseJPType)  (PjpParent->jp_Type) = (BaseJPType) + pop1 - 2

// Allocate and populate a Leaf1-N:
//
// Build MAKELEAF_EVEN() and MAKELEAF_ODD() using macros for common code.

#define MAKELEAF_SUB1(AllocLeaf,ValueArea,LeafType)                     \
        ALLOCLEAF(AllocLeaf);                                           \
        JUDYLCODE(Pjv = ValueArea(Pjll, pop1))


#define MAKELEAF_SUB2(cLevel,JPType)                                    \
{                                                                       \
        Word_t D_cdP0;                                                  \
        assert(pop1 - 1 <= cJU_POP0MASK(cLevel));                       \
        D_cdP0 = (*PIndex & cJU_DCDMASK(cLevel)) | (pop1 - 1);          \
        JU_JPSETADT(PjpParent, (Word_t)PjllRaw, D_cdP0, JPType);        \
}


#define MAKELEAF_EVEN(cLevel,JPType,AllocLeaf,ValueArea,LeafType)       \
        MAKELEAF_SUB1(AllocLeaf,ValueArea,LeafType);                    \
        COPYTOLEAF_EVEN(Pjll, LeafType);                                \
        MAKELEAF_SUB2(cLevel, JPType)

#define MAKELEAF_ODD(cLevel,JPType,AllocLeaf,ValueArea,Copy)            \
        MAKELEAF_SUB1(AllocLeaf,ValueArea,LeafType);                    \
        COPYTOLEAF_ODD(cLevel, Pjll, Copy);                             \
        MAKELEAF_SUB2(cLevel, JPType)

// Ensure that the indexes to be stored in immediate indexes or a leaf are
// sorted:
//
// This check is pure overhead, but required in order to protect the Judy array
// against caller error, to avoid a later corruption or core dump from a
// seemingly valid Judy array.  Do this check piecemeal at the leaf level while
// the indexes are already in the cache.  Higher-level order-checking occurs
// while building branches.
//
// Note:  Any sorting error in the expanse of a single immediate indexes JP or
// a leaf => save no indexes in that expanse.

#define CHECKLEAFORDER                                                  \
        {                                                               \
            for (offset = 1; offset < pop1; ++offset)                   \
            {                                                           \
                if (PIndex[offset - 1] >= PIndex[offset])               \
                {                                                       \
                    SETJPNULL_PARENT;                                   \
                    *PPop1 = 0;                                         \
                    JU_SET_ERRNO_NONNULL(Pjpm, JU_ERRNO_UNSORTED);      \
                    return(FALSE);                                      \
                }                                                       \
            }                                                           \
        }


// ------ START OF CODE ------

        assert( Level >= 1);
        assert( Level <= cJU_ROOTSTATE);
        assert((Level <  cJU_ROOTSTATE) || (pop1 > cJU_LEAFW_MAXPOP1));


// CHECK FOR TOP LEVEL:
//
// Special case:  If at the top level (PjpParent is in the JPM), a top-level
// branch must be created, even if its a BranchL with just one JP.  (The JPM
// cannot point to a leaf because the leaf would have to be a lower-level,
// higher-capacity leaf under a narrow pointer (otherwise a root-level leaf
// would suffice), and the JPMs JP cant handle a narrow pointer because the
// jp_DcdPopO field isnt big enough.)  Otherwise continue to check for a pop1
// small enough to support immediate indexes or a leaf before giving up and
// making a lower-level branch.

        if (Level == cJU_ROOTSTATE)
        {
            levelsub = cJU_ROOTSTATE;
            goto BuildBranch2;
        }
        assert(Level < cJU_ROOTSTATE);


// SKIP JPIMMED_*_01:
//
// Immeds with pop1 == 1 should be handled in-line during branch construction.

        assert(pop1 > 1);


// BUILD JPIMMED_*_02+:
//
// The starting address of the indexes depends on Judy1 or JudyL; also, JudyL
// includes a pointer to a values-only leaf.

        if (pop1 <= immed_maxpop1[Level])      // note: always < root level.
        {
            JUDY1CODE(uint8_t * Pjll = (uint8_t *) (PjpParent->jp_1Index);)
            JUDYLCODE(uint8_t * Pjll = (uint8_t *) (PjpParent->jp_LIndex);)

            CHECKLEAFORDER;             // indexes to be stored are sorted.

#ifdef JUDYL
            if ((PjvRaw = j__udyLAllocJV(pop1, Pjpm)) == (Pjv_t) NULL)
                NOMEM;
            (PjpParent->jp_Addr) = (Word_t) PjvRaw;
            Pjv = P_JV(PjvRaw);
#endif

            switch (Level)
            {
            case 1: COPYTOLEAF_EVEN(Pjll, uint8_t);
                    SETIMMTYPE(cJU_JPIMMED_1_02);
                    break;
#if (defined(JUDY1) || defined(JU_64BIT))
            case 2: COPYTOLEAF_EVEN(Pjll, uint16_t);
                    SETIMMTYPE(cJU_JPIMMED_2_02);
                    break;
            case 3: COPYTOLEAF_ODD(3, Pjll, JU_COPY3_LONG_TO_PINDEX);
                    SETIMMTYPE(cJU_JPIMMED_3_02);
                    break;
#endif
#if (defined(JUDY1) && defined(JU_64BIT))
            case 4: COPYTOLEAF_EVEN(Pjll, uint32_t);
                    SETIMMTYPE(cJ1_JPIMMED_4_02);
                    break;
            case 5: COPYTOLEAF_ODD(5, Pjll, JU_COPY5_LONG_TO_PINDEX);
                    SETIMMTYPE(cJ1_JPIMMED_5_02);
                    break;
            case 6: COPYTOLEAF_ODD(6, Pjll, JU_COPY6_LONG_TO_PINDEX);
                    SETIMMTYPE(cJ1_JPIMMED_6_02);
                    break;
            case 7: COPYTOLEAF_ODD(7, Pjll, JU_COPY7_LONG_TO_PINDEX);
                    SETIMMTYPE(cJ1_JPIMMED_7_02);
                    break;
#endif
            default: assert(FALSE);     // should be impossible.
            }

            return(TRUE);               // note: no children => no *PPop1 mods.

        } // JPIMMED_*_02+


// BUILD JPLEAF*:
//
// This code is a little tricky.  The method is:  For each level starting at
// the present Level down through levelsub = 1, and then as a special case for
// LeafB1 and FullPop (which are also at levelsub = 1 but have different
// capacity, see later), check if pop1 fits in a leaf (using leaf_maxpop1[])
// at that level.  If so, except for Level == levelsub, check if all of the
// current indexes to be stored are in the same (narrow) subexpanse, that is,
// the digits from Level to levelsub + 1, inclusive, are identical between the
// first and last index in the (sorted) list (in PIndex).  If this condition is
// satisfied at any level, build a leaf at that level (under a narrow pointer
// if Level > levelsub).
//
// Note:  Doing the search in this order results in storing the indexes in
// "least compressed form."

        for (levelsub = Level; levelsub >= 1; --levelsub)
        {
            Pjll_t PjllRaw;
            Pjll_t Pjll;

// Check if pop1 is too large to fit in a leaf at levelsub; if so, try the next
// lower level:

            if (pop1 > leaf_maxpop1[levelsub]) continue;

// If pop1 fits in a leaf at levelsub, but levelsub is lower than Level, must
// also check whether all the indexes in the expanse to store can in fact be
// placed under a narrow pointer; if not, a leaf cannot be used, at this or any
// lower level (levelsub):

            if ((levelsub < Level) && (! SAMESUBEXP(levelsub)))
                goto BuildBranch;       // cant use a narrow, need a branch.

// Ensure valid pop1 and all indexes are in fact common through Level:

            assert(pop1 <= cJU_POP0MASK(Level) + 1);
            assert(! ((PIndex[0] ^ PIndex[pop1 - 1]) & cJU_DCDMASK(Level)));

            CHECKLEAFORDER;             // indexes to be stored are sorted.

// Build correct type of leaf:
//
// Note:  The jp_DcdPopO and jp_Type assignments in MAKELEAF_* happen correctly
// for the levelsub (not Level) of the new leaf, even if its under a narrow
// pointer.

            switch (levelsub)
            {
#if (defined(JUDYL) || (! defined(JU_64BIT)))
            case 1: MAKELEAF_EVEN(1, cJU_JPLEAF1, j__udyAllocJLL1,
                                  JL_LEAF1VALUEAREA, uint8_t);
                    break;
#endif
            case 2: MAKELEAF_EVEN(2, cJU_JPLEAF2, j__udyAllocJLL2,
                                  JL_LEAF2VALUEAREA, uint16_t);
                    break;
            case 3: MAKELEAF_ODD( 3, cJU_JPLEAF3, j__udyAllocJLL3,
                                  JL_LEAF3VALUEAREA, JU_COPY3_LONG_TO_PINDEX);
                    break;
#ifdef JU_64BIT
            case 4: MAKELEAF_EVEN(4, cJU_JPLEAF4, j__udyAllocJLL4,
                                  JL_LEAF4VALUEAREA, uint32_t);
                    break;
            case 5: MAKELEAF_ODD( 5, cJU_JPLEAF5, j__udyAllocJLL5,
                                  JL_LEAF5VALUEAREA, JU_COPY5_LONG_TO_PINDEX);
                    break;
            case 6: MAKELEAF_ODD( 6, cJU_JPLEAF6, j__udyAllocJLL6,
                                  JL_LEAF6VALUEAREA, JU_COPY6_LONG_TO_PINDEX);
                    break;
            case 7: MAKELEAF_ODD( 7, cJU_JPLEAF7, j__udyAllocJLL7,
                                  JL_LEAF7VALUEAREA, JU_COPY7_LONG_TO_PINDEX);
                    break;
#endif
            default: assert(FALSE);     // should be impossible.
            }

            return(TRUE);               // note: no children => no *PPop1 mods.

        } // JPLEAF*


// BUILD JPLEAF_B1 OR JPFULLPOPU1:
//
// See above about JPLEAF*.  If pop1 doesnt fit in any level of linear leaf,
// it might still fit in a LeafB1 or FullPop, perhaps under a narrow pointer.

        if ((Level == 1) || SAMESUBEXP(1))      // same until last digit.
        {
            Pjlb_t PjlbRaw;                     // for bitmap leaf.
            Pjlb_t Pjlb;

            assert(pop1 <= cJU_JPFULLPOPU1_POP0 + 1);
            CHECKLEAFORDER;             // indexes to be stored are sorted.

#ifdef JUDY1

// JPFULLPOPU1:

            if (pop1 == cJU_JPFULLPOPU1_POP0 + 1)
            {
                Word_t  Addr  = PjpParent->jp_Addr;
                Word_t  DcdP0 = (*PIndex & cJU_DCDMASK(1))
                                        | cJU_JPFULLPOPU1_POP0;
                JU_JPSETADT(PjpParent, Addr, DcdP0, cJ1_JPFULLPOPU1);

                return(TRUE);
            }
#endif

// JPLEAF_B1:

            if ((PjlbRaw = j__udyAllocJLB1(Pjpm)) == (Pjlb_t) NULL)
                NOMEM;
            Pjlb = P_JLB(PjlbRaw);

            for (offset = 0; offset < pop1; ++offset)
                JU_BITMAPSETL(Pjlb, PIndex[offset]);

            retval = TRUE;              // default.

#ifdef JUDYL

// Build subexpanse values-only leaves (LeafVs) under LeafB1:

            for (offset = 0; offset < cJU_NUMSUBEXPL; ++offset)
            {
                if (! (pop1sub = j__udyCountBitsL(JU_JLB_BITMAP(Pjlb, offset))))
                    continue;           // skip empty subexpanse.

// Allocate one LeafV = JP subarray; if out of memory, clear bitmaps for higher
// subexpanses and adjust *PPop1:

                if ((PjvRaw = j__udyLAllocJV(pop1sub, Pjpm))
                 == (Pjv_t) NULL)
                {
                    for (/* null */; offset < cJU_NUMSUBEXPL; ++offset)
                    {
                        *PPop1 -= j__udyCountBitsL(JU_JLB_BITMAP(Pjlb, offset));
                        JU_JLB_BITMAP(Pjlb, offset) = 0;
                    }

                    retval = FALSE;
                    break;
                }

// Populate values-only leaf and save the pointer to it:

                Pjv = P_JV(PjvRaw);
                JU_COPYMEM(Pjv, PValue, pop1sub);
                JL_JLB_PVALUE(Pjlb, offset) = PjvRaw;   // first-tier pointer.
                PValue += pop1sub;

            } // for each subexpanse

#endif // JUDYL

// Attach new LeafB1 to parent JP; note use of *PPop1 possibly < pop1:

            JU_JPSETADT(PjpParent, (Word_t) PjlbRaw, 
                    (*PIndex & cJU_DCDMASK(1)) | (*PPop1 - 1), cJU_JPLEAF_B1);

            return(retval);

        } // JPLEAF_B1 or JPFULLPOPU1


// BUILD JPBRANCH_U*:
//
// Arriving at BuildBranch means Level < top level but the pop1 is too large
// for immediate indexes or a leaf, even under a narrow pointer, including a
// LeafB1 or FullPop at level 1.  This implies SAMESUBEXP(1) == FALSE, that is,
// the indexes to be stored "branch" at level 2 or higher.

BuildBranch:    // come here directly if a leaf wont work.

        assert(Level >= 2);
        assert(Level < cJU_ROOTSTATE);
        assert(! SAMESUBEXP(1));                // sanity check, see above.

// Determine the appropriate level for a new branch node; see if a narrow
// pointer can be used:
//
// This can be confusing.  The branch is required at the lowest level L where
// the indexes to store are not in the same subexpanse at level L-1.  Work down
// from Level to tree level 3, which is 1 above the lowest tree level = 2 at
// which a branch can be used.  Theres no need to check SAMESUBEXP at level 2
// because its known to be false at level 2-1 = 1.
//
// Note:  Unlike for a leaf node, a narrow pointer is always used for a branch
// if possible, that is, maximum compression is always used, except at the top
// level of the tree, where a JPM cannot support a narrow pointer, meaning a
// top BranchL can have a single JP (fanout = 1); but that case jumps directly
// to BuildBranch2.
//
// Note:  For 32-bit systems the only usable values for a narrow pointer are
// Level = 3 and levelsub = 2; 64-bit systems have many more choices; but
// hopefully this for-loop is fast enough even on a 32-bit system.
//
// TBD:  If not fast enough, #ifdef JU_64BIT and handle the 32-bit case faster.

        for (levelsub = Level; levelsub >= 3; --levelsub)  // see above.
            if (! SAMESUBEXP(levelsub - 1))     // at limit of narrow pointer.
                break;                          // put branch at levelsub.

BuildBranch2:   // come here directly for Level = levelsub = cJU_ROOTSTATE.

        assert(levelsub >= 2);
        assert(levelsub <= Level);

// Initially build a BranchU:
//
// Always start with a BranchU because the number of populated subexpanses is
// not yet known.  Use digitmask, digitshifted, and digitshincr to avoid
// expensive variable shifts within JU_DIGITATSTATE within the loop.
//
// TBD:  The use of digitmask, etc. results in more increment operations per
// loop, is there an even faster way?
//
// TBD:  Would it pay to pre-count the populated JPs (subexpanses) and
// pre-compress the branch, that is, build a BranchL or BranchB immediately,
// also taking account of opportunistic uncompression rules?  Probably not
// because at high levels of the tree there might be huge numbers of indexes
// (hence cache lines) to scan in the PIndex array to determine the fanout
// (number of JPs) needed.

        if ((PjbuRaw = j__udyAllocJBU(Pjpm)) == (Pjbu_t) NULL) NOMEM;
        Pjbu = P_JBU(PjbuRaw);

        JPtype_null       = cJU_JPNULL1 + levelsub - 2;  // in new BranchU.
        JU_JPSETADT(&JPnull, 0, 0, JPtype_null);

        Pjp               = Pjbu->jbu_jp;           // for convenience in loop.
        numJPs            = 0;                      // non-null in the BranchU.
        digitmask         = cJU_MASKATSTATE(levelsub);   // see above.
        digitshincr       = 1UL << (cJU_BITSPERBYTE * (levelsub - 1));
        retval            = TRUE;

// Scan and populate JPs (subexpanses):
//
// Look for all indexes matching each digit in the BranchU (at the correct
// levelsub), and meanwhile notice any sorting error.  Increment PIndex (and
// PValue) and reduce pop1 for each subexpanse handled successfully.

        for (digit = digitshifted = 0;
             digit < cJU_BRANCHUNUMJPS;
             ++digit, digitshifted += digitshincr, ++Pjp)
        {
            DBGCODE(Word_t pop1subprev;)
            assert(pop1 != 0);          // end of indexes is handled elsewhere.

// Count indexes in digits subexpanse:

            for (pop1sub = 0; pop1sub < pop1; ++pop1sub)
                if (digitshifted != (PIndex[pop1sub] & digitmask)) break;

// Empty subexpanse (typical, performance path) or sorting error (rare):

            if (pop1sub == 0)
            {
                if (digitshifted < (PIndex[0] & digitmask))
                { SETJPNULL(Pjp); continue; }           // empty subexpanse.

                assert(pop1 < *PPop1);  // did save >= 1 index and decr pop1.
                JU_SET_ERRNO_NONNULL(Pjpm, JU_ERRNO_UNSORTED);
                goto AbandonBranch;
            }

// Non-empty subexpanse:
//
// First shortcut by handling pop1sub == 1 (JPIMMED_*_01) inline locally.

            if (pop1sub == 1)                   // note: can be at root level.
            {
                Word_t Addr = 0;
      JUDYLCODE(Addr    = (Word_t) (*PValue++);)
                JU_JPSETADT(Pjp, Addr, *PIndex, cJU_JPIMMED_1_01 + levelsub -2);

                ++numJPs;

                if (--pop1) { ++PIndex; continue; }  // more indexes to store.

                ++digit; ++Pjp;                 // skip JP just saved.
                goto ClearBranch;               // save time.
            }

// Recurse to populate one digits (subexpanses) JP; if successful, skip
// indexes (and values) just stored (performance path), except when expanse is
// completely stored:

            DBGCODE(pop1subprev = pop1sub;)

            if (j__udyInsArray(Pjp, levelsub - 1, &pop1sub, (PWord_t) PIndex,
#ifdef JUDYL
                               (Pjv_t) PValue,
#endif
                               Pjpm))
            {                                   // complete success.
                ++numJPs;
                assert(pop1subprev == pop1sub);
                assert(pop1 >= pop1sub);

                if ((pop1 -= pop1sub) != 0)     // more indexes to store:
                {
                    PIndex += pop1sub;          // skip indexes just stored.
          JUDYLCODE(PValue += pop1sub;)
                    continue;
                }
                // else leave PIndex in BranchUs expanse.

// No more indexes to store in BranchUs expanse:

                ++digit; ++Pjp;                 // skip JP just saved.
                goto ClearBranch;               // save time.
            }

// Handle any error at a lower level of recursion:
//
// In case of partial success, pop1sub != 0, but it was reduced from the value
// passed to j__udyInsArray(); skip this JP later during ClearBranch.

            assert(pop1subprev > pop1sub);      // check j__udyInsArray().
            assert(pop1        > pop1sub);      // check j__udyInsArray().

            if (pop1sub)                        // partial success.
            { ++digit; ++Pjp; ++numJPs; }       // skip JP just saved.

            pop1 -= pop1sub;                    // deduct saved indexes if any.

// Same-level sorting error, or any lower-level error; abandon the rest of the
// branch:
//
// Arrive here with pop1 = remaining unsaved indexes (always non-zero).  Adjust
// the *PPop1 value to record and return, modify retval, and use ClearBranch to
// finish up.

AbandonBranch:
            assert(pop1 != 0);                  // more to store, see above.
            assert(pop1 <= *PPop1);             // sanity check.

            *PPop1 -= pop1;                     // deduct unsaved indexes.
            pop1    = 0;                        // to avoid error later.
            retval  = FALSE;

// Error (rare), or end of indexes while traversing new BranchU (performance
// path); either way, mark the remaining JPs, if any, in the BranchU as nulls
// and exit the loop:
//
// Arrive here with digit and Pjp set to the first JP to set to null.

ClearBranch:
            for (/* null */; digit < cJU_BRANCHUNUMJPS; ++digit, ++Pjp)
                SETJPNULL(Pjp);
            break;                              // saves one more compare.

        } // for each digit


// FINISH JPBRANCH_U*:
//
// Arrive here with a BranchU built under Pjbu, numJPs set, and either:  retval
// == TRUE and *PPop1 unmodified, or else retval == FALSE, *PPop1 set to the
// actual number of indexes saved (possibly 0 for complete failure at a lower
// level upon the first call of j__udyInsArray()), and the Judy error set in
// Pjpm.  Either way, PIndex points to an index within the expanse just
// handled.

        Pjbany = (Word_t) PjbuRaw;              // default = use this BranchU.
        JPtype = branchU_JPtype[levelsub];

// Check for complete failure above:

        assert((! retval) || *PPop1);           // sanity check.

        if ((! retval) && (*PPop1 == 0))        // nothing stored, full failure.
        {
            j__udyFreeJBU(PjbuRaw, Pjpm);
            SETJPNULL_PARENT;
            return(FALSE);
        }

// Complete or partial success so far; watch for sorting error after the
// maximum digit (255) in the BranchU, which is indicated by having more
// indexes to store in the BranchUs expanse:
//
// For example, if an index to store has a digit of 255 at levelsub, followed
// by an index with a digit of 254, the for-loop above runs out of digits
// without reducing pop1 to 0.

        if (pop1 != 0)
        {
            JU_SET_ERRNO_NONNULL(Pjpm, JU_ERRNO_UNSORTED);
            *PPop1 -= pop1;             // deduct unsaved indexes.
            retval  = FALSE;
        }
        assert(*PPop1 != 0);            // branch (still) cannot be empty.


// OPTIONALLY COMPRESS JPBRANCH_U*:
//
// See if the BranchU should be compressed to a BranchL or BranchB; if so, do
// that and free the BranchU; otherwise just use the existing BranchU.  Follow
// the same rules as in JudyIns.c (version 4.95):  Only check local population
// (cJU_OPP_UNCOMP_POP0) for BranchL, and only check global memory efficiency
// (JU_OPP_UNCOMPRESS) for BranchB.  TBD:  Have the rules changed?
//
// Note:  Because of differing order of operations, the latter compression
// might not result in the same set of branch nodes as a series of sequential
// insertions.
//
// Note:  Allocating a BranchU only to sometimes convert it to a BranchL or
// BranchB is unfortunate, but attempting to work with a temporary BranchU on
// the stack and then allocate and keep it as a BranchU in many cases is worse
// in terms of error handling.


// COMPRESS JPBRANCH_U* TO JPBRANCH_L*:

        if (numJPs <= cJU_BRANCHLMAXJPS)        // JPs fit in a BranchL.
        {
            Pjbl_t PjblRaw = (Pjbl_t) NULL;     // new BranchL; init for cc.
            Pjbl_t Pjbl;

            if ((*PPop1 > JU_BRANCHL_MAX_POP)   // pop too high.
             || ((PjblRaw = j__udyAllocJBL(Pjpm)) == (Pjbl_t) NULL))
            {                                   // cant alloc BranchL.
                goto SetParent;                 // just keep BranchU.
            }

            Pjbl = P_JBL(PjblRaw);

// Copy BranchU JPs to BranchL:

            (Pjbl->jbl_NumJPs) = numJPs;
            offset = 0;

            for (digit = 0; digit < cJU_BRANCHUNUMJPS; ++digit)
            {
                if ((((Pjbu->jbu_jp) + digit)->jp_Type) == JPtype_null)
                    continue;

                (Pjbl->jbl_Expanse[offset  ]) = digit;
                (Pjbl->jbl_jp     [offset++]) = Pjbu->jbu_jp[digit];
            }
            assert(offset == numJPs);           // found same number.

// Free the BranchU and prepare to use the new BranchL instead:

            j__udyFreeJBU(PjbuRaw, Pjpm);

            Pjbany = (Word_t) PjblRaw;
            JPtype = branchL_JPtype[levelsub];

        } // compress to BranchL


// COMPRESS JPBRANCH_U* TO JPBRANCH_B*:
//
// If unable to allocate the BranchB or any JP subarray, free all related
// memory and just keep the BranchU.
//
// Note:  This use of JU_OPP_UNCOMPRESS is a bit conservative because the
// BranchU is already allocated while the (presumably smaller) BranchB is not,
// the opposite of how its used in single-insert code.

        else
        {
            Pjbb_t PjbbRaw = (Pjbb_t) NULL;     // new BranchB; init for cc.
            Pjbb_t Pjbb;
            Pjp_t  Pjp2;                        // in BranchU.

            if ((*PPop1 > JU_BRANCHB_MAX_POP)   // pop too high.
             || ((PjbbRaw = j__udyAllocJBB(Pjpm)) == (Pjbb_t) NULL))
            {                                   // cant alloc BranchB.
                goto SetParent;                 // just keep BranchU.
            }

            Pjbb = P_JBB(PjbbRaw);

// Set bits in bitmap for populated subexpanses:

            Pjp2 = Pjbu->jbu_jp;

            for (digit = 0; digit < cJU_BRANCHUNUMJPS; ++digit)
                if ((((Pjbu->jbu_jp) + digit)->jp_Type) != JPtype_null)
                    JU_BITMAPSETB(Pjbb, digit);

// Copy non-null JPs to BranchB JP subarrays:

            for (offset = 0; offset < cJU_NUMSUBEXPB; ++offset)
            {
                Pjp_t PjparrayRaw;
                Pjp_t Pjparray;

                if (! (numJPs = j__udyCountBitsB(JU_JBB_BITMAP(Pjbb, offset))))
                    continue;                   // skip empty subexpanse.

// If unable to allocate a JP subarray, free all BranchB memory so far and
// continue to use the BranchU:

                if ((PjparrayRaw = j__udyAllocJBBJP(numJPs, Pjpm))
                    == (Pjp_t) NULL)
                {
                    while (offset-- > 0)
                    {
                        if (JU_JBB_PJP(Pjbb, offset) == (Pjp_t) NULL) continue;

                        j__udyFreeJBBJP(JU_JBB_PJP(Pjbb, offset),
                                 j__udyCountBitsB(JU_JBB_BITMAP(Pjbb, offset)),
                                        Pjpm);
                    }
                    j__udyFreeJBB(PjbbRaw, Pjpm);
                    goto SetParent;             // keep BranchU.
                }

// Set one JP subarray pointer and copy the subexpanses JPs to the subarray:
//
// Scan the BranchU for non-null JPs until numJPs JPs are copied.

                JU_JBB_PJP(Pjbb, offset) = PjparrayRaw;
                Pjparray = P_JP(PjparrayRaw);

                while (numJPs-- > 0)
                {
                    while ((Pjp2->jp_Type) == JPtype_null)
                    {
                        ++Pjp2;
                        assert(Pjp2 < (Pjbu->jbu_jp) + cJU_BRANCHUNUMJPS);
                    }
                    *Pjparray++ = *Pjp2++;
                }
            } // for each subexpanse

// Free the BranchU and prepare to use the new BranchB instead:

            j__udyFreeJBU(PjbuRaw, Pjpm);

            Pjbany = (Word_t) PjbbRaw;
            JPtype = branchB_JPtype[levelsub];

        } // compress to BranchB


// COMPLETE OR PARTIAL SUCCESS:
//
// Attach new branch (under Pjp, with JPtype) to parent JP; note use of *PPop1,
// possibly reduced due to partial failure.

SetParent:
        (PjpParent->jp_Addr) = Pjbany;
        (PjpParent->jp_Type) = JPtype;

        if (Level < cJU_ROOTSTATE)              // PjpParent not in JPM:
        {
            Word_t DcdP0 = (*PIndex & cJU_DCDMASK(levelsub)) | (*PPop1 - 1);

            JU_JPSETADT(PjpParent ,Pjbany, DcdP0, JPtype);
        }

        return(retval);

} // j__udyInsArray()
