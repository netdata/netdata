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

// @(#) $Revision: 4.17 $ $Source: /judy/src/JudyCommon/JudyInsertBranch.c $

// BranchL insertion functions for Judy1 and JudyL.
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

extern int j__udyCreateBranchL(Pjp_t, Pjp_t, uint8_t *, Word_t, Pvoid_t);


// ****************************************************************************
// __ J U D Y   I N S E R T   B R A N C H
//
// Insert 2-element BranchL in between Pjp and Pjp->jp_Addr.
//
// Return -1 if out of memory, otherwise return 1.

FUNCTION int j__udyInsertBranch(
	Pjp_t	Pjp,		// JP containing narrow pointer.
	Word_t	Index,		// outlier to Pjp.
	Word_t	BranchLevel,	// of what JP points to, mapped from JP type.
	Pjpm_t	Pjpm)		// for global accounting.
{
	jp_t	JP2 [2];
	jp_t	JP;
	Pjp_t	PjpNull;
	Word_t	XorExp;
	Word_t	Inew, Iold;
	Word_t  DCDMask;	// initially for original BranchLevel.
	int	Ret;
	uint8_t	Exp2[2];
	uint8_t	DecodeByteN, DecodeByteO;

//	Get the current mask for the DCD digits:

	DCDMask = cJU_DCDMASK(BranchLevel);

//	Obtain Dcd bits that differ between Index and JP, shifted so the
//	digit for BranchLevel is the LSB:

	XorExp = ((Index ^ JU_JPDCDPOP0(Pjp)) & (cJU_ALLONES >> cJU_BITSPERBYTE))
	       >> (BranchLevel * cJU_BITSPERBYTE);
	assert(XorExp);		// Index must be an outlier.

//	Count levels between object under narrow pointer and the level at which
//	the outlier diverges from it, which is always at least initial
//	BranchLevel + 1, to end up with the level (JP type) at which to insert
//	the new intervening BranchL:

	do { ++BranchLevel; } while ((XorExp >>= cJU_BITSPERBYTE));
	assert((BranchLevel > 1) && (BranchLevel < cJU_ROOTSTATE));

//	Get the MSB (highest digit) that differs between the old expanse and
//	the new Index to insert:

	DecodeByteO = JU_DIGITATSTATE(JU_JPDCDPOP0(Pjp), BranchLevel);
	DecodeByteN = JU_DIGITATSTATE(Index,	         BranchLevel);

	assert(DecodeByteO != DecodeByteN);

//	Determine sorted order for old expanse and new Index digits:

	if (DecodeByteN > DecodeByteO)	{ Iold = 0; Inew = 1; }
	else				{ Iold = 1; Inew = 0; }

//	Copy old JP into staging area for new Branch
	JP2 [Iold] = *Pjp;
	Exp2[Iold] = DecodeByteO;
	Exp2[Inew] = DecodeByteN;

//	Create a 2 Expanse Linear branch
//
//	Note: Pjp->jp_Addr is set by j__udyCreateBranchL()

	Ret = j__udyCreateBranchL(Pjp, JP2, Exp2, 2, Pjpm);
	if (Ret == -1) return(-1);

//	Get Pjp to the NULL of where to do insert
	PjpNull	= ((P_JBL(Pjp->jp_Addr))->jbl_jp) + Inew;

//	Convert to a cJU_JPIMMED_*_01 at the correct level:
//	Build JP and set type below to: cJU_JPIMMED_X_01
        JU_JPSETADT(PjpNull, 0, Index, cJU_JPIMMED_1_01 - 2 + BranchLevel);

//	Return pointer to Value area in cJU_JPIMMED_X_01
	JUDYLCODE(Pjpm->jpm_PValue = (Pjv_t) PjpNull;)

//	The old JP now points to a BranchL that is at higher level.  Therefore
//	it contains excess DCD bits (in the least significant position) that
//	must be removed (zeroed); that is, they become part of the Pop0
//	subfield.  Note that the remaining (lower) bytes in the Pop0 field do
//	not change.
//
//	Take from the old DCDMask, which went "down" to a lower BranchLevel,
//	and zero any high bits that are still in the mask at the new, higher
//	BranchLevel; then use this mask to zero the bits in jp_DcdPopO:

//	Set old JP to a BranchL at correct level

	Pjp->jp_Type = cJU_JPBRANCH_L2 - 2 + BranchLevel;
	DCDMask		^= cJU_DCDMASK(BranchLevel);
	DCDMask		 = ~DCDMask & JU_JPDCDPOP0(Pjp);
        JP = *Pjp;
        JU_JPSETADT(Pjp, JP.jp_Addr, DCDMask, JP.jp_Type);

	return(1);

} // j__udyInsertBranch()
