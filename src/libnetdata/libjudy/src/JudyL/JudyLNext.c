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

// @(#) $Revision: 4.54 $ $Source: /judy/src/JudyCommon/JudyPrevNext.c $
//
// Judy*Prev() and Judy*Next() functions for Judy1 and JudyL.
// Compile with one of -DJUDY1 or -DJUDYL.
//
// Compile with -DJUDYNEXT for the Judy*Next() function; otherwise defaults to
// Judy*Prev().

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


// ****************************************************************************
// J U D Y   1   P R E V
// J U D Y   1   N E X T
// J U D Y   L   P R E V
// J U D Y   L   N E X T
//
// See the manual entry for the API.
//
// OVERVIEW OF Judy*Prev():
//
// Use a reentrant switch statement (state machine, SM1 = "get") to decode the
// callers *PIndex-1, starting with the (PArray), through branches, if
// any, down to an immediate or a leaf.  Look for *PIndex-1 in that leaf, and
// if found, return it.
//
// A dead end is either a branch that does not contain a JP for the appropriate
// digit in *PIndex-1, or a leaf that does not contain the undecoded digits of
// *PIndex-1.  Upon reaching a dead end, backtrack through the leaf/branches
// that were just traversed, using a list (history) of parent JPs that is built
// while going forward in SM1Get.  Start with the current leaf or branch.  In a
// backtracked leaf, look for an Index less than *PIndex-1.  In each
// backtracked branch, look "sideways" for the next JP, if any, lower than the
// one for the digit (from *PIndex-1) that was previously decoded.  While
// backtracking, if a leaf has no previous Index or a branch has no lower JP,
// go to its parent branch in turn.  Upon reaching the JRP, return failure, "no
// previous Index".  The backtrack process is sufficiently different from
// SM1Get to merit its own separate reentrant switch statement (SM2 =
// "backtrack").
//
// While backtracking, upon finding a lower JP in a branch, there is certain to
// be a "prev" Index under that JP (unless the Judy array is corrupt).
// Traverse forward again, this time taking the last (highest, right-most) JP
// in each branch, and the last (highest) Index upon reaching an immediate or a
// leaf.  This traversal is sufficiently different from SM1Get and SM2Backtrack
// to merit its own separate reentrant switch statement (SM3 = "findlimit").
//
// "Decode" bytes in JPs complicate this process a little.  In SM1Get, when a
// JP is a narrow pointer, that is, when states are skipped (so the skipped
// digits are stored in jp_DcdPopO), compare the relevant digits to the same
// digits in *PIndex-1.  If they are EQUAL, proceed in SM1Get as before.  If
// jp_DcdPopOs digits are GREATER, treat the JP as a dead end and proceed in
// SM2Backtrack.  If jp_DcdPopOs digits are LESS, treat the JP as if it had
// just been found during a backtrack and proceed directly in SM3Findlimit.
//
// Note that Decode bytes can be ignored in SM3Findlimit; they dont matter.
// Also note that in practice the Decode bytes are routinely compared with
// *PIndex-1 because thats simpler and no slower than first testing for
// narrowness.
//
// Decode bytes also make it unnecessary to construct the Index to return (the
// revised *PIndex) during the search.  This step is deferred until finding an
// Index during backtrack or findlimit, before returning it.  The first digit
// of *PIndex is derived (saved) based on which JP is used in a JRP branch.
// The remaining digits are obtained from the jp_DcdPopO field in the JP (if
// any) above the immediate or leaf containing the found (prev) Index, plus the
// remaining digit(s) in the immediate or leaf itself.  In the case of a LEAFW,
// the Index to return is found directly in the leaf.
//
// Note:  Theoretically, as described above, upon reaching a dead end, SM1Get
// passes control to SM2Backtrack to look sideways, even in a leaf.  Actually
// its a little more efficient for the SM1Get leaf cases to shortcut this and
// take care of the sideways searches themselves.  Hence the history list only
// contains branch JPs, and SM2Backtrack only handles branches.  In fact, even
// the branch handling cases in SM1Get do some shortcutting (sideways
// searching) to avoid pushing history and calling SM2Backtrack unnecessarily.
//
// Upon reaching an Index to return after backtracking, *PIndex must be
// modified to the found Index.  In principle this could be done by building
// the Index from a saved rootdigit (in the top branch) plus the Dcd bytes from
// the parent JP plus the appropriate Index bytes from the leaf.  However,
// Immediates are difficult because their parent JPs lack one (last) digit.  So
// instead just build the *PIndex to return "top down" while backtracking and
// findlimiting.
//
// This function is written iteratively for speed, rather than recursively.
//
// CAVEATS:
//
// Why use a backtrack list (history stack), since it has finite size?  The
// size is small for Judy on both 32-bit and 64-bit systems, and a list (really
// just an array) is fast to maintain and use.  Other alternatives include
// doing a lookahead (lookaside) in each branch while traversing forward
// (decoding), and restarting from the top upon a dead end.
//
// A lookahead means noting the last branch traversed which contained a
// non-null JP lower than the one specified by a digit in *PIndex-1, and
// returning to that point for SM3Findlimit.  This seems like a good idea, and
// should be pretty cheap for linear and bitmap branches, but it could result
// in up to 31 unnecessary additional cache line fills (in extreme cases) for
// every uncompressed branch traversed.  We have considered means of attaching
// to or hiding within an uncompressed branch (in null JPs) a "cache line map"
// or other structure, such as an offset to the next non-null JP, that would
// speed this up, but it seems unnecessary merely to avoid having a
// finite-length list (array).  (If JudySL is ever made "native", the finite
// list length will be an issue.)
//
// Restarting at the top of the Judy array after a dead end requires a careful
// modification of *PIndex-1 to decrement the digit for the parent branch and
// set the remaining lower digits to all 1s.  This must be repeated each time a
// parent branch contains another dead end, so even though it should all happen
// in cache, the CPU time can be excessive.  (For JudySL or an equivalent
// "infinitely deep" Judy array, consider a hybrid of a large, finite,
// "circular" list and a restart-at-top when the list is backtracked to
// exhaustion.)
//
// Why search for *PIndex-1 instead of *PIndex during SM1Get?  In rare
// instances this prevents an unnecessary decode down the wrong path followed
// by a backtrack; its pretty cheap to set up initially; and it means the
// SM1Get machine can simply return if/when it finds that Index.
//
// TBD:  Wed like to enhance this function to make successive searches faster.
// This would require saving some previous state, including the previous Index
// returned, and in which leaf it was found.  If the next call is for the same
// Index and the array has not been modified, start at the same leaf.  This
// should be much easier to implement since this is iterative rather than
// recursive code.
//
// VARIATIONS FOR Judy*Next():
//
// The Judy*Next() code is nearly a perfect mirror of the Judy*Prev() code.
// See the Judy*Prev() overview comments, and mentally switch the following:
//
// - "*PIndex-1"  => "*PIndex+1"
// - "less than"  => "greater than"
// - "lower"      => "higher"
// - "lowest"     => "highest"
// - "next-left"  => "next-right"
// - "right-most" => "left-most"
//
// Note:  SM3Findlimit could be called SM3Findmax/SM3Findmin, but a common name
// for both Prev and Next means many fewer ifdefs in this code.
//
// TBD:  Currently this code traverses a JP whether its expanse is partially or
// completely full (populated).  For Judy1 (only), since there is no value area
// needed, consider shortcutting to a "success" return upon encountering a full
// JP in SM1Get (or even SM3Findlimit?)  A full JP looks like this:
//
//	(((JU_JPDCDPOP0(Pjp) ^ cJU_ALLONES) & cJU_POP0MASK(cLevel)) == 0)

#ifdef JUDY1
#ifdef JUDYPREV
FUNCTION int Judy1Prev
#else
FUNCTION int Judy1Next
#endif
#else
#ifdef JUDYPREV
FUNCTION PPvoid_t JudyLPrev
#else
FUNCTION PPvoid_t JudyLNext
#endif
#endif
        (
	Pcvoid_t  PArray,	// Judy array to search.
	Word_t *  PIndex,	// starting point and result.
	PJError_t PJError	// optional, for returning error info.
        )
{
	Pjp_t	  Pjp, Pjp2;	// current JPs.
	Pjbl_t	  Pjbl;		// Pjp->jp_Addr masked and cast to types:
	Pjbb_t	  Pjbb;
	Pjbu_t	  Pjbu;

// Note:  The following initialization is not strictly required but it makes
// gcc -Wall happy because there is an "impossible" path from Immed handling to
// SM1LeafLImm code that looks like Pjll might be used before set:

	Pjll_t	  Pjll = (Pjll_t) NULL;
	Word_t	  state;	// current state in SM.
	Word_t	  digit;	// next digit to decode from Index.

// Note:  The following initialization is not strictly required but it makes
// gcc -Wall happy because there is an "impossible" path from Immed handling to
// SM1LeafLImm code (for JudyL & JudyPrev only) that looks like pop1 might be
// used before set:

#if (defined(JUDYL) && defined(JUDYPREV))
	Word_t	  pop1 = 0;	// in a leaf.
#else
	Word_t	  pop1;		// in a leaf.
#endif
	int	  offset;	// linear branch/leaf, from j__udySearchLeaf*().
	int	  subexp;	// subexpanse in a bitmap branch.
	Word_t	  bitposmask;	// bit in bitmap for Index.

// History for SM2Backtrack:
//
// For a given histnum, APjphist[histnum] is a parent JP that points to a
// branch, and Aoffhist[histnum] is the offset of the NEXT JP in the branch to
// which the parent JP points.  The meaning of Aoffhist[histnum] depends on the
// type of branch to which the parent JP points:
//
// Linear:  Offset of the next JP in the JP list.
//
// Bitmap:  Which subexpanse, plus the offset of the next JP in the
// subexpanses JP list (to avoid bit-counting again), plus for Judy*Next(),
// hidden one byte to the left, which digit, because Judy*Next() also needs
// this.
//
// Uncompressed:  Digit, which is actually the offset of the JP in the branch.
//
// Note:  Only branch JPs are stored in APjphist[] because, as explained
// earlier, SM1Get shortcuts sideways searches in leaves (and even in branches
// in some cases), so SM2Backtrack only handles branches.

#define	HISTNUMMAX cJU_ROOTSTATE	// maximum branches traversable.
	Pjp_t	APjphist[HISTNUMMAX];	// list of branch JPs traversed.
	int	Aoffhist[HISTNUMMAX];	// list of next JP offsets; see above.
	int	histnum = 0;		// number of JPs now in list.


// ----------------------------------------------------------------------------
// M A C R O S
//
// These are intended to make the code a bit more readable and less redundant.


// "PUSH" AND "POP" Pjp AND offset ON HISTORY STACKS:
//
// Note:  Ensure a corrupt Judy array does not overflow *hist[].  Meanwhile,
// underflowing *hist[] simply means theres no more room to backtrack =>
// "no previous/next Index".

#define	HISTPUSH(Pjp,Offset)			\
	APjphist[histnum] = (Pjp);		\
	Aoffhist[histnum] = (Offset);		\
						\
	if (++histnum >= HISTNUMMAX)		\
	{					\
	    JU_SET_ERRNO(PJError, JU_ERRNO_CORRUPT) \
	    JUDY1CODE(return(JERRI );)		\
	    JUDYLCODE(return(PPJERR);)		\
	}

#define	HISTPOP(Pjp,Offset)			\
	if ((histnum--) < 1) JU_RET_NOTFOUND;	\
	(Pjp)	 = APjphist[histnum];		\
	(Offset) = Aoffhist[histnum]

// How to pack/unpack Aoffhist[] values for bitmap branches:

#ifdef JUDYPREV

#define	HISTPUSHBOFF(Subexp,Offset,Digit)	  \
	(((Subexp) * cJU_BITSPERSUBEXPB) | (Offset))

#define	HISTPOPBOFF(Subexp,Offset,Digit)	  \
	(Subexp)  = (Offset) / cJU_BITSPERSUBEXPB; \
	(Offset) %= cJU_BITSPERSUBEXPB
#else

#define	HISTPUSHBOFF(Subexp,Offset,Digit)	 \
	 (((Digit) << cJU_BITSPERBYTE)		 \
	| ((Subexp) * cJU_BITSPERSUBEXPB) | (Offset))

#define	HISTPOPBOFF(Subexp,Offset,Digit)	 \
	(Digit)   = (Offset) >> cJU_BITSPERBYTE; \
	(Subexp)  = ((Offset) & JU_LEASTBYTESMASK(1)) / cJU_BITSPERSUBEXPB; \
	(Offset) %= cJU_BITSPERSUBEXPB
#endif


// CHECK FOR NULL JP:

#define	JPNULL(Type)  (((Type) >= cJU_JPNULL1) && ((Type) <= cJU_JPNULLMAX))


// SEARCH A BITMAP:
//
// This is a weak analog of j__udySearchLeaf*() for bitmaps.  Return the actual
// or next-left position, base 0, of Digit in the single uint32_t bitmap, also
// given a Bitposmask for Digit.
//
// Unlike j__udySearchLeaf*(), the offset is not returned bit-complemented if
// Digits bit is unset, because the caller can check the bitmap themselves to
// determine that.  Also, if Digits bit is unset, the returned offset is to
// the next-left JP (including -1), not to the "ideal" position for the Index =
// next-right JP.
//
// Shortcut and skip calling j__udyCountBits*() if the bitmap is full, in which
// case (Digit % cJU_BITSPERSUBEXP*) itself is the base-0 offset.
//
// TBD for Judy*Next():  Should this return next-right instead of next-left?
// That is, +1 from current value?  Maybe not, if Digits bit IS set, +1 would
// be wrong.

#define	SEARCHBITMAPB(Bitmap,Digit,Bitposmask)				\
	(((Bitmap) == cJU_FULLBITMAPB) ? (Digit % cJU_BITSPERSUBEXPB) :	\
	 j__udyCountBitsB((Bitmap) & JU_MASKLOWERINC(Bitposmask)) - 1)

#define	SEARCHBITMAPL(Bitmap,Digit,Bitposmask)				\
	(((Bitmap) == cJU_FULLBITMAPL) ? (Digit % cJU_BITSPERSUBEXPL) :	\
	 j__udyCountBitsL((Bitmap) & JU_MASKLOWERINC(Bitposmask)) - 1)

#ifdef JUDYPREV
// Equivalent to search for the highest offset in Bitmap:

#define	SEARCHBITMAPMAXB(Bitmap)				  \
	(((Bitmap) == cJU_FULLBITMAPB) ? cJU_BITSPERSUBEXPB - 1 : \
	 j__udyCountBitsB(Bitmap) - 1)

#define	SEARCHBITMAPMAXL(Bitmap)				  \
	(((Bitmap) == cJU_FULLBITMAPL) ? cJU_BITSPERSUBEXPL - 1 : \
	 j__udyCountBitsL(Bitmap) - 1)
#endif


// CHECK DECODE BYTES:
//
// Check Decode bytes in a JP against the equivalent portion of *PIndex.  If
// *PIndex is lower (for Judy*Prev()) or higher (for Judy*Next()), this JP is a
// dead end (the same as if it had been absent in a linear or bitmap branch or
// null in an uncompressed branch), enter SM2Backtrack; otherwise enter
// SM3Findlimit to find the highest/lowest Index under this JP, as if the code
// had already backtracked to this JP.

#ifdef JUDYPREV
#define	CDcmp__ <
#else
#define	CDcmp__ >
#endif

#define	CHECKDCD(cState)						\
	if (JU_DCDNOTMATCHINDEX(*PIndex, Pjp, cState))	                \
	{								\
	    if ((*PIndex		& cJU_DCDMASK(cState))		\
	      CDcmp__(JU_JPDCDPOP0(Pjp) & cJU_DCDMASK(cState)))		\
	    {								\
		goto SM2Backtrack;					\
	    }								\
	    goto SM3Findlimit;						\
	}


// PREPARE TO HANDLE A LEAFW OR JRP BRANCH IN SM1:
//
// Extract a state-dependent digit from Index in a "constant" way, then jump to
// common code for multiple cases.

#define	SM1PREPB(cState,Next)				\
	state = (cState);				\
	digit = JU_DIGITATSTATE(*PIndex, cState);	\
	goto Next


// PREPARE TO HANDLE A LEAFW OR JRP BRANCH IN SM3:
//
// Optionally save Dcd bytes into *PIndex, then save state and jump to common
// code for multiple cases.

#define	SM3PREPB_DCD(cState,Next)			\
	JU_SETDCD(*PIndex, Pjp, cState);	        \
	SM3PREPB(cState,Next)

#define	SM3PREPB(cState,Next)  state = (cState); goto Next


// ----------------------------------------------------------------------------
// CHECK FOR SHORTCUTS:
//
// Error out if PIndex is null.  Execute JU_RET_NOTFOUND if the Judy array is
// empty or *PIndex is already the minimum/maximum Index possible.
//
// Note:  As documented, in case of failure *PIndex may be modified.

	if (PIndex == (PWord_t) NULL)
	{
	    JU_SET_ERRNO(PJError, JU_ERRNO_NULLPINDEX);
	    JUDY1CODE(return(JERRI );)
	    JUDYLCODE(return(PPJERR);)
	}

#ifdef JUDYPREV
	if ((PArray == (Pvoid_t) NULL) || ((*PIndex)-- == 0))
#else
	if ((PArray == (Pvoid_t) NULL) || ((*PIndex)++ == cJU_ALLONES))
#endif
	    JU_RET_NOTFOUND;


// HANDLE JRP:
//
// Before even entering SM1Get, check the JRP type.  For JRP branches, traverse
// the JPM; handle LEAFW leaves directly; but look for the most common cases
// first.

// ROOT-STATE LEAF that starts with a Pop0 word; just look within the leaf:
//
// If *PIndex is in the leaf, return it; otherwise return the Index, if any,
// below where it would belong.

	if (JU_LEAFW_POP0(PArray) < cJU_LEAFW_MAXPOP1) // must be a LEAFW
	{
	    Pjlw_t Pjlw = P_JLW(PArray);	// first word of leaf.
	    pop1 = Pjlw[0] + 1;

	    if ((offset = j__udySearchLeafW(Pjlw + 1, pop1, *PIndex))
		>= 0)				// Index is present.
	    {
		assert(offset < pop1);			  // in expected range.
		JU_RET_FOUND_LEAFW(Pjlw, pop1, offset); // *PIndex is set.
	    }

#ifdef JUDYPREV
	    if ((offset = ~offset) == 0)	// no next-left Index.
#else
	    if ((offset = ~offset) >= pop1)	// no next-right Index.
#endif
		JU_RET_NOTFOUND;

	    assert(offset <= pop1);		// valid result.

#ifdef JUDYPREV
	    *PIndex = Pjlw[offset--];		// next-left Index, base 1.
#else
	    *PIndex = Pjlw[offset + 1];		// next-right Index, base 1.
#endif
	    JU_RET_FOUND_LEAFW(Pjlw, pop1, offset);	// base 0.

	}
	else	// JRP BRANCH
	{
	    Pjpm_t Pjpm = P_JPM(PArray);
	    Pjp = &(Pjpm->jpm_JP);

//	    goto SM1Get;
	}

// ============================================================================
// STATE MACHINE 1 -- GET INDEX:
//
// Search for *PIndex (already decremented/incremented so as to be inclusive).
// If found, return it.  Otherwise in theory hand off to SM2Backtrack or
// SM3Findlimit, but in practice "shortcut" by first sideways searching the
// current branch or leaf upon hitting a dead end.  During sideways search,
// modify *PIndex to a new path taken.
//
// ENTRY:  Pjp points to next JP to interpret, whose Decode bytes have not yet
// been checked.  This JP is not yet listed in history.
//
// Note:  Check Decode bytes at the start of each loop, not after looking up a
// new JP, so its easy to do constant shifts/masks, although this requires
// cautious handling of Pjp, offset, and *hist[] for correct entry to
// SM2Backtrack.
//
// EXIT:  Return, or branch to SM2Backtrack or SM3Findlimit with correct
// interface, as described elsewhere.
//
// WARNING:  For run-time efficiency the following cases replicate code with
// varying constants, rather than using common code with variable values!

SM1Get:				// return here for next branch/leaf.

	switch (JU_JPTYPE(Pjp))
	{


// ----------------------------------------------------------------------------
// LINEAR BRANCH:
//
// Check Decode bytes, if any, in the current JP, then search for a JP for the
// next digit in *PIndex.

	case cJU_JPBRANCH_L2: CHECKDCD(2); SM1PREPB(2, SM1BranchL);
	case cJU_JPBRANCH_L3: CHECKDCD(3); SM1PREPB(3, SM1BranchL);
#ifdef JU_64BIT
	case cJU_JPBRANCH_L4: CHECKDCD(4); SM1PREPB(4, SM1BranchL);
	case cJU_JPBRANCH_L5: CHECKDCD(5); SM1PREPB(5, SM1BranchL);
	case cJU_JPBRANCH_L6: CHECKDCD(6); SM1PREPB(6, SM1BranchL);
	case cJU_JPBRANCH_L7: CHECKDCD(7); SM1PREPB(7, SM1BranchL);
#endif
	case cJU_JPBRANCH_L:		   SM1PREPB(cJU_ROOTSTATE, SM1BranchL);

// Common code (state-independent) for all cases of linear branches:

SM1BranchL:
	    Pjbl = P_JBL(Pjp->jp_Addr);

// Found JP matching current digit in *PIndex; record parent JP and the next
// JPs offset, and iterate to the next JP:

	    if ((offset = j__udySearchLeaf1((Pjll_t) (Pjbl->jbl_Expanse),
					     Pjbl->jbl_NumJPs, digit)) >= 0)
	    {
		HISTPUSH(Pjp, offset);
		Pjp = (Pjbl->jbl_jp) + offset;
		goto SM1Get;
	    }

// Dead end, no JP in BranchL for next digit in *PIndex:
//
// Get the ideal location of digits JP, and if theres no next-left/right JP
// in the BranchL, shortcut and start backtracking one level up; ignore the
// current Pjp because it points to a BranchL with no next-left/right JP.

#ifdef JUDYPREV
	    if ((offset = (~offset) - 1) < 0)	// no next-left JP in BranchL.
#else
	    if ((offset = (~offset)) >= Pjbl->jbl_NumJPs)  // no next-right.
#endif
		goto SM2Backtrack;

// Theres a next-left/right JP in the current BranchL; save its digit in
// *PIndex and shortcut to SM3Findlimit:

	    JU_SETDIGIT(*PIndex, Pjbl->jbl_Expanse[offset], state);
	    Pjp = (Pjbl->jbl_jp) + offset;
	    goto SM3Findlimit;


// ----------------------------------------------------------------------------
// BITMAP BRANCH:
//
// Check Decode bytes, if any, in the current JP, then look for a JP for the
// next digit in *PIndex.

	case cJU_JPBRANCH_B2: CHECKDCD(2); SM1PREPB(2, SM1BranchB);
	case cJU_JPBRANCH_B3: CHECKDCD(3); SM1PREPB(3, SM1BranchB);
#ifdef JU_64BIT
	case cJU_JPBRANCH_B4: CHECKDCD(4); SM1PREPB(4, SM1BranchB);
	case cJU_JPBRANCH_B5: CHECKDCD(5); SM1PREPB(5, SM1BranchB);
	case cJU_JPBRANCH_B6: CHECKDCD(6); SM1PREPB(6, SM1BranchB);
	case cJU_JPBRANCH_B7: CHECKDCD(7); SM1PREPB(7, SM1BranchB);
#endif
	case cJU_JPBRANCH_B:		   SM1PREPB(cJU_ROOTSTATE, SM1BranchB);

// Common code (state-independent) for all cases of bitmap branches:

SM1BranchB:
	    Pjbb = P_JBB(Pjp->jp_Addr);

// Locate the digits JP in the subexpanse list, if present, otherwise the
// offset of the next-left JP, if any:

	    subexp     = digit / cJU_BITSPERSUBEXPB;
	    assert(subexp < cJU_NUMSUBEXPB);	// falls in expected range.
	    bitposmask = JU_BITPOSMASKB(digit);
	    offset     = SEARCHBITMAPB(JU_JBB_BITMAP(Pjbb, subexp), digit,
				       bitposmask);
	    // right range:
	    assert((offset >= -1) && (offset < (int) cJU_BITSPERSUBEXPB));

// Found JP matching current digit in *PIndex:
//
// Record the parent JP and the next JPs offset; and iterate to the next JP.

//	    if (JU_BITMAPTESTB(Pjbb, digit))			// slower.
	    if (JU_JBB_BITMAP(Pjbb, subexp) & bitposmask)	// faster.
	    {
		// not negative since at least one bit is set:
		assert(offset >= 0);

		HISTPUSH(Pjp, HISTPUSHBOFF(subexp, offset, digit));

		if ((Pjp = P_JP(JU_JBB_PJP(Pjbb, subexp))) == (Pjp_t) NULL)
		{
		    JU_SET_ERRNO(PJError, JU_ERRNO_CORRUPT);
		    JUDY1CODE(return(JERRI );)
		    JUDYLCODE(return(PPJERR);)
		}

		Pjp += offset;
		goto SM1Get;		// iterate to next JP.
	    }

// Dead end, no JP in BranchB for next digit in *PIndex:
//
// If theres a next-left/right JP in the current BranchB, shortcut to
// SM3Findlimit.  Note:  offset is already set to the correct value for the
// next-left/right JP.

#ifdef JUDYPREV
	    if (offset >= 0)		// next-left JP is in this subexpanse.
		goto SM1BranchBFindlimit;

	    while (--subexp >= 0)		// search next-left subexpanses.
#else
	    if (JU_JBB_BITMAP(Pjbb, subexp) & JU_MASKHIGHEREXC(bitposmask))
	    {
		++offset;			// next-left => next-right.
		goto SM1BranchBFindlimit;
	    }

	    while (++subexp < cJU_NUMSUBEXPB)	// search next-right subexps.
#endif
	    {
		if (! JU_JBB_PJP(Pjbb, subexp)) continue;  // empty subexpanse.

#ifdef JUDYPREV
		offset = SEARCHBITMAPMAXB(JU_JBB_BITMAP(Pjbb, subexp));
		// expected range:
		assert((offset >= 0) && (offset < cJU_BITSPERSUBEXPB));
#else
		offset = 0;
#endif

// Save the next-left/right JPs digit in *PIndex:

SM1BranchBFindlimit:
		JU_BITMAPDIGITB(digit, subexp, JU_JBB_BITMAP(Pjbb, subexp),
				offset);
		JU_SETDIGIT(*PIndex, digit, state);

		if ((Pjp = P_JP(JU_JBB_PJP(Pjbb, subexp))) == (Pjp_t) NULL)
		{
		    JU_SET_ERRNO(PJError, JU_ERRNO_CORRUPT);
		    JUDY1CODE(return(JERRI );)
		    JUDYLCODE(return(PPJERR);)
		}

		Pjp += offset;
		goto SM3Findlimit;
	    }

// Theres no next-left/right JP in the BranchB:
//
// Shortcut and start backtracking one level up; ignore the current Pjp because
// it points to a BranchB with no next-left/right JP.

	    goto SM2Backtrack;


// ----------------------------------------------------------------------------
// UNCOMPRESSED BRANCH:
//
// Check Decode bytes, if any, in the current JP, then look for a JP for the
// next digit in *PIndex.

	case cJU_JPBRANCH_U2: CHECKDCD(2); SM1PREPB(2, SM1BranchU);
	case cJU_JPBRANCH_U3: CHECKDCD(3); SM1PREPB(3, SM1BranchU);
#ifdef JU_64BIT
	case cJU_JPBRANCH_U4: CHECKDCD(4); SM1PREPB(4, SM1BranchU);
	case cJU_JPBRANCH_U5: CHECKDCD(5); SM1PREPB(5, SM1BranchU);
	case cJU_JPBRANCH_U6: CHECKDCD(6); SM1PREPB(6, SM1BranchU);
	case cJU_JPBRANCH_U7: CHECKDCD(7); SM1PREPB(7, SM1BranchU);
#endif
	case cJU_JPBRANCH_U:		   SM1PREPB(cJU_ROOTSTATE, SM1BranchU);

// Common code (state-independent) for all cases of uncompressed branches:

SM1BranchU:
	    Pjbu = P_JBU(Pjp->jp_Addr);
	    Pjp2 = (Pjbu->jbu_jp) + digit;

// Found JP matching current digit in *PIndex:
//
// Record the parent JP and the next JPs digit, and iterate to the next JP.
//
// TBD:  Instead of this, just goto SM1Get, and add cJU_JPNULL* cases to the
// SM1Get state machine?  Then backtrack?  However, it means you cant detect
// an inappropriate cJU_JPNULL*, when it occurs in other than a BranchU, and
// return JU_RET_CORRUPT.

	    if (! JPNULL(JU_JPTYPE(Pjp2)))	// digit has a JP.
	    {
		HISTPUSH(Pjp, digit);
		Pjp = Pjp2;
		goto SM1Get;
	    }

// Dead end, no JP in BranchU for next digit in *PIndex:
//
// Search for a next-left/right JP in the current BranchU, and if one is found,
// save its digit in *PIndex and shortcut to SM3Findlimit:

#ifdef JUDYPREV
	    while (digit >= 1)
	    {
		Pjp = (Pjbu->jbu_jp) + (--digit);
#else
	    while (digit < cJU_BRANCHUNUMJPS - 1)
	    {
		Pjp = (Pjbu->jbu_jp) + (++digit);
#endif
		if (JPNULL(JU_JPTYPE(Pjp))) continue;

		JU_SETDIGIT(*PIndex, digit, state);
		goto SM3Findlimit;
	    }

// Theres no next-left/right JP in the BranchU:
//
// Shortcut and start backtracking one level up; ignore the current Pjp because
// it points to a BranchU with no next-left/right JP.

	    goto SM2Backtrack;


// ----------------------------------------------------------------------------
// LINEAR LEAF:
//
// Check Decode bytes, if any, in the current JP, then search the leaf for
// *PIndex.

#define	SM1LEAFL(Func)					\
	Pjll   = P_JLL(Pjp->jp_Addr);			\
	pop1   = JU_JPLEAF_POP0(Pjp) + 1;	        \
	offset = Func(Pjll, pop1, *PIndex);		\
	goto SM1LeafLImm

#if (defined(JUDYL) || (! defined(JU_64BIT)))
	case cJU_JPLEAF1:  CHECKDCD(1); SM1LEAFL(j__udySearchLeaf1);
#endif
	case cJU_JPLEAF2:  CHECKDCD(2); SM1LEAFL(j__udySearchLeaf2);
	case cJU_JPLEAF3:  CHECKDCD(3); SM1LEAFL(j__udySearchLeaf3);

#ifdef JU_64BIT
	case cJU_JPLEAF4:  CHECKDCD(4); SM1LEAFL(j__udySearchLeaf4);
	case cJU_JPLEAF5:  CHECKDCD(5); SM1LEAFL(j__udySearchLeaf5);
	case cJU_JPLEAF6:  CHECKDCD(6); SM1LEAFL(j__udySearchLeaf6);
	case cJU_JPLEAF7:  CHECKDCD(7); SM1LEAFL(j__udySearchLeaf7);
#endif

// Common code (state-independent) for all cases of linear leaves and
// immediates:

SM1LeafLImm:
	    if (offset >= 0)		// *PIndex is in LeafL / Immed.
#ifdef JUDY1
		JU_RET_FOUND;
#else
	    {				// JudyL is trickier...
		switch (JU_JPTYPE(Pjp))
		{
#if (defined(JUDYL) || (! defined(JU_64BIT)))
		case cJU_JPLEAF1: JU_RET_FOUND_LEAF1(Pjll, pop1, offset);
#endif
		case cJU_JPLEAF2: JU_RET_FOUND_LEAF2(Pjll, pop1, offset);
		case cJU_JPLEAF3: JU_RET_FOUND_LEAF3(Pjll, pop1, offset);
#ifdef JU_64BIT
		case cJU_JPLEAF4: JU_RET_FOUND_LEAF4(Pjll, pop1, offset);
		case cJU_JPLEAF5: JU_RET_FOUND_LEAF5(Pjll, pop1, offset);
		case cJU_JPLEAF6: JU_RET_FOUND_LEAF6(Pjll, pop1, offset);
		case cJU_JPLEAF7: JU_RET_FOUND_LEAF7(Pjll, pop1, offset);
#endif

		case cJU_JPIMMED_1_01:
		case cJU_JPIMMED_2_01:
		case cJU_JPIMMED_3_01:
#ifdef JU_64BIT
		case cJU_JPIMMED_4_01:
		case cJU_JPIMMED_5_01:
		case cJU_JPIMMED_6_01:
		case cJU_JPIMMED_7_01:
#endif
		    JU_RET_FOUND_IMM_01(Pjp);

		case cJU_JPIMMED_1_02:
		case cJU_JPIMMED_1_03:
#ifdef JU_64BIT
		case cJU_JPIMMED_1_04:
		case cJU_JPIMMED_1_05:
		case cJU_JPIMMED_1_06:
		case cJU_JPIMMED_1_07:
		case cJU_JPIMMED_2_02:
		case cJU_JPIMMED_2_03:
		case cJU_JPIMMED_3_02:
#endif
		    JU_RET_FOUND_IMM(Pjp, offset);
		}

		JU_SET_ERRNO(PJError, JU_ERRNO_CORRUPT);  // impossible?
		JUDY1CODE(return(JERRI );)
		JUDYLCODE(return(PPJERR);)

	    } // found *PIndex

#endif // JUDYL

// Dead end, no Index in LeafL / Immed for remaining digit(s) in *PIndex:
//
// Get the ideal location of Index, and if theres no next-left/right Index in
// the LeafL / Immed, shortcut and start backtracking one level up; ignore the
// current Pjp because it points to a LeafL / Immed with no next-left/right
// Index.

#ifdef JUDYPREV
	    if ((offset = (~offset) - 1) < 0)	// no next-left Index.
#else
	    if ((offset = (~offset)) >= pop1)	// no next-right Index.
#endif
		goto SM2Backtrack;

// Theres a next-left/right Index in the current LeafL / Immed; shortcut by
// copying its digit(s) to *PIndex and returning it.
//
// Unfortunately this is pretty hairy, especially avoiding endian issues.
//
// The cJU_JPLEAF* cases are very similar to same-index-size cJU_JPIMMED* cases
// for *_02 and above, but must return differently, at least for JudyL, so
// spell them out separately here at the cost of a little redundant code for
// Judy1.

	    switch (JU_JPTYPE(Pjp))
	    {
#if (defined(JUDYL) || (! defined(JU_64BIT)))
	    case cJU_JPLEAF1:

		JU_SETDIGIT1(*PIndex, ((uint8_t *) Pjll)[offset]);
		JU_RET_FOUND_LEAF1(Pjll, pop1, offset);
#endif

	    case cJU_JPLEAF2:

		*PIndex = (*PIndex & (~JU_LEASTBYTESMASK(2)))
			| ((uint16_t *) Pjll)[offset];
		JU_RET_FOUND_LEAF2(Pjll, pop1, offset);

	    case cJU_JPLEAF3:
	    {
		Word_t lsb;
		JU_COPY3_PINDEX_TO_LONG(lsb, ((uint8_t *) Pjll) + (3 * offset));
		*PIndex = (*PIndex & (~JU_LEASTBYTESMASK(3))) | lsb;
		JU_RET_FOUND_LEAF3(Pjll, pop1, offset);
	    }

#ifdef JU_64BIT
	    case cJU_JPLEAF4:

		*PIndex = (*PIndex & (~JU_LEASTBYTESMASK(4)))
			| ((uint32_t *) Pjll)[offset];
		JU_RET_FOUND_LEAF4(Pjll, pop1, offset);

	    case cJU_JPLEAF5:
	    {
		Word_t lsb;
		JU_COPY5_PINDEX_TO_LONG(lsb, ((uint8_t *) Pjll) + (5 * offset));
		*PIndex = (*PIndex & (~JU_LEASTBYTESMASK(5))) | lsb;
		JU_RET_FOUND_LEAF5(Pjll, pop1, offset);
	    }

	    case cJU_JPLEAF6:
	    {
		Word_t lsb;
		JU_COPY6_PINDEX_TO_LONG(lsb, ((uint8_t *) Pjll) + (6 * offset));
		*PIndex = (*PIndex & (~JU_LEASTBYTESMASK(6))) | lsb;
		JU_RET_FOUND_LEAF6(Pjll, pop1, offset);
	    }

	    case cJU_JPLEAF7:
	    {
		Word_t lsb;
		JU_COPY7_PINDEX_TO_LONG(lsb, ((uint8_t *) Pjll) + (7 * offset));
		*PIndex = (*PIndex & (~JU_LEASTBYTESMASK(7))) | lsb;
		JU_RET_FOUND_LEAF7(Pjll, pop1, offset);
	    }

#endif // JU_64BIT

#define	SET_01(cState)  JU_SETDIGITS(*PIndex, JU_JPDCDPOP0(Pjp), cState)

	    case cJU_JPIMMED_1_01: SET_01(1); goto SM1Imm_01;
	    case cJU_JPIMMED_2_01: SET_01(2); goto SM1Imm_01;
	    case cJU_JPIMMED_3_01: SET_01(3); goto SM1Imm_01;
#ifdef JU_64BIT
	    case cJU_JPIMMED_4_01: SET_01(4); goto SM1Imm_01;
	    case cJU_JPIMMED_5_01: SET_01(5); goto SM1Imm_01;
	    case cJU_JPIMMED_6_01: SET_01(6); goto SM1Imm_01;
	    case cJU_JPIMMED_7_01: SET_01(7); goto SM1Imm_01;
#endif
SM1Imm_01:	JU_RET_FOUND_IMM_01(Pjp);

// Shorthand for where to find start of Index bytes array:

#ifdef JUDY1
#define	PJI (Pjp->jp_1Index)
#else
#define	PJI (Pjp->jp_LIndex)
#endif

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
		JU_SETDIGIT1(*PIndex, ((uint8_t *) PJI)[offset]);
		JU_RET_FOUND_IMM(Pjp, offset);

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
		*PIndex = (*PIndex & (~JU_LEASTBYTESMASK(2)))
			| ((uint16_t *) PJI)[offset];
		JU_RET_FOUND_IMM(Pjp, offset);
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
	    {
		Word_t lsb;
		JU_COPY3_PINDEX_TO_LONG(lsb, ((uint8_t *) PJI) + (3 * offset));
		*PIndex = (*PIndex & (~JU_LEASTBYTESMASK(3))) | lsb;
		JU_RET_FOUND_IMM(Pjp, offset);
	    }
#endif

#if (defined(JUDY1) && defined(JU_64BIT))
	    case cJ1_JPIMMED_4_02:
	    case cJ1_JPIMMED_4_03:

		*PIndex = (*PIndex & (~JU_LEASTBYTESMASK(4)))
			| ((uint32_t *) PJI)[offset];
		JU_RET_FOUND_IMM(Pjp, offset);

	    case cJ1_JPIMMED_5_02:
	    case cJ1_JPIMMED_5_03:
	    {
		Word_t lsb;
		JU_COPY5_PINDEX_TO_LONG(lsb, ((uint8_t *) PJI) + (5 * offset));
		*PIndex = (*PIndex & (~JU_LEASTBYTESMASK(5))) | lsb;
		JU_RET_FOUND_IMM(Pjp, offset);
	    }

	    case cJ1_JPIMMED_6_02:
	    {
		Word_t lsb;
		JU_COPY6_PINDEX_TO_LONG(lsb, ((uint8_t *) PJI) + (6 * offset));
		*PIndex = (*PIndex & (~JU_LEASTBYTESMASK(6))) | lsb;
		JU_RET_FOUND_IMM(Pjp, offset);
	    }

	    case cJ1_JPIMMED_7_02:
	    {
		Word_t lsb;
		JU_COPY7_PINDEX_TO_LONG(lsb, ((uint8_t *) PJI) + (7 * offset));
		*PIndex = (*PIndex & (~JU_LEASTBYTESMASK(7))) | lsb;
		JU_RET_FOUND_IMM(Pjp, offset);
	    }

#endif // (JUDY1 && JU_64BIT)

	    } // switch for not-found *PIndex

	    JU_SET_ERRNO(PJError, JU_ERRNO_CORRUPT);	// impossible?
	    JUDY1CODE(return(JERRI );)
	    JUDYLCODE(return(PPJERR);)


// ----------------------------------------------------------------------------
// BITMAP LEAF:
//
// Check Decode bytes, if any, in the current JP, then look in the leaf for
// *PIndex.

	case cJU_JPLEAF_B1:
	{
	    Pjlb_t Pjlb;
	    CHECKDCD(1);

	    Pjlb	= P_JLB(Pjp->jp_Addr);
	    digit       = JU_DIGITATSTATE(*PIndex, 1);
	    subexp      = JU_SUBEXPL(digit);
	    bitposmask  = JU_BITPOSMASKL(digit);
	    assert(subexp < cJU_NUMSUBEXPL);	// falls in expected range.

// *PIndex exists in LeafB1:

//	    if (JU_BITMAPTESTL(Pjlb, digit))			// slower.
	    if (JU_JLB_BITMAP(Pjlb, subexp) & bitposmask)	// faster.
	    {
#ifdef JUDYL				// needs offset at this point:
		offset = SEARCHBITMAPL(JU_JLB_BITMAP(Pjlb, subexp), digit, bitposmask);
#endif
		JU_RET_FOUND_LEAF_B1(Pjlb, subexp, offset);
//	== return((PPvoid_t) (P_JV(JL_JLB_PVALUE(Pjlb, subexp)) + (offset)));
	    }

// Dead end, no Index in LeafB1 for remaining digit in *PIndex:
//
// If theres a next-left/right Index in the current LeafB1, which for
// Judy*Next() is true if any bits are set for higher Indexes, shortcut by
// returning it.  Note:  For Judy*Prev(), offset is set here to the correct
// value for the next-left JP.

	    offset = SEARCHBITMAPL(JU_JLB_BITMAP(Pjlb, subexp), digit,
				   bitposmask);
	    // right range:
	    assert((offset >= -1) && (offset < (int) cJU_BITSPERSUBEXPL));

#ifdef JUDYPREV
	    if (offset >= 0)		// next-left JP is in this subexpanse.
		goto SM1LeafB1Findlimit;

	    while (--subexp >= 0)		// search next-left subexpanses.
#else
	    if (JU_JLB_BITMAP(Pjlb, subexp) & JU_MASKHIGHEREXC(bitposmask))
	    {
		++offset;			// next-left => next-right.
		goto SM1LeafB1Findlimit;
	    }

	    while (++subexp < cJU_NUMSUBEXPL)	// search next-right subexps.
#endif
	    {
		if (! JU_JLB_BITMAP(Pjlb, subexp)) continue;  // empty subexp.

#ifdef JUDYPREV
		offset = SEARCHBITMAPMAXL(JU_JLB_BITMAP(Pjlb, subexp));
		// expected range:
		assert((offset >= 0) && (offset < (int) cJU_BITSPERSUBEXPL));
#else
		offset = 0;
#endif

// Save the next-left/right Indexess digit in *PIndex:

SM1LeafB1Findlimit:
		JU_BITMAPDIGITL(digit, subexp, JU_JLB_BITMAP(Pjlb, subexp), offset);
		JU_SETDIGIT1(*PIndex, digit);
		JU_RET_FOUND_LEAF_B1(Pjlb, subexp, offset);
//	== return((PPvoid_t) (P_JV(JL_JLB_PVALUE(Pjlb, subexp)) + (offset)));
	    }

// Theres no next-left/right Index in the LeafB1:
//
// Shortcut and start backtracking one level up; ignore the current Pjp because
// it points to a LeafB1 with no next-left/right Index.

	    goto SM2Backtrack;

	} // case cJU_JPLEAF_B1

#ifdef JUDY1
// ----------------------------------------------------------------------------
// FULL POPULATION:
//
// If the Decode bytes match, *PIndex is found (without modification).

	case cJ1_JPFULLPOPU1:

	    CHECKDCD(1);
	    JU_RET_FOUND_FULLPOPU1;
#endif


// ----------------------------------------------------------------------------
// IMMEDIATE:

#ifdef JUDYPREV
#define	SM1IMM_SETPOP1(cPop1)
#else
#define SM1IMM_SETPOP1(cPop1)  pop1 = (cPop1)
#endif

#define	SM1IMM(Func,cPop1)				\
	SM1IMM_SETPOP1(cPop1);				\
	offset = Func((Pjll_t) (PJI), cPop1, *PIndex);	\
	goto SM1LeafLImm

// Special case for Pop1 = 1 Immediate JPs:
//
// If *PIndex is in the immediate, offset is 0, otherwise the binary NOT of the
// offset where it belongs, 0 or 1, same as from the search functions.

#ifdef JUDYPREV
#define	SM1IMM_01_SETPOP1
#else
#define SM1IMM_01_SETPOP1  pop1 = 1
#endif

#define	SM1IMM_01							  \
	SM1IMM_01_SETPOP1;						  \
	offset = ((JU_JPDCDPOP0(Pjp) <  JU_TRIMTODCDSIZE(*PIndex)) ? ~1 : \
		  (JU_JPDCDPOP0(Pjp) == JU_TRIMTODCDSIZE(*PIndex)) ?  0 : \
								     ~0); \
	goto SM1LeafLImm

	case cJU_JPIMMED_1_01:
	case cJU_JPIMMED_2_01:
	case cJU_JPIMMED_3_01:
#ifdef JU_64BIT
	case cJU_JPIMMED_4_01:
	case cJU_JPIMMED_5_01:
	case cJU_JPIMMED_6_01:
	case cJU_JPIMMED_7_01:
#endif
	    SM1IMM_01;

// TBD:  Doug says it would be OK to have fewer calls and calculate arg 2, here
// and in Judy*Count() also.

	case cJU_JPIMMED_1_02:  SM1IMM(j__udySearchLeaf1,  2);
	case cJU_JPIMMED_1_03:  SM1IMM(j__udySearchLeaf1,  3);
#if (defined(JUDY1) || defined(JU_64BIT))
	case cJU_JPIMMED_1_04:  SM1IMM(j__udySearchLeaf1,  4);
	case cJU_JPIMMED_1_05:  SM1IMM(j__udySearchLeaf1,  5);
	case cJU_JPIMMED_1_06:  SM1IMM(j__udySearchLeaf1,  6);
	case cJU_JPIMMED_1_07:  SM1IMM(j__udySearchLeaf1,  7);
#endif
#if (defined(JUDY1) && defined(JU_64BIT))
	case cJ1_JPIMMED_1_08:  SM1IMM(j__udySearchLeaf1,  8);
	case cJ1_JPIMMED_1_09:  SM1IMM(j__udySearchLeaf1,  9);
	case cJ1_JPIMMED_1_10:  SM1IMM(j__udySearchLeaf1, 10);
	case cJ1_JPIMMED_1_11:  SM1IMM(j__udySearchLeaf1, 11);
	case cJ1_JPIMMED_1_12:  SM1IMM(j__udySearchLeaf1, 12);
	case cJ1_JPIMMED_1_13:  SM1IMM(j__udySearchLeaf1, 13);
	case cJ1_JPIMMED_1_14:  SM1IMM(j__udySearchLeaf1, 14);
	case cJ1_JPIMMED_1_15:  SM1IMM(j__udySearchLeaf1, 15);
#endif

#if (defined(JUDY1) || defined(JU_64BIT))
	case cJU_JPIMMED_2_02:  SM1IMM(j__udySearchLeaf2,  2);
	case cJU_JPIMMED_2_03:  SM1IMM(j__udySearchLeaf2,  3);
#endif
#if (defined(JUDY1) && defined(JU_64BIT))
	case cJ1_JPIMMED_2_04:  SM1IMM(j__udySearchLeaf2,  4);
	case cJ1_JPIMMED_2_05:  SM1IMM(j__udySearchLeaf2,  5);
	case cJ1_JPIMMED_2_06:  SM1IMM(j__udySearchLeaf2,  6);
	case cJ1_JPIMMED_2_07:  SM1IMM(j__udySearchLeaf2,  7);
#endif

#if (defined(JUDY1) || defined(JU_64BIT))
	case cJU_JPIMMED_3_02:  SM1IMM(j__udySearchLeaf3,  2);
#endif
#if (defined(JUDY1) && defined(JU_64BIT))
	case cJ1_JPIMMED_3_03:  SM1IMM(j__udySearchLeaf3,  3);
	case cJ1_JPIMMED_3_04:  SM1IMM(j__udySearchLeaf3,  4);
	case cJ1_JPIMMED_3_05:  SM1IMM(j__udySearchLeaf3,  5);

	case cJ1_JPIMMED_4_02:  SM1IMM(j__udySearchLeaf4,  2);
	case cJ1_JPIMMED_4_03:  SM1IMM(j__udySearchLeaf4,  3);

	case cJ1_JPIMMED_5_02:  SM1IMM(j__udySearchLeaf5,  2);
	case cJ1_JPIMMED_5_03:  SM1IMM(j__udySearchLeaf5,  3);

	case cJ1_JPIMMED_6_02:  SM1IMM(j__udySearchLeaf6,  2);

	case cJ1_JPIMMED_7_02:  SM1IMM(j__udySearchLeaf7,  2);
#endif


// ----------------------------------------------------------------------------
// INVALID JP TYPE:

	default: JU_SET_ERRNO(PJError, JU_ERRNO_CORRUPT);
		 JUDY1CODE(return(JERRI );)
		 JUDYLCODE(return(PPJERR);)

	} // SM1Get switch.

	/*NOTREACHED*/


// ============================================================================
// STATE MACHINE 2 -- BACKTRACK BRANCH TO PREVIOUS JP:
//
// Look for the next-left/right JP in a branch, backing up the history list as
// necessary.  Upon finding a next-left/right JP, modify the corresponding
// digit in *PIndex before passing control to SM3Findlimit.
//
// Note:  As described earlier, only branch JPs are expected here; other types
// fall into the default case.
//
// Note:  If a found JP contains needed Dcd bytes, thats OK, theyre copied to
// *PIndex in SM3Findlimit.
//
// TBD:  This code has a lot in common with similar code in the shortcut cases
// in SM1Get.  Can combine this code somehow?
//
// ENTRY:  List, possibly empty, of JPs and offsets in APjphist[] and
// Aoffhist[]; see earlier comments.
//
// EXIT:  Execute JU_RET_NOTFOUND if no previous/next JP; otherwise jump to
// SM3Findlimit to resume a new but different downward search.

SM2Backtrack:		// come or return here for first/next sideways search.

	HISTPOP(Pjp, offset);

	switch (JU_JPTYPE(Pjp))
	{


// ----------------------------------------------------------------------------
// LINEAR BRANCH:

	case cJU_JPBRANCH_L2: state = 2;	     goto SM2BranchL;
	case cJU_JPBRANCH_L3: state = 3;	     goto SM2BranchL;
#ifdef JU_64BIT
	case cJU_JPBRANCH_L4: state = 4;	     goto SM2BranchL;
	case cJU_JPBRANCH_L5: state = 5;	     goto SM2BranchL;
	case cJU_JPBRANCH_L6: state = 6;	     goto SM2BranchL;
	case cJU_JPBRANCH_L7: state = 7;	     goto SM2BranchL;
#endif
	case cJU_JPBRANCH_L:  state = cJU_ROOTSTATE; goto SM2BranchL;

SM2BranchL:
#ifdef JUDYPREV
	    if (--offset < 0) goto SM2Backtrack;  // no next-left JP in BranchL.
#endif
	    Pjbl = P_JBL(Pjp->jp_Addr);
#ifdef JUDYNEXT
	    if (++offset >= (Pjbl->jbl_NumJPs)) goto SM2Backtrack;
						// no next-right JP in BranchL.
#endif

// Theres a next-left/right JP in the current BranchL; save its digit in
// *PIndex and continue with SM3Findlimit:

	    JU_SETDIGIT(*PIndex, Pjbl->jbl_Expanse[offset], state);
	    Pjp = (Pjbl->jbl_jp) + offset;
	    goto SM3Findlimit;


// ----------------------------------------------------------------------------
// BITMAP BRANCH:

	case cJU_JPBRANCH_B2: state = 2;	     goto SM2BranchB;
	case cJU_JPBRANCH_B3: state = 3;	     goto SM2BranchB;
#ifdef JU_64BIT
	case cJU_JPBRANCH_B4: state = 4;	     goto SM2BranchB;
	case cJU_JPBRANCH_B5: state = 5;	     goto SM2BranchB;
	case cJU_JPBRANCH_B6: state = 6;	     goto SM2BranchB;
	case cJU_JPBRANCH_B7: state = 7;	     goto SM2BranchB;
#endif
	case cJU_JPBRANCH_B:  state = cJU_ROOTSTATE; goto SM2BranchB;

SM2BranchB:
	    Pjbb = P_JBB(Pjp->jp_Addr);
	    HISTPOPBOFF(subexp, offset, digit);		// unpack values.

// If theres a next-left/right JP in the current BranchB, which for
// Judy*Next() is true if any bits are set for higher Indexes, continue to
// SM3Findlimit:
//
// Note:  offset is set to the JP previously traversed; go one to the
// left/right.

#ifdef JUDYPREV
	    if (offset > 0)		// next-left JP is in this subexpanse.
	    {
		--offset;
		goto SM2BranchBFindlimit;
	    }

	    while (--subexp >= 0)		// search next-left subexpanses.
#else
	    if (JU_JBB_BITMAP(Pjbb, subexp)
	      & JU_MASKHIGHEREXC(JU_BITPOSMASKB(digit)))
	    {
		++offset;			// next-left => next-right.
		goto SM2BranchBFindlimit;
	    }

	    while (++subexp < cJU_NUMSUBEXPB)	// search next-right subexps.
#endif
	    {
		if (! JU_JBB_PJP(Pjbb, subexp)) continue;  // empty subexpanse.

#ifdef JUDYPREV
		offset = SEARCHBITMAPMAXB(JU_JBB_BITMAP(Pjbb, subexp));
		// expected range:
		assert((offset >= 0) && (offset < cJU_BITSPERSUBEXPB));
#else
		offset = 0;
#endif

// Save the next-left/right JPs digit in *PIndex:

SM2BranchBFindlimit:
		JU_BITMAPDIGITB(digit, subexp, JU_JBB_BITMAP(Pjbb, subexp),
				offset);
		JU_SETDIGIT(*PIndex, digit, state);

		if ((Pjp = P_JP(JU_JBB_PJP(Pjbb, subexp))) == (Pjp_t) NULL)
		{
		    JU_SET_ERRNO(PJError, JU_ERRNO_CORRUPT);
		    JUDY1CODE(return(JERRI );)
		    JUDYLCODE(return(PPJERR);)
		}

		Pjp += offset;
		goto SM3Findlimit;
	    }

// Theres no next-left/right JP in the BranchB:

	    goto SM2Backtrack;


// ----------------------------------------------------------------------------
// UNCOMPRESSED BRANCH:

	case cJU_JPBRANCH_U2: state = 2;	     goto SM2BranchU;
	case cJU_JPBRANCH_U3: state = 3;	     goto SM2BranchU;
#ifdef JU_64BIT
	case cJU_JPBRANCH_U4: state = 4;	     goto SM2BranchU;
	case cJU_JPBRANCH_U5: state = 5;	     goto SM2BranchU;
	case cJU_JPBRANCH_U6: state = 6;	     goto SM2BranchU;
	case cJU_JPBRANCH_U7: state = 7;	     goto SM2BranchU;
#endif
	case cJU_JPBRANCH_U:  state = cJU_ROOTSTATE; goto SM2BranchU;

SM2BranchU:

// Search for a next-left/right JP in the current BranchU, and if one is found,
// save its digit in *PIndex and continue to SM3Findlimit:

	    Pjbu  = P_JBU(Pjp->jp_Addr);
	    digit = offset;

#ifdef JUDYPREV
	    while (digit >= 1)
	    {
		Pjp = (Pjbu->jbu_jp) + (--digit);
#else
	    while (digit < cJU_BRANCHUNUMJPS - 1)
	    {
		Pjp = (Pjbu->jbu_jp) + (++digit);
#endif
		if (JPNULL(JU_JPTYPE(Pjp))) continue;

		JU_SETDIGIT(*PIndex, digit, state);
		goto SM3Findlimit;
	    }

// Theres no next-left/right JP in the BranchU:

	    goto SM2Backtrack;


// ----------------------------------------------------------------------------
// INVALID JP TYPE:

	default: JU_SET_ERRNO(PJError, JU_ERRNO_CORRUPT);
		 JUDY1CODE(return(JERRI );)
		 JUDYLCODE(return(PPJERR);)

	} // SM2Backtrack switch.

	/*NOTREACHED*/


// ============================================================================
// STATE MACHINE 3 -- FIND LIMIT JP/INDEX:
//
// Look for the highest/lowest (right/left-most) JP in each branch and the
// highest/lowest Index in a leaf or immediate, and return it.  While
// traversing, modify appropriate digit(s) in *PIndex to reflect the path
// taken, including Dcd bytes in each JP (which could hold critical missing
// digits for skipped branches).
//
// ENTRY:  Pjp set to a JP under which to find max/min JPs (if a branch JP) or
// a max/min Index and return (if a leaf or immediate JP).
//
// EXIT:  Execute JU_RET_FOUND* upon reaching a leaf or immediate.  Should be
// impossible to fail, unless the Judy array is corrupt.

SM3Findlimit:		// come or return here for first/next branch/leaf.

	switch (JU_JPTYPE(Pjp))
	{
// ----------------------------------------------------------------------------
// LINEAR BRANCH:
//
// Simply use the highest/lowest (right/left-most) JP in the BranchL, but first
// copy the Dcd bytes to *PIndex if there are any (only if state <
// cJU_ROOTSTATE - 1).

	case cJU_JPBRANCH_L2:  SM3PREPB_DCD(2, SM3BranchL);
#ifndef JU_64BIT
	case cJU_JPBRANCH_L3:  SM3PREPB(    3, SM3BranchL);
#else
	case cJU_JPBRANCH_L3:  SM3PREPB_DCD(3, SM3BranchL);
	case cJU_JPBRANCH_L4:  SM3PREPB_DCD(4, SM3BranchL);
	case cJU_JPBRANCH_L5:  SM3PREPB_DCD(5, SM3BranchL);
	case cJU_JPBRANCH_L6:  SM3PREPB_DCD(6, SM3BranchL);
	case cJU_JPBRANCH_L7:  SM3PREPB(    7, SM3BranchL);
#endif
	case cJU_JPBRANCH_L:   SM3PREPB(    cJU_ROOTSTATE, SM3BranchL);

SM3BranchL:
	    Pjbl = P_JBL(Pjp->jp_Addr);

#ifdef JUDYPREV
	    if ((offset = (Pjbl->jbl_NumJPs) - 1) < 0)
#else
	    offset = 0; if ((Pjbl->jbl_NumJPs) == 0)
#endif
	    {
		JU_SET_ERRNO(PJError, JU_ERRNO_CORRUPT);
		JUDY1CODE(return(JERRI );)
		JUDYLCODE(return(PPJERR);)
	    }

	    JU_SETDIGIT(*PIndex, Pjbl->jbl_Expanse[offset], state);
	    Pjp = (Pjbl->jbl_jp) + offset;
	    goto SM3Findlimit;


// ----------------------------------------------------------------------------
// BITMAP BRANCH:
//
// Look for the highest/lowest (right/left-most) non-null subexpanse, then use
// the highest/lowest JP in that subexpanse, but first copy Dcd bytes, if there
// are any (only if state < cJU_ROOTSTATE - 1), to *PIndex.

	case cJU_JPBRANCH_B2:  SM3PREPB_DCD(2, SM3BranchB);
#ifndef JU_64BIT
	case cJU_JPBRANCH_B3:  SM3PREPB(    3, SM3BranchB);
#else
	case cJU_JPBRANCH_B3:  SM3PREPB_DCD(3, SM3BranchB);
	case cJU_JPBRANCH_B4:  SM3PREPB_DCD(4, SM3BranchB);
	case cJU_JPBRANCH_B5:  SM3PREPB_DCD(5, SM3BranchB);
	case cJU_JPBRANCH_B6:  SM3PREPB_DCD(6, SM3BranchB);
	case cJU_JPBRANCH_B7:  SM3PREPB(    7, SM3BranchB);
#endif
	case cJU_JPBRANCH_B:   SM3PREPB(    cJU_ROOTSTATE, SM3BranchB);

SM3BranchB:
	    Pjbb   = P_JBB(Pjp->jp_Addr);
#ifdef JUDYPREV
	    subexp = cJU_NUMSUBEXPB;

	    while (! (JU_JBB_BITMAP(Pjbb, --subexp)))  // find non-empty subexp.
	    {
		if (subexp <= 0)		    // wholly empty bitmap.
		{
		    JU_SET_ERRNO(PJError, JU_ERRNO_CORRUPT);
		    JUDY1CODE(return(JERRI );)
		    JUDYLCODE(return(PPJERR);)
		}
	    }

	    offset = SEARCHBITMAPMAXB(JU_JBB_BITMAP(Pjbb, subexp));
	    // expected range:
	    assert((offset >= 0) && (offset < cJU_BITSPERSUBEXPB));
#else
	    subexp = -1;

	    while (! (JU_JBB_BITMAP(Pjbb, ++subexp)))  // find non-empty subexp.
	    {
		if (subexp >= cJU_NUMSUBEXPB - 1)      // didnt find one.
		{
		    JU_SET_ERRNO(PJError, JU_ERRNO_CORRUPT);
		    JUDY1CODE(return(JERRI );)
		    JUDYLCODE(return(PPJERR);)
		}
	    }

	    offset = 0;
#endif

	    JU_BITMAPDIGITB(digit, subexp, JU_JBB_BITMAP(Pjbb, subexp), offset);
	    JU_SETDIGIT(*PIndex, digit, state);

	    if ((Pjp = P_JP(JU_JBB_PJP(Pjbb, subexp))) == (Pjp_t) NULL)
	    {
		JU_SET_ERRNO(PJError, JU_ERRNO_CORRUPT);
		JUDY1CODE(return(JERRI );)
		JUDYLCODE(return(PPJERR);)
	    }

	    Pjp += offset;
	    goto SM3Findlimit;


// ----------------------------------------------------------------------------
// UNCOMPRESSED BRANCH:
//
// Look for the highest/lowest (right/left-most) non-null JP, and use it, but
// first copy Dcd bytes to *PIndex if there are any (only if state <
// cJU_ROOTSTATE - 1).

	case cJU_JPBRANCH_U2:  SM3PREPB_DCD(2, SM3BranchU);
#ifndef JU_64BIT
	case cJU_JPBRANCH_U3:  SM3PREPB(    3, SM3BranchU);
#else
	case cJU_JPBRANCH_U3:  SM3PREPB_DCD(3, SM3BranchU);
	case cJU_JPBRANCH_U4:  SM3PREPB_DCD(4, SM3BranchU);
	case cJU_JPBRANCH_U5:  SM3PREPB_DCD(5, SM3BranchU);
	case cJU_JPBRANCH_U6:  SM3PREPB_DCD(6, SM3BranchU);
	case cJU_JPBRANCH_U7:  SM3PREPB(    7, SM3BranchU);
#endif
	case cJU_JPBRANCH_U:   SM3PREPB(    cJU_ROOTSTATE, SM3BranchU);

SM3BranchU:
	    Pjbu  = P_JBU(Pjp->jp_Addr);
#ifdef JUDYPREV
	    digit = cJU_BRANCHUNUMJPS;

	    while (digit >= 1)
	    {
		Pjp = (Pjbu->jbu_jp) + (--digit);
#else

	    for (digit = 0; digit < cJU_BRANCHUNUMJPS; ++digit)
	    {
		Pjp = (Pjbu->jbu_jp) + digit;
#endif
		if (JPNULL(JU_JPTYPE(Pjp))) continue;

		JU_SETDIGIT(*PIndex, digit, state);
		goto SM3Findlimit;
	    }

// No non-null JPs in BranchU:

	    JU_SET_ERRNO(PJError, JU_ERRNO_CORRUPT);
	    JUDY1CODE(return(JERRI );)
	    JUDYLCODE(return(PPJERR);)


// ----------------------------------------------------------------------------
// LINEAR LEAF:
//
// Simply use the highest/lowest (right/left-most) Index in the LeafL, but the
// details vary depending on leaf Index Size.  First copy Dcd bytes, if there
// are any (only if state < cJU_ROOTSTATE - 1), to *PIndex.

#define	SM3LEAFLDCD(cState)				\
	JU_SETDCD(*PIndex, Pjp, cState);	        \
	SM3LEAFLNODCD

#ifdef JUDY1
#define	SM3LEAFL_SETPOP1		// not needed in any cases.
#else
#define	SM3LEAFL_SETPOP1  pop1 = JU_JPLEAF_POP0(Pjp) + 1
#endif

#ifdef JUDYPREV
#define	SM3LEAFLNODCD			\
	Pjll = P_JLL(Pjp->jp_Addr);	\
	SM3LEAFL_SETPOP1;		\
	offset = JU_JPLEAF_POP0(Pjp); assert(offset >= 0)
#else
#define	SM3LEAFLNODCD			\
	Pjll = P_JLL(Pjp->jp_Addr);	\
	SM3LEAFL_SETPOP1;		\
	offset = 0; assert(JU_JPLEAF_POP0(Pjp) >= 0);
#endif

#if (defined(JUDYL) || (! defined(JU_64BIT)))
	case cJU_JPLEAF1:

	    SM3LEAFLDCD(1);
	    JU_SETDIGIT1(*PIndex, ((uint8_t *) Pjll)[offset]);
	    JU_RET_FOUND_LEAF1(Pjll, pop1, offset);
#endif

	case cJU_JPLEAF2:

	    SM3LEAFLDCD(2);
	    *PIndex = (*PIndex & (~JU_LEASTBYTESMASK(2)))
		    | ((uint16_t *) Pjll)[offset];
	    JU_RET_FOUND_LEAF2(Pjll, pop1, offset);

#ifndef JU_64BIT
	case cJU_JPLEAF3:
	{
	    Word_t lsb;
	    SM3LEAFLNODCD;
	    JU_COPY3_PINDEX_TO_LONG(lsb, ((uint8_t *) Pjll) + (3 * offset));
	    *PIndex = (*PIndex & (~JU_LEASTBYTESMASK(3))) | lsb;
	    JU_RET_FOUND_LEAF3(Pjll, pop1, offset);
	}

#else
	case cJU_JPLEAF3:
	{
	    Word_t lsb;
	    SM3LEAFLDCD(3);
	    JU_COPY3_PINDEX_TO_LONG(lsb, ((uint8_t *) Pjll) + (3 * offset));
	    *PIndex = (*PIndex & (~JU_LEASTBYTESMASK(3))) | lsb;
	    JU_RET_FOUND_LEAF3(Pjll, pop1, offset);
	}

	case cJU_JPLEAF4:

	    SM3LEAFLDCD(4);
	    *PIndex = (*PIndex & (~JU_LEASTBYTESMASK(4)))
		    | ((uint32_t *) Pjll)[offset];
	    JU_RET_FOUND_LEAF4(Pjll, pop1, offset);

	case cJU_JPLEAF5:
	{
	    Word_t lsb;
	    SM3LEAFLDCD(5);
	    JU_COPY5_PINDEX_TO_LONG(lsb, ((uint8_t *) Pjll) + (5 * offset));
	    *PIndex = (*PIndex & (~JU_LEASTBYTESMASK(5))) | lsb;
	    JU_RET_FOUND_LEAF5(Pjll, pop1, offset);
	}

	case cJU_JPLEAF6:
	{
	    Word_t lsb;
	    SM3LEAFLDCD(6);
	    JU_COPY6_PINDEX_TO_LONG(lsb, ((uint8_t *) Pjll) + (6 * offset));
	    *PIndex = (*PIndex & (~JU_LEASTBYTESMASK(6))) | lsb;
	    JU_RET_FOUND_LEAF6(Pjll, pop1, offset);
	}

	case cJU_JPLEAF7:
	{
	    Word_t lsb;
	    SM3LEAFLNODCD;
	    JU_COPY7_PINDEX_TO_LONG(lsb, ((uint8_t *) Pjll) + (7 * offset));
	    *PIndex = (*PIndex & (~JU_LEASTBYTESMASK(7))) | lsb;
	    JU_RET_FOUND_LEAF7(Pjll, pop1, offset);
	}
#endif


// ----------------------------------------------------------------------------
// BITMAP LEAF:
//
// Look for the highest/lowest (right/left-most) non-null subexpanse, then use
// the highest/lowest Index in that subexpanse, but first copy Dcd bytes
// (always present since state 1 < cJU_ROOTSTATE) to *PIndex.

	case cJU_JPLEAF_B1:
	{
	    Pjlb_t Pjlb;

	    JU_SETDCD(*PIndex, Pjp, 1);

	    Pjlb   = P_JLB(Pjp->jp_Addr);
#ifdef JUDYPREV
	    subexp = cJU_NUMSUBEXPL;

	    while (! JU_JLB_BITMAP(Pjlb, --subexp))  // find non-empty subexp.
	    {
		if (subexp <= 0)		// wholly empty bitmap.
		{
		    JU_SET_ERRNO(PJError, JU_ERRNO_CORRUPT);
		    JUDY1CODE(return(JERRI );)
		    JUDYLCODE(return(PPJERR);)
		}
	    }

// TBD:  Might it be faster to just use a variant of BITMAPDIGIT*() that yields
// the digit for the right-most Index with a bit set?

	    offset = SEARCHBITMAPMAXL(JU_JLB_BITMAP(Pjlb, subexp));
	    // expected range:
	    assert((offset >= 0) && (offset < cJU_BITSPERSUBEXPL));
#else
	    subexp = -1;

	    while (! JU_JLB_BITMAP(Pjlb, ++subexp))  // find non-empty subexp.
	    {
		if (subexp >= cJU_NUMSUBEXPL - 1)    // didnt find one.
		{
		    JU_SET_ERRNO(PJError, JU_ERRNO_CORRUPT);
		    JUDY1CODE(return(JERRI );)
		    JUDYLCODE(return(PPJERR);)
		}
	    }

	    offset = 0;
#endif

	    JU_BITMAPDIGITL(digit, subexp, JU_JLB_BITMAP(Pjlb, subexp), offset);
	    JU_SETDIGIT1(*PIndex, digit);
	    JU_RET_FOUND_LEAF_B1(Pjlb, subexp, offset);
//	== return((PPvoid_t) (P_JV(JL_JLB_PVALUE(Pjlb, subexp)) + (offset)));

	} // case cJU_JPLEAF_B1

#ifdef JUDY1
// ----------------------------------------------------------------------------
// FULL POPULATION:
//
// Copy Dcd bytes to *PIndex (always present since state 1 < cJU_ROOTSTATE),
// then set the highest/lowest possible digit as the LSB in *PIndex.

	case cJ1_JPFULLPOPU1:

	    JU_SETDCD(   *PIndex, Pjp, 1);
#ifdef JUDYPREV
	    JU_SETDIGIT1(*PIndex, cJU_BITSPERBITMAP - 1);
#else
	    JU_SETDIGIT1(*PIndex, 0);
#endif
	    JU_RET_FOUND_FULLPOPU1;
#endif // JUDY1


// ----------------------------------------------------------------------------
// IMMEDIATE:
//
// Simply use the highest/lowest (right/left-most) Index in the Imm, but the
// details vary depending on leaf Index Size and pop1.  Note:  There are no Dcd
// bytes in an Immediate JP, but in a cJU_JPIMMED_*_01 JP, the field holds the
// least bytes of the immediate Index.

	case cJU_JPIMMED_1_01: SET_01(1); goto SM3Imm_01;
	case cJU_JPIMMED_2_01: SET_01(2); goto SM3Imm_01;
	case cJU_JPIMMED_3_01: SET_01(3); goto SM3Imm_01;
#ifdef JU_64BIT
	case cJU_JPIMMED_4_01: SET_01(4); goto SM3Imm_01;
	case cJU_JPIMMED_5_01: SET_01(5); goto SM3Imm_01;
	case cJU_JPIMMED_6_01: SET_01(6); goto SM3Imm_01;
	case cJU_JPIMMED_7_01: SET_01(7); goto SM3Imm_01;
#endif
SM3Imm_01:	JU_RET_FOUND_IMM_01(Pjp);

#ifdef JUDYPREV
#define	SM3IMM_OFFSET(cPop1)  (cPop1) - 1	// highest.
#else
#define	SM3IMM_OFFSET(cPop1)  0			// lowest.
#endif

#define	SM3IMM(cPop1,Next)		\
	offset = SM3IMM_OFFSET(cPop1);	\
	goto Next

	case cJU_JPIMMED_1_02: SM3IMM( 2, SM3Imm1);
	case cJU_JPIMMED_1_03: SM3IMM( 3, SM3Imm1);
#if (defined(JUDY1) || defined(JU_64BIT))
	case cJU_JPIMMED_1_04: SM3IMM( 4, SM3Imm1);
	case cJU_JPIMMED_1_05: SM3IMM( 5, SM3Imm1);
	case cJU_JPIMMED_1_06: SM3IMM( 6, SM3Imm1);
	case cJU_JPIMMED_1_07: SM3IMM( 7, SM3Imm1);
#endif
#if (defined(JUDY1) && defined(JU_64BIT))
	case cJ1_JPIMMED_1_08: SM3IMM( 8, SM3Imm1);
	case cJ1_JPIMMED_1_09: SM3IMM( 9, SM3Imm1);
	case cJ1_JPIMMED_1_10: SM3IMM(10, SM3Imm1);
	case cJ1_JPIMMED_1_11: SM3IMM(11, SM3Imm1);
	case cJ1_JPIMMED_1_12: SM3IMM(12, SM3Imm1);
	case cJ1_JPIMMED_1_13: SM3IMM(13, SM3Imm1);
	case cJ1_JPIMMED_1_14: SM3IMM(14, SM3Imm1);
	case cJ1_JPIMMED_1_15: SM3IMM(15, SM3Imm1);
#endif

SM3Imm1:    JU_SETDIGIT1(*PIndex, ((uint8_t *) PJI)[offset]);
	    JU_RET_FOUND_IMM(Pjp, offset);

#if (defined(JUDY1) || defined(JU_64BIT))
	case cJU_JPIMMED_2_02: SM3IMM(2, SM3Imm2);
	case cJU_JPIMMED_2_03: SM3IMM(3, SM3Imm2);
#endif
#if (defined(JUDY1) && defined(JU_64BIT))
	case cJ1_JPIMMED_2_04: SM3IMM(4, SM3Imm2);
	case cJ1_JPIMMED_2_05: SM3IMM(5, SM3Imm2);
	case cJ1_JPIMMED_2_06: SM3IMM(6, SM3Imm2);
	case cJ1_JPIMMED_2_07: SM3IMM(7, SM3Imm2);
#endif

#if (defined(JUDY1) || defined(JU_64BIT))
SM3Imm2:    *PIndex = (*PIndex & (~JU_LEASTBYTESMASK(2)))
		    | ((uint16_t *) PJI)[offset];
	    JU_RET_FOUND_IMM(Pjp, offset);
#endif

#if (defined(JUDY1) || defined(JU_64BIT))
	case cJU_JPIMMED_3_02: SM3IMM(2, SM3Imm3);
#endif
#if (defined(JUDY1) && defined(JU_64BIT))
	case cJ1_JPIMMED_3_03: SM3IMM(3, SM3Imm3);
	case cJ1_JPIMMED_3_04: SM3IMM(4, SM3Imm3);
	case cJ1_JPIMMED_3_05: SM3IMM(5, SM3Imm3);
#endif

#if (defined(JUDY1) || defined(JU_64BIT))
SM3Imm3:
	{
	    Word_t lsb;
	    JU_COPY3_PINDEX_TO_LONG(lsb, ((uint8_t *) PJI) + (3 * offset));
	    *PIndex = (*PIndex & (~JU_LEASTBYTESMASK(3))) | lsb;
	    JU_RET_FOUND_IMM(Pjp, offset);
	}
#endif

#if (defined(JUDY1) && defined(JU_64BIT))
	case cJ1_JPIMMED_4_02: SM3IMM(2, SM3Imm4);
	case cJ1_JPIMMED_4_03: SM3IMM(3, SM3Imm4);

SM3Imm4:    *PIndex = (*PIndex & (~JU_LEASTBYTESMASK(4)))
		    | ((uint32_t *) PJI)[offset];
	    JU_RET_FOUND_IMM(Pjp, offset);

	case cJ1_JPIMMED_5_02: SM3IMM(2, SM3Imm5);
	case cJ1_JPIMMED_5_03: SM3IMM(3, SM3Imm5);

SM3Imm5:
	{
	    Word_t lsb;
	    JU_COPY5_PINDEX_TO_LONG(lsb, ((uint8_t *) PJI) + (5 * offset));
	    *PIndex = (*PIndex & (~JU_LEASTBYTESMASK(5))) | lsb;
	    JU_RET_FOUND_IMM(Pjp, offset);
	}

	case cJ1_JPIMMED_6_02: SM3IMM(2, SM3Imm6);

SM3Imm6:
	{
	    Word_t lsb;
	    JU_COPY6_PINDEX_TO_LONG(lsb, ((uint8_t *) PJI) + (6 * offset));
	    *PIndex = (*PIndex & (~JU_LEASTBYTESMASK(6))) | lsb;
	    JU_RET_FOUND_IMM(Pjp, offset);
	}

	case cJ1_JPIMMED_7_02: SM3IMM(2, SM3Imm7);

SM3Imm7:
	{
	    Word_t lsb;
	    JU_COPY7_PINDEX_TO_LONG(lsb, ((uint8_t *) PJI) + (7 * offset));
	    *PIndex = (*PIndex & (~JU_LEASTBYTESMASK(7))) | lsb;
	    JU_RET_FOUND_IMM(Pjp, offset);
	}
#endif // (JUDY1 && JU_64BIT)


// ----------------------------------------------------------------------------
// OTHER CASES:

	default: JU_SET_ERRNO(PJError, JU_ERRNO_CORRUPT);
		 JUDY1CODE(return(JERRI );)
		 JUDYLCODE(return(PPJERR);)

	} // SM3Findlimit switch.

	/*NOTREACHED*/

} // Judy1Prev() / Judy1Next() / JudyLPrev() / JudyLNext()
