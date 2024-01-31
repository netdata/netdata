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

// @(#) $Revision: 4.5 $ $Source: /judy/src/JudyCommon/JudyMemUsed.c $
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

#ifdef JUDY1
FUNCTION Word_t Judy1MemUsed
#else  // JUDYL
FUNCTION Word_t JudyLMemUsed
#endif
        (
	Pcvoid_t PArray 	// from which to retrieve.
        )
{
	Word_t	 Words = 0;

        if (PArray == (Pcvoid_t) NULL) return(0);

	if (JU_LEAFW_POP0(PArray) < cJU_LEAFW_MAXPOP1) // must be a LEAFW
	{
	    Pjlw_t Pjlw = P_JLW(PArray);		// first word of leaf.
	    Words = JU_LEAFWPOPTOWORDS(Pjlw[0] + 1);	// based on pop1.
	}
	else
	{
	    Pjpm_t Pjpm = P_JPM(PArray);
	    Words = Pjpm->jpm_TotalMemWords;
	}

	return(Words * sizeof(Word_t));		// convert to bytes.

} // Judy1MemUsed() / JudyLMemUsed()
