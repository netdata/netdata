#ifndef _JUDYPRIVATE1L_INCLUDED
#define	_JUDYPRIVATE1L_INCLUDED
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

// @(#) $Revision: 4.31 $ $Source: /judy/src/JudyCommon/JudyPrivate1L.h $

// ****************************************************************************
// Declare common cJU_* names for JP Types that occur in both Judy1 and JudyL,
// for use by code that ifdefs JUDY1 and JUDYL.  Only JP Types common to both
// Judy1 and JudyL are #defined here with equivalent cJU_* names.  JP Types
// unique to only Judy1 or JudyL are listed in comments, so the type lists
// match the Judy1.h and JudyL.h files.
//
// This file also defines cJU_* for other JP-related constants and functions
// that some shared JUDY1/JUDYL code finds handy.
//
// At least in principle this file should be included AFTER Judy1.h or JudyL.h.
//
// WARNING:  This file must be kept consistent with the enums in Judy1.h and
// JudyL.h.
//
// TBD:  You might think, why not define common cJU_* enums in, say,
// JudyPrivate.h, and then inherit them into superset enums in Judy1.h and
// JudyL.h?  The problem is that the enum lists for each class (cJ1_* and
// cJL_*) must be numerically "packed" into the correct order, for two reasons:
// (1) allow the compiler to generate "tight" switch statements with no wasted
// slots (although this is not very big), and (2) allow calculations using the
// enum values, although this is also not an issue if the calculations are only
// within each cJ*_JPIMMED_*_* class and the members are packed within the
// class.

#ifdef JUDY1

#define	cJU_JRPNULL		cJ1_JRPNULL
#define	cJU_JPNULL1		cJ1_JPNULL1
#define	cJU_JPNULL2		cJ1_JPNULL2
#define	cJU_JPNULL3		cJ1_JPNULL3
#ifdef JU_64BIT
#define	cJU_JPNULL4		cJ1_JPNULL4
#define	cJU_JPNULL5		cJ1_JPNULL5
#define	cJU_JPNULL6		cJ1_JPNULL6
#define	cJU_JPNULL7		cJ1_JPNULL7
#endif
#define	cJU_JPNULLMAX		cJ1_JPNULLMAX
#define	cJU_JPBRANCH_L2		cJ1_JPBRANCH_L2
#define	cJU_JPBRANCH_L3		cJ1_JPBRANCH_L3
#ifdef JU_64BIT
#define	cJU_JPBRANCH_L4		cJ1_JPBRANCH_L4
#define	cJU_JPBRANCH_L5		cJ1_JPBRANCH_L5
#define	cJU_JPBRANCH_L6		cJ1_JPBRANCH_L6
#define	cJU_JPBRANCH_L7		cJ1_JPBRANCH_L7
#endif
#define	cJU_JPBRANCH_L		cJ1_JPBRANCH_L
#define	j__U_BranchBJPPopToWords j__1_BranchBJPPopToWords
#define	cJU_JPBRANCH_B2		cJ1_JPBRANCH_B2
#define	cJU_JPBRANCH_B3		cJ1_JPBRANCH_B3
#ifdef JU_64BIT
#define	cJU_JPBRANCH_B4		cJ1_JPBRANCH_B4
#define	cJU_JPBRANCH_B5		cJ1_JPBRANCH_B5
#define	cJU_JPBRANCH_B6		cJ1_JPBRANCH_B6
#define	cJU_JPBRANCH_B7		cJ1_JPBRANCH_B7
#endif
#define	cJU_JPBRANCH_B		cJ1_JPBRANCH_B
#define	cJU_JPBRANCH_U2		cJ1_JPBRANCH_U2
#define	cJU_JPBRANCH_U3		cJ1_JPBRANCH_U3
#ifdef JU_64BIT
#define	cJU_JPBRANCH_U4		cJ1_JPBRANCH_U4
#define	cJU_JPBRANCH_U5		cJ1_JPBRANCH_U5
#define	cJU_JPBRANCH_U6		cJ1_JPBRANCH_U6
#define	cJU_JPBRANCH_U7		cJ1_JPBRANCH_U7
#endif
#define	cJU_JPBRANCH_U		cJ1_JPBRANCH_U
#ifndef JU_64BIT
#define	cJU_JPLEAF1		cJ1_JPLEAF1
#endif
#define	cJU_JPLEAF2		cJ1_JPLEAF2
#define	cJU_JPLEAF3		cJ1_JPLEAF3
#ifdef JU_64BIT
#define	cJU_JPLEAF4		cJ1_JPLEAF4
#define	cJU_JPLEAF5		cJ1_JPLEAF5
#define	cJU_JPLEAF6		cJ1_JPLEAF6
#define	cJU_JPLEAF7		cJ1_JPLEAF7
#endif
#define	cJU_JPLEAF_B1		cJ1_JPLEAF_B1
//				cJ1_JPFULLPOPU1
#define	cJU_JPIMMED_1_01	cJ1_JPIMMED_1_01
#define	cJU_JPIMMED_2_01	cJ1_JPIMMED_2_01
#define	cJU_JPIMMED_3_01	cJ1_JPIMMED_3_01
#ifdef JU_64BIT
#define	cJU_JPIMMED_4_01	cJ1_JPIMMED_4_01
#define	cJU_JPIMMED_5_01	cJ1_JPIMMED_5_01
#define	cJU_JPIMMED_6_01	cJ1_JPIMMED_6_01
#define	cJU_JPIMMED_7_01	cJ1_JPIMMED_7_01
#endif
#define	cJU_JPIMMED_1_02	cJ1_JPIMMED_1_02
#define	cJU_JPIMMED_1_03	cJ1_JPIMMED_1_03
#define	cJU_JPIMMED_1_04	cJ1_JPIMMED_1_04
#define	cJU_JPIMMED_1_05	cJ1_JPIMMED_1_05
#define	cJU_JPIMMED_1_06	cJ1_JPIMMED_1_06
#define	cJU_JPIMMED_1_07	cJ1_JPIMMED_1_07
#ifdef JU_64BIT
//				cJ1_JPIMMED_1_08
//				cJ1_JPIMMED_1_09
//				cJ1_JPIMMED_1_10
//				cJ1_JPIMMED_1_11
//				cJ1_JPIMMED_1_12
//				cJ1_JPIMMED_1_13
//				cJ1_JPIMMED_1_14
//				cJ1_JPIMMED_1_15
#endif
#define	cJU_JPIMMED_2_02	cJ1_JPIMMED_2_02
#define	cJU_JPIMMED_2_03	cJ1_JPIMMED_2_03
#ifdef JU_64BIT
//				cJ1_JPIMMED_2_04
//				cJ1_JPIMMED_2_05
//				cJ1_JPIMMED_2_06
//				cJ1_JPIMMED_2_07
#endif
#define	cJU_JPIMMED_3_02	cJ1_JPIMMED_3_02
#ifdef JU_64BIT
//				cJ1_JPIMMED_3_03
//				cJ1_JPIMMED_3_04
//				cJ1_JPIMMED_3_05
//				cJ1_JPIMMED_4_02
//				cJ1_JPIMMED_4_03
//				cJ1_JPIMMED_5_02
//				cJ1_JPIMMED_5_03
//				cJ1_JPIMMED_6_02
//				cJ1_JPIMMED_7_02
#endif
#define	cJU_JPIMMED_CAP		cJ1_JPIMMED_CAP

#else // JUDYL ****************************************************************

#define	cJU_JRPNULL		cJL_JRPNULL
#define	cJU_JPNULL1		cJL_JPNULL1
#define	cJU_JPNULL2		cJL_JPNULL2
#define	cJU_JPNULL3		cJL_JPNULL3
#ifdef JU_64BIT
#define	cJU_JPNULL4		cJL_JPNULL4
#define	cJU_JPNULL5		cJL_JPNULL5
#define	cJU_JPNULL6		cJL_JPNULL6
#define	cJU_JPNULL7		cJL_JPNULL7
#endif
#define	cJU_JPNULLMAX		cJL_JPNULLMAX
#define	cJU_JPBRANCH_L2		cJL_JPBRANCH_L2
#define	cJU_JPBRANCH_L3		cJL_JPBRANCH_L3
#ifdef JU_64BIT
#define	cJU_JPBRANCH_L4		cJL_JPBRANCH_L4
#define	cJU_JPBRANCH_L5		cJL_JPBRANCH_L5
#define	cJU_JPBRANCH_L6		cJL_JPBRANCH_L6
#define	cJU_JPBRANCH_L7		cJL_JPBRANCH_L7
#endif
#define	cJU_JPBRANCH_L		cJL_JPBRANCH_L
#define	j__U_BranchBJPPopToWords j__L_BranchBJPPopToWords
#define	cJU_JPBRANCH_B2		cJL_JPBRANCH_B2
#define	cJU_JPBRANCH_B3		cJL_JPBRANCH_B3
#ifdef JU_64BIT
#define	cJU_JPBRANCH_B4		cJL_JPBRANCH_B4
#define	cJU_JPBRANCH_B5		cJL_JPBRANCH_B5
#define	cJU_JPBRANCH_B6		cJL_JPBRANCH_B6
#define	cJU_JPBRANCH_B7		cJL_JPBRANCH_B7
#endif
#define	cJU_JPBRANCH_B		cJL_JPBRANCH_B
#define	cJU_JPBRANCH_U2		cJL_JPBRANCH_U2
#define	cJU_JPBRANCH_U3		cJL_JPBRANCH_U3
#ifdef JU_64BIT
#define	cJU_JPBRANCH_U4		cJL_JPBRANCH_U4
#define	cJU_JPBRANCH_U5		cJL_JPBRANCH_U5
#define	cJU_JPBRANCH_U6		cJL_JPBRANCH_U6
#define	cJU_JPBRANCH_U7		cJL_JPBRANCH_U7
#endif
#define	cJU_JPBRANCH_U		cJL_JPBRANCH_U
#define	cJU_JPLEAF1		cJL_JPLEAF1
#define	cJU_JPLEAF2		cJL_JPLEAF2
#define	cJU_JPLEAF3		cJL_JPLEAF3
#ifdef JU_64BIT
#define	cJU_JPLEAF4		cJL_JPLEAF4
#define	cJU_JPLEAF5		cJL_JPLEAF5
#define	cJU_JPLEAF6		cJL_JPLEAF6
#define	cJU_JPLEAF7		cJL_JPLEAF7
#endif
#define	cJU_JPLEAF_B1		cJL_JPLEAF_B1
#define	cJU_JPIMMED_1_01	cJL_JPIMMED_1_01
#define	cJU_JPIMMED_2_01	cJL_JPIMMED_2_01
#define	cJU_JPIMMED_3_01	cJL_JPIMMED_3_01
#ifdef JU_64BIT
#define	cJU_JPIMMED_4_01	cJL_JPIMMED_4_01
#define	cJU_JPIMMED_5_01	cJL_JPIMMED_5_01
#define	cJU_JPIMMED_6_01	cJL_JPIMMED_6_01
#define	cJU_JPIMMED_7_01	cJL_JPIMMED_7_01
#endif
#define	cJU_JPIMMED_1_02	cJL_JPIMMED_1_02
#define	cJU_JPIMMED_1_03	cJL_JPIMMED_1_03
#ifdef JU_64BIT
#define	cJU_JPIMMED_1_04	cJL_JPIMMED_1_04
#define	cJU_JPIMMED_1_05	cJL_JPIMMED_1_05
#define	cJU_JPIMMED_1_06	cJL_JPIMMED_1_06
#define	cJU_JPIMMED_1_07	cJL_JPIMMED_1_07
#define	cJU_JPIMMED_2_02	cJL_JPIMMED_2_02
#define	cJU_JPIMMED_2_03	cJL_JPIMMED_2_03
#define	cJU_JPIMMED_3_02	cJL_JPIMMED_3_02
#endif
#define	cJU_JPIMMED_CAP		cJL_JPIMMED_CAP

#endif // JUDYL


// ****************************************************************************
// cJU*_ other than JP types:

#ifdef JUDY1

#define	cJU_LEAFW_MAXPOP1	cJ1_LEAFW_MAXPOP1
#ifndef JU_64BIT
#define	cJU_LEAF1_MAXPOP1	cJ1_LEAF1_MAXPOP1
#endif
#define	cJU_LEAF2_MAXPOP1	cJ1_LEAF2_MAXPOP1
#define	cJU_LEAF3_MAXPOP1	cJ1_LEAF3_MAXPOP1
#ifdef JU_64BIT
#define	cJU_LEAF4_MAXPOP1	cJ1_LEAF4_MAXPOP1
#define	cJU_LEAF5_MAXPOP1	cJ1_LEAF5_MAXPOP1
#define	cJU_LEAF6_MAXPOP1	cJ1_LEAF6_MAXPOP1
#define	cJU_LEAF7_MAXPOP1	cJ1_LEAF7_MAXPOP1
#endif
#define	cJU_IMMED1_MAXPOP1	cJ1_IMMED1_MAXPOP1
#define	cJU_IMMED2_MAXPOP1	cJ1_IMMED2_MAXPOP1
#define	cJU_IMMED3_MAXPOP1	cJ1_IMMED3_MAXPOP1
#ifdef JU_64BIT
#define	cJU_IMMED4_MAXPOP1	cJ1_IMMED4_MAXPOP1
#define	cJU_IMMED5_MAXPOP1	cJ1_IMMED5_MAXPOP1
#define	cJU_IMMED6_MAXPOP1	cJ1_IMMED6_MAXPOP1
#define	cJU_IMMED7_MAXPOP1	cJ1_IMMED7_MAXPOP1
#endif

#define	JU_LEAF1POPTOWORDS(Pop1)	J1_LEAF1POPTOWORDS(Pop1)
#define	JU_LEAF2POPTOWORDS(Pop1)	J1_LEAF2POPTOWORDS(Pop1)
#define	JU_LEAF3POPTOWORDS(Pop1)	J1_LEAF3POPTOWORDS(Pop1)
#ifdef JU_64BIT
#define	JU_LEAF4POPTOWORDS(Pop1)	J1_LEAF4POPTOWORDS(Pop1)
#define	JU_LEAF5POPTOWORDS(Pop1)	J1_LEAF5POPTOWORDS(Pop1)
#define	JU_LEAF6POPTOWORDS(Pop1)	J1_LEAF6POPTOWORDS(Pop1)
#define	JU_LEAF7POPTOWORDS(Pop1)	J1_LEAF7POPTOWORDS(Pop1)
#endif
#define	JU_LEAFWPOPTOWORDS(Pop1)	J1_LEAFWPOPTOWORDS(Pop1)

#ifndef JU_64BIT
#define	JU_LEAF1GROWINPLACE(Pop1)	J1_LEAF1GROWINPLACE(Pop1)
#endif
#define	JU_LEAF2GROWINPLACE(Pop1)	J1_LEAF2GROWINPLACE(Pop1)
#define	JU_LEAF3GROWINPLACE(Pop1)	J1_LEAF3GROWINPLACE(Pop1)
#ifdef JU_64BIT
#define	JU_LEAF4GROWINPLACE(Pop1)	J1_LEAF4GROWINPLACE(Pop1)
#define	JU_LEAF5GROWINPLACE(Pop1)	J1_LEAF5GROWINPLACE(Pop1)
#define	JU_LEAF6GROWINPLACE(Pop1)	J1_LEAF6GROWINPLACE(Pop1)
#define	JU_LEAF7GROWINPLACE(Pop1)	J1_LEAF7GROWINPLACE(Pop1)
#endif
#define	JU_LEAFWGROWINPLACE(Pop1)	J1_LEAFWGROWINPLACE(Pop1)

#define	j__udyCreateBranchL	j__udy1CreateBranchL
#define	j__udyCreateBranchB	j__udy1CreateBranchB
#define	j__udyCreateBranchU	j__udy1CreateBranchU
#define	j__udyCascade1		j__udy1Cascade1
#define	j__udyCascade2		j__udy1Cascade2
#define	j__udyCascade3		j__udy1Cascade3
#ifdef JU_64BIT
#define	j__udyCascade4		j__udy1Cascade4
#define	j__udyCascade5		j__udy1Cascade5
#define	j__udyCascade6		j__udy1Cascade6
#define	j__udyCascade7		j__udy1Cascade7
#endif
#define	j__udyCascadeL		j__udy1CascadeL
#define	j__udyInsertBranch	j__udy1InsertBranch

#define	j__udyBranchBToBranchL	j__udy1BranchBToBranchL
#ifndef JU_64BIT
#define	j__udyLeafB1ToLeaf1	j__udy1LeafB1ToLeaf1
#endif
#define	j__udyLeaf1ToLeaf2	j__udy1Leaf1ToLeaf2
#define	j__udyLeaf2ToLeaf3	j__udy1Leaf2ToLeaf3
#ifndef JU_64BIT
#define	j__udyLeaf3ToLeafW	j__udy1Leaf3ToLeafW
#else
#define	j__udyLeaf3ToLeaf4	j__udy1Leaf3ToLeaf4
#define	j__udyLeaf4ToLeaf5	j__udy1Leaf4ToLeaf5
#define	j__udyLeaf5ToLeaf6	j__udy1Leaf5ToLeaf6
#define	j__udyLeaf6ToLeaf7	j__udy1Leaf6ToLeaf7
#define	j__udyLeaf7ToLeafW	j__udy1Leaf7ToLeafW
#endif

#define	jpm_t			j1pm_t
#define	Pjpm_t			Pj1pm_t

#define	jlb_t			j1lb_t
#define	Pjlb_t			Pj1lb_t

#define	JU_JLB_BITMAP		J1_JLB_BITMAP

#define	j__udyAllocJPM		j__udy1AllocJ1PM
#define	j__udyAllocJBL		j__udy1AllocJBL
#define	j__udyAllocJBB		j__udy1AllocJBB
#define	j__udyAllocJBBJP	j__udy1AllocJBBJP
#define	j__udyAllocJBU		j__udy1AllocJBU
#ifndef JU_64BIT
#define	j__udyAllocJLL1		j__udy1AllocJLL1
#endif
#define	j__udyAllocJLL2		j__udy1AllocJLL2
#define	j__udyAllocJLL3		j__udy1AllocJLL3
#ifdef JU_64BIT
#define	j__udyAllocJLL4		j__udy1AllocJLL4
#define	j__udyAllocJLL5		j__udy1AllocJLL5
#define	j__udyAllocJLL6		j__udy1AllocJLL6
#define	j__udyAllocJLL7		j__udy1AllocJLL7
#endif
#define	j__udyAllocJLW		j__udy1AllocJLW
#define	j__udyAllocJLB1		j__udy1AllocJLB1
#define	j__udyFreeJPM		j__udy1FreeJ1PM
#define	j__udyFreeJBL		j__udy1FreeJBL
#define	j__udyFreeJBB		j__udy1FreeJBB
#define	j__udyFreeJBBJP		j__udy1FreeJBBJP
#define	j__udyFreeJBU		j__udy1FreeJBU
#ifndef JU_64BIT
#define	j__udyFreeJLL1		j__udy1FreeJLL1
#endif
#define	j__udyFreeJLL2		j__udy1FreeJLL2
#define	j__udyFreeJLL3		j__udy1FreeJLL3
#ifdef JU_64BIT
#define	j__udyFreeJLL4		j__udy1FreeJLL4
#define	j__udyFreeJLL5		j__udy1FreeJLL5
#define	j__udyFreeJLL6		j__udy1FreeJLL6
#define	j__udyFreeJLL7		j__udy1FreeJLL7
#endif
#define	j__udyFreeJLW		j__udy1FreeJLW
#define	j__udyFreeJLB1		j__udy1FreeJLB1
#define	j__udyFreeSM		j__udy1FreeSM

#define	j__uMaxWords		j__u1MaxWords

#ifdef DEBUG
#define	JudyCheckPop		Judy1CheckPop
#endif

#else // JUDYL ****************************************************************

#define	cJU_LEAFW_MAXPOP1	cJL_LEAFW_MAXPOP1
#define	cJU_LEAF1_MAXPOP1	cJL_LEAF1_MAXPOP1
#define	cJU_LEAF2_MAXPOP1	cJL_LEAF2_MAXPOP1
#define	cJU_LEAF3_MAXPOP1	cJL_LEAF3_MAXPOP1
#ifdef JU_64BIT
#define	cJU_LEAF4_MAXPOP1	cJL_LEAF4_MAXPOP1
#define	cJU_LEAF5_MAXPOP1	cJL_LEAF5_MAXPOP1
#define	cJU_LEAF6_MAXPOP1	cJL_LEAF6_MAXPOP1
#define	cJU_LEAF7_MAXPOP1	cJL_LEAF7_MAXPOP1
#endif
#define	cJU_IMMED1_MAXPOP1	cJL_IMMED1_MAXPOP1
#define	cJU_IMMED2_MAXPOP1	cJL_IMMED2_MAXPOP1
#define	cJU_IMMED3_MAXPOP1	cJL_IMMED3_MAXPOP1
#ifdef JU_64BIT
#define	cJU_IMMED4_MAXPOP1	cJL_IMMED4_MAXPOP1
#define	cJU_IMMED5_MAXPOP1	cJL_IMMED5_MAXPOP1
#define	cJU_IMMED6_MAXPOP1	cJL_IMMED6_MAXPOP1
#define	cJU_IMMED7_MAXPOP1	cJL_IMMED7_MAXPOP1
#endif

#define	JU_LEAF1POPTOWORDS(Pop1)	JL_LEAF1POPTOWORDS(Pop1)
#define	JU_LEAF2POPTOWORDS(Pop1)	JL_LEAF2POPTOWORDS(Pop1)
#define	JU_LEAF3POPTOWORDS(Pop1)	JL_LEAF3POPTOWORDS(Pop1)
#ifdef JU_64BIT
#define	JU_LEAF4POPTOWORDS(Pop1)	JL_LEAF4POPTOWORDS(Pop1)
#define	JU_LEAF5POPTOWORDS(Pop1)	JL_LEAF5POPTOWORDS(Pop1)
#define	JU_LEAF6POPTOWORDS(Pop1)	JL_LEAF6POPTOWORDS(Pop1)
#define	JU_LEAF7POPTOWORDS(Pop1)	JL_LEAF7POPTOWORDS(Pop1)
#endif
#define	JU_LEAFWPOPTOWORDS(Pop1)	JL_LEAFWPOPTOWORDS(Pop1)

#define	JU_LEAF1GROWINPLACE(Pop1)	JL_LEAF1GROWINPLACE(Pop1)
#define	JU_LEAF2GROWINPLACE(Pop1)	JL_LEAF2GROWINPLACE(Pop1)
#define	JU_LEAF3GROWINPLACE(Pop1)	JL_LEAF3GROWINPLACE(Pop1)
#ifdef JU_64BIT
#define	JU_LEAF4GROWINPLACE(Pop1)	JL_LEAF4GROWINPLACE(Pop1)
#define	JU_LEAF5GROWINPLACE(Pop1)	JL_LEAF5GROWINPLACE(Pop1)
#define	JU_LEAF6GROWINPLACE(Pop1)	JL_LEAF6GROWINPLACE(Pop1)
#define	JU_LEAF7GROWINPLACE(Pop1)	JL_LEAF7GROWINPLACE(Pop1)
#endif
#define	JU_LEAFWGROWINPLACE(Pop1)	JL_LEAFWGROWINPLACE(Pop1)

#define	j__udyCreateBranchL	j__udyLCreateBranchL
#define	j__udyCreateBranchB	j__udyLCreateBranchB
#define	j__udyCreateBranchU	j__udyLCreateBranchU
#define	j__udyCascade1		j__udyLCascade1
#define	j__udyCascade2		j__udyLCascade2
#define	j__udyCascade3		j__udyLCascade3
#ifdef JU_64BIT
#define	j__udyCascade4		j__udyLCascade4
#define	j__udyCascade5		j__udyLCascade5
#define	j__udyCascade6		j__udyLCascade6
#define	j__udyCascade7		j__udyLCascade7
#endif
#define	j__udyCascadeL		j__udyLCascadeL
#define	j__udyInsertBranch	j__udyLInsertBranch

#define	j__udyBranchBToBranchL	j__udyLBranchBToBranchL
#define	j__udyLeafB1ToLeaf1	j__udyLLeafB1ToLeaf1
#define	j__udyLeaf1ToLeaf2	j__udyLLeaf1ToLeaf2
#define	j__udyLeaf2ToLeaf3	j__udyLLeaf2ToLeaf3
#ifndef JU_64BIT
#define	j__udyLeaf3ToLeafW	j__udyLLeaf3ToLeafW
#else
#define	j__udyLeaf3ToLeaf4	j__udyLLeaf3ToLeaf4
#define	j__udyLeaf4ToLeaf5	j__udyLLeaf4ToLeaf5
#define	j__udyLeaf5ToLeaf6	j__udyLLeaf5ToLeaf6
#define	j__udyLeaf6ToLeaf7	j__udyLLeaf6ToLeaf7
#define	j__udyLeaf7ToLeafW	j__udyLLeaf7ToLeafW
#endif

#define	jpm_t			jLpm_t
#define	Pjpm_t			PjLpm_t

#define	jlb_t			jLlb_t
#define	Pjlb_t			PjLlb_t

#define	JU_JLB_BITMAP		JL_JLB_BITMAP

#define	j__udyAllocJPM		j__udyLAllocJLPM
#define	j__udyAllocJBL		j__udyLAllocJBL
#define	j__udyAllocJBB		j__udyLAllocJBB
#define	j__udyAllocJBBJP	j__udyLAllocJBBJP
#define	j__udyAllocJBU		j__udyLAllocJBU
#define	j__udyAllocJLL1		j__udyLAllocJLL1
#define	j__udyAllocJLL2		j__udyLAllocJLL2
#define	j__udyAllocJLL3		j__udyLAllocJLL3
#ifdef JU_64BIT
#define	j__udyAllocJLL4		j__udyLAllocJLL4
#define	j__udyAllocJLL5		j__udyLAllocJLL5
#define	j__udyAllocJLL6		j__udyLAllocJLL6
#define	j__udyAllocJLL7		j__udyLAllocJLL7
#endif
#define	j__udyAllocJLW		j__udyLAllocJLW
#define	j__udyAllocJLB1		j__udyLAllocJLB1
//				j__udyLAllocJV
#define	j__udyFreeJPM		j__udyLFreeJLPM
#define	j__udyFreeJBL		j__udyLFreeJBL
#define	j__udyFreeJBB		j__udyLFreeJBB
#define	j__udyFreeJBBJP		j__udyLFreeJBBJP
#define	j__udyFreeJBU		j__udyLFreeJBU
#define	j__udyFreeJLL1		j__udyLFreeJLL1
#define	j__udyFreeJLL2		j__udyLFreeJLL2
#define	j__udyFreeJLL3		j__udyLFreeJLL3
#ifdef JU_64BIT
#define	j__udyFreeJLL4		j__udyLFreeJLL4
#define	j__udyFreeJLL5		j__udyLFreeJLL5
#define	j__udyFreeJLL6		j__udyLFreeJLL6
#define	j__udyFreeJLL7		j__udyLFreeJLL7
#endif
#define	j__udyFreeJLW		j__udyLFreeJLW
#define	j__udyFreeJLB1		j__udyLFreeJLB1
#define	j__udyFreeSM		j__udyLFreeSM
//				j__udyLFreeJV

#define	j__uMaxWords		j__uLMaxWords

#ifdef DEBUG
#define	JudyCheckPop		JudyLCheckPop
#endif

#endif // JUDYL

#endif // _JUDYPRIVATE1L_INCLUDED
