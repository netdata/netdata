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

// @(#) $Revision: 4.25 $ $Source: /judy/src/JudyCommon/JudyDecascade.c $
//
// "Decascade" support functions for JudyDel.c:  These functions convert
// smaller-index-size leaves to larger-index-size leaves, and also, bitmap
// leaves (LeafB1s) to Leaf1s, and some types of branches to smaller branches
// at the same index size.  Some "decascading" occurs explicitly in JudyDel.c,
// but rare or large subroutines appear as functions here, and the overhead to
// call them is negligible.
//
// Compile with one of -DJUDY1 or -DJUDYL.  Note:  Function names are converted
// to Judy1 or JudyL specific values by external #defines.

#if (! (defined(JUDY1) || defined(JUDYL)))
#error:  One of -DJUDY1 or -DJUDYL must be specified.
#endif

#ifdef JUDY1
#include "Judy1.h"
#endif
#ifdef JUDYL
#include "JudyL.h"
#endif

#include "JudyPrivate1L.h"

DBGCODE(extern void JudyCheckSorted(Pjll_t Pjll, Word_t Pop1, long IndexSize);)


// ****************************************************************************
// __ J U D Y   C O P Y   2   T O   3
//
// Copy one or more 2-byte Indexes to a series of 3-byte Indexes.

FUNCTION static void j__udyCopy2to3(
	uint8_t *  PDest,	// to where to copy 3-byte Indexes.
	uint16_t * PSrc,	// from where to copy 2-byte indexes.
	Word_t     Pop1,	// number of Indexes to copy.
	Word_t     MSByte)	// most-significant byte, prefix to each Index.
{
	Word_t	   Temp;	// for building 3-byte Index.

	assert(Pop1);

        do {
	    Temp = MSByte | *PSrc++;
	    JU_COPY3_LONG_TO_PINDEX(PDest, Temp);
	    PDest += 3;
        } while (--Pop1);

} // j__udyCopy2to3()


#ifdef JU_64BIT

// ****************************************************************************
// __ J U D Y   C O P Y   3   T O   4
//
// Copy one or more 3-byte Indexes to a series of 4-byte Indexes.

FUNCTION static void j__udyCopy3to4(
	uint32_t * PDest,	// to where to copy 4-byte Indexes.
	uint8_t *  PSrc,	// from where to copy 3-byte indexes.
	Word_t     Pop1,	// number of Indexes to copy.
	Word_t     MSByte)	// most-significant byte, prefix to each Index.
{
	Word_t	   Temp;	// for building 4-byte Index.

	assert(Pop1);

        do {
	    JU_COPY3_PINDEX_TO_LONG(Temp, PSrc);
	    Temp |= MSByte;
	    PSrc += 3;
	    *PDest++ = Temp;		// truncates to uint32_t.
        } while (--Pop1);

} // j__udyCopy3to4()


// ****************************************************************************
// __ J U D Y   C O P Y   4   T O   5
//
// Copy one or more 4-byte Indexes to a series of 5-byte Indexes.

FUNCTION static void j__udyCopy4to5(
	uint8_t *  PDest,	// to where to copy 4-byte Indexes.
	uint32_t * PSrc,	// from where to copy 4-byte indexes.
	Word_t     Pop1,	// number of Indexes to copy.
	Word_t     MSByte)	// most-significant byte, prefix to each Index.
{
	Word_t	   Temp;	// for building 5-byte Index.

	assert(Pop1);

        do {
	    Temp = MSByte | *PSrc++;
	    JU_COPY5_LONG_TO_PINDEX(PDest, Temp);
	    PDest += 5;
        } while (--Pop1);

} // j__udyCopy4to5()


// ****************************************************************************
// __ J U D Y   C O P Y   5   T O   6
//
// Copy one or more 5-byte Indexes to a series of 6-byte Indexes.

FUNCTION static void j__udyCopy5to6(
	uint8_t * PDest,	// to where to copy 6-byte Indexes.
	uint8_t * PSrc,		// from where to copy 5-byte indexes.
	Word_t    Pop1,		// number of Indexes to copy.
	Word_t    MSByte)	// most-significant byte, prefix to each Index.
{
	Word_t	  Temp;		// for building 6-byte Index.

	assert(Pop1);

        do {
	    JU_COPY5_PINDEX_TO_LONG(Temp, PSrc);
	    Temp |= MSByte;
	    JU_COPY6_LONG_TO_PINDEX(PDest, Temp);
	    PSrc  += 5;
	    PDest += 6;
        } while (--Pop1);

} // j__udyCopy5to6()


// ****************************************************************************
// __ J U D Y   C O P Y   6   T O   7
//
// Copy one or more 6-byte Indexes to a series of 7-byte Indexes.

FUNCTION static void j__udyCopy6to7(
	uint8_t * PDest,	// to where to copy 6-byte Indexes.
	uint8_t * PSrc,		// from where to copy 5-byte indexes.
	Word_t    Pop1,		// number of Indexes to copy.
	Word_t    MSByte)	// most-significant byte, prefix to each Index.
{
	Word_t	  Temp;		// for building 6-byte Index.

	assert(Pop1);

        do {
	    JU_COPY6_PINDEX_TO_LONG(Temp, PSrc);
	    Temp |= MSByte;
	    JU_COPY7_LONG_TO_PINDEX(PDest, Temp);
	    PSrc  += 6;
	    PDest += 7;
        } while (--Pop1);

} // j__udyCopy6to7()

#endif // JU_64BIT


#ifndef JU_64BIT // 32-bit

// ****************************************************************************
// __ J U D Y   C O P Y   3   T O   W
//
// Copy one or more 3-byte Indexes to a series of longs (words, always 4-byte).

FUNCTION static void j__udyCopy3toW(
	PWord_t   PDest,	// to where to copy full-word Indexes.
	uint8_t * PSrc,		// from where to copy 3-byte indexes.
	Word_t    Pop1,		// number of Indexes to copy.
	Word_t    MSByte)	// most-significant byte, prefix to each Index.
{
	assert(Pop1);

        do {
	    JU_COPY3_PINDEX_TO_LONG(*PDest, PSrc);
	    *PDest++ |= MSByte;
	    PSrc     += 3;
        } while (--Pop1);

} // j__udyCopy3toW()


#else // JU_64BIT

// ****************************************************************************
// __ J U D Y   C O P Y   7   T O   W
//
// Copy one or more 7-byte Indexes to a series of longs (words, always 8-byte).

FUNCTION static void j__udyCopy7toW(
	PWord_t   PDest,	// to where to copy full-word Indexes.
	uint8_t * PSrc,		// from where to copy 7-byte indexes.
	Word_t    Pop1,		// number of Indexes to copy.
	Word_t    MSByte)	// most-significant byte, prefix to each Index.
{
	assert(Pop1);

        do {
	    JU_COPY7_PINDEX_TO_LONG(*PDest, PSrc);
	    *PDest++ |= MSByte;
	    PSrc     += 7;
        } while (--Pop1);

} // j__udyCopy7toW()

#endif // JU_64BIT


// ****************************************************************************
// __ J U D Y   B R A N C H   B   T O   B R A N C H   L
//
// When a BranchB shrinks to have few enough JPs, call this function to convert
// it to a BranchL.  Return 1 for success, or -1 for failure (with details in
// Pjpm).

FUNCTION int j__udyBranchBToBranchL(
	Pjp_t	Pjp,		// points to BranchB to shrink.
	Pvoid_t	Pjpm)		// for global accounting.
{
	Pjbb_t	PjbbRaw;	// old BranchB to shrink.
	Pjbb_t	Pjbb;
	Pjbl_t	PjblRaw;	// new BranchL to create.
	Pjbl_t	Pjbl;
	Word_t	Digit;		// in BranchB.
	Word_t  NumJPs;		// non-null JPs in BranchB.
	uint8_t Expanse[cJU_BRANCHLMAXJPS];	// for building jbl_Expanse[].
	Pjp_t	Pjpjbl;		// current JP in BranchL.
	Word_t  SubExp;		// in BranchB.

	assert(JU_JPTYPE(Pjp) >= cJU_JPBRANCH_B2);
	assert(JU_JPTYPE(Pjp) <= cJU_JPBRANCH_B);

	PjbbRaw	= (Pjbb_t) (Pjp->jp_Addr);
	Pjbb	= P_JBB(PjbbRaw);

// Copy 1-byte subexpanse digits from BranchB to temporary buffer for BranchL,
// for each bit set in the BranchB:
//
// TBD:  The following supports variable-sized linear branches, but they are no
// longer variable; this could be simplified to save the copying.
//
// TBD:  Since cJU_BRANCHLMAXJP == 7 now, and cJU_BRANCHUNUMJPS == 256, the
// following might be inefficient; is there a faster way to do it?  At least
// skip wholly empty subexpanses?

	for (NumJPs = Digit = 0; Digit < cJU_BRANCHUNUMJPS; ++Digit)
	{
	    if (JU_BITMAPTESTB(Pjbb, Digit))
	    {
		Expanse[NumJPs++] = Digit;
		assert(NumJPs <= cJU_BRANCHLMAXJPS);	// required of caller.
	    }
	}

// Allocate and populate the BranchL:

	if ((PjblRaw = j__udyAllocJBL(Pjpm)) == (Pjbl_t) NULL) return(-1);
	Pjbl = P_JBL(PjblRaw);

	JU_COPYMEM(Pjbl->jbl_Expanse, Expanse, NumJPs);

	Pjbl->jbl_NumJPs = NumJPs;
	DBGCODE(JudyCheckSorted((Pjll_t) (Pjbl->jbl_Expanse), NumJPs, 1);)

// Copy JPs from each BranchB subexpanse subarray:

	Pjpjbl = P_JP(Pjbl->jbl_jp);	// start at first JP in array.

	for (SubExp = 0; SubExp < cJU_NUMSUBEXPB; ++SubExp)
	{
	    Pjp_t PjpRaw = JU_JBB_PJP(Pjbb, SubExp);	// current Pjp.
	    Pjp_t Pjp;

	    if (PjpRaw == (Pjp_t) NULL) continue;  // skip empty subexpanse.
	    Pjp = P_JP(PjpRaw);

	    NumJPs = j__udyCountBitsB(JU_JBB_BITMAP(Pjbb, SubExp));
	    assert(NumJPs);
	    JU_COPYMEM(Pjpjbl, Pjp, NumJPs);	 // one subarray at a time.

	    Pjpjbl += NumJPs;
	    j__udyFreeJBBJP(PjpRaw, NumJPs, Pjpm);	// subarray.
	}
	j__udyFreeJBB(PjbbRaw, Pjpm);		// BranchB itself.

// Finish up:  Calculate new JP type (same index size = level in new class),
// and tie new BranchB into parent JP:

	Pjp->jp_Type += cJU_JPBRANCH_L - cJU_JPBRANCH_B;
	Pjp->jp_Addr  = (Word_t) PjblRaw;

	return(1);

} // j__udyBranchBToBranchL()


#ifdef notdef

// ****************************************************************************
// __ J U D Y   B R A N C H   U   T O   B R A N C H   B
//
// When a BranchU shrinks to need little enough memory, call this function to
// convert it to a BranchB to save memory (at the cost of some speed).  Return
// 1 for success, or -1 for failure (with details in Pjpm).
//
// TBD:  Fill out if/when needed.  Not currently used in JudyDel.c for reasons
// explained there.

FUNCTION int j__udyBranchUToBranchB(
	Pjp_t	Pjp,		// points to BranchU to shrink.
	Pvoid_t	Pjpm)		// for global accounting.
{
	assert(FALSE);
	return(1);
}
#endif // notdef


#if (defined(JUDYL) || (! defined(JU_64BIT)))

// ****************************************************************************
// __ J U D Y   L E A F   B 1   T O   L E A F   1
//
// Shrink a bitmap leaf (cJU_LEAFB1) to linear leaf (cJU_JPLEAF1).
// Return 1 for success, or -1 for failure (with details in Pjpm).
//
// Note:  This function is different than the other JudyLeaf*ToLeaf*()
// functions because it receives a Pjp, not just a leaf, and handles its own
// allocation and free, in order to allow the caller to continue with a LeafB1
// if allocation fails.

FUNCTION int j__udyLeafB1ToLeaf1(
	Pjp_t	  Pjp,		// points to LeafB1 to shrink.
	Pvoid_t	  Pjpm)		// for global accounting.
{
	Pjlb_t    PjlbRaw;	// bitmap in old leaf.
	Pjlb_t    Pjlb;
	Pjll_t	  PjllRaw;	// new Leaf1.
	uint8_t	* Pleaf1;	// Leaf1 pointer type.
	Word_t    Digit;	// in LeafB1 bitmap.
#ifdef JUDYL
	Pjv_t	  PjvNew;	// value area in new Leaf1.
	Word_t    Pop1;
	Word_t    SubExp;
#endif

	assert(JU_JPTYPE(Pjp) == cJU_JPLEAF_B1);
	assert(((JU_JPDCDPOP0(Pjp) & 0xFF) + 1) == cJU_LEAF1_MAXPOP1);

// Allocate JPLEAF1 and prepare pointers:

	if ((PjllRaw = j__udyAllocJLL1(cJU_LEAF1_MAXPOP1, Pjpm)) == 0)
	    return(-1);

	Pleaf1	= (uint8_t *) P_JLL(PjllRaw);
	PjlbRaw	= (Pjlb_t) (Pjp->jp_Addr);
	Pjlb	= P_JLB(PjlbRaw);
	JUDYLCODE(PjvNew = JL_LEAF1VALUEAREA(Pleaf1, cJL_LEAF1_MAXPOP1);)

// Copy 1-byte indexes from old LeafB1 to new Leaf1:

	for (Digit = 0; Digit < cJU_BRANCHUNUMJPS; ++Digit)
	    if (JU_BITMAPTESTL(Pjlb, Digit))
		*Pleaf1++ = Digit;

#ifdef JUDYL

// Copy all old-LeafB1 value areas from value subarrays to new Leaf1:

	for (SubExp = 0; SubExp < cJU_NUMSUBEXPL; ++SubExp)
	{
	    Pjv_t PjvRaw = JL_JLB_PVALUE(Pjlb, SubExp);
	    Pjv_t Pjv    = P_JV(PjvRaw);

	    if (Pjv == (Pjv_t) NULL) continue;	// skip empty subarray.

	    Pop1 = j__udyCountBitsL(JU_JLB_BITMAP(Pjlb, SubExp));  // subarray.
	    assert(Pop1);

	    JU_COPYMEM(PjvNew, Pjv, Pop1);		// copy value areas.
	    j__udyLFreeJV(PjvRaw, Pop1, Pjpm);
	    PjvNew += Pop1;				// advance through new.
	}

	assert((((Word_t) Pleaf1) - (Word_t) P_JLL(PjllRaw))
	    == (PjvNew - JL_LEAF1VALUEAREA(P_JLL(PjllRaw), cJL_LEAF1_MAXPOP1)));
#endif // JUDYL

	DBGCODE(JudyCheckSorted((Pjll_t) P_JLL(PjllRaw),
			    (((Word_t) Pleaf1) - (Word_t) P_JLL(PjllRaw)), 1);)

// Finish up:  Free the old LeafB1 and plug the new Leaf1 into the JP:
//
// Note:  jp_DcdPopO does not change here.

	j__udyFreeJLB1(PjlbRaw, Pjpm);

	Pjp->jp_Addr = (Word_t) PjllRaw;
	Pjp->jp_Type = cJU_JPLEAF1;

	return(1);

} // j__udyLeafB1ToLeaf1()

#endif // (JUDYL || (! JU_64BIT))


// ****************************************************************************
// __ J U D Y   L E A F   1   T O   L E A F   2
//
// Copy 1-byte Indexes from a LeafB1 or Leaf1 to 2-byte Indexes in a Leaf2.
// Pjp MUST be one of:  cJU_JPLEAF_B1, cJU_JPLEAF1, or cJU_JPIMMED_1_*.
// Return number of Indexes copied.
//
// TBD:  In this and all following functions, the caller should already be able
// to compute the Pop1 return value, so why return it?

FUNCTION Word_t  j__udyLeaf1ToLeaf2(
	uint16_t * PLeaf2,	// destination uint16_t * Index portion of leaf.
#ifdef JUDYL
	Pjv_t	   Pjv2,	// destination value part of leaf.
#endif
	Pjp_t	   Pjp,		// 1-byte-index object from which to copy.
	Word_t     MSByte,	// most-significant byte, prefix to each Index.
	Pvoid_t	   Pjpm)	// for global accounting.
{
	Word_t	   Pop1;	// Indexes in leaf.
	Word_t	   Offset;	// in linear leaf list.
JUDYLCODE(Pjv_t	   Pjv1Raw;)	// source object value area.
JUDYLCODE(Pjv_t	   Pjv1;)

	switch (JU_JPTYPE(Pjp))
	{


// JPLEAF_B1:

	case cJU_JPLEAF_B1:
	{
	    Pjlb_t Pjlb = P_JLB(Pjp->jp_Addr);
	    Word_t Digit;	// in LeafB1 bitmap.
  JUDYLCODE(Word_t SubExp;)	// in LeafB1.

	    Pop1 = JU_JPBRANCH_POP0(Pjp, 1) + 1; assert(Pop1);

// Copy 1-byte indexes from old LeafB1 to new Leaf2, including splicing in
// the missing MSByte needed in the Leaf2:

	    for (Digit = 0; Digit < cJU_BRANCHUNUMJPS; ++Digit)
		if (JU_BITMAPTESTL(Pjlb, Digit))
		    *PLeaf2++ = MSByte | Digit;

#ifdef JUDYL

// Copy all old-LeafB1 value areas from value subarrays to new Leaf2:

	    for (SubExp = 0; SubExp < cJU_NUMSUBEXPL; ++SubExp)
	    {
		Word_t SubExpPop1;

		Pjv1Raw = JL_JLB_PVALUE(Pjlb, SubExp);
		if (Pjv1Raw == (Pjv_t) NULL) continue;	// skip empty.
		Pjv1 = P_JV(Pjv1Raw);

		SubExpPop1 = j__udyCountBitsL(JU_JLB_BITMAP(Pjlb, SubExp));
		assert(SubExpPop1);

		JU_COPYMEM(Pjv2, Pjv1, SubExpPop1);	// copy value areas.
		j__udyLFreeJV(Pjv1Raw, SubExpPop1, Pjpm);
		Pjv2 += SubExpPop1;			// advance through new.
	    }
#endif // JUDYL

	    j__udyFreeJLB1((Pjlb_t) (Pjp->jp_Addr), Pjpm);  // LeafB1 itself.
	    return(Pop1);

	} // case cJU_JPLEAF_B1


#if (defined(JUDYL) || (! defined(JU_64BIT)))

// JPLEAF1:

	case cJU_JPLEAF1:
	{
	    uint8_t * PLeaf1 = (uint8_t *) P_JLL(Pjp->jp_Addr);

	    Pop1 = JU_JPBRANCH_POP0(Pjp, 1) + 1; assert(Pop1);
	    JUDYLCODE(Pjv1 = JL_LEAF1VALUEAREA(PLeaf1, Pop1);)

// Copy all Index bytes including splicing in missing MSByte needed in Leaf2
// (plus, for JudyL, value areas):

	    for (Offset = 0; Offset < Pop1; ++Offset)
	    {
		PLeaf2[Offset] = MSByte | PLeaf1[Offset];
		JUDYLCODE(Pjv2[Offset] = Pjv1[Offset];)
	    }
	    j__udyFreeJLL1((Pjll_t) (Pjp->jp_Addr), Pop1, Pjpm);
	    return(Pop1);
	}
#endif // (JUDYL || (! JU_64BIT))


// JPIMMED_1_01:
//
// Note:  jp_DcdPopO has 3 [7] bytes of Index (all but most significant byte),
// so the assignment to PLeaf2[] truncates and MSByte is not needed.

	case cJU_JPIMMED_1_01:
	{
	    PLeaf2[0] = JU_JPDCDPOP0(Pjp);	// see above.
	    JUDYLCODE(Pjv2[0] = Pjp->jp_Addr;)
	    return(1);
	}


// JPIMMED_1_0[2+]:

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
	{
	    Pop1 = JU_JPTYPE(Pjp) - cJU_JPIMMED_1_02 + 2; assert(Pop1);
	    JUDYLCODE(Pjv1Raw = (Pjv_t) (Pjp->jp_Addr);)
	    JUDYLCODE(Pjv1    = P_JV(Pjv1Raw);)

	    for (Offset = 0; Offset < Pop1; ++Offset)
	    {
#ifdef JUDY1
		PLeaf2[Offset] = MSByte | Pjp->jp_1Index[Offset];
#else
		PLeaf2[Offset] = MSByte | Pjp->jp_LIndex[Offset];
		Pjv2  [Offset] = Pjv1[Offset];
#endif
	    }
	    JUDYLCODE(j__udyLFreeJV(Pjv1Raw, Pop1, Pjpm);)
	    return(Pop1);
	}


// UNEXPECTED CASES, including JPNULL1, should be handled by caller:

	default: assert(FALSE); break;

	} // switch

	return(0);

} // j__udyLeaf1ToLeaf2()


// *****************************************************************************
// __ J U D Y   L E A F   2   T O   L E A F   3
//
// Copy 2-byte Indexes from a Leaf2 to 3-byte Indexes in a Leaf3.
// Pjp MUST be one of:  cJU_JPLEAF2 or cJU_JPIMMED_2_*.
// Return number of Indexes copied.
//
// Note:  By the time this function is called to compress a level-3 branch to a
// Leaf3, the branch has no narrow pointers under it, meaning only level-2
// objects are below it and must be handled here.

FUNCTION Word_t  j__udyLeaf2ToLeaf3(
	uint8_t * PLeaf3,	// destination "uint24_t *" Index part of leaf.
#ifdef JUDYL
	Pjv_t	  Pjv3,		// destination value part of leaf.
#endif
	Pjp_t	  Pjp,		// 2-byte-index object from which to copy.
	Word_t    MSByte,	// most-significant byte, prefix to each Index.
	Pvoid_t	  Pjpm)		// for global accounting.
{
	Word_t	  Pop1;		// Indexes in leaf.
#if (defined(JUDYL) && defined(JU_64BIT))
	Pjv_t	  Pjv2Raw;	// source object value area.
#endif
JUDYLCODE(Pjv_t	  Pjv2;)

	switch (JU_JPTYPE(Pjp))
	{


// JPLEAF2:

	case cJU_JPLEAF2:
	{
	    uint16_t * PLeaf2 = (uint16_t *) P_JLL(Pjp->jp_Addr);

	    Pop1 = JU_JPLEAF_POP0(Pjp) + 1; assert(Pop1);
	    j__udyCopy2to3(PLeaf3, PLeaf2, Pop1, MSByte);
#ifdef JUDYL
	    Pjv2 = JL_LEAF2VALUEAREA(PLeaf2, Pop1);
	    JU_COPYMEM(Pjv3, Pjv2, Pop1);
#endif
	    j__udyFreeJLL2((Pjll_t) (Pjp->jp_Addr), Pop1, Pjpm);
	    return(Pop1);
	}


// JPIMMED_2_01:
//
// Note:  jp_DcdPopO has 3 [7] bytes of Index (all but most significant byte),
// so the "assignment" to PLeaf3[] is exact [truncates] and MSByte is not
// needed.

	case cJU_JPIMMED_2_01:
	{
	    JU_COPY3_LONG_TO_PINDEX(PLeaf3, JU_JPDCDPOP0(Pjp));	// see above.
	    JUDYLCODE(Pjv3[0] = Pjp->jp_Addr;)
	    return(1);
	}


// JPIMMED_2_0[2+]:

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
	{
	    JUDY1CODE(uint16_t * PLeaf2 = (uint16_t *) (Pjp->jp_1Index);)
	    JUDYLCODE(uint16_t * PLeaf2 = (uint16_t *) (Pjp->jp_LIndex);)

	    Pop1 = JU_JPTYPE(Pjp) - cJU_JPIMMED_2_02 + 2; assert(Pop1);
	    j__udyCopy2to3(PLeaf3, PLeaf2, Pop1, MSByte);
#ifdef JUDYL
	    Pjv2Raw = (Pjv_t) (Pjp->jp_Addr);
	    Pjv2    = P_JV(Pjv2Raw);
	    JU_COPYMEM(Pjv3, Pjv2, Pop1);
	    j__udyLFreeJV(Pjv2Raw, Pop1, Pjpm);
#endif
	    return(Pop1);
	}
#endif // (JUDY1 || JU_64BIT)


// UNEXPECTED CASES, including JPNULL2, should be handled by caller:

	default: assert(FALSE); break;

	} // switch

	return(0);

} // j__udyLeaf2ToLeaf3()


#ifdef JU_64BIT

// ****************************************************************************
// __ J U D Y   L E A F   3   T O   L E A F   4
//
// Copy 3-byte Indexes from a Leaf3 to 4-byte Indexes in a Leaf4.
// Pjp MUST be one of:  cJU_JPLEAF3 or cJU_JPIMMED_3_*.
// Return number of Indexes copied.
//
// Note:  By the time this function is called to compress a level-4 branch to a
// Leaf4, the branch has no narrow pointers under it, meaning only level-3
// objects are below it and must be handled here.

FUNCTION Word_t  j__udyLeaf3ToLeaf4(
	uint32_t * PLeaf4,	// destination uint32_t * Index part of leaf.
#ifdef JUDYL
	Pjv_t	   Pjv4,	// destination value part of leaf.
#endif
	Pjp_t	   Pjp,		// 3-byte-index object from which to copy.
	Word_t     MSByte,	// most-significant byte, prefix to each Index.
	Pvoid_t	   Pjpm)	// for global accounting.
{
	Word_t	   Pop1;	// Indexes in leaf.
JUDYLCODE(Pjv_t	   Pjv3Raw;)	// source object value area.
JUDYLCODE(Pjv_t	   Pjv3;)

	switch (JU_JPTYPE(Pjp))
	{


// JPLEAF3:

	case cJU_JPLEAF3:
	{
	    uint8_t * PLeaf3 = (uint8_t *) P_JLL(Pjp->jp_Addr);

	    Pop1 = JU_JPLEAF_POP0(Pjp) + 1; assert(Pop1);
	    j__udyCopy3to4(PLeaf4, (uint8_t *) PLeaf3, Pop1, MSByte);
#ifdef JUDYL
	    Pjv3 = JL_LEAF3VALUEAREA(PLeaf3, Pop1);
	    JU_COPYMEM(Pjv4, Pjv3, Pop1);
#endif
	    j__udyFreeJLL3((Pjll_t) (Pjp->jp_Addr), Pop1, Pjpm);
	    return(Pop1);
	}


// JPIMMED_3_01:
//
// Note:  jp_DcdPopO has 7 bytes of Index (all but most significant byte), so
// the assignment to PLeaf4[] truncates and MSByte is not needed.

	case cJU_JPIMMED_3_01:
	{
	    PLeaf4[0] = JU_JPDCDPOP0(Pjp);	// see above.
	    JUDYLCODE(Pjv4[0] = Pjp->jp_Addr;)
	    return(1);
	}


// JPIMMED_3_0[2+]:

	case cJU_JPIMMED_3_02:
#ifdef JUDY1
	case cJ1_JPIMMED_3_03:
	case cJ1_JPIMMED_3_04:
	case cJ1_JPIMMED_3_05:
#endif
	{
	    JUDY1CODE(uint8_t * PLeaf3 = (uint8_t *) (Pjp->jp_1Index);)
	    JUDYLCODE(uint8_t * PLeaf3 = (uint8_t *) (Pjp->jp_LIndex);)

	    JUDY1CODE(Pop1 = JU_JPTYPE(Pjp) - cJU_JPIMMED_3_02 + 2;)
	    JUDYLCODE(Pop1 = 2;)

	    j__udyCopy3to4(PLeaf4, PLeaf3, Pop1, MSByte);
#ifdef JUDYL
	    Pjv3Raw = (Pjv_t) (Pjp->jp_Addr);
	    Pjv3    = P_JV(Pjv3Raw);
	    JU_COPYMEM(Pjv4, Pjv3, Pop1);
	    j__udyLFreeJV(Pjv3Raw, Pop1, Pjpm);
#endif
	    return(Pop1);
	}


// UNEXPECTED CASES, including JPNULL3, should be handled by caller:

	default: assert(FALSE); break;

	} // switch

	return(0);

} // j__udyLeaf3ToLeaf4()


// Note:  In all following j__udyLeaf*ToLeaf*() functions, JPIMMED_*_0[2+]
// cases exist for Judy1 (&& 64-bit) only.  JudyL has no equivalent Immeds.


// *****************************************************************************
// __ J U D Y   L E A F   4   T O   L E A F   5
//
// Copy 4-byte Indexes from a Leaf4 to 5-byte Indexes in a Leaf5.
// Pjp MUST be one of:  cJU_JPLEAF4 or cJU_JPIMMED_4_*.
// Return number of Indexes copied.
//
// Note:  By the time this function is called to compress a level-5 branch to a
// Leaf5, the branch has no narrow pointers under it, meaning only level-4
// objects are below it and must be handled here.

FUNCTION Word_t  j__udyLeaf4ToLeaf5(
	uint8_t * PLeaf5,	// destination "uint40_t *" Index part of leaf.
#ifdef JUDYL
	Pjv_t	  Pjv5,		// destination value part of leaf.
#endif
	Pjp_t	  Pjp,		// 4-byte-index object from which to copy.
	Word_t    MSByte,	// most-significant byte, prefix to each Index.
	Pvoid_t	  Pjpm)		// for global accounting.
{
	Word_t	  Pop1;		// Indexes in leaf.
JUDYLCODE(Pjv_t	  Pjv4;)	// source object value area.

	switch (JU_JPTYPE(Pjp))
	{


// JPLEAF4:

	case cJU_JPLEAF4:
	{
	    uint32_t * PLeaf4 = (uint32_t *) P_JLL(Pjp->jp_Addr);

	    Pop1 = JU_JPLEAF_POP0(Pjp) + 1; assert(Pop1);
	    j__udyCopy4to5(PLeaf5, PLeaf4, Pop1, MSByte);
#ifdef JUDYL
	    Pjv4 = JL_LEAF4VALUEAREA(PLeaf4, Pop1);
	    JU_COPYMEM(Pjv5, Pjv4, Pop1);
#endif
	    j__udyFreeJLL4((Pjll_t) (Pjp->jp_Addr), Pop1, Pjpm);
	    return(Pop1);
	}


// JPIMMED_4_01:
//
// Note:  jp_DcdPopO has 7 bytes of Index (all but most significant byte), so
// the assignment to PLeaf5[] truncates and MSByte is not needed.

	case cJU_JPIMMED_4_01:
	{
	    JU_COPY5_LONG_TO_PINDEX(PLeaf5, JU_JPDCDPOP0(Pjp));	// see above.
	    JUDYLCODE(Pjv5[0] = Pjp->jp_Addr;)
	    return(1);
	}


#ifdef JUDY1

// JPIMMED_4_0[4+]:

	case cJ1_JPIMMED_4_02:
	case cJ1_JPIMMED_4_03:
	{
	    uint32_t * PLeaf4 = (uint32_t *) (Pjp->jp_1Index);

	    Pop1 = JU_JPTYPE(Pjp) - cJ1_JPIMMED_4_02 + 2;
	    j__udyCopy4to5(PLeaf5, PLeaf4, Pop1, MSByte);
	    return(Pop1);
	}
#endif // JUDY1


// UNEXPECTED CASES, including JPNULL4, should be handled by caller:

	default: assert(FALSE); break;

	} // switch

	return(0);

} // j__udyLeaf4ToLeaf5()


// ****************************************************************************
// __ J U D Y   L E A F   5   T O   L E A F   6
//
// Copy 5-byte Indexes from a Leaf5 to 6-byte Indexes in a Leaf6.
// Pjp MUST be one of:  cJU_JPLEAF5 or cJU_JPIMMED_5_*.
// Return number of Indexes copied.
//
// Note:  By the time this function is called to compress a level-6 branch to a
// Leaf6, the branch has no narrow pointers under it, meaning only level-5
// objects are below it and must be handled here.

FUNCTION Word_t  j__udyLeaf5ToLeaf6(
	uint8_t * PLeaf6,	// destination uint8_t * Index part of leaf.
#ifdef JUDYL
	Pjv_t	  Pjv6,		// destination value part of leaf.
#endif
	Pjp_t	  Pjp,		// 5-byte-index object from which to copy.
	Word_t    MSByte,	// most-significant byte, prefix to each Index.
	Pvoid_t	  Pjpm)		// for global accounting.
{
	Word_t	  Pop1;		// Indexes in leaf.
JUDYLCODE(Pjv_t	  Pjv5;)	// source object value area.

	switch (JU_JPTYPE(Pjp))
	{


// JPLEAF5:

	case cJU_JPLEAF5:
	{
	    uint8_t * PLeaf5 = (uint8_t *) P_JLL(Pjp->jp_Addr);

	    Pop1 = JU_JPLEAF_POP0(Pjp) + 1; assert(Pop1);
	    j__udyCopy5to6(PLeaf6, PLeaf5, Pop1, MSByte);
#ifdef JUDYL
	    Pjv5 = JL_LEAF5VALUEAREA(PLeaf5, Pop1);
	    JU_COPYMEM(Pjv6, Pjv5, Pop1);
#endif
	    j__udyFreeJLL5((Pjll_t) (Pjp->jp_Addr), Pop1, Pjpm);
	    return(Pop1);
	}


// JPIMMED_5_01:
//
// Note:  jp_DcdPopO has 7 bytes of Index (all but most significant byte), so
// the assignment to PLeaf6[] truncates and MSByte is not needed.

	case cJU_JPIMMED_5_01:
	{
	    JU_COPY6_LONG_TO_PINDEX(PLeaf6, JU_JPDCDPOP0(Pjp));	// see above.
	    JUDYLCODE(Pjv6[0] = Pjp->jp_Addr;)
	    return(1);
	}


#ifdef JUDY1

// JPIMMED_5_0[2+]:

	case cJ1_JPIMMED_5_02:
	case cJ1_JPIMMED_5_03:
	{
	    uint8_t * PLeaf5 = (uint8_t *) (Pjp->jp_1Index);

	    Pop1 = JU_JPTYPE(Pjp) - cJ1_JPIMMED_5_02 + 2;
	    j__udyCopy5to6(PLeaf6, PLeaf5, Pop1, MSByte);
	    return(Pop1);
	}
#endif // JUDY1


// UNEXPECTED CASES, including JPNULL5, should be handled by caller:

	default: assert(FALSE); break;

	} // switch

	return(0);

} // j__udyLeaf5ToLeaf6()


// *****************************************************************************
// __ J U D Y   L E A F   6   T O   L E A F   7
//
// Copy 6-byte Indexes from a Leaf2 to 7-byte Indexes in a Leaf7.
// Pjp MUST be one of:  cJU_JPLEAF6 or cJU_JPIMMED_6_*.
// Return number of Indexes copied.
//
// Note:  By the time this function is called to compress a level-7 branch to a
// Leaf7, the branch has no narrow pointers under it, meaning only level-6
// objects are below it and must be handled here.

FUNCTION Word_t  j__udyLeaf6ToLeaf7(
	uint8_t * PLeaf7,	// destination "uint24_t *" Index part of leaf.
#ifdef JUDYL
	Pjv_t	  Pjv7,		// destination value part of leaf.
#endif
	Pjp_t	  Pjp,		// 6-byte-index object from which to copy.
	Word_t    MSByte,	// most-significant byte, prefix to each Index.
	Pvoid_t	  Pjpm)		// for global accounting.
{
	Word_t	  Pop1;		// Indexes in leaf.
JUDYLCODE(Pjv_t	  Pjv6;)	// source object value area.

	switch (JU_JPTYPE(Pjp))
	{


// JPLEAF6:

	case cJU_JPLEAF6:
	{
	    uint8_t * PLeaf6 = (uint8_t *) P_JLL(Pjp->jp_Addr);

	    Pop1 = JU_JPLEAF_POP0(Pjp) + 1;
	    j__udyCopy6to7(PLeaf7, PLeaf6, Pop1, MSByte);
#ifdef JUDYL
	    Pjv6 = JL_LEAF6VALUEAREA(PLeaf6, Pop1);
	    JU_COPYMEM(Pjv7, Pjv6, Pop1);
#endif
	    j__udyFreeJLL6((Pjll_t) (Pjp->jp_Addr), Pop1, Pjpm);
	    return(Pop1);
	}


// JPIMMED_6_01:
//
// Note:  jp_DcdPopO has 7 bytes of Index (all but most significant byte), so
// the "assignment" to PLeaf7[] is exact and MSByte is not needed.

	case cJU_JPIMMED_6_01:
	{
	    JU_COPY7_LONG_TO_PINDEX(PLeaf7, JU_JPDCDPOP0(Pjp));	// see above.
	    JUDYLCODE(Pjv7[0] = Pjp->jp_Addr;)
	    return(1);
	}


#ifdef JUDY1

// JPIMMED_6_02:

	case cJ1_JPIMMED_6_02:
	{
	    uint8_t * PLeaf6 = (uint8_t *) (Pjp->jp_1Index);

	    j__udyCopy6to7(PLeaf7, PLeaf6, /* Pop1 = */ 2, MSByte);
	    return(2);
	}
#endif // JUDY1


// UNEXPECTED CASES, including JPNULL6, should be handled by caller:

	default: assert(FALSE); break;

	} // switch

	return(0);

} // j__udyLeaf6ToLeaf7()

#endif // JU_64BIT


#ifndef JU_64BIT // 32-bit version first

// ****************************************************************************
// __ J U D Y   L E A F   3   T O   L E A F   W
//
// Copy 3-byte Indexes from a Leaf3 to 4-byte Indexes in a LeafW.  Pjp MUST be
// one of:  cJU_JPLEAF3 or cJU_JPIMMED_3_*.  Return number of Indexes copied.
//
// Note:  By the time this function is called to compress a level-L branch to a
// LeafW, the branch has no narrow pointers under it, meaning only level-3
// objects are below it and must be handled here.

FUNCTION Word_t  j__udyLeaf3ToLeafW(
	Pjlw_t	Pjlw,		// destination Index part of leaf.
#ifdef JUDYL
	Pjv_t	PjvW,		// destination value part of leaf.
#endif
	Pjp_t	Pjp,		// 3-byte-index object from which to copy.
	Word_t	MSByte,		// most-significant byte, prefix to each Index.
	Pvoid_t	Pjpm)		// for global accounting.
{
	Word_t	Pop1;		// Indexes in leaf.
JUDYLCODE(Pjv_t Pjv3;)		// source object value area.

	switch (JU_JPTYPE(Pjp))
	{


// JPLEAF3:

	case cJU_JPLEAF3:
	{
	    uint8_t * PLeaf3 = (uint8_t *) P_JLL(Pjp->jp_Addr);

	    Pop1 = JU_JPLEAF_POP0(Pjp) + 1;
	    j__udyCopy3toW((PWord_t) Pjlw, PLeaf3, Pop1, MSByte);
#ifdef JUDYL
	    Pjv3 = JL_LEAF3VALUEAREA(PLeaf3, Pop1);
	    JU_COPYMEM(PjvW, Pjv3, Pop1);
#endif
	    j__udyFreeJLL3((Pjll_t) (Pjp->jp_Addr), Pop1, Pjpm);
	    return(Pop1);
	}


// JPIMMED_3_01:
//
// Note:  jp_DcdPopO has 3 bytes of Index (all but most significant byte), and
// MSByte must be ord in.

	case cJU_JPIMMED_3_01:
	{
	    Pjlw[0] = MSByte | JU_JPDCDPOP0(Pjp);		// see above.
	    JUDYLCODE(PjvW[0] = Pjp->jp_Addr;)
	    return(1);
	}


#ifdef JUDY1

// JPIMMED_3_02:

	case cJU_JPIMMED_3_02:
	{
	    uint8_t * PLeaf3 = (uint8_t *) (Pjp->jp_1Index);

	    j__udyCopy3toW((PWord_t) Pjlw, PLeaf3, /* Pop1 = */ 2, MSByte);
	    return(2);
	}
#endif // JUDY1


// UNEXPECTED CASES, including JPNULL3, should be handled by caller:

	default: assert(FALSE); break;

	} // switch

	return(0);

} // j__udyLeaf3ToLeafW()


#else // JU_64BIT


// ****************************************************************************
// __ J U D Y   L E A F   7   T O   L E A F   W
//
// Copy 7-byte Indexes from a Leaf7 to 8-byte Indexes in a LeafW.
// Pjp MUST be one of:  cJU_JPLEAF7 or cJU_JPIMMED_7_*.
// Return number of Indexes copied.
//
// Note:  By the time this function is called to compress a level-L branch to a
// LeafW, the branch has no narrow pointers under it, meaning only level-7
// objects are below it and must be handled here.

FUNCTION Word_t  j__udyLeaf7ToLeafW(
	Pjlw_t	Pjlw,		// destination Index part of leaf.
#ifdef JUDYL
	Pjv_t	PjvW,		// destination value part of leaf.
#endif
	Pjp_t	Pjp,		// 7-byte-index object from which to copy.
	Word_t	MSByte,		// most-significant byte, prefix to each Index.
	Pvoid_t	Pjpm)		// for global accounting.
{
	Word_t	Pop1;		// Indexes in leaf.
JUDYLCODE(Pjv_t	Pjv7;)		// source object value area.

	switch (JU_JPTYPE(Pjp))
	{


// JPLEAF7:

	case cJU_JPLEAF7:
	{
	    uint8_t * PLeaf7 = (uint8_t *) P_JLL(Pjp->jp_Addr);

	    Pop1 = JU_JPLEAF_POP0(Pjp) + 1;
	    j__udyCopy7toW((PWord_t) Pjlw, PLeaf7, Pop1, MSByte);
#ifdef JUDYL
	    Pjv7 = JL_LEAF7VALUEAREA(PLeaf7, Pop1);
	    JU_COPYMEM(PjvW, Pjv7, Pop1);
#endif
	    j__udyFreeJLL7((Pjll_t) (Pjp->jp_Addr), Pop1, Pjpm);
	    return(Pop1);
	}


// JPIMMED_7_01:
//
// Note:  jp_DcdPopO has 7 bytes of Index (all but most significant byte), and
// MSByte must be ord in.

	case cJU_JPIMMED_7_01:
	{
	    Pjlw[0] = MSByte | JU_JPDCDPOP0(Pjp);		// see above.
	    JUDYLCODE(PjvW[0] = Pjp->jp_Addr;)
	    return(1);
	}


#ifdef JUDY1

// JPIMMED_7_02:

	case cJ1_JPIMMED_7_02:
	{
	    uint8_t * PLeaf7 = (uint8_t *) (Pjp->jp_1Index);

	    j__udyCopy7toW((PWord_t) Pjlw, PLeaf7, /* Pop1 = */ 2, MSByte);
	    return(2);
	}
#endif


// UNEXPECTED CASES, including JPNULL7, should be handled by caller:

	default: assert(FALSE); break;

	} // switch

	return(0);

} // j__udyLeaf7ToLeafW()

#endif // JU_64BIT
