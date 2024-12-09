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

// @(#) $Revision: 4.33 $ $Source: /judy/src/JudyCommon/JudyMalloc.c $
// ************************************************************************ //
//                    JUDY - Memory Allocater                             //
//                              -by-					  //
//		         Douglas L. Baskins				  //
//			  Hewlett Packard				  //
//                        Fort Collins, Co				  //
//                         (970) 229-2027				  //
//									  //
// ************************************************************************ //

// JUDY INCLUDE FILES
#include "Judy.h"

// ****************************************************************************
// J U D Y   M A L L O C
//
// Allocate RAM.  This is the single location in Judy code that calls
// malloc(3C).  Note:  JPM accounting occurs at a higher level.

Word_t JudyMalloc(
	Word_t Words)
{
	Word_t Addr;

	Addr = (Word_t) malloc(Words * sizeof(Word_t));
	return(Addr);

} // JudyMalloc()


// ****************************************************************************
// J U D Y   F R E E

void JudyFree(
	void * PWord,
	Word_t Words)
{
	(void) Words;
	free(PWord);

} // JudyFree()


// ****************************************************************************
// J U D Y   M A L L O C
//
// Higher-level "wrapper" for allocating objects that need not be in RAM,
// although at this time they are in fact only in RAM.  Later we hope that some
// entire subtrees (at a JPM or branch) can be "virtual", so their allocations
// and frees should go through this level.

Word_t JudyMallocVirtual(
	Word_t Words)
{
	return(JudyMalloc(Words));

} // JudyMallocVirtual()


// ****************************************************************************
// J U D Y   F R E E

void JudyFreeVirtual(
	void * PWord,
	Word_t Words)
{
        JudyFree(PWord, Words);

} // JudyFreeVirtual()
