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

// @(#) $Revision: 4.51 $ $Source: /judy/src/JudyCommon/JudyFreeArray.c $
//
// Judy1FreeArray() and JudyLFreeArray() functions for Judy1 and JudyL.
// Compile with one of -DJUDY1 or -DJUDYL.
// Return the number of bytes freed from the array.

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


// ****************************************************************************
// J U D Y   1   F R E E   A R R A Y
// J U D Y   L   F R E E   A R R A Y
//
// See the Judy*(3C) manual entry for details.
//
// This code is written recursively, at least at first, because thats much
// simpler.  Hope its fast enough.

#ifdef JUDY1
FUNCTION Word_t Judy1FreeArray
#else
FUNCTION Word_t JudyLFreeArray
#endif
        (
	PPvoid_t  PPArray,	// array to free.
	PJError_t PJError	// optional, for returning error info.
        )
{
	jpm_t	  jpm;		// local to accumulate free statistics.

// CHECK FOR NULL POINTER (error by caller):

	if (PPArray == (PPvoid_t) NULL)
	{
	    JU_SET_ERRNO(PJError, JU_ERRNO_NULLPPARRAY);
	    return(JERR);
	}

	DBGCODE(JudyCheckPop(*PPArray);)

// Zero jpm.jpm_Pop0 (meaning the array will be empty in a moment) for accurate
// logging in TRACEMI2.

	jpm.jpm_Pop0	      = 0;		// see above.
	jpm.jpm_TotalMemWords = 0;		// initialize memory freed.

// 	Empty array:

	if (P_JLW(*PPArray) == (Pjlw_t) NULL) return(0);

// PROCESS TOP LEVEL "JRP" BRANCHES AND LEAF:

	if (JU_LEAFW_POP0(*PPArray) < cJU_LEAFW_MAXPOP1) // must be a LEAFW
	{
	    Pjlw_t Pjlw = P_JLW(*PPArray);	// first word of leaf.

	    j__udyFreeJLW(Pjlw, Pjlw[0] + 1, &jpm);
	    *PPArray = (Pvoid_t) NULL;		// make an empty array.
	    return (-(jpm.jpm_TotalMemWords * cJU_BYTESPERWORD));  // see above.
	}
	else

// Rootstate leaves:  just free the leaf:

// Common code for returning the amount of memory freed.
//
// Note:  In a an ordinary LEAFW, pop0 = *PPArray[0].
//
// Accumulate (negative) words freed, while freeing objects.
// Return the positive bytes freed.

	{
	    Pjpm_t Pjpm	    = P_JPM(*PPArray);
	    Word_t TotalMem = Pjpm->jpm_TotalMemWords;

	    j__udyFreeSM(&(Pjpm->jpm_JP), &jpm);  // recurse through tree.
	    j__udyFreeJPM(Pjpm, &jpm);

// Verify the array was not corrupt.  This means that amount of memory freed
// (which is negative) is equal to the initial amount:

	    if (TotalMem + jpm.jpm_TotalMemWords)
	    {
		JU_SET_ERRNO(PJError, JU_ERRNO_CORRUPT);
		return(JERR);
	    }

	    *PPArray = (Pvoid_t) NULL;		// make an empty array.
	    return (TotalMem * cJU_BYTESPERWORD);
	}

} // Judy1FreeArray() / JudyLFreeArray()


// ****************************************************************************
// __ J U D Y   F R E E   S M
//
// Given a pointer to a JP, recursively visit and free (depth first) all nodes
// in a Judy array BELOW the JP, but not the JP itself.  Accumulate in *Pjpm
// the total words freed (as a negative value).  "SM" = State Machine.
//
// Note:  Corruption is not detected at this level because during a FreeArray,
// if the code hasnt already core dumped, its better to remain silent, even
// if some memory has not been freed, than to bother the caller about the
// corruption.  TBD:  Is this true?  If not, must list all legitimate JPNULL
// and JPIMMED above first, and revert to returning bool_t (see 4.34).

FUNCTION void j__udyFreeSM(
	Pjp_t	Pjp,		// top of Judy (top-state).
	Pjpm_t	Pjpm)		// to return words freed.
{
	Word_t	Pop1;

	switch (JU_JPTYPE(Pjp))
	{

#ifdef JUDY1

// FULL EXPANSE -- nothing to free  for this jp_Type.

	case cJ1_JPFULLPOPU1:
	    break;
#endif

// JUDY BRANCH -- free the sub-tree depth first:

// LINEAR BRANCH -- visit each JP in the JBLs list, then free the JBL:
//
// Note:  There are no null JPs in a JBL.

	case cJU_JPBRANCH_L:
	case cJU_JPBRANCH_L2:
	case cJU_JPBRANCH_L3:
#ifdef JU_64BIT
	case cJU_JPBRANCH_L4:
	case cJU_JPBRANCH_L5:
	case cJU_JPBRANCH_L6:
	case cJU_JPBRANCH_L7:
#endif // JU_64BIT
	{
	    Pjbl_t Pjbl = P_JBL(Pjp->jp_Addr);
	    Word_t offset;

	    for (offset = 0; offset < Pjbl->jbl_NumJPs; ++offset)
	        j__udyFreeSM((Pjbl->jbl_jp) + offset, Pjpm);

	    j__udyFreeJBL((Pjbl_t) (Pjp->jp_Addr), Pjpm);
	    break;
	}


// BITMAP BRANCH -- visit each JP in the JBBs list based on the bitmap, also
//
// Note:  There are no null JPs in a JBB.

	case cJU_JPBRANCH_B:
	case cJU_JPBRANCH_B2:
	case cJU_JPBRANCH_B3:
#ifdef JU_64BIT
	case cJU_JPBRANCH_B4:
	case cJU_JPBRANCH_B5:
	case cJU_JPBRANCH_B6:
	case cJU_JPBRANCH_B7:
#endif // JU_64BIT
	{
	    Word_t subexp;
	    Word_t offset;
	    Word_t jpcount;

	    Pjbb_t Pjbb = P_JBB(Pjp->jp_Addr);

	    for (subexp = 0; subexp < cJU_NUMSUBEXPB; ++subexp)
	    {
	        jpcount = j__udyCountBitsB(JU_JBB_BITMAP(Pjbb, subexp));

	        if (jpcount)
	        {
		    for (offset = 0; offset < jpcount; ++offset)
		    {
		       j__udyFreeSM(P_JP(JU_JBB_PJP(Pjbb, subexp)) + offset,
				    Pjpm);
		    }
		    j__udyFreeJBBJP(JU_JBB_PJP(Pjbb, subexp), jpcount, Pjpm);
	        }
	    }
	    j__udyFreeJBB((Pjbb_t) (Pjp->jp_Addr), Pjpm);

	    break;
	}


// UNCOMPRESSED BRANCH -- visit each JP in the JBU array, then free the JBU
// itself:
//
// Note:  Null JPs are handled during recursion at a lower state.

	case cJU_JPBRANCH_U:
	case cJU_JPBRANCH_U2:
	case cJU_JPBRANCH_U3:
#ifdef JU_64BIT
	case cJU_JPBRANCH_U4:
	case cJU_JPBRANCH_U5:
	case cJU_JPBRANCH_U6:
	case cJU_JPBRANCH_U7:
#endif // JU_64BIT
	{
	    Word_t offset;
	    Pjbu_t Pjbu = P_JBU(Pjp->jp_Addr);

	    for (offset = 0; offset < cJU_BRANCHUNUMJPS; ++offset)
	        j__udyFreeSM((Pjbu->jbu_jp) + offset, Pjpm);

	    j__udyFreeJBU((Pjbu_t) (Pjp->jp_Addr), Pjpm);
	    break;
	}


// -- Cases below here terminate and do not recurse. --


// LINEAR LEAF -- just free the leaf; size is computed from jp_Type:
//
// Note:  cJU_JPLEAF1 is a special case, see discussion in ../Judy1/Judy1.h

#if (defined(JUDYL) || (! defined(JU_64BIT)))
	case cJU_JPLEAF1:
	    Pop1 = JU_JPLEAF_POP0(Pjp) + 1;
	    j__udyFreeJLL1((Pjll_t) (Pjp->jp_Addr), Pop1, Pjpm);
	    break;
#endif

	case cJU_JPLEAF2:
	    Pop1 = JU_JPLEAF_POP0(Pjp) + 1;
	    j__udyFreeJLL2((Pjll_t) (Pjp->jp_Addr), Pop1, Pjpm);
	    break;

	case cJU_JPLEAF3:
	    Pop1 = JU_JPLEAF_POP0(Pjp) + 1;
	    j__udyFreeJLL3((Pjll_t) (Pjp->jp_Addr), Pop1, Pjpm);
	    break;

#ifdef JU_64BIT
	case cJU_JPLEAF4:
	    Pop1 = JU_JPLEAF_POP0(Pjp) + 1;
	    j__udyFreeJLL4((Pjll_t) (Pjp->jp_Addr), Pop1, Pjpm);
	    break;

	case cJU_JPLEAF5:
	    Pop1 = JU_JPLEAF_POP0(Pjp) + 1;
	    j__udyFreeJLL5((Pjll_t) (Pjp->jp_Addr), Pop1, Pjpm);
	    break;

	case cJU_JPLEAF6:
	    Pop1 = JU_JPLEAF_POP0(Pjp) + 1;
	    j__udyFreeJLL6((Pjll_t) (Pjp->jp_Addr), Pop1, Pjpm);
	    break;

	case cJU_JPLEAF7:
	    Pop1 = JU_JPLEAF_POP0(Pjp) + 1;
	    j__udyFreeJLL7((Pjll_t) (Pjp->jp_Addr), Pop1, Pjpm);
	    break;
#endif // JU_64BIT


// BITMAP LEAF -- free sub-expanse arrays of JPs, then free the JBB.

	case cJU_JPLEAF_B1:
	{
#ifdef JUDYL
	    Word_t subexp;
	    Word_t jpcount;
	    Pjlb_t Pjlb = P_JLB(Pjp->jp_Addr);

// Free the value areas in the bitmap leaf:

	    for (subexp = 0; subexp < cJU_NUMSUBEXPL; ++subexp)
	    {
	        jpcount = j__udyCountBitsL(JU_JLB_BITMAP(Pjlb, subexp));

	        if (jpcount)
		    j__udyLFreeJV(JL_JLB_PVALUE(Pjlb, subexp), jpcount, Pjpm);
	    }
#endif // JUDYL

	    j__udyFreeJLB1((Pjlb_t) (Pjp->jp_Addr), Pjpm);
	    break;

	} // case cJU_JPLEAF_B1

#ifdef JUDYL


// IMMED*:
//
// For JUDYL, all non JPIMMED_*_01s have a LeafV which must be freed:

	case cJU_JPIMMED_1_02:
	case cJU_JPIMMED_1_03:
#ifdef JU_64BIT
	case cJU_JPIMMED_1_04:
	case cJU_JPIMMED_1_05:
	case cJU_JPIMMED_1_06:
	case cJU_JPIMMED_1_07:
#endif
	    Pop1 = JU_JPTYPE(Pjp) - cJU_JPIMMED_1_02 + 2;
	    j__udyLFreeJV((Pjv_t) (Pjp->jp_Addr), Pop1, Pjpm);
	    break;

#ifdef JU_64BIT
	case cJU_JPIMMED_2_02:
	case cJU_JPIMMED_2_03:

	    Pop1 = JU_JPTYPE(Pjp) - cJU_JPIMMED_2_02 + 2;
	    j__udyLFreeJV((Pjv_t) (Pjp->jp_Addr), Pop1, Pjpm);
	    break;

	case cJU_JPIMMED_3_02:
	    j__udyLFreeJV((Pjv_t) (Pjp->jp_Addr), 2, Pjpm);
	    break;

#endif // JU_64BIT
#endif // JUDYL


// OTHER JPNULL, JPIMMED, OR UNEXPECTED TYPE -- nothing to free for this type:
//
// Note:  Lump together no-op and invalid JP types; see function header
// comments.

	default: break;

	} // switch (JU_JPTYPE(Pjp))

} // j__udyFreeSM()
