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

// @(#) $Revision: 4.7 $ $Source: /judy/src/JudyCommon/JudyMemActive.c $
//
// Return number of bytes of memory used to support a Judy1/L array.
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

FUNCTION static Word_t j__udyGetMemActive(Pjp_t);


// ****************************************************************************
// J U D Y   1   M E M   A C T I V E
// J U D Y   L   M E M   A C T I V E

#ifdef JUDY1
FUNCTION Word_t Judy1MemActive
#else
FUNCTION Word_t JudyLMemActive
#endif
        (
	Pcvoid_t PArray	        // from which to retrieve.
        )
{
	if (PArray == (Pcvoid_t)NULL) return(0);

	if (JU_LEAFW_POP0(PArray) < cJU_LEAFW_MAXPOP1) // must be a LEAFW
        {
	    Pjlw_t Pjlw = P_JLW(PArray);	// first word of leaf.
            Word_t Words = Pjlw[0] + 1;		// population.
#ifdef JUDY1
            return((Words + 1) * sizeof(Word_t));
#else
            return(((Words * 2) + 1) * sizeof(Word_t));
#endif
        }
	else
	{
	    Pjpm_t Pjpm = P_JPM(PArray);
	    return(j__udyGetMemActive(&Pjpm->jpm_JP) + sizeof(jpm_t));
	}

} // JudyMemActive()


// ****************************************************************************
// __ J U D Y   G E T   M E M   A C T I V E

FUNCTION static Word_t j__udyGetMemActive(
	Pjp_t  Pjp)		// top of subtree.
{
	Word_t offset;		// in a branch.
	Word_t Bytes = 0;	// actual bytes used at this level.
	Word_t IdxSz;		// bytes per index in leaves

	switch (JU_JPTYPE(Pjp))
	{

	case cJU_JPBRANCH_L2:
	case cJU_JPBRANCH_L3:
#ifdef JU_64BIT
	case cJU_JPBRANCH_L4:
	case cJU_JPBRANCH_L5:
	case cJU_JPBRANCH_L6:
	case cJU_JPBRANCH_L7:
#endif
	case cJU_JPBRANCH_L:
	{
	    Pjbl_t Pjbl = P_JBL(Pjp->jp_Addr);

	    for (offset = 0; offset < (Pjbl->jbl_NumJPs); ++offset)
	        Bytes += j__udyGetMemActive((Pjbl->jbl_jp) + offset);

	    return(Bytes + sizeof(jbl_t));
	}

	case cJU_JPBRANCH_B2:
	case cJU_JPBRANCH_B3:
#ifdef JU_64BIT
	case cJU_JPBRANCH_B4:
	case cJU_JPBRANCH_B5:
	case cJU_JPBRANCH_B6:
	case cJU_JPBRANCH_B7:
#endif
	case cJU_JPBRANCH_B:
	{
	    Word_t subexp;
	    Word_t jpcount;
	    Pjbb_t Pjbb = P_JBB(Pjp->jp_Addr);

	    for (subexp = 0; subexp < cJU_NUMSUBEXPB; ++subexp)
	    {
	        jpcount = j__udyCountBitsB(JU_JBB_BITMAP(Pjbb, subexp));
                Bytes  += jpcount * sizeof(jp_t);

		for (offset = 0; offset < jpcount; ++offset)
		{
		    Bytes += j__udyGetMemActive(P_JP(JU_JBB_PJP(Pjbb, subexp))
			   + offset);
		}
	    }

	    return(Bytes + sizeof(jbb_t));
	}

	case cJU_JPBRANCH_U2:
	case cJU_JPBRANCH_U3:
#ifdef JU_64BIT
	case cJU_JPBRANCH_U4:
	case cJU_JPBRANCH_U5:
	case cJU_JPBRANCH_U6:
	case cJU_JPBRANCH_U7:
#endif
	case cJU_JPBRANCH_U:
        {
	    Pjbu_t Pjbu = P_JBU(Pjp->jp_Addr);

            for (offset = 0; offset < cJU_BRANCHUNUMJPS; ++offset)
	    {
		if (((Pjbu->jbu_jp[offset].jp_Type) >= cJU_JPNULL1)
		 && ((Pjbu->jbu_jp[offset].jp_Type) <= cJU_JPNULLMAX))
		{
		    continue;		// skip null JP to save time.
		}

	        Bytes += j__udyGetMemActive(Pjbu->jbu_jp + offset);
	    }

	    return(Bytes + sizeof(jbu_t));
        }


// -- Cases below here terminate and do not recurse. --

#if (defined(JUDYL) || (! defined(JU_64BIT)))
        case cJU_JPLEAF1: IdxSz = 1; goto LeafWords;
#endif
	case cJU_JPLEAF2: IdxSz = 2; goto LeafWords;
	case cJU_JPLEAF3: IdxSz = 3; goto LeafWords;
#ifdef JU_64BIT
	case cJU_JPLEAF4: IdxSz = 4; goto LeafWords;
	case cJU_JPLEAF5: IdxSz = 5; goto LeafWords;
	case cJU_JPLEAF6: IdxSz = 6; goto LeafWords;
	case cJU_JPLEAF7: IdxSz = 7; goto LeafWords;
#endif
LeafWords:

#ifdef JUDY1
            return(IdxSz * (JU_JPLEAF_POP0(Pjp) + 1));
#else
            return((IdxSz + sizeof(Word_t))
		 * (JU_JPLEAF_POP0(Pjp) + 1));
#endif
	case cJU_JPLEAF_B1:
	{
#ifdef JUDY1
            return(sizeof(jlb_t));
#else
            Bytes = (JU_JPLEAF_POP0(Pjp) + 1) * sizeof(Word_t);

	    return(Bytes + sizeof(jlb_t));
#endif
	}

	JUDY1CODE(case cJ1_JPFULLPOPU1: return(0);)

#ifdef JUDY1
#define J__Mpy 0
#else
#define J__Mpy sizeof(Word_t)
#endif

	case cJU_JPIMMED_1_01:	return(0);
	case cJU_JPIMMED_2_01:	return(0);
	case cJU_JPIMMED_3_01:	return(0);
#ifdef JU_64BIT
	case cJU_JPIMMED_4_01:	return(0);
	case cJU_JPIMMED_5_01:	return(0);
	case cJU_JPIMMED_6_01:	return(0);
	case cJU_JPIMMED_7_01:	return(0);
#endif

	case cJU_JPIMMED_1_02:	return(J__Mpy * 2);
	case cJU_JPIMMED_1_03:	return(J__Mpy * 3);
#if (defined(JUDY1) || defined(JU_64BIT))
	case cJU_JPIMMED_1_04:	return(J__Mpy * 4);
	case cJU_JPIMMED_1_05:	return(J__Mpy * 5);
	case cJU_JPIMMED_1_06:	return(J__Mpy * 6);
	case cJU_JPIMMED_1_07:	return(J__Mpy * 7);
#endif
#if (defined(JUDY1) && defined(JU_64BIT))
	case cJ1_JPIMMED_1_08:	return(0);
	case cJ1_JPIMMED_1_09:	return(0);
	case cJ1_JPIMMED_1_10:	return(0);
	case cJ1_JPIMMED_1_11:	return(0);
	case cJ1_JPIMMED_1_12:	return(0);
	case cJ1_JPIMMED_1_13:	return(0);
	case cJ1_JPIMMED_1_14:	return(0);
	case cJ1_JPIMMED_1_15:	return(0);
#endif

#if (defined(JUDY1) || defined(JU_64BIT))
	case cJU_JPIMMED_2_02:	return(J__Mpy * 2);
	case cJU_JPIMMED_2_03:	return(J__Mpy * 3);
#endif
#if (defined(JUDY1) && defined(JU_64BIT))
	case cJ1_JPIMMED_2_04:	return(0);
	case cJ1_JPIMMED_2_05:	return(0);
	case cJ1_JPIMMED_2_06:	return(0);
	case cJ1_JPIMMED_2_07:	return(0);
#endif

#if (defined(JUDY1) || defined(JU_64BIT))
	case cJU_JPIMMED_3_02:	return(J__Mpy * 2);
#endif
#if (defined(JUDY1) && defined(JU_64BIT))
	case cJ1_JPIMMED_3_03:	return(0);
	case cJ1_JPIMMED_3_04:	return(0);
	case cJ1_JPIMMED_3_05:	return(0);

	case cJ1_JPIMMED_4_02:	return(0);
	case cJ1_JPIMMED_4_03:	return(0);
	case cJ1_JPIMMED_5_02:	return(0);
	case cJ1_JPIMMED_5_03:	return(0);
	case cJ1_JPIMMED_6_02:	return(0);
	case cJ1_JPIMMED_7_02:	return(0);
#endif

	} // switch (JU_JPTYPE(Pjp))

	return(0);			// to make some compilers happy.

} // j__udyGetMemActive()
