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

// @(#) $Revision: 4.32 $ $Source: /judy/src/JudyCommon/JudyPrevNextEmpty.c $
//
// Judy*PrevEmpty() and Judy*NextEmpty() functions for Judy1 and JudyL.
// Compile with one of -DJUDY1 or -DJUDYL.
//
// Compile with -DJUDYNEXT for the Judy*NextEmpty() function; otherwise
// defaults to Judy*PrevEmpty().
//
// Compile with -DTRACEJPSE to trace JP traversals.
//
// This file is separate from JudyPrevNext.c because it differs too greatly for
// ifdefs.  This might be a bit surprising, but there are two reasons:
//
// - First, down in the details, searching for an empty index (SearchEmpty) is
//   remarkably asymmetric with searching for a valid index (SearchValid),
//   mainly with respect to:  No return of a value area for JudyL; partially-
//   full versus totally-full JPs; and handling of narrow pointers.
//
// - Second, we chose to implement SearchEmpty without a backtrack stack or
//   backtrack engine, partly as an experiment, and partly because we think
//   restarting from the top of the tree is less likely for SearchEmpty than
//   for SearchValid, because empty indexes are more likely than valid indexes.
//
// A word about naming:  A prior version of this feature (see 4.13) was named
// Judy*Free(), but there were concerns about that being read as a verb rather
// than an adjective.  After prolonged debate and based on user input, we
// changed "Free" to "Empty".

#if (! (defined(JUDY1) || defined(JUDYL)))
#error:  One of -DJUDY1 or -DJUDYL must be specified.
#endif

#ifndef JUDYNEXT
#ifndef JUDYPREV
#define	JUDYPREV 1		// neither set => use default.
#endif
#endif

#ifdef JUDY1
#include "Judy1.h"
#else
#include "JudyL.h"
#endif

#include "JudyPrivate1L.h"

#ifdef TRACEJPSE
#include "JudyPrintJP.c"
#endif


// ****************************************************************************
// J U D Y   1   P R E V   E M P T Y
// J U D Y   1   N E X T   E M P T Y
// J U D Y   L   P R E V   E M P T Y
// J U D Y   L   N E X T   E M P T Y
//
// See the manual entry for the API.
//
// OVERVIEW OF Judy*PrevEmpty() / Judy*NextEmpty():
//
// See also for comparison the equivalent comments in JudyPrevNext.c.
//
// Take the callers *PIndex and subtract/add 1, but watch out for
// underflow/overflow, which means "no previous/next empty index found."  Use a
// reentrant switch statement (state machine, see SMGetRestart and
// SMGetContinue) to decode Index, starting with the JRP (PArray), through a
// JPM and branches, if any, down to an immediate or a leaf.  Look for Index in
// that immediate or leaf, and if not found (invalid index), return success
// (Index is empty).
//
// This search can result in a dead end where taking a different path is
// required.  There are four kinds of dead ends:
//
// BRANCH PRIMARY dead end:  Encountering a fully-populated JP for the
// appropriate digit in Index.  Search sideways in the branch for the
// previous/next absent/null/non-full JP, and if one is found, set Index to the
// highest/lowest index possible in that JPs expanse.  Then if the JP is an
// absent or null JP, return success; otherwise for a non-full JP, traverse
// through the partially populated JP.
//
// BRANCH SECONDARY dead end:  Reaching the end of a branch during a sideways
// search after a branch primary dead end.  Set Index to the lowest/highest
// index possible in the whole branchs expanse (one higher/lower than the
// previous/next branchs expanse), then restart at the top of the tree, which
// includes pre-decrementing/incrementing Index (again) and watching for
// underflow/overflow (again).
//
// LEAF PRIMARY dead end:  Finding a valid (non-empty) index in an immediate or
// leaf matching Index.  Search sideways in the immediate/leaf for the
// previous/next empty index; if found, set *PIndex to match and return success.
//
// LEAF SECONDARY dead end:  Reaching the end of an immediate or leaf during a
// sideways search after a leaf primary dead end.  Just as for a branch
// secondary dead end, restart at the top of the tree with Index set to the
// lowest/highest index possible in the whole immediate/leafs expanse.
// TBD:  If leaf secondary dead end occurs, could shortcut and treat it as a
// branch primary dead end; but this would require remembering the parent
// branchs type and offset (a "one-deep stack"), and also wrestling with
// narrow pointers, at least for leaves (but not for immediates).
//
// Note some ASYMMETRIES between SearchValid and SearchEmpty:
//
// - The SearchValid code, upon descending through a narrow pointer, if Index
//   is outside the expanse of the subsidiary node (effectively a secondary
//   dead end), must decide whether to backtrack or findlimit.  But the
//   SearchEmpty code simply returns success (Index is empty).
//
// - Similarly, the SearchValid code, upon finding no previous/next index in
//   the expanse of a narrow pointer (again, a secondary dead end), can simply
//   start to backtrack at the parent JP.  But the SearchEmpty code would have
//   to first determine whether or not the parent JPs narrow expanse contains
//   a previous/next empty index outside the subexpanse.  Rather than keeping a
//   parent state stack and backtracking this way, upon a secondary dead end,
//   the SearchEmpty code simply restarts at the top of the tree, whether or
//   not a narrow pointer is involved.  Again, see the equivalent comments in
//   JudyPrevNext.c for comparison.
//
// This function is written iteratively for speed, rather than recursively.
//
// TBD:  Wed like to enhance this function to make successive searches faster.
// This would require saving some previous state, including the previous Index
// returned, and in which leaf it was found.  If the next call is for the same
// Index and the array has not been modified, start at the same leaf.  This
// should be much easier to implement since this is iterative rather than
// recursive code.

#ifdef JUDY1
#ifdef JUDYPREV
FUNCTION int Judy1PrevEmpty
#else
FUNCTION int Judy1NextEmpty
#endif
#else
#ifdef JUDYPREV
FUNCTION int JudyLPrevEmpty
#else
FUNCTION int JudyLNextEmpty
#endif
#endif
        (
	Pcvoid_t  PArray,	// Judy array to search.
	Word_t *  PIndex,	// starting point and result.
	PJError_t PJError	// optional, for returning error info.
        )
{
	Word_t	  Index;	// fast copy, in a register.
	Pjp_t	  Pjp;		// current JP.
	Pjbl_t	  Pjbl;		// Pjp->jp_Addr masked and cast to types:
	Pjbb_t	  Pjbb;
	Pjbu_t	  Pjbu;
	Pjlb_t	  Pjlb;
	PWord_t	  Pword;	// alternate name for use by GET* macros.

	Word_t	  digit;	// next digit to decode from Index.
	Word_t	  digits;	// current state in SM = digits left to decode.
	Word_t	  pop0;		// in a leaf.
	Word_t	  pop0mask;	// precalculated to avoid variable shifts.
	long	  offset;	// within a branch or leaf (can be large).
	int	  subexp;	// subexpanse in a bitmap branch.
	BITMAPB_t bitposmaskB;	// bit in bitmap for bitmap branch.
	BITMAPL_t bitposmaskL;	// bit in bitmap for bitmap leaf.
	Word_t	  possfullJP1;	// JP types for possibly full subexpanses:
	Word_t	  possfullJP2;
	Word_t	  possfullJP3;


// ----------------------------------------------------------------------------
// M A C R O S
//
// These are intended to make the code a bit more readable and less redundant.


// CHECK FOR NULL JP:
//
// TBD:  In principle this can be reduced (here and in other *.c files) to just
// the latter clause since no Type should ever be below cJU_JPNULL1, but in
// fact some root pointer types can be lower, so for safety do both checks.

#define	JPNULL(Type)  (((Type) >= cJU_JPNULL1) && ((Type) <= cJU_JPNULLMAX))


// CHECK FOR A FULL JP:
//
// Given a JP, indicate if it is fully populated.  Use digits, pop0mask, and
// possfullJP1..3 in the context.
//
// This is a difficult problem because it requires checking the Pop0 bits for
// all-ones, but the number of bytes depends on the JP type, which is not
// directly related to the parent branchs type or level -- the JPs child
// could be under a narrow pointer (hence not full).  The simple answer
// requires switching on or otherwise calculating the JP type, which could be
// slow.  Instead, in SMPREPB* precalculate pop0mask and also record in
// possfullJP1..3 the child JP (branch) types that could possibly be full (one
// level down), and use them here.  For level-2 branches (with digits == 2),
// the test for a full child depends on Judy1/JudyL.
//
// Note:  This cannot be applied to the JP in a JPM because it doesnt have
// enough pop0 digits.
//
// TBD:  JPFULL_BRANCH diligently checks for BranchL or BranchB, where neither
// of those can ever be full as it turns out.  Could just check for a BranchU
// at the right level.  Also, pop0mask might be overkill, its not used much,
// so perhaps just call cJU_POP0MASK(digits - 1) here?
//
// First, JPFULL_BRANCH checks for a full expanse for a JP whose child can be a
// branch, that is, a JP in a branch at level 3 or higher:

#define	JPFULL_BRANCH(Pjp)						\
	  ((((JU_JPDCDPOP0(Pjp) ^ cJU_ALLONES) & pop0mask) == 0)	\
	&& ((JU_JPTYPE(Pjp) == possfullJP1)				\
	 || (JU_JPTYPE(Pjp) == possfullJP2)				\
	 || (JU_JPTYPE(Pjp) == possfullJP3)))

#ifdef JUDY1
#define	JPFULL(Pjp)							\
	((digits == 2) ?						\
	 (JU_JPTYPE(Pjp) == cJ1_JPFULLPOPU1) : JPFULL_BRANCH(Pjp))
#else
#define	JPFULL(Pjp)							\
	((digits == 2) ?						\
	   (JU_JPTYPE(Pjp) == cJU_JPLEAF_B1)				\
	 && (((JU_JPDCDPOP0(Pjp) & cJU_POP0MASK(1)) == cJU_POP0MASK(1))) : \
	 JPFULL_BRANCH(Pjp))
#endif


// RETURN SUCCESS:
//
// This hides the need to set *PIndex back to the local value of Index -- use a
// local value for faster operation.  Note that the callers *PIndex is ALWAYS
// modified upon success, at least decremented/incremented.

#define	RET_SUCCESS { *PIndex = Index; return(1); }


// RETURN A CORRUPTION:

#define	RET_CORRUPT { JU_SET_ERRNO(PJError, JU_ERRNO_CORRUPT); return(JERRI); }


// SEARCH A BITMAP BRANCH:
//
// This is a weak analog of j__udySearchLeaf*() for bitmap branches.  Return
// the actual or next-left position, base 0, of Digit in a BITMAPB_t bitmap
// (subexpanse of a full bitmap), also given a Bitposmask for Digit.  The
// position is the offset within the set bits.
//
// Unlike j__udySearchLeaf*(), the offset is not returned bit-complemented if
// Digits bit is unset, because the caller can check the bitmap themselves to
// determine that.  Also, if Digits bit is unset, the returned offset is to
// the next-left JP or index (including -1), not to the "ideal" position for
// the index = next-right JP or index.
//
// Shortcut and skip calling j__udyCountBitsB() if the bitmap is full, in which
// case (Digit % cJU_BITSPERSUBEXPB) itself is the base-0 offset.

#define	SEARCHBITMAPB(Bitmap,Digit,Bitposmask)				\
	(((Bitmap) == cJU_FULLBITMAPB) ? (Digit % cJU_BITSPERSUBEXPB) :	\
	 j__udyCountBitsB((Bitmap) & JU_MASKLOWERINC(Bitposmask)) - 1)

#ifdef JUDYPREV
// Equivalent to search for the highest offset in Bitmap, that is, one less
// than the number of bits set:

#define	SEARCHBITMAPMAXB(Bitmap)					\
	(((Bitmap) == cJU_FULLBITMAPB) ? cJU_BITSPERSUBEXPB - 1 :	\
	 j__udyCountBitsB(Bitmap) - 1)
#endif


// CHECK DECODE BYTES:
//
// Check Decode bytes in a JP against the equivalent portion of Index.  If they
// dont match, Index is outside the subexpanse of a narrow pointer, hence is
// empty.

#define	CHECKDCD(cDigits) \
	if (JU_DCDNOTMATCHINDEX(Index, Pjp, cDigits)) RET_SUCCESS


// REVISE REMAINDER OF INDEX:
//
// Put one digit in place in Index and clear/set the lower digits, if any, so
// the resulting Index is at the start/end of an expanse, or just clear/set the
// least digits.
//
// Actually, to make simple use of JU_LEASTBYTESMASK, first clear/set all least
// digits of Index including the digit to be overridden, then set the value of
// that one digit.  If Digits == 1 the first operation is redundant, but either
// very fast or even removed by the optimizer.

#define	CLEARLEASTDIGITS(Digits) Index &= ~JU_LEASTBYTESMASK(Digits)
#define	SETLEASTDIGITS(  Digits) Index |=  JU_LEASTBYTESMASK(Digits)

#define	CLEARLEASTDIGITS_D(Digit,Digits)	\
	{					\
	    CLEARLEASTDIGITS(Digits);		\
	    JU_SETDIGIT(Index, Digit, Digits);	\
	}

#define	SETLEASTDIGITS_D(Digit,Digits)		\
	{					\
	    SETLEASTDIGITS(Digits);		\
	    JU_SETDIGIT(Index, Digit, Digits);	\
	}


// SET REMAINDER OF INDEX AND THEN RETURN OR CONTINUE:

#define	SET_AND_RETURN(OpLeastDigits,Digit,Digits)	\
	{						\
	    OpLeastDigits(Digit, Digits);		\
	    RET_SUCCESS;				\
	}

#define	SET_AND_CONTINUE(OpLeastDigits,Digit,Digits)	\
	{						\
	    OpLeastDigits(Digit, Digits);		\
	    goto SMGetContinue;				\
	}


// PREPARE TO HANDLE A LEAFW OR JP BRANCH IN THE STATE MACHINE:
//
// Extract a state-dependent digit from Index in a "constant" way, then jump to
// common code for multiple cases.
//
// TBD:  Should this macro do more, such as preparing variable-shift masks for
// use in CLEARLEASTDIGITS and SETLEASTDIGITS?

#define	SMPREPB(cDigits,Next,PossFullJP1,PossFullJP2,PossFullJP3)	\
	digits	 = (cDigits);						\
	digit	 = JU_DIGITATSTATE(Index, cDigits);			\
	pop0mask = cJU_POP0MASK((cDigits) - 1);	 /* for branchs JPs */	\
	possfullJP1 = (PossFullJP1);					\
	possfullJP2 = (PossFullJP2);					\
	possfullJP3 = (PossFullJP3);					\
	goto Next

// Variations for specific-level branches and for shorthands:
//
// Note:  SMPREPB2 need not initialize possfullJP* because JPFULL does not use
// them for digits == 2, but gcc -Wall isnt quite smart enough to see this, so
// waste a bit of time and space to get rid of the warning:

#define	SMPREPB2(Next)				\
	digits	 = 2;				\
	digit	 = JU_DIGITATSTATE(Index, 2);	\
	pop0mask = cJU_POP0MASK(1);  /* for branchs JPs */ \
	possfullJP1 = possfullJP2 = possfullJP3 = 0;	    \
	goto Next

#define	SMPREPB3(Next) SMPREPB(3,	      Next, cJU_JPBRANCH_L2, \
						    cJU_JPBRANCH_B2, \
						    cJU_JPBRANCH_U2)
#ifndef JU_64BIT
#define	SMPREPBL(Next) SMPREPB(cJU_ROOTSTATE, Next, cJU_JPBRANCH_L3, \
						    cJU_JPBRANCH_B3, \
						    cJU_JPBRANCH_U3)
#else
#define	SMPREPB4(Next) SMPREPB(4,	      Next, cJU_JPBRANCH_L3, \
						    cJU_JPBRANCH_B3, \
						    cJU_JPBRANCH_U3)
#define	SMPREPB5(Next) SMPREPB(5,	      Next, cJU_JPBRANCH_L4, \
						    cJU_JPBRANCH_B4, \
						    cJU_JPBRANCH_U4)
#define	SMPREPB6(Next) SMPREPB(6,	      Next, cJU_JPBRANCH_L5, \
						    cJU_JPBRANCH_B5, \
						    cJU_JPBRANCH_U5)
#define	SMPREPB7(Next) SMPREPB(7,	      Next, cJU_JPBRANCH_L6, \
						    cJU_JPBRANCH_B6, \
						    cJU_JPBRANCH_U6)
#define	SMPREPBL(Next) SMPREPB(cJU_ROOTSTATE, Next, cJU_JPBRANCH_L7, \
						    cJU_JPBRANCH_B7, \
						    cJU_JPBRANCH_U7)
#endif


// RESTART AFTER SECONDARY DEAD END:
//
// Set Index to the first/last index in the branch or leaf subexpanse and start
// over at the top of the tree.

#ifdef JUDYPREV
#define	SMRESTART(Digits) { CLEARLEASTDIGITS(Digits); goto SMGetRestart; }
#else
#define	SMRESTART(Digits) { SETLEASTDIGITS(  Digits); goto SMGetRestart; }
#endif


// CHECK EDGE OF LEAFS EXPANSE:
//
// Given the LSBs of the lowest/highest valid index in a leaf (or equivalently
// in an immediate JP), the level (index size) of the leaf, and the full index
// to return (as Index in the context) already set to the full index matching
// the lowest/highest one, determine if there is an empty index in the leafs
// expanse below/above the lowest/highest index, which is true if the
// lowest/highest index is not at the "edge" of the leafs expanse based on its
// LSBs.  If so, return Index decremented/incremented; otherwise restart at the
// top of the tree.
//
// Note:  In many cases Index is already at the right spot and calling
// SMRESTART instead of just going directly to SMGetRestart is a bit of
// overkill.
//
// Note:  Variable shift occurs if Digits is not a constant.

#ifdef JUDYPREV
#define	LEAF_EDGE(MinIndex,Digits)			\
	{						\
	    if (MinIndex) { --Index; RET_SUCCESS; }	\
	    SMRESTART(Digits);				\
	}
#else
#define	LEAF_EDGE(MaxIndex,Digits)			\
	{						\
	    if ((MaxIndex) != JU_LEASTBYTES(cJU_ALLONES, Digits)) \
	    { ++Index; RET_SUCCESS; }			\
	    SMRESTART(Digits);				\
	}
#endif

// Same as above except Index is not already set to match the lowest/highest
// index, so do that before decrementing/incrementing it:

#ifdef JUDYPREV
#define	LEAF_EDGE_SET(MinIndex,Digits)	\
	{				\
	    if (MinIndex)		\
	    { JU_SETDIGITS(Index, MinIndex, Digits); --Index; RET_SUCCESS; } \
	    SMRESTART(Digits);		\
	}
#else
#define	LEAF_EDGE_SET(MaxIndex,Digits)	\
	{				\
	    if ((MaxIndex) != JU_LEASTBYTES(cJU_ALLONES, Digits))	    \
	    { JU_SETDIGITS(Index, MaxIndex, Digits); ++Index; RET_SUCCESS; } \
	    SMRESTART(Digits);		\
	}
#endif


// FIND A HOLE (EMPTY INDEX) IN AN IMMEDIATE OR LEAF:
//
// Given an index location in a leaf (or equivalently an immediate JP) known to
// contain a usable hole (an empty index less/greater than Index), and the LSBs
// of a minimum/maximum index to locate, find the previous/next empty index and
// return it.
//
// Note:  "Even" index sizes (1,2,4[,8] bytes) have corresponding native C
// types; "odd" index sizes dont, but they are not represented here because
// they are handled completely differently; see elsewhere.

#ifdef JUDYPREV

#define	LEAF_HOLE_EVEN(cDigits,Pjll,IndexLSB)				\
	{								\
	    while (*(Pjll) > (IndexLSB)) --(Pjll); /* too high */	\
	    if (*(Pjll) < (IndexLSB)) RET_SUCCESS  /* Index is empty */	\
	    while (*(--(Pjll)) == --(IndexLSB)) /* null, find a hole */;\
	    JU_SETDIGITS(Index, IndexLSB, cDigits);			\
	    RET_SUCCESS;						\
	}
#else
#define	LEAF_HOLE_EVEN(cDigits,Pjll,IndexLSB)				\
	{								\
	    while (*(Pjll) < (IndexLSB)) ++(Pjll); /* too low */	\
	    if (*(Pjll) > (IndexLSB)) RET_SUCCESS  /* Index is empty */	\
	    while (*(++(Pjll)) == ++(IndexLSB)) /* null, find a hole */;\
	    JU_SETDIGITS(Index, IndexLSB, cDigits);			\
	    RET_SUCCESS;						\
	}
#endif


// SEARCH FOR AN EMPTY INDEX IN AN IMMEDIATE OR LEAF:
//
// Given a pointer to the first index in a leaf (or equivalently an immediate
// JP), the population of the leaf, and a first empty Index to find (inclusive,
// as Index in the context), where Index is known to fall within the expanse of
// the leaf to search, efficiently find the previous/next empty index in the
// leaf, if any.  For simplicity the following overview is stated in terms of
// Judy*NextEmpty() only, but the same concepts apply symmetrically for
// Judy*PrevEmpty().  Also, in each case the comparisons are for the LSBs of
// Index and leaf indexes, according to the leafs level.
//
// 1.  If Index is GREATER than the last (highest) index in the leaf
//     (maxindex), return success, Index is empty.  (Remember, Index is known
//     to be in the leafs expanse.)
//
// 2.  If Index is EQUAL to maxindex:  If maxindex is not at the edge of the
//     leafs expanse, increment Index and return success, there is an empty
//     Index one higher than any in the leaf; otherwise restart with Index
//     reset to the upper edge of the leafs expanse.  Note:  This might cause
//     an extra cache line fill, but this is OK for repeatedly-called search
//     code, and it saves CPU time.
//
// 3.  If Index is LESS than maxindex, check for "dense to end of leaf":
//     Subtract Index from maxindex, and back up that many slots in the leaf.
//     If the resulting offset is not before the start of the leaf then compare
//     the index at this offset (baseindex) with Index:
//
// 3a.  If GREATER, the leaf must be corrupt, since indexes are sorted and
//      there are no duplicates.
//
// 3b.  If EQUAL, the leaf is "dense" from Index to maxindex, meaning there is
//      no reason to search it.  "Slide right" to the high end of the leaf
//      (modify Index to maxindex) and continue with step 2 above.
//
// 3c.  If LESS, continue with step 4.
//
// 4.  If the offset based on maxindex minus Index falls BEFORE the start of
//     the leaf, or if, per 3c above, baseindex is LESS than Index, the leaf is
//     guaranteed "not dense to the end" and a usable empty Index must exist.
//     This supports a more efficient search loop.  Start at the FIRST index in
//     the leaf, or one BEYOND baseindex, respectively, and search the leaf as
//     follows, comparing each current index (currindex) with Index:
//
// 4a.  If LESS, keep going to next index.  Note:  This is certain to terminate
//      because maxindex is known to be greater than Index, hence the loop can
//      be small and fast.
//
// 4b.  If EQUAL, loop and increment Index until finding currindex greater than
//      Index, and return success with the modified Index.
//
// 4c.  If GREATER, return success, Index (unmodified) is empty.
//
// Note:  These are macros rather than functions for speed.

#ifdef JUDYPREV

#define	JSLE_EVEN(Addr,Pop0,cDigits,LeafType)				\
	{								\
	    LeafType * PjllLSB  = (LeafType *) (Addr);			\
	    LeafType   IndexLSB = Index;	/* auto-masking */	\
									\
	/* Index before or at start of leaf: */				\
									\
	    if (*PjllLSB >= IndexLSB)		/* no need to search */	\
	    {								\
		if (*PjllLSB > IndexLSB) RET_SUCCESS; /* Index empty */	\
		LEAF_EDGE(*PjllLSB, cDigits);				\
	    }								\
									\
	/* Index in or after leaf: */					\
									\
	    offset = IndexLSB - *PjllLSB;	/* tentative offset  */	\
	    if (offset <= (Pop0))		/* can check density */	\
	    {								\
		PjllLSB += offset;		/* move to slot */	\
									\
		if (*PjllLSB <= IndexLSB)	/* dense or corrupt */	\
		{							\
		    if (*PjllLSB == IndexLSB)	/* dense, check edge */	\
			LEAF_EDGE_SET(PjllLSB[-offset], cDigits);	\
		    RET_CORRUPT;					\
		}							\
		--PjllLSB;	/* not dense, start at previous */	\
	    }								\
	    else PjllLSB = ((LeafType *) (Addr)) + (Pop0); /* start at max */ \
									\
	    LEAF_HOLE_EVEN(cDigits, PjllLSB, IndexLSB);			\
	}

// JSLE_ODD is completely different from JSLE_EVEN because its important to
// minimize copying odd indexes to compare them (see 4.14).  Furthermore, a
// very complex version (4.17, but abandoned before fully debugged) that
// avoided calling j__udySearchLeaf*() ran twice as fast as 4.14, but still
// half as fast as SearchValid.  Doug suggested that to minimize complexity and
// share common code we should use j__udySearchLeaf*() for the initial search
// to establish if Index is empty, which should be common.  If Index is valid
// in a leaf or immediate indexes, odds are good that an empty Index is nearby,
// so for simplicity just use a *COPY* function to linearly search the
// remainder.
//
// TBD:  Pathological case?  Average performance should be good, but worst-case
// might suffer.  When Search says the initial Index is valid, so a linear
// copy-and-compare is begun, if the caller builds fairly large leaves with
// dense clusters AND frequently does a SearchEmpty at one end of such a
// cluster, performance wont be very good.  Might a dense-check help?  This
// means checking offset against the index at offset, and then against the
// first/last index in the leaf.  We doubt the pathological case will appear
// much in real applications because they will probably alternate SearchValid
// and SearchEmpty calls.

#define	JSLE_ODD(cDigits,Pjll,Pop0,Search,Copy)				\
	{								\
	    Word_t IndexLSB;		/* least bytes only */		\
	    Word_t IndexFound;		/* in leaf	    */		\
									\
	    if ((offset = Search(Pjll, (Pop0) + 1, Index)) < 0)		\
		RET_SUCCESS;		/* Index is empty */		\
									\
	    IndexLSB = JU_LEASTBYTES(Index, cDigits);			\
	    offset  *= (cDigits);					\
									\
	    while ((offset -= (cDigits)) >= 0)				\
	    {				/* skip until empty or start */	\
		Copy(IndexFound, ((uint8_t *) (Pjll)) + offset);	\
		if (IndexFound != (--IndexLSB))	/* found an empty */	\
		{ JU_SETDIGITS(Index, IndexLSB, cDigits); RET_SUCCESS; }\
	    }								\
	    LEAF_EDGE_SET(IndexLSB, cDigits);				\
	}

#else // JUDYNEXT

#define	JSLE_EVEN(Addr,Pop0,cDigits,LeafType)				\
	{								\
	    LeafType * PjllLSB   = ((LeafType *) (Addr)) + (Pop0);	\
	    LeafType   IndexLSB = Index;	/* auto-masking */	\
									\
	/* Index at or after end of leaf: */				\
									\
	    if (*PjllLSB <= IndexLSB)		/* no need to search */	\
	    {								\
		if (*PjllLSB < IndexLSB) RET_SUCCESS;  /* Index empty */\
		LEAF_EDGE(*PjllLSB, cDigits);				\
	    }								\
									\
	/* Index before or in leaf: */					\
									\
	    offset = *PjllLSB - IndexLSB;	/* tentative offset  */	\
	    if (offset <= (Pop0))		/* can check density */	\
	    {								\
		PjllLSB -= offset;		/* move to slot */	\
									\
		if (*PjllLSB >= IndexLSB)	/* dense or corrupt */	\
		{							\
		    if (*PjllLSB == IndexLSB)	/* dense, check edge */	\
			LEAF_EDGE_SET(PjllLSB[offset], cDigits);	\
		    RET_CORRUPT;					\
		}							\
		++PjllLSB;		/* not dense, start at next */	\
	    }								\
	    else PjllLSB = (LeafType *) (Addr);	/* start at minimum */	\
									\
	    LEAF_HOLE_EVEN(cDigits, PjllLSB, IndexLSB);			\
	}

#define	JSLE_ODD(cDigits,Pjll,Pop0,Search,Copy)				\
	{								\
	    Word_t IndexLSB;		/* least bytes only */		\
	    Word_t IndexFound;		/* in leaf	    */		\
	    int	   offsetmax;		/* in bytes	    */		\
									\
	    if ((offset = Search(Pjll, (Pop0) + 1, Index)) < 0)		\
		RET_SUCCESS;			/* Index is empty */	\
									\
	    IndexLSB  = JU_LEASTBYTES(Index, cDigits);			\
	    offset   *= (cDigits);					\
	    offsetmax = (Pop0) * (cDigits);	/* single multiply */	\
									\
	    while ((offset += (cDigits)) <= offsetmax)			\
	    {				/* skip until empty or end */	\
		Copy(IndexFound, ((uint8_t *) (Pjll)) + offset);	\
		if (IndexFound != (++IndexLSB))	/* found an empty */	\
		{ JU_SETDIGITS(Index, IndexLSB, cDigits); RET_SUCCESS; } \
	    }								\
	    LEAF_EDGE_SET(IndexLSB, cDigits);				\
	}

#endif // JUDYNEXT

// Note:  Immediate indexes never fill a single index group, so for odd index
// sizes, save time by calling JSLE_ODD_IMM instead of JSLE_ODD.

#define	j__udySearchLeafEmpty1(Addr,Pop0) \
	JSLE_EVEN(Addr, Pop0, 1, uint8_t)

#define	j__udySearchLeafEmpty2(Addr,Pop0) \
	JSLE_EVEN(Addr, Pop0, 2, uint16_t)

#define	j__udySearchLeafEmpty3(Addr,Pop0) \
	JSLE_ODD(3, Addr, Pop0, j__udySearchLeaf3, JU_COPY3_PINDEX_TO_LONG)

#ifndef JU_64BIT

#define	j__udySearchLeafEmptyL(Addr,Pop0) \
	JSLE_EVEN(Addr, Pop0, 4, Word_t)

#else

#define	j__udySearchLeafEmpty4(Addr,Pop0) \
	JSLE_EVEN(Addr, Pop0, 4, uint32_t)

#define	j__udySearchLeafEmpty5(Addr,Pop0) \
	JSLE_ODD(5, Addr, Pop0, j__udySearchLeaf5, JU_COPY5_PINDEX_TO_LONG)

#define	j__udySearchLeafEmpty6(Addr,Pop0) \
	JSLE_ODD(6, Addr, Pop0, j__udySearchLeaf6, JU_COPY6_PINDEX_TO_LONG)

#define	j__udySearchLeafEmpty7(Addr,Pop0) \
	JSLE_ODD(7, Addr, Pop0, j__udySearchLeaf7, JU_COPY7_PINDEX_TO_LONG)

#define	j__udySearchLeafEmptyL(Addr,Pop0) \
	JSLE_EVEN(Addr, Pop0, 8, Word_t)

#endif // JU_64BIT


// ----------------------------------------------------------------------------
// START OF CODE:
//
// CHECK FOR SHORTCUTS:
//
// Error out if PIndex is null.

	if (PIndex == (PWord_t) NULL)
	{
	    JU_SET_ERRNO(PJError, JU_ERRNO_NULLPINDEX);
	    return(JERRI);
	}

	Index = *PIndex;			// fast local copy.

// Set and pre-decrement/increment Index, watching for underflow/overflow:
//
// An out-of-bounds Index means failure:  No previous/next empty index.

SMGetRestart:		// return here with revised Index.

#ifdef JUDYPREV
	if (Index-- == 0) return(0);
#else
	if (++Index == 0) return(0);
#endif

// An empty array with an in-bounds (not underflowed/overflowed) Index means
// success:
//
// Note:  This check is redundant after restarting at SMGetRestart, but should
// take insignificant time.

	if (PArray == (Pvoid_t) NULL) RET_SUCCESS;

// ----------------------------------------------------------------------------
// ROOT-LEVEL LEAF that starts with a Pop0 word; just look within the leaf:
//
// If Index is not in the leaf, return success; otherwise return the first
// empty Index, if any, below/above where it would belong.

	if (JU_LEAFW_POP0(PArray) < cJU_LEAFW_MAXPOP1) // must be a LEAFW
	{
	    Pjlw_t Pjlw = P_JLW(PArray);	// first word of leaf.
	    pop0 = Pjlw[0];

#ifdef	JUDY1
	    if (pop0 == 0)			// special case.
	    {
#ifdef JUDYPREV
		if ((Index != Pjlw[1]) || (Index-- != 0)) RET_SUCCESS;
#else
		if ((Index != Pjlw[1]) || (++Index != 0)) RET_SUCCESS;
#endif
		return(0);		// no previous/next empty index.
	    }
#endif // JUDY1

	    j__udySearchLeafEmptyL(Pjlw + 1, pop0);

//  No return -- thanks ALAN

	}
	else

// ----------------------------------------------------------------------------
// HANDLE JRP Branch:
//
// For JRP branches, traverse the JPM; handle LEAFW
// directly; but look for the most common cases first.

	{
	    Pjpm_t Pjpm = P_JPM(PArray);
	    Pjp = &(Pjpm->jpm_JP);

//	    goto SMGetContinue;
	}


// ============================================================================
// STATE MACHINE -- GET INDEX:
//
// Search for Index (already decremented/incremented so as to be an inclusive
// search).  If not found (empty index), return success.  Otherwise do a
// previous/next search, and if successful modify Index to the empty index
// found.  See function header comments.
//
// ENTRY:  Pjp points to next JP to interpret, whose Decode bytes have not yet
// been checked.
//
// Note:  Check Decode bytes at the start of each loop, not after looking up a
// new JP, so its easy to do constant shifts/masks.
//
// EXIT:  Return, or branch to SMGetRestart with modified Index, or branch to
// SMGetContinue with a modified Pjp, as described elsewhere.
//
// WARNING:  For run-time efficiency the following cases replicate code with
// varying constants, rather than using common code with variable values!

SMGetContinue:			// return here for next branch/leaf.

#ifdef TRACEJPSE
	JudyPrintJP(Pjp, "sf", __LINE__);
#endif

	switch (JU_JPTYPE(Pjp))
	{


// ----------------------------------------------------------------------------
// LINEAR BRANCH:
//
// Check Decode bytes, if any, in the current JP, then search for a JP for the
// next digit in Index.

	case cJU_JPBRANCH_L2: CHECKDCD(2); SMPREPB2(SMBranchL);
	case cJU_JPBRANCH_L3: CHECKDCD(3); SMPREPB3(SMBranchL);
#ifdef JU_64BIT
	case cJU_JPBRANCH_L4: CHECKDCD(4); SMPREPB4(SMBranchL);
	case cJU_JPBRANCH_L5: CHECKDCD(5); SMPREPB5(SMBranchL);
	case cJU_JPBRANCH_L6: CHECKDCD(6); SMPREPB6(SMBranchL);
	case cJU_JPBRANCH_L7: CHECKDCD(7); SMPREPB7(SMBranchL);
#endif
	case cJU_JPBRANCH_L:		   SMPREPBL(SMBranchL);

// Common code (state-independent) for all cases of linear branches:

SMBranchL:
	    Pjbl = P_JBL(Pjp->jp_Addr);

// First, check if Indexs expanse (digit) is below/above the first/last
// populated expanse in the BranchL, in which case Index is empty; otherwise
// find the offset of the lowest/highest populated expanse at or above/below
// digit, if any:
//
// Note:  The for-loop is guaranteed to exit eventually because the first/last
// expanse is known to be a terminator.
//
// Note:  Cannot use j__udySearchLeaf*Empty1() here because it only applies to
// leaves and does not know about partial versus full JPs, unlike the use of
// j__udySearchLeaf1() for BranchLs in SearchValid code.  Also, since linear
// leaf expanse lists are small, dont waste time calling j__udySearchLeaf1(),
// just scan the expanse list.

#ifdef JUDYPREV
	    if ((Pjbl->jbl_Expanse[0]) > digit) RET_SUCCESS;

	    for (offset = (Pjbl->jbl_NumJPs) - 1; /* null */; --offset)
#else
	    if ((Pjbl->jbl_Expanse[(Pjbl->jbl_NumJPs) - 1]) < digit)
		RET_SUCCESS;

	    for (offset = 0; /* null */; ++offset)
#endif
	    {

// Too low/high, keep going; or too high/low, meaning the loop passed a hole
// and the initial Index is empty:

#ifdef JUDYPREV
		if ((Pjbl->jbl_Expanse[offset]) > digit) continue;
		if ((Pjbl->jbl_Expanse[offset]) < digit) RET_SUCCESS;
#else
		if ((Pjbl->jbl_Expanse[offset]) < digit) continue;
		if ((Pjbl->jbl_Expanse[offset]) > digit) RET_SUCCESS;
#endif

// Found expanse matching digit; if its not full, traverse through it:

		if (! JPFULL((Pjbl->jbl_jp) + offset))
		{
		    Pjp = (Pjbl->jbl_jp) + offset;
		    goto SMGetContinue;
		}

// Common code:  While searching for a lower/higher hole or a non-full JP, upon
// finding a lower/higher hole, adjust Index using the revised digit and
// return; or upon finding a consecutive lower/higher expanse, if the expanses
// JP is non-full, modify Index and traverse through the JP:

#define	BRANCHL_CHECK(OpIncDec,OpLeastDigits,Digit,Digits)	\
	{							\
	    if ((Pjbl->jbl_Expanse[offset]) != OpIncDec digit)	\
		SET_AND_RETURN(OpLeastDigits, Digit, Digits);	\
								\
	    if (! JPFULL((Pjbl->jbl_jp) + offset))		\
	    {							\
		Pjp = (Pjbl->jbl_jp) + offset;			\
		SET_AND_CONTINUE(OpLeastDigits, Digit, Digits);	\
	    }							\
	}

// BranchL primary dead end:  Expanse matching Index/digit is full (rare except
// for dense/sequential indexes):
//
// Search for a lower/higher hole, a non-full JP, or the end of the expanse
// list, while decrementing/incrementing digit.

#ifdef JUDYPREV
		while (--offset >= 0)
		    BRANCHL_CHECK(--, SETLEASTDIGITS_D, digit, digits)
#else
		while (++offset < Pjbl->jbl_NumJPs)
		    BRANCHL_CHECK(++, CLEARLEASTDIGITS_D, digit, digits)
#endif

// Passed end of BranchL expanse list after finding a matching but full
// expanse:
//
// Digit now matches the lowest/highest expanse, which is a full expanse; if
// digit is at the end of BranchLs expanse (no hole before/after), break out
// of the loop; otherwise modify Index to the next lower/higher digit and
// return success:

#ifdef JUDYPREV
		if (digit == 0) break;
		--digit; SET_AND_RETURN(SETLEASTDIGITS_D, digit, digits);
#else
		if (digit == JU_LEASTBYTES(cJU_ALLONES, 1)) break;
		++digit; SET_AND_RETURN(CLEARLEASTDIGITS_D, digit, digits);
#endif
	    } // for-loop

// BranchL secondary dead end, no non-full previous/next JP:

	    SMRESTART(digits);


// ----------------------------------------------------------------------------
// BITMAP BRANCH:
//
// Check Decode bytes, if any, in the current JP, then search for a JP for the
// next digit in Index.

	case cJU_JPBRANCH_B2: CHECKDCD(2); SMPREPB2(SMBranchB);
	case cJU_JPBRANCH_B3: CHECKDCD(3); SMPREPB3(SMBranchB);
#ifdef JU_64BIT
	case cJU_JPBRANCH_B4: CHECKDCD(4); SMPREPB4(SMBranchB);
	case cJU_JPBRANCH_B5: CHECKDCD(5); SMPREPB5(SMBranchB);
	case cJU_JPBRANCH_B6: CHECKDCD(6); SMPREPB6(SMBranchB);
	case cJU_JPBRANCH_B7: CHECKDCD(7); SMPREPB7(SMBranchB);
#endif
	case cJU_JPBRANCH_B:		   SMPREPBL(SMBranchB);

// Common code (state-independent) for all cases of bitmap branches:

SMBranchB:
	    Pjbb = P_JBB(Pjp->jp_Addr);

// Locate the digits JP in the subexpanse list, if present:

	    subexp     = digit / cJU_BITSPERSUBEXPB;
	    assert(subexp < cJU_NUMSUBEXPB);	// falls in expected range.
	    bitposmaskB = JU_BITPOSMASKB(digit);

// Absent JP = no JP matches current digit in Index:

//	    if (! JU_BITMAPTESTB(Pjbb, digit))			// slower.
	    if (! (JU_JBB_BITMAP(Pjbb, subexp) & bitposmaskB))	// faster.
		RET_SUCCESS;

// Non-full JP matches current digit in Index:
//
// Iterate to the subsidiary non-full JP.

	    offset = SEARCHBITMAPB(JU_JBB_BITMAP(Pjbb, subexp), digit,
				   bitposmaskB);
	    // not negative since at least one bit is set:
	    assert(offset >= 0);
	    assert(offset < (int) cJU_BITSPERSUBEXPB);

// Watch for null JP subarray pointer with non-null bitmap (a corruption):

	    if ((Pjp = P_JP(JU_JBB_PJP(Pjbb, subexp)))
	     == (Pjp_t) NULL) RET_CORRUPT;

	    Pjp += offset;
	    if (! JPFULL(Pjp)) goto SMGetContinue;

// BranchB primary dead end:
//
// Upon hitting a full JP in a BranchB for the next digit in Index, search
// sideways for a previous/next absent JP (unset bit) or non-full JP (set bit
// with non-full JP); first in the current bitmap subexpanse, then in
// lower/higher subexpanses.  Upon entry, Pjp points to a known-unusable JP,
// ready to decrement/increment.
//
// Note:  The preceding code is separate from this loop because Index does not
// need revising (see SET_AND_*()) if the initial index is an empty index.
//
// TBD:  For speed, shift bitposmaskB instead of using JU_BITMAPTESTB or
// JU_BITPOSMASKB, but this shift has knowledge of bit order that really should
// be encapsulated in a header file.

#define	BRANCHB_CHECKBIT(OpLeastDigits)					\
    if (! (JU_JBB_BITMAP(Pjbb, subexp) & bitposmaskB))  /* absent JP */	\
	SET_AND_RETURN(OpLeastDigits, digit, digits)

#define	BRANCHB_CHECKJPFULL(OpLeastDigits)				\
    if (! JPFULL(Pjp))							\
	SET_AND_CONTINUE(OpLeastDigits, digit, digits)

#define	BRANCHB_STARTSUBEXP(OpLeastDigits)				\
    if (! JU_JBB_BITMAP(Pjbb, subexp)) /* empty subexpanse, shortcut */ \
	SET_AND_RETURN(OpLeastDigits, digit, digits)			\
    if ((Pjp = P_JP(JU_JBB_PJP(Pjbb, subexp))) == (Pjp_t) NULL) RET_CORRUPT

#ifdef JUDYPREV

	    --digit;				// skip initial digit.
	    bitposmaskB >>= 1;			// see TBD above.

BranchBNextSubexp:	// return here to check next bitmap subexpanse.

	    while (bitposmaskB)			// more bits to check in subexp.
	    {
		BRANCHB_CHECKBIT(SETLEASTDIGITS_D);
		--Pjp;				// previous in subarray.
		BRANCHB_CHECKJPFULL(SETLEASTDIGITS_D);
		assert(digit >= 0);
		--digit;
		bitposmaskB >>= 1;
	    }

	    if (subexp-- > 0)			// more subexpanses.
	    {
		BRANCHB_STARTSUBEXP(SETLEASTDIGITS_D);
		Pjp += SEARCHBITMAPMAXB(JU_JBB_BITMAP(Pjbb, subexp)) + 1;
		bitposmaskB = (1U << (cJU_BITSPERSUBEXPB - 1));
		goto BranchBNextSubexp;
	    }

#else // JUDYNEXT

	    ++digit;				// skip initial digit.
	    bitposmaskB <<= 1;			// note:  BITMAPB_t.

BranchBNextSubexp:	// return here to check next bitmap subexpanse.

	    while (bitposmaskB)			// more bits to check in subexp.
	    {
		BRANCHB_CHECKBIT(CLEARLEASTDIGITS_D);
		++Pjp;				// previous in subarray.
		BRANCHB_CHECKJPFULL(CLEARLEASTDIGITS_D);
		assert(digit < cJU_SUBEXPPERSTATE);
		++digit;
		bitposmaskB <<= 1;		// note:  BITMAPB_t.
	    }

	    if (++subexp < cJU_NUMSUBEXPB)	// more subexpanses.
	    {
		BRANCHB_STARTSUBEXP(CLEARLEASTDIGITS_D);
		--Pjp;				// pre-decrement.
		bitposmaskB = 1;
		goto BranchBNextSubexp;
	    }

#endif // JUDYNEXT

// BranchB secondary dead end, no non-full previous/next JP:

	    SMRESTART(digits);


// ----------------------------------------------------------------------------
// UNCOMPRESSED BRANCH:
//
// Check Decode bytes, if any, in the current JP, then search for a JP for the
// next digit in Index.

	case cJU_JPBRANCH_U2: CHECKDCD(2); SMPREPB2(SMBranchU);
	case cJU_JPBRANCH_U3: CHECKDCD(3); SMPREPB3(SMBranchU);
#ifdef JU_64BIT
	case cJU_JPBRANCH_U4: CHECKDCD(4); SMPREPB4(SMBranchU);
	case cJU_JPBRANCH_U5: CHECKDCD(5); SMPREPB5(SMBranchU);
	case cJU_JPBRANCH_U6: CHECKDCD(6); SMPREPB6(SMBranchU);
	case cJU_JPBRANCH_U7: CHECKDCD(7); SMPREPB7(SMBranchU);
#endif
	case cJU_JPBRANCH_U:		   SMPREPBL(SMBranchU);

// Common code (state-independent) for all cases of uncompressed branches:

SMBranchU:
	    Pjbu = P_JBU(Pjp->jp_Addr);
	    Pjp	 = (Pjbu->jbu_jp) + digit;

// Absent JP = null JP for current digit in Index:

	    if (JPNULL(JU_JPTYPE(Pjp))) RET_SUCCESS;

// Non-full JP matches current digit in Index:
//
// Iterate to the subsidiary JP.

	    if (! JPFULL(Pjp)) goto SMGetContinue;

// BranchU primary dead end:
//
// Upon hitting a full JP in a BranchU for the next digit in Index, search
// sideways for a previous/next null or non-full JP.  BRANCHU_CHECKJP() is
// shorthand for common code.
//
// Note:  The preceding code is separate from this loop because Index does not
// need revising (see SET_AND_*()) if the initial index is an empty index.

#define	BRANCHU_CHECKJP(OpIncDec,OpLeastDigits)			\
	{							\
	    OpIncDec Pjp;					\
								\
	    if (JPNULL(JU_JPTYPE(Pjp)))				\
		SET_AND_RETURN(OpLeastDigits, digit, digits)	\
								\
	    if (! JPFULL(Pjp))					\
		SET_AND_CONTINUE(OpLeastDigits, digit, digits)	\
	}

#ifdef JUDYPREV
	    while (digit-- > 0)
		BRANCHU_CHECKJP(--, SETLEASTDIGITS_D);
#else
	    while (++digit < cJU_BRANCHUNUMJPS)
		BRANCHU_CHECKJP(++, CLEARLEASTDIGITS_D);
#endif

// BranchU secondary dead end, no non-full previous/next JP:

	    SMRESTART(digits);


// ----------------------------------------------------------------------------
// LINEAR LEAF:
//
// Check Decode bytes, if any, in the current JP, then search the leaf for the
// previous/next empty index starting at Index.  Primary leaf dead end is
// hidden within j__udySearchLeaf*Empty*().  In case of secondary leaf dead
// end, restart at the top of the tree.
//
// Note:  Pword is the name known to GET*; think of it as Pjlw.

#define	SMLEAFL(cDigits,Func)                   \
	Pword = (PWord_t) P_JLW(Pjp->jp_Addr);  \
	pop0  = JU_JPLEAF_POP0(Pjp);            \
	Func(Pword, pop0)

#if (defined(JUDYL) || (! defined(JU_64BIT)))
	case cJU_JPLEAF1:  CHECKDCD(1); SMLEAFL(1, j__udySearchLeafEmpty1);
#endif
	case cJU_JPLEAF2:  CHECKDCD(2); SMLEAFL(2, j__udySearchLeafEmpty2);
	case cJU_JPLEAF3:  CHECKDCD(3); SMLEAFL(3, j__udySearchLeafEmpty3);

#ifdef JU_64BIT
	case cJU_JPLEAF4:  CHECKDCD(4); SMLEAFL(4, j__udySearchLeafEmpty4);
	case cJU_JPLEAF5:  CHECKDCD(5); SMLEAFL(5, j__udySearchLeafEmpty5);
	case cJU_JPLEAF6:  CHECKDCD(6); SMLEAFL(6, j__udySearchLeafEmpty6);
	case cJU_JPLEAF7:  CHECKDCD(7); SMLEAFL(7, j__udySearchLeafEmpty7);
#endif


// ----------------------------------------------------------------------------
// BITMAP LEAF:
//
// Check Decode bytes, if any, in the current JP, then search the leaf for the
// previous/next empty index starting at Index.

	case cJU_JPLEAF_B1:

	    CHECKDCD(1);

	    Pjlb	= P_JLB(Pjp->jp_Addr);
	    digit	= JU_DIGITATSTATE(Index, 1);
	    subexp	= digit / cJU_BITSPERSUBEXPL;
	    bitposmaskL	= JU_BITPOSMASKL(digit);
	    assert(subexp < cJU_NUMSUBEXPL);	// falls in expected range.

// Absent index = no index matches current digit in Index:

//	    if (! JU_BITMAPTESTL(Pjlb, digit))			// slower.
	    if (! (JU_JLB_BITMAP(Pjlb, subexp) & bitposmaskL))	// faster.
		RET_SUCCESS;

// LeafB1 primary dead end:
//
// Upon hitting a valid (non-empty) index in a LeafB1 for the last digit in
// Index, search sideways for a previous/next absent index, first in the
// current bitmap subexpanse, then in lower/higher subexpanses.
// LEAFB1_CHECKBIT() is shorthand for common code to handle one bit in one
// bitmap subexpanse.
//
// Note:  The preceding code is separate from this loop because Index does not
// need revising (see SET_AND_*()) if the initial index is an empty index.
//
// TBD:  For speed, shift bitposmaskL instead of using JU_BITMAPTESTL or
// JU_BITPOSMASKL, but this shift has knowledge of bit order that really should
// be encapsulated in a header file.

#define	LEAFB1_CHECKBIT(OpLeastDigits)				\
	if (! (JU_JLB_BITMAP(Pjlb, subexp) & bitposmaskL))	\
	    SET_AND_RETURN(OpLeastDigits, digit, 1)

#define	LEAFB1_STARTSUBEXP(OpLeastDigits)			\
	if (! JU_JLB_BITMAP(Pjlb, subexp)) /* empty subexp */	\
	    SET_AND_RETURN(OpLeastDigits, digit, 1)

#ifdef JUDYPREV

	    --digit;				// skip initial digit.
	    bitposmaskL >>= 1;			// see TBD above.

LeafB1NextSubexp:	// return here to check next bitmap subexpanse.

	    while (bitposmaskL)			// more bits to check in subexp.
	    {
		LEAFB1_CHECKBIT(SETLEASTDIGITS_D);
		assert(digit >= 0);
		--digit;
		bitposmaskL >>= 1;
	    }

	    if (subexp-- > 0)		// more subexpanses.
	    {
		LEAFB1_STARTSUBEXP(SETLEASTDIGITS_D);
		bitposmaskL = (1UL << (cJU_BITSPERSUBEXPL - 1));
		goto LeafB1NextSubexp;
	    }

#else // JUDYNEXT

	    ++digit;				// skip initial digit.
	    bitposmaskL <<= 1;			// note:  BITMAPL_t.

LeafB1NextSubexp:	// return here to check next bitmap subexpanse.

	    while (bitposmaskL)			// more bits to check in subexp.
	    {
		LEAFB1_CHECKBIT(CLEARLEASTDIGITS_D);
		assert(digit < cJU_SUBEXPPERSTATE);
		++digit;
		bitposmaskL <<= 1;		// note:  BITMAPL_t.
	    }

	    if (++subexp < cJU_NUMSUBEXPL)	// more subexpanses.
	    {
		LEAFB1_STARTSUBEXP(CLEARLEASTDIGITS_D);
		bitposmaskL = 1;
		goto LeafB1NextSubexp;
	    }

#endif // JUDYNEXT

// LeafB1 secondary dead end, no empty index:

	    SMRESTART(1);


#ifdef JUDY1
// ----------------------------------------------------------------------------
// FULL POPULATION:
//
// If the Decode bytes do not match, Index is empty (without modification);
// otherwise restart.

	case cJ1_JPFULLPOPU1:

	    CHECKDCD(1);
	    SMRESTART(1);
#endif


// ----------------------------------------------------------------------------
// IMMEDIATE:
//
// Pop1 = 1 Immediate JPs:
//
// If Index is not in the immediate JP, return success; otherwise check if
// there is an empty index below/above the immediate JPs index, and if so,
// return success with modified Index, else restart.
//
// Note:  Doug says its fast enough to calculate the index size (digits) in
// the following; no need to set it separately for each case.

	case cJU_JPIMMED_1_01:
	case cJU_JPIMMED_2_01:
	case cJU_JPIMMED_3_01:
#ifdef JU_64BIT
	case cJU_JPIMMED_4_01:
	case cJU_JPIMMED_5_01:
	case cJU_JPIMMED_6_01:
	case cJU_JPIMMED_7_01:
#endif
	    if (JU_JPDCDPOP0(Pjp) != JU_TRIMTODCDSIZE(Index)) RET_SUCCESS;
	    digits = JU_JPTYPE(Pjp) - cJU_JPIMMED_1_01 + 1;
	    LEAF_EDGE(JU_LEASTBYTES(JU_JPDCDPOP0(Pjp), digits), digits);

// Immediate JPs with Pop1 > 1:

#define	IMM_MULTI(Func,BaseJPType)			\
	JUDY1CODE(Pword = (PWord_t) (Pjp->jp_1Index);)	\
	JUDYLCODE(Pword = (PWord_t) (Pjp->jp_LIndex);)	\
	Func(Pword, JU_JPTYPE(Pjp) - (BaseJPType) + 1)

	case cJU_JPIMMED_1_02:
	case cJU_JPIMMED_1_03:
#if (defined(JUDY1) || defined(JU_64BIT))
	case cJU_JPIMMED_1_04:
	case cJU_JPIMMED_1_05:
	case cJU_JPIMMED_1_06:
	case cJU_JPIMMED_1_07:
#endif
#if (defined(JUDY1) && defined(JU_64BIT))
	case cJ1_JPIMMED_1_08:
	case cJ1_JPIMMED_1_09:
	case cJ1_JPIMMED_1_10:
	case cJ1_JPIMMED_1_11:
	case cJ1_JPIMMED_1_12:
	case cJ1_JPIMMED_1_13:
	case cJ1_JPIMMED_1_14:
	case cJ1_JPIMMED_1_15:
#endif
	    IMM_MULTI(j__udySearchLeafEmpty1, cJU_JPIMMED_1_02);

#if (defined(JUDY1) || defined(JU_64BIT))
	case cJU_JPIMMED_2_02:
	case cJU_JPIMMED_2_03:
#endif
#if (defined(JUDY1) && defined(JU_64BIT))
	case cJ1_JPIMMED_2_04:
	case cJ1_JPIMMED_2_05:
	case cJ1_JPIMMED_2_06:
	case cJ1_JPIMMED_2_07:
#endif
#if (defined(JUDY1) || defined(JU_64BIT))
	    IMM_MULTI(j__udySearchLeafEmpty2, cJU_JPIMMED_2_02);
#endif

#if (defined(JUDY1) || defined(JU_64BIT))
	case cJU_JPIMMED_3_02:
#endif
#if (defined(JUDY1) && defined(JU_64BIT))
	case cJ1_JPIMMED_3_03:
	case cJ1_JPIMMED_3_04:
	case cJ1_JPIMMED_3_05:
#endif
#if (defined(JUDY1) || defined(JU_64BIT))
	    IMM_MULTI(j__udySearchLeafEmpty3, cJU_JPIMMED_3_02);
#endif

#if (defined(JUDY1) && defined(JU_64BIT))
	case cJ1_JPIMMED_4_02:
	case cJ1_JPIMMED_4_03:
	    IMM_MULTI(j__udySearchLeafEmpty4, cJ1_JPIMMED_4_02);

	case cJ1_JPIMMED_5_02:
	case cJ1_JPIMMED_5_03:
	    IMM_MULTI(j__udySearchLeafEmpty5, cJ1_JPIMMED_5_02);

	case cJ1_JPIMMED_6_02:
	    IMM_MULTI(j__udySearchLeafEmpty6, cJ1_JPIMMED_6_02);

	case cJ1_JPIMMED_7_02:
	    IMM_MULTI(j__udySearchLeafEmpty7, cJ1_JPIMMED_7_02);
#endif


// ----------------------------------------------------------------------------
// INVALID JP TYPE:

	default: RET_CORRUPT;

	} // SMGet switch.

} // Judy1PrevEmpty() / Judy1NextEmpty() / JudyLPrevEmpty() / JudyLNextEmpty()
