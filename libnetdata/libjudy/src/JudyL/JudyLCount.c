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

// @(#) $Revision: 4.78 $ $Source: /judy/src/JudyCommon/JudyCount.c $
//
// Judy*Count() function for Judy1 and JudyL.
// Compile with one of -DJUDY1 or -DJUDYL.
//
// Compile with -DNOSMARTJBB, -DNOSMARTJBU, and/or -DNOSMARTJLB to build a
// version with cache line optimizations deleted, for testing.
//
// Compile with -DSMARTMETRICS to obtain global variables containing smart
// cache line metrics.  Note:  Dont turn this on simultaneously for this file
// and JudyByCount.c because they export the same globals.
//
// Judy*Count() returns the "count of Indexes" (inclusive) between the two
// specified limits (Indexes).  This code is remarkably fast.  It traverses the
// "Judy array" data structure.
//
// This count code is the GENERIC untuned version (minimum code size).  It
// might be possible to tuned to a specific architecture to be faster.
// However, in real applications, with a modern machine, it is expected that
// the instruction times will be swamped by cache line fills.
// ****************************************************************************

#if (! (defined(JUDY1) || defined(JUDYL)))
#error:  One of -DJUDY1 or -DJUDYL must be specified.
#endif

#ifdef JUDY1
#include "Judy1.h"
#else
#include "JudyL.h"
#endif

#include "JudyPrivate1L.h"


// define a phoney that is for sure

#define cJU_LEAFW       cJU_JPIMMED_CAP

// Avoid duplicate symbols since this file is multi-compiled:

#ifdef SMARTMETRICS
#ifdef JUDY1
Word_t jbb_upward   = 0;	// counts of directions taken:
Word_t jbb_downward = 0;
Word_t jbu_upward   = 0;
Word_t jbu_downward = 0;
Word_t jlb_upward   = 0;
Word_t jlb_downward = 0;
#else
extern Word_t jbb_upward;
extern Word_t jbb_downward;
extern Word_t jbu_upward;
extern Word_t jbu_downward;
extern Word_t jlb_upward;
extern Word_t jlb_downward;
#endif
#endif


// FORWARD DECLARATIONS (prototypes):

static	Word_t j__udy1LCountSM(const Pjp_t Pjp, const Word_t Index,
			       const Pjpm_t Pjpm);

// Each of Judy1 and JudyL get their own private (static) version of this
// function:

static	int j__udyCountLeafB1(const Pjll_t Pjll, const Word_t Pop1,
			      const Word_t Index);

// These functions are not static because they are exported to Judy*ByCount():
//
// TBD:  Should be made static for performance reasons?  And thus duplicated?
//
// Note:  There really are two different functions, but for convenience they
// are referred to here with a generic name.

#ifdef JUDY1
#define	j__udyJPPop1 j__udy1JPPop1
#else
#define	j__udyJPPop1 j__udyLJPPop1
#endif

Word_t j__udyJPPop1(const Pjp_t Pjp);


// LOCAL ERROR HANDLING:
//
// The Judy*Count() functions are unusual because they return 0 instead of JERR
// for an error.  In this source file, define C_JERR for clarity.

#define	C_JERR 0


// ****************************************************************************
// J U D Y   1   C O U N T
// J U D Y   L   C O U N T
//
// See the manual entry for details.
//
// This code is written recursively, at least at first, because thats much
// simpler; hope its fast enough.

#ifdef JUDY1
FUNCTION Word_t Judy1Count
#else
FUNCTION Word_t JudyLCount
#endif
        (
	Pcvoid_t  PArray,	// JRP to first branch/leaf in SM.
	Word_t	  Index1,	// starting Index.
	Word_t	  Index2,	// ending Index.
	PJError_t PJError	// optional, for returning error info.
        )
{
	jpm_t	  fakejpm;	// local temporary for small arrays.
	Pjpm_t	  Pjpm;		// top JPM or local temporary for error info.
	jp_t	  fakejp;	// constructed for calling j__udy1LCountSM().
	Pjp_t	  Pjp;		// JP to pass to j__udy1LCountSM().
	Word_t	  pop1;		// total for the array.
	Word_t	  pop1above1;	// indexes at or above Index1, inclusive.
	Word_t	  pop1above2;	// indexes at or above Index2, exclusive.
	int	  retcode;	// from Judy*First() calls.
JUDYLCODE(PPvoid_t PPvalue);	// from JudyLFirst() calls.


// CHECK FOR SHORTCUTS:
//
// As documented, return C_JERR if the Judy array is empty or Index1 > Index2.

	if ((PArray == (Pvoid_t) NULL) || (Index1 > Index2))
	{
	    JU_SET_ERRNO(PJError, JU_ERRNO_NONE);
	    return(C_JERR);
	}

// If Index1 == Index2, simply check if the specified Index is set; pass
// through the return value from Judy1Test() or JudyLGet() with appropriate
// translations.

	if (Index1 == Index2)
	{
#ifdef JUDY1
	    retcode = Judy1Test(PArray, Index1, PJError);

	    if (retcode == JERRI) return(C_JERR);	// pass through error.

	    if (retcode == 0)
	    {
		JU_SET_ERRNO(PJError, JU_ERRNO_NONE);
		return(C_JERR);
	    }
#else
	    PPvalue = JudyLGet(PArray, Index1, PJError);

	    if (PPvalue == PPJERR) return(C_JERR);	// pass through error.

	    if (PPvalue == (PPvoid_t) NULL)		// Index is not set.
	    {
		JU_SET_ERRNO(PJError, JU_ERRNO_NONE);
		return(C_JERR);
	    }
#endif
	    return(1);					// single index is set.
	}


// CHECK JRP TYPE:
//
// Use an if/then for speed rather than a switch, and put the most common cases
// first.
//
// Note:  Since even cJU_LEAFW types require counting between two Indexes,
// prepare them here for common code below that calls j__udy1LCountSM(), rather
// than handling them even more specially here.

	if (JU_LEAFW_POP0(PArray) < cJU_LEAFW_MAXPOP1) // must be a LEAFW
	{
	    Pjlw_t Pjlw	   = P_JLW(PArray);	// first word of leaf.
	    Pjpm	   = & fakejpm;
	    Pjp		   = & fakejp;
	    Pjp->jp_Addr   = (Word_t) Pjlw;
	    Pjp->jp_Type   = cJU_LEAFW;
	    Pjpm->jpm_Pop0 = Pjlw[0];		// from first word of leaf.
	    pop1	   = Pjpm->jpm_Pop0 + 1;
	}
	else
	{
	    Pjpm = P_JPM(PArray);
	    Pjp	 = &(Pjpm->jpm_JP);
	    pop1 = (Pjpm->jpm_Pop0) + 1;	// note: can roll over to 0.

#if (defined(JUDY1) && (! defined(JU_64BIT)))
	    if (pop1 == 0)		// rare special case of full array:
	    {
		Word_t count = Index2 - Index1 + 1;	// can roll over again.

		if (count == 0)
		{
		    JU_SET_ERRNO(PJError, JU_ERRNO_FULL);
		    return(C_JERR);
		}
		return(count);
	    }
#else
	    assert(pop1);	// JudyL or 64-bit cannot create a full array!
#endif
	}


// COUNT POP1 ABOVE INDEX1, INCLUSIVE:

	assert(pop1);		// just to be safe.

	if (Index1 == 0)	// shortcut, pop1above1 is entire population:
	{
	    pop1above1 = pop1;
	}
	else			// find first valid Index above Index1, if any:
	{
#ifdef JUDY1
	    if ((retcode = Judy1First(PArray, & Index1, PJError)) == JERRI)
		return(C_JERR);			// pass through error.
#else
	    if ((PPvalue = JudyLFirst(PArray, & Index1, PJError)) == PPJERR)
		return(C_JERR);			// pass through error.

	    retcode = (PPvalue != (PPvoid_t) NULL);	// found a next Index.
#endif

// If theres no Index at or above Index1, just return C_JERR (early exit):

	    if (retcode == 0)
	    {
		JU_SET_ERRNO(PJError, JU_ERRNO_NONE);
		return(C_JERR);
	    }

// If a first/next Index was found, call the counting motor starting with that
// known valid Index, meaning the return should be positive, not C_JERR except
// in case of a real error:

	    if ((pop1above1 = j__udy1LCountSM(Pjp, Index1, Pjpm)) == C_JERR)
	    {
		JU_COPY_ERRNO(PJError, Pjpm);	// pass through error.
		return(C_JERR);
	    }
	}


// COUNT POP1 ABOVE INDEX2, EXCLUSIVE, AND RETURN THE DIFFERENCE:
//
// In principle, calculate the ordinal of each Index and take the difference,
// with caution about off-by-one errors due to the specified Indexes being set
// or unset.  In practice:
//
// - The ordinals computed here are inverse ordinals, that is, the populations
//   ABOVE the specified Indexes (Index1 inclusive, Index2 exclusive), so
//   subtract pop1above2 from pop1above1, rather than vice-versa.
//
// - Index1s result already includes a count for Index1 and/or Index2 if
//   either is set, so calculate pop1above2 exclusive of Index2.
//
// TBD:  If Index1 and Index2 fall in the same expanse in the top-state
// branch(es), would it be faster to walk the SM only once, to their divergence
// point, before calling j__udy1LCountSM() or equivalent?  Possibly a non-issue
// if a top-state pop1 becomes stored with each Judy1 array.  Also, consider
// whether the first call of j__udy1LCountSM() fills the cache, for common tree
// branches, for the second call.
//
// As for pop1above1, look for shortcuts for special cases when pop1above2 is
// zero.  Otherwise call the counting "motor".

	    assert(pop1above1);		// just to be safe.

	    if (Index2++ == cJU_ALLONES) return(pop1above1); // Index2 at limit.

#ifdef JUDY1
	    if ((retcode = Judy1First(PArray, & Index2, PJError)) == JERRI)
		return(C_JERR);
#else
	    if ((PPvalue = JudyLFirst(PArray, & Index2, PJError)) == PPJERR)
		return(C_JERR);

	    retcode = (PPvalue != (PPvoid_t) NULL);	// found a next Index.
#endif
	    if (retcode == 0) return(pop1above1);  // no Index above Index2.

// Just as for Index1, j__udy1LCountSM() cannot return 0 (locally == C_JERR)
// except in case of a real error:

	    if ((pop1above2 = j__udy1LCountSM(Pjp, Index2, Pjpm)) == C_JERR)
	    {
		JU_COPY_ERRNO(PJError, Pjpm);		// pass through error.
		return(C_JERR);
	    }

	    if (pop1above1 == pop1above2)
	    {
		JU_SET_ERRNO(PJError, JU_ERRNO_NONE);
		return(C_JERR);
	    }

	    return(pop1above1 - pop1above2);

} // Judy1Count() / JudyLCount()


// ****************************************************************************
// __ J U D Y 1 L   C O U N T   S M
//
// Given a pointer to a JP (with invalid jp_DcdPopO at cJU_ROOTSTATE), a known
// valid Index, and a Pjpm for returning error info, recursively visit a Judy
// array state machine (SM) and return the count of Indexes, including Index,
// through the end of the Judy array at this state or below.  In case of error
// or a count of 0 (should never happen), return C_JERR with appropriate
// JU_ERRNO in the Pjpm.
//
// Note:  This function is not told the current state because its encoded in
// the JP Type.
//
// Method:  To minimize cache line fills, while studying each branch, if Index
// resides above the midpoint of the branch (which often consists of multiple
// cache lines), ADD the populations at or above Index; otherwise, SUBTRACT
// from the population of the WHOLE branch (available from the JP) the
// populations at or above Index.  This is especially tricky for bitmap
// branches.
//
// Note:  Unlike, say, the Ins and Del walk routines, this function returns the
// same type of returns as Judy*Count(), so it can use *_SET_ERRNO*() macros
// the same way.

FUNCTION static Word_t j__udy1LCountSM(
const	Pjp_t	Pjp,		// top of Judy (sub)SM.
const	Word_t	Index,		// count at or above this Index.
const	Pjpm_t	Pjpm)		// for returning error info.
{
	Pjbl_t	Pjbl;		// Pjp->jp_Addr masked and cast to types:
	Pjbb_t	Pjbb;
	Pjbu_t	Pjbu;
	Pjll_t	Pjll;		// a Judy lower-level linear leaf.

	Word_t	digit;		// next digit to decode from Index.
	long	jpnum;		// JP number in a branch (base 0).
	int	offset;		// index ordinal within a leaf, base 0.
	Word_t	pop1;		// total population of an expanse.
	Word_t	pop1above;	// to return.

// Common code to check Decode bits in a JP against the equivalent portion of
// Index; XOR together, then mask bits of interest; must be all 0:
//
// Note:  Why does this code only assert() compliance rather than actively
// checking for outliers?  Its because Index is supposed to be valid, hence
// always match any Dcd bits traversed.
//
// Note:  This assertion turns out to be always true for cState = 3 on 32-bit
// and 7 on 64-bit, but its harmless, probably removed by the compiler.

#define	CHECKDCD(Pjp,cState) \
	assert(! JU_DCDNOTMATCHINDEX(Index, Pjp, cState))

// Common code to prepare to handle a root-level or lower-level branch:
// Extract a state-dependent digit from Index in a "constant" way, obtain the
// total population for the branch in a state-dependent way, and then branch to
// common code for multiple cases:
//
// For root-level branches, the state is always cJU_ROOTSTATE, and the
// population is received in Pjpm->jpm_Pop0.
//
// Note:  The total population is only needed in cases where the common code
// "counts up" instead of down to minimize cache line fills.  However, its
// available cheaply, and its better to do it with a constant shift (constant
// state value) instead of a variable shift later "when needed".

#define	PREPB_ROOT(Pjp,Next)				\
	digit = JU_DIGITATSTATE(Index, cJU_ROOTSTATE);	\
	pop1  = (Pjpm->jpm_Pop0) + 1;			\
	goto Next

#define	PREPB(Pjp,cState,Next)				\
	digit = JU_DIGITATSTATE(Index, cState);		\
	pop1  = JU_JPBRANCH_POP0(Pjp, (cState)) + 1;    \
	goto Next


// SWITCH ON JP TYPE:
//
// WARNING:  For run-time efficiency the following cases replicate code with
// varying constants, rather than using common code with variable values!

	switch (JU_JPTYPE(Pjp))
	{


// ----------------------------------------------------------------------------
// ROOT-STATE LEAF that starts with a Pop0 word; just count within the leaf:

	case cJU_LEAFW:
	{
	    Pjlw_t Pjlw = P_JLW(Pjp->jp_Addr);		// first word of leaf.

	    assert((Pjpm->jpm_Pop0) + 1 == Pjlw[0] + 1);  // sent correctly.
	    offset = j__udySearchLeafW(Pjlw + 1, Pjpm->jpm_Pop0 + 1, Index);
	    assert(offset >= 0);			// Index must exist.
	    assert(offset < (Pjpm->jpm_Pop0) + 1);	// Index be in range.
	    return((Pjpm->jpm_Pop0) + 1 - offset);	// INCLUSIVE of Index.
	}

// ----------------------------------------------------------------------------
// LINEAR BRANCH; count populations in JPs in the JBL ABOVE the next digit in
// Index, and recurse for the next digit in Index:
//
// Note:  There are no null JPs in a JBL; watch out for pop1 == 0.
//
// Note:  A JBL should always fit in one cache line => no need to count up
// versus down to save cache line fills.  (PREPB() sets pop1 for no reason.)

	case cJU_JPBRANCH_L2:  CHECKDCD(Pjp, 2); PREPB(Pjp, 2, BranchL);
	case cJU_JPBRANCH_L3:  CHECKDCD(Pjp, 3); PREPB(Pjp, 3, BranchL);

#ifdef JU_64BIT
	case cJU_JPBRANCH_L4:  CHECKDCD(Pjp, 4); PREPB(Pjp, 4, BranchL);
	case cJU_JPBRANCH_L5:  CHECKDCD(Pjp, 5); PREPB(Pjp, 5, BranchL);
	case cJU_JPBRANCH_L6:  CHECKDCD(Pjp, 6); PREPB(Pjp, 6, BranchL);
	case cJU_JPBRANCH_L7:  CHECKDCD(Pjp, 7); PREPB(Pjp, 7, BranchL);
#endif
	case cJU_JPBRANCH_L:   PREPB_ROOT(Pjp, BranchL);

// Common code (state-independent) for all cases of linear branches:

BranchL:

	Pjbl      = P_JBL(Pjp->jp_Addr);
	jpnum     = Pjbl->jbl_NumJPs;			// above last JP.
	pop1above = 0;

	while (digit < (Pjbl->jbl_Expanse[--jpnum]))	 // still ABOVE digit.
	{
	    if ((pop1 = j__udyJPPop1((Pjbl->jbl_jp) + jpnum)) == cJU_ALLONES)
	    {
		JU_SET_ERRNO_NONNULL(Pjpm, JU_ERRNO_CORRUPT);
		return(C_JERR);
	    }

	    pop1above += pop1;
	    assert(jpnum > 0);				// should find digit.
	}

	assert(digit == (Pjbl->jbl_Expanse[jpnum]));	// should find digit.

	pop1 = j__udy1LCountSM((Pjbl->jbl_jp) + jpnum, Index, Pjpm);
	if (pop1 == C_JERR) return(C_JERR);		// pass error up.

	assert(pop1above + pop1);
	return(pop1above + pop1);


// ----------------------------------------------------------------------------
// BITMAP BRANCH; count populations in JPs in the JBB ABOVE the next digit in
// Index, and recurse for the next digit in Index:
//
// Note:  There are no null JPs in a JBB; watch out for pop1 == 0.

	case cJU_JPBRANCH_B2:  CHECKDCD(Pjp, 2); PREPB(Pjp, 2, BranchB);
	case cJU_JPBRANCH_B3:  CHECKDCD(Pjp, 3); PREPB(Pjp, 3, BranchB);
#ifdef JU_64BIT
	case cJU_JPBRANCH_B4:  CHECKDCD(Pjp, 4); PREPB(Pjp, 4, BranchB);
	case cJU_JPBRANCH_B5:  CHECKDCD(Pjp, 5); PREPB(Pjp, 5, BranchB);
	case cJU_JPBRANCH_B6:  CHECKDCD(Pjp, 6); PREPB(Pjp, 6, BranchB);
	case cJU_JPBRANCH_B7:  CHECKDCD(Pjp, 7); PREPB(Pjp, 7, BranchB);
#endif
	case cJU_JPBRANCH_B:   PREPB_ROOT(Pjp, BranchB);

// Common code (state-independent) for all cases of bitmap branches:

BranchB:
	{
	    long   subexp;	// for stepping through layer 1 (subexpanses).
	    long   findsub;	// subexpanse containing   Index (digit).
	    Word_t findbit;	// bit	      representing Index (digit).
	    Word_t lowermask;	// bits for indexes at or below Index.
	    Word_t jpcount;	// JPs in a subexpanse.
	    Word_t clbelow;	// cache lines below digits cache line.
	    Word_t clabove;	// cache lines above digits cache line.

	    Pjbb      = P_JBB(Pjp->jp_Addr);
	    findsub   = digit / cJU_BITSPERSUBEXPB;
	    findbit   = digit % cJU_BITSPERSUBEXPB;
	    lowermask = JU_MASKLOWERINC(JU_BITPOSMASKB(findbit));
	    clbelow   = clabove = 0;	// initial/default => always downward.

	    assert(JU_BITMAPTESTB(Pjbb, digit)); // digit must have a JP.
	    assert(findsub < cJU_NUMSUBEXPB);	 // falls in expected range.

// Shorthand for one subexpanse in a bitmap and for one JP in a bitmap branch:
//
// Note: BMPJP0 exists separately to support assertions.

#define	BMPJP0(Subexp)       (P_JP(JU_JBB_PJP(Pjbb, Subexp)))
#define	BMPJP(Subexp,JPnum)  (BMPJP0(Subexp) + (JPnum))

#ifndef NOSMARTJBB  // enable to turn off smart code for comparison purposes.

// FIGURE OUT WHICH DIRECTION CAUSES FEWER CACHE LINE FILLS; adding the pop1s
// in JPs above Indexs JP, or subtracting the pop1s in JPs below Indexs JP.
//
// This is tricky because, while each set bit in the bitmap represents a JP,
// the JPs are scattered over cJU_NUMSUBEXPB subexpanses, each of which can
// contain JPs packed into multiple cache lines, and this code must visit every
// JP either BELOW or ABOVE the JP for Index.
//
// Number of cache lines required to hold a linear list of the given number of
// JPs, assuming the first JP is at the start of a cache line or the JPs in
// jpcount fit wholly within a single cache line, which is ensured by
// JudyMalloc():

#define	CLPERJPS(jpcount) \
	((((jpcount) * cJU_WORDSPERJP) + cJU_WORDSPERCL - 1) / cJU_WORDSPERCL)

// Count cache lines below/above for each subexpanse:

	    for (subexp = 0; subexp < cJU_NUMSUBEXPB; ++subexp)
	    {
		jpcount = j__udyCountBitsB(JU_JBB_BITMAP(Pjbb, subexp));

// When at the subexpanse containing Index (digit), add cache lines
// below/above appropriately, excluding the cache line containing the JP for
// Index itself:

		if	(subexp <  findsub)  clbelow += CLPERJPS(jpcount);
		else if (subexp >  findsub)  clabove += CLPERJPS(jpcount);
		else // (subexp == findsub)
		{
		    Word_t clfind;	// cache line containing Index (digit).

		    clfind = CLPERJPS(j__udyCountBitsB(
				    JU_JBB_BITMAP(Pjbb, subexp) & lowermask));

		    assert(clfind > 0);	 // digit itself should have 1 CL.
		    clbelow += clfind - 1;
		    clabove += CLPERJPS(jpcount) - clfind;
		}
	    }
#endif // ! NOSMARTJBB

// Note:  Its impossible to get through the following "if" without setting
// jpnum -- see some of the assertions below -- but gcc -Wall doesnt know
// this, so preset jpnum to make it happy:

	    jpnum = 0;


// COUNT POPULATION FOR A BITMAP BRANCH, in whichever direction should result
// in fewer cache line fills:
//
// Note:  If the remainder of Index is zero, pop1above is the pop1 of the
// entire expanse and theres no point in recursing to lower levels; but this
// should be so rare that its not worth checking for;
// Judy1Count()/JudyLCount() never even calls the motor for Index == 0 (all
// bytes).


// COUNT UPWARD, subtracting each "below or at" JPs pop1 from the whole
// expanses pop1:
//
// Note:  If this causes clbelow + 1 cache line fills including JPs cache
// line, thats OK; at worst this is the same as clabove.

	    if (clbelow < clabove)
	    {
#ifdef SMARTMETRICS
		++jbb_upward;
#endif
		pop1above = pop1;		// subtract JPs at/below Index.

// Count JPs for which to accrue pop1s in this subexpanse:
//
// TBD:  If JU_JBB_BITMAP is cJU_FULLBITMAPB, dont bother counting.

		for (subexp = 0; subexp <= findsub; ++subexp)
		{
		    jpcount = j__udyCountBitsB((subexp < findsub) ?
				      JU_JBB_BITMAP(Pjbb, subexp) :
				      JU_JBB_BITMAP(Pjbb, subexp) & lowermask);

		    // should always find findbit:
		    assert((subexp < findsub) || jpcount);

// Subtract pop1s from JPs BELOW OR AT Index (digit):
//
// Note:  The pop1 for Indexs JP itself is partially added back later at a
// lower state.
//
// Note:  An empty subexpanse (jpcount == 0) is handled "for free".
//
// Note:  Must be null JP subexp pointer in empty subexpanse and non-empty in
// non-empty subexpanse:

		    assert(   jpcount  || (BMPJP0(subexp) == (Pjp_t) NULL));
		    assert((! jpcount) || (BMPJP0(subexp) != (Pjp_t) NULL));

		    for (jpnum = 0; jpnum < jpcount; ++jpnum)
		    {
			if ((pop1 = j__udyJPPop1(BMPJP(subexp, jpnum)))
			    == cJU_ALLONES)
			{
			    JU_SET_ERRNO_NONNULL(Pjpm, JU_ERRNO_CORRUPT);
			    return(C_JERR);
			}

			pop1above -= pop1;
		    }

		    jpnum = jpcount - 1;	// make correct for digit.
		}
	    }

// COUNT DOWNWARD, adding each "above" JPs pop1:

	    else
	    {
		long jpcountbf;			// below findbit, inclusive.
#ifdef SMARTMETRICS
		++jbb_downward;
#endif
		pop1above = 0;			// add JPs above Index.
		jpcountbf = 0;			// until subexp == findsub.

// Count JPs for which to accrue pop1s in this subexpanse:
//
// This is more complicated than counting upward because the scan of digits
// subexpanse must count ALL JPs, to know where to START counting down, and
// ALSO note the offset of digits JP to know where to STOP counting down.

		for (subexp = cJU_NUMSUBEXPB - 1; subexp >= findsub; --subexp)
		{
		    jpcount = j__udyCountBitsB(JU_JBB_BITMAP(Pjbb, subexp));

		    // should always find findbit:
		    assert((subexp > findsub) || jpcount);

		    if (! jpcount) continue;	// empty subexpanse, save time.

// Count JPs below digit, inclusive:

		    if (subexp == findsub)
		    {
			jpcountbf = j__udyCountBitsB(JU_JBB_BITMAP(Pjbb, subexp)
						  & lowermask);
		    }

		    // should always find findbit:
		    assert((subexp > findsub) || jpcountbf);
		    assert(jpcount >= jpcountbf);	// proper relationship.

// Add pop1s from JPs ABOVE Index (digit):

		    // no null JP subexp pointers:
		    assert(BMPJP0(subexp) != (Pjp_t) NULL);

		    for (jpnum = jpcount - 1; jpnum >= jpcountbf; --jpnum)
		    {
			if ((pop1 = j__udyJPPop1(BMPJP(subexp, jpnum)))
			    == cJU_ALLONES)
			{
			    JU_SET_ERRNO_NONNULL(Pjpm, JU_ERRNO_CORRUPT);
			    return(C_JERR);
			}

			pop1above += pop1;
		    }
		    // jpnum is now correct for digit.
		}
	    } // else.

// Return the net population ABOVE the digits JP at this state (in this JBB)
// plus the population AT OR ABOVE Index in the SM under the digits JP:

	    pop1 = j__udy1LCountSM(BMPJP(findsub, jpnum), Index, Pjpm);
	    if (pop1 == C_JERR) return(C_JERR);		// pass error up.

	    assert(pop1above + pop1);
	    return(pop1above + pop1);

	} // case.


// ----------------------------------------------------------------------------
// UNCOMPRESSED BRANCH; count populations in JPs in the JBU ABOVE the next
// digit in Index, and recurse for the next digit in Index:
//
// Note:  If the remainder of Index is zero, pop1above is the pop1 of the
// entire expanse and theres no point in recursing to lower levels; but this
// should be so rare that its not worth checking for;
// Judy1Count()/JudyLCount() never even calls the motor for Index == 0 (all
// bytes).

	case cJU_JPBRANCH_U2:  CHECKDCD(Pjp, 2); PREPB(Pjp, 2, BranchU);
	case cJU_JPBRANCH_U3:  CHECKDCD(Pjp, 3); PREPB(Pjp, 3, BranchU);
#ifdef JU_64BIT
	case cJU_JPBRANCH_U4:  CHECKDCD(Pjp, 4); PREPB(Pjp, 4, BranchU);
	case cJU_JPBRANCH_U5:  CHECKDCD(Pjp, 5); PREPB(Pjp, 5, BranchU);
	case cJU_JPBRANCH_U6:  CHECKDCD(Pjp, 6); PREPB(Pjp, 6, BranchU);
	case cJU_JPBRANCH_U7:  CHECKDCD(Pjp, 7); PREPB(Pjp, 7, BranchU);
#endif
	case cJU_JPBRANCH_U:   PREPB_ROOT(Pjp, BranchU);

// Common code (state-independent) for all cases of uncompressed branches:

BranchU:
	    Pjbu = P_JBU(Pjp->jp_Addr);

#ifndef NOSMARTJBU  // enable to turn off smart code for comparison purposes.

// FIGURE OUT WHICH WAY CAUSES FEWER CACHE LINE FILLS; adding the JPs above
// Indexs JP, or subtracting the JPs below Indexs JP.
//
// COUNT UPWARD, subtracting the pop1 of each JP BELOW OR AT Index, from the
// whole expanses pop1:

	    if (digit < (cJU_BRANCHUNUMJPS / 2))
	    {
		pop1above = pop1;		// subtract JPs below Index.
#ifdef SMARTMETRICS
		++jbu_upward;
#endif
		for (jpnum = 0; jpnum <= digit; ++jpnum)
		{
		    if ((Pjbu->jbu_jp[jpnum].jp_Type) <= cJU_JPNULLMAX)
			continue;	// shortcut, save a function call.

		    if ((pop1 = j__udyJPPop1(Pjbu->jbu_jp + jpnum))
		     == cJU_ALLONES)
		    {
			JU_SET_ERRNO_NONNULL(Pjpm, JU_ERRNO_CORRUPT);
			return(C_JERR);
		    }

		    pop1above -= pop1;
		}
	    }

// COUNT DOWNWARD, simply adding the pop1 of each JP ABOVE Index:

	    else
#endif // NOSMARTJBU
	    {
		assert(digit < cJU_BRANCHUNUMJPS);
#ifdef SMARTMETRICS
		++jbu_downward;
#endif
		pop1above = 0;			// add JPs above Index.

		for (jpnum = cJU_BRANCHUNUMJPS - 1; jpnum > digit; --jpnum)
		{
		    if ((Pjbu->jbu_jp[jpnum].jp_Type) <= cJU_JPNULLMAX)
			continue;	// shortcut, save a function call.

		    if ((pop1 = j__udyJPPop1(Pjbu->jbu_jp + jpnum))
		     == cJU_ALLONES)
		    {
			JU_SET_ERRNO_NONNULL(Pjpm, JU_ERRNO_CORRUPT);
			return(C_JERR);
		    }

		    pop1above += pop1;
		}
	    }

	    if ((pop1 = j__udy1LCountSM(Pjbu->jbu_jp + digit, Index, Pjpm))
	     == C_JERR) return(C_JERR);		// pass error up.

	    assert(pop1above + pop1);
	    return(pop1above + pop1);


// ----------------------------------------------------------------------------
// LEAF COUNT MACROS:
//
// LEAF*ABOVE() are common code for different JP types (linear leaves, bitmap
// leaves, and immediates) and different leaf Index Sizes, which result in
// calling different leaf search functions.  Linear leaves get the leaf address
// from jp_Addr and the Population from jp_DcdPopO, while immediates use Pjp
// itself as the leaf address and get Population from jp_Type.

#define	LEAFLABOVE(Func)				\
	Pjll = P_JLL(Pjp->jp_Addr);			\
	pop1 = JU_JPLEAF_POP0(Pjp) + 1;	                \
	LEAFABOVE(Func, Pjll, pop1)

#define	LEAFB1ABOVE(Func) LEAFLABOVE(Func)  // different Func, otherwise same.

#ifdef JUDY1
#define	IMMABOVE(Func,Pop1)	\
	Pjll = (Pjll_t) Pjp;	\
	LEAFABOVE(Func, Pjll, Pop1)
#else
// Note:  For JudyL immediates with >= 2 Indexes, the index bytes are in a
// different place than for Judy1:

#define	IMMABOVE(Func,Pop1) \
	LEAFABOVE(Func, (Pjll_t) (Pjp->jp_LIndex), Pop1)
#endif

// For all leaf types, the population AT OR ABOVE is the total pop1 less the
// offset of Index; and Index should always be found:

#define	LEAFABOVE(Func,Pjll,Pop1)		\
	offset = Func(Pjll, Pop1, Index);	\
	assert(offset >= 0);			\
	assert(offset < (Pop1));		\
	return((Pop1) - offset)

// IMMABOVE_01 handles the special case of an immediate JP with 1 index, which
// the search functions arent used for anyway:
//
// The target Index should be the one in this Immediate, in which case the
// count above (inclusive) is always 1.

#define	IMMABOVE_01						\
	assert((JU_JPDCDPOP0(Pjp)) == JU_TRIMTODCDSIZE(Index));	\
	return(1)


// ----------------------------------------------------------------------------
// LINEAR LEAF; search the leaf for Index; size is computed from jp_Type:

#if (defined(JUDYL) || (! defined(JU_64BIT)))
	case cJU_JPLEAF1:  LEAFLABOVE(j__udySearchLeaf1);
#endif
	case cJU_JPLEAF2:  LEAFLABOVE(j__udySearchLeaf2);
	case cJU_JPLEAF3:  LEAFLABOVE(j__udySearchLeaf3);

#ifdef JU_64BIT
	case cJU_JPLEAF4:  LEAFLABOVE(j__udySearchLeaf4);
	case cJU_JPLEAF5:  LEAFLABOVE(j__udySearchLeaf5);
	case cJU_JPLEAF6:  LEAFLABOVE(j__udySearchLeaf6);
	case cJU_JPLEAF7:  LEAFLABOVE(j__udySearchLeaf7);
#endif


// ----------------------------------------------------------------------------
// BITMAP LEAF; search the leaf for Index:
//
// Since the bitmap describes Indexes digitally rather than linearly, this is
// not really a search, but just a count.

	case cJU_JPLEAF_B1:  LEAFB1ABOVE(j__udyCountLeafB1);


#ifdef JUDY1
// ----------------------------------------------------------------------------
// FULL POPULATION:
//
// Return the count of Indexes AT OR ABOVE Index, which is the total population
// of the expanse (a constant) less the value of the undecoded digit remaining
// in Index (its base-0 offset in the expanse), which yields an inclusive count
// above.
//
// TBD:  This only supports a 1-byte full expanse.  Should this extract a
// stored value for pop0 and possibly more LSBs of Index, to handle larger full
// expanses?

	case cJ1_JPFULLPOPU1:
	    return(cJU_JPFULLPOPU1_POP0 + 1 - JU_DIGITATSTATE(Index, 1));
#endif


// ----------------------------------------------------------------------------
// IMMEDIATE:

	case cJU_JPIMMED_1_01:  IMMABOVE_01;
	case cJU_JPIMMED_2_01:  IMMABOVE_01;
	case cJU_JPIMMED_3_01:  IMMABOVE_01;
#ifdef JU_64BIT
	case cJU_JPIMMED_4_01:  IMMABOVE_01;
	case cJU_JPIMMED_5_01:  IMMABOVE_01;
	case cJU_JPIMMED_6_01:  IMMABOVE_01;
	case cJU_JPIMMED_7_01:  IMMABOVE_01;
#endif

	case cJU_JPIMMED_1_02:  IMMABOVE(j__udySearchLeaf1,  2);
	case cJU_JPIMMED_1_03:  IMMABOVE(j__udySearchLeaf1,  3);
#if (defined(JUDY1) || defined(JU_64BIT))
	case cJU_JPIMMED_1_04:  IMMABOVE(j__udySearchLeaf1,  4);
	case cJU_JPIMMED_1_05:  IMMABOVE(j__udySearchLeaf1,  5);
	case cJU_JPIMMED_1_06:  IMMABOVE(j__udySearchLeaf1,  6);
	case cJU_JPIMMED_1_07:  IMMABOVE(j__udySearchLeaf1,  7);
#endif
#if (defined(JUDY1) && defined(JU_64BIT))
	case cJ1_JPIMMED_1_08:  IMMABOVE(j__udySearchLeaf1,  8);
	case cJ1_JPIMMED_1_09:  IMMABOVE(j__udySearchLeaf1,  9);
	case cJ1_JPIMMED_1_10:  IMMABOVE(j__udySearchLeaf1, 10);
	case cJ1_JPIMMED_1_11:  IMMABOVE(j__udySearchLeaf1, 11);
	case cJ1_JPIMMED_1_12:  IMMABOVE(j__udySearchLeaf1, 12);
	case cJ1_JPIMMED_1_13:  IMMABOVE(j__udySearchLeaf1, 13);
	case cJ1_JPIMMED_1_14:  IMMABOVE(j__udySearchLeaf1, 14);
	case cJ1_JPIMMED_1_15:  IMMABOVE(j__udySearchLeaf1, 15);
#endif

#if (defined(JUDY1) || defined(JU_64BIT))
	case cJU_JPIMMED_2_02:  IMMABOVE(j__udySearchLeaf2,  2);
	case cJU_JPIMMED_2_03:  IMMABOVE(j__udySearchLeaf2,  3);
#endif
#if (defined(JUDY1) && defined(JU_64BIT))
	case cJ1_JPIMMED_2_04:  IMMABOVE(j__udySearchLeaf2,  4);
	case cJ1_JPIMMED_2_05:  IMMABOVE(j__udySearchLeaf2,  5);
	case cJ1_JPIMMED_2_06:  IMMABOVE(j__udySearchLeaf2,  6);
	case cJ1_JPIMMED_2_07:  IMMABOVE(j__udySearchLeaf2,  7);
#endif

#if (defined(JUDY1) || defined(JU_64BIT))
	case cJU_JPIMMED_3_02:  IMMABOVE(j__udySearchLeaf3,  2);
#endif
#if (defined(JUDY1) && defined(JU_64BIT))
	case cJ1_JPIMMED_3_03:  IMMABOVE(j__udySearchLeaf3,  3);
	case cJ1_JPIMMED_3_04:  IMMABOVE(j__udySearchLeaf3,  4);
	case cJ1_JPIMMED_3_05:  IMMABOVE(j__udySearchLeaf3,  5);

	case cJ1_JPIMMED_4_02:  IMMABOVE(j__udySearchLeaf4,  2);
	case cJ1_JPIMMED_4_03:  IMMABOVE(j__udySearchLeaf4,  3);

	case cJ1_JPIMMED_5_02:  IMMABOVE(j__udySearchLeaf5,  2);
	case cJ1_JPIMMED_5_03:  IMMABOVE(j__udySearchLeaf5,  3);

	case cJ1_JPIMMED_6_02:  IMMABOVE(j__udySearchLeaf6,  2);

	case cJ1_JPIMMED_7_02:  IMMABOVE(j__udySearchLeaf7,  2);
#endif


// ----------------------------------------------------------------------------
// OTHER CASES:

	default: JU_SET_ERRNO_NONNULL(Pjpm, JU_ERRNO_CORRUPT); return(C_JERR);

	} // switch on JP type

	/*NOTREACHED*/

} // j__udy1LCountSM()


// ****************************************************************************
// J U D Y   C O U N T   L E A F   B 1
//
// This is a private analog of the j__udySearchLeaf*() functions for counting
// in bitmap 1-byte leaves.  Since a bitmap leaf describes Indexes digitally
// rather than linearly, this is not really a search, but just a count of the
// valid Indexes == set bits below or including Index, which should be valid.
// Return the "offset" (really the ordinal), 0 .. Pop1 - 1, of Index in Pjll;
// if Indexs bit is not set (which should never happen, so this is DEBUG-mode
// only), return the 1s-complement equivalent (== negative offset minus 1).
//
// Note:  The source code for this function looks identical for both Judy1 and
// JudyL, but the JU_JLB_BITMAP macro varies.
//
// Note:  For simpler calling, the first arg is of type Pjll_t but then cast to
// Pjlb_t.

FUNCTION static int j__udyCountLeafB1(
const	Pjll_t	Pjll,		// bitmap leaf, as Pjll_t for consistency.
const	Word_t	Pop1,		// Population of whole leaf.
const	Word_t	Index)		// to which to count.
{
	Pjlb_t	Pjlb	= (Pjlb_t) Pjll;	// to proper type.
	Word_t	digit   = Index & cJU_MASKATSTATE(1);
	Word_t	findsub = digit / cJU_BITSPERSUBEXPL;
	Word_t	findbit = digit % cJU_BITSPERSUBEXPL;
	int	count;		// in leaf through Index.
	long	subexp;		// for stepping through subexpanses.


// COUNT UPWARD:
//
// The entire bitmap should fit in one cache line, but still try to save some
// CPU time by counting the fewest possible number of subexpanses from the
// bitmap.

#ifndef NOSMARTJLB  // enable to turn off smart code for comparison purposes.

	if (findsub < (cJU_NUMSUBEXPL / 2))
	{
#ifdef SMARTMETRICS
	    ++jlb_upward;
#endif
	    count = 0;

	    for (subexp = 0; subexp < findsub; ++subexp)
	    {
		count += ((JU_JLB_BITMAP(Pjlb, subexp) == cJU_FULLBITMAPL) ?
			  cJU_BITSPERSUBEXPL :
			  j__udyCountBitsL(JU_JLB_BITMAP(Pjlb, subexp)));
	    }

// This count includes findbit, which should be set, resulting in a base-1
// offset:

	    count += j__udyCountBitsL(JU_JLB_BITMAP(Pjlb, findsub)
				& JU_MASKLOWERINC(JU_BITPOSMASKL(findbit)));

	    DBGCODE(if (! JU_BITMAPTESTL(Pjlb, digit)) return(~count);)
	    assert(count >= 1);
	    return(count - 1);		// convert to base-0 offset.
	}
#endif // NOSMARTJLB


// COUNT DOWNWARD:
//
// Count the valid Indexes above or at Index, and subtract from Pop1.

#ifdef SMARTMETRICS
	++jlb_downward;
#endif
	count = Pop1;			// base-1 for now.

	for (subexp = cJU_NUMSUBEXPL - 1; subexp > findsub; --subexp)
	{
	    count -= ((JU_JLB_BITMAP(Pjlb, subexp) == cJU_FULLBITMAPL) ?
		      cJU_BITSPERSUBEXPL :
		      j__udyCountBitsL(JU_JLB_BITMAP(Pjlb, subexp)));
	}

// This count includes findbit, which should be set, resulting in a base-0
// offset:

	count -= j__udyCountBitsL(JU_JLB_BITMAP(Pjlb, findsub)
				& JU_MASKHIGHERINC(JU_BITPOSMASKL(findbit)));

	DBGCODE(if (! JU_BITMAPTESTL(Pjlb, digit)) return(~count);)
	assert(count >= 0);		// should find Index itself.
	return(count);			// is already a base-0 offset.

} // j__udyCountLeafB1()


// ****************************************************************************
// J U D Y   J P   P O P 1
//
// This function takes any type of JP other than a root-level JP (cJU_LEAFW* or
// cJU_JPBRANCH* with no number suffix) and extracts the Pop1 from it.  In some
// sense this is a wrapper around the JU_JP*_POP0 macros.  Why write it as a
// function instead of a complex macro containing a trinary?  (See version
// Judy1.h version 4.17.)  We think its cheaper to call a function containing
// a switch statement with "constant" cases than to do the variable
// calculations in a trinary.
//
// For invalid JP Types return cJU_ALLONES.  Note that this is an impossibly
// high Pop1 for any JP below a top level branch.

FUNCTION Word_t j__udyJPPop1(
const	Pjp_t Pjp)		// JP to count.
{
	switch (JU_JPTYPE(Pjp))
	{
#ifdef notdef // caller should shortcut and not even call with these:

	case cJU_JPNULL1:
	case cJU_JPNULL2:
	case cJU_JPNULL3:  return(0);
#ifdef JU_64BIT
	case cJU_JPNULL4:
	case cJU_JPNULL5:
	case cJU_JPNULL6:
	case cJU_JPNULL7:  return(0);
#endif
#endif // notdef

	case cJU_JPBRANCH_L2:
	case cJU_JPBRANCH_B2:
	case cJU_JPBRANCH_U2: return(JU_JPBRANCH_POP0(Pjp,2) + 1);

	case cJU_JPBRANCH_L3:
	case cJU_JPBRANCH_B3:
	case cJU_JPBRANCH_U3: return(JU_JPBRANCH_POP0(Pjp,3) + 1);

#ifdef JU_64BIT
	case cJU_JPBRANCH_L4:
	case cJU_JPBRANCH_B4:
	case cJU_JPBRANCH_U4: return(JU_JPBRANCH_POP0(Pjp,4) + 1);

	case cJU_JPBRANCH_L5:
	case cJU_JPBRANCH_B5:
	case cJU_JPBRANCH_U5: return(JU_JPBRANCH_POP0(Pjp,5) + 1);

	case cJU_JPBRANCH_L6:
	case cJU_JPBRANCH_B6:
	case cJU_JPBRANCH_U6: return(JU_JPBRANCH_POP0(Pjp,6) + 1);

	case cJU_JPBRANCH_L7:
	case cJU_JPBRANCH_B7:
	case cJU_JPBRANCH_U7: return(JU_JPBRANCH_POP0(Pjp,7) + 1);
#endif

#if (defined(JUDYL) || (! defined(JU_64BIT)))
	case cJU_JPLEAF1:
#endif
	case cJU_JPLEAF2:
	case cJU_JPLEAF3:
#ifdef JU_64BIT
	case cJU_JPLEAF4:
	case cJU_JPLEAF5:
	case cJU_JPLEAF6:
	case cJU_JPLEAF7:
#endif
	case cJU_JPLEAF_B1:	return(JU_JPLEAF_POP0(Pjp) + 1);

#ifdef JUDY1
	case cJ1_JPFULLPOPU1:	return(cJU_JPFULLPOPU1_POP0 + 1);
#endif

	case cJU_JPIMMED_1_01:
	case cJU_JPIMMED_2_01:
	case cJU_JPIMMED_3_01:	return(1);
#ifdef JU_64BIT
	case cJU_JPIMMED_4_01:
	case cJU_JPIMMED_5_01:
	case cJU_JPIMMED_6_01:
	case cJU_JPIMMED_7_01:	return(1);
#endif

	case cJU_JPIMMED_1_02:	return(2);
	case cJU_JPIMMED_1_03:	return(3);
#if (defined(JUDY1) || defined(JU_64BIT))
	case cJU_JPIMMED_1_04:	return(4);
	case cJU_JPIMMED_1_05:	return(5);
	case cJU_JPIMMED_1_06:	return(6);
	case cJU_JPIMMED_1_07:	return(7);
#endif
#if (defined(JUDY1) && defined(JU_64BIT))
	case cJ1_JPIMMED_1_08:	return(8);
	case cJ1_JPIMMED_1_09:	return(9);
	case cJ1_JPIMMED_1_10:	return(10);
	case cJ1_JPIMMED_1_11:	return(11);
	case cJ1_JPIMMED_1_12:	return(12);
	case cJ1_JPIMMED_1_13:	return(13);
	case cJ1_JPIMMED_1_14:	return(14);
	case cJ1_JPIMMED_1_15:	return(15);
#endif

#if (defined(JUDY1) || defined(JU_64BIT))
	case cJU_JPIMMED_2_02:	return(2);
	case cJU_JPIMMED_2_03:	return(3);
#endif
#if (defined(JUDY1) && defined(JU_64BIT))
	case cJ1_JPIMMED_2_04:	return(4);
	case cJ1_JPIMMED_2_05:	return(5);
	case cJ1_JPIMMED_2_06:	return(6);
	case cJ1_JPIMMED_2_07:	return(7);
#endif

#if (defined(JUDY1) || defined(JU_64BIT))
	case cJU_JPIMMED_3_02:	return(2);
#endif
#if (defined(JUDY1) && defined(JU_64BIT))
	case cJ1_JPIMMED_3_03:	return(3);
	case cJ1_JPIMMED_3_04:	return(4);
	case cJ1_JPIMMED_3_05:	return(5);

	case cJ1_JPIMMED_4_02:	return(2);
	case cJ1_JPIMMED_4_03:	return(3);

	case cJ1_JPIMMED_5_02:	return(2);
	case cJ1_JPIMMED_5_03:	return(3);

	case cJ1_JPIMMED_6_02:	return(2);

	case cJ1_JPIMMED_7_02:	return(2);
#endif

	default:		return(cJU_ALLONES);
	}

	/*NOTREACHED*/

} // j__udyJPPop1()
