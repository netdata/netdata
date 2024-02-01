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

// @(#) $Revision: 4.45 $ $Source: /judy/src/JudyCommon/JudyMallocIF.c $
//
// Judy malloc/free interface functions for Judy1 and JudyL.
//
// Compile with one of -DJUDY1 or -DJUDYL.
//
// Compile with -DTRACEMI (Malloc Interface) to turn on tracing of malloc/free
// calls at the interface level.  (See also TRACEMF in lower-level code.)
// Use -DTRACEMI2 for a terser format suitable for trace analysis.
//
// There can be malloc namespace bits in the LSBs of "raw" addresses from most,
// but not all, of the j__udy*Alloc*() functions; see also JudyPrivate.h.  To
// test the Judy code, compile this file with -DMALLOCBITS and use debug flavor
// only (for assertions).  This test ensures that (a) all callers properly mask
// the namespace bits out before dereferencing a pointer (or else a core dump
// occurs), and (b) all callers send "raw" (unmasked) addresses to
// j__udy*Free*() calls.
//
// Note:  Currently -DDEBUG turns on MALLOCBITS automatically.

#if (! (defined(JUDY1) || defined(JUDYL)))
#error:  One of -DJUDY1 or -DJUDYL must be specified.
#endif

#ifdef JUDY1
#include "Judy1.h"
#else
#include "JudyL.h"
#endif

#include "JudyPrivate1L.h"

// Set "hidden" global j__uMaxWords to the maximum number of words to allocate
// to any one array (large enough to have a JPM, otherwise j__uMaxWords is
// ignored), to trigger a fake malloc error when the number is exceeded.  Note,
// this code is always executed, not #ifdefd, because its virtually free.
//
// Note:  To keep the MALLOC macro faster and simpler, set j__uMaxWords to
// MAXINT, not zero, by default.

Word_t j__uMaxWords = ~0UL;

// This macro hides the faking of a malloc failure:
//
// Note:  To keep this fast, just compare WordsPrev to j__uMaxWords without the
// complexity of first adding WordsNow, meaning the trigger point is not
// exactly where you might assume, but it shouldnt matter.

#define MALLOC(MallocFunc,WordsPrev,WordsNow) \
        (((WordsPrev) > j__uMaxWords) ? 0UL : MallocFunc(WordsNow))

// Clear words starting at address:
//
// Note:  Only use this for objects that care; in other cases, it doesnt
// matter if the objects memory is pre-zeroed.

#define ZEROWORDS(Addr,Words)                   \
        {                                       \
            Word_t  Words__ = (Words);          \
            PWord_t Addr__  = (PWord_t) (Addr); \
            while (Words__--) *Addr__++ = 0UL;  \
        }

#ifdef TRACEMI

// TRACING SUPPORT:
//
// Note:  For TRACEMI, use a format for address printing compatible with other
// tracing facilities; in particular, %x not %lx, to truncate the "noisy" high
// part on 64-bit systems.
//
// TBD: The trace macros need fixing for alternate address types.
//
// Note:  TRACEMI2 supports trace analysis no matter the underlying malloc/free
// engine used.

#include <stdio.h>

static Word_t j__udyMemSequence = 0L;   // event sequence number.

#define TRACE_ALLOC5(a,b,c,d,e)   (void) printf(a, (b), c, d)
#define TRACE_FREE5( a,b,c,d,e)   (void) printf(a, (b), c, d)
#define TRACE_ALLOC6(a,b,c,d,e,f) (void) printf(a, (b), c, d, e)
#define TRACE_FREE6( a,b,c,d,e,f) (void) printf(a, (b), c, d, e)

#else

#ifdef TRACEMI2

#include <stdio.h>

#define b_pw cJU_BYTESPERWORD

#define TRACE_ALLOC5(a,b,c,d,e)   \
            (void) printf("a %lx %lx %lx\n", (b), (d) * b_pw, e)
#define TRACE_FREE5( a,b,c,d,e)   \
            (void) printf("f %lx %lx %lx\n", (b), (d) * b_pw, e)
#define TRACE_ALLOC6(a,b,c,d,e,f)         \
            (void) printf("a %lx %lx %lx\n", (b), (e) * b_pw, f)
#define TRACE_FREE6( a,b,c,d,e,f)         \
            (void) printf("f %lx %lx %lx\n", (b), (e) * b_pw, f)

static Word_t j__udyMemSequence = 0L;   // event sequence number.

#else

#define TRACE_ALLOC5(a,b,c,d,e)   // null.
#define TRACE_FREE5( a,b,c,d,e)   // null.
#define TRACE_ALLOC6(a,b,c,d,e,f) // null.
#define TRACE_FREE6( a,b,c,d,e,f) // null.

#endif // ! TRACEMI2
#endif // ! TRACEMI


// MALLOC NAMESPACE SUPPORT:

#if (defined(DEBUG) && (! defined(MALLOCBITS))) // for now, DEBUG => MALLOCBITS:
#define MALLOCBITS 1
#endif

#ifdef MALLOCBITS
#define MALLOCBITS_VALUE 0x3    // bit pattern to use.
#define MALLOCBITS_MASK  0x7    // note: matches mask__ in JudyPrivate.h.

#define MALLOCBITS_SET( Type,Addr) \
        ((Addr) = (Type) ((Word_t) (Addr) |  MALLOCBITS_VALUE))
#define MALLOCBITS_TEST(Type,Addr) \
        assert((((Word_t) (Addr)) & MALLOCBITS_MASK) == MALLOCBITS_VALUE); \
        ((Addr) = (Type) ((Word_t) (Addr) & ~MALLOCBITS_VALUE))
#else
#define MALLOCBITS_SET( Type,Addr)  // null.
#define MALLOCBITS_TEST(Type,Addr)  // null.
#endif


// SAVE ERROR INFORMATION IN A Pjpm:
//
// "Small" (invalid) Addr values are used to distinguish overrun and no-mem
// errors.  (TBD, non-zero invalid values are no longer returned from
// lower-level functions, that is, JU_ERRNO_OVERRUN is no longer detected.)

#define J__UDYSETALLOCERROR(Addr)                                       \
        {                                                               \
            JU_ERRID(Pjpm) = __LINE__;                                  \
            if ((Word_t) (Addr) > 0) JU_ERRNO(Pjpm) = JU_ERRNO_OVERRUN; \
            else                     JU_ERRNO(Pjpm) = JU_ERRNO_NOMEM;   \
            return(0);                                                  \
        }


// ****************************************************************************
// ALLOCATION FUNCTIONS:
//
// To help the compiler catch coding errors, each function returns a specific
// object type.
//
// Note:  Only j__udyAllocJPM() and j__udyAllocJLW() return multiple values <=
// sizeof(Word_t) to indicate the type of memory allocation failure.  Other
// allocation functions convert this failure to a JU_ERRNO.


// Note:  Unlike other j__udyAlloc*() functions, Pjpms are returned non-raw,
// that is, without malloc namespace or root pointer type bits:

FUNCTION Pjpm_t j__udyAllocJPM(void)
{
        Word_t Words = (sizeof(jpm_t) + cJU_BYTESPERWORD - 1) / cJU_BYTESPERWORD;
        Pjpm_t Pjpm  = (Pjpm_t) MALLOC(JudyMalloc, Words, Words);

        assert((Words * cJU_BYTESPERWORD) == sizeof(jpm_t));

        if ((Word_t) Pjpm > sizeof(Word_t))
        {
            ZEROWORDS(Pjpm, Words);
            Pjpm->jpm_TotalMemWords = Words;
        }

        TRACE_ALLOC5("0x%x %8lu = j__udyAllocJPM(), Words = %lu\n",
                     Pjpm, j__udyMemSequence++, Words, cJU_LEAFW_MAXPOP1 + 1);
        // MALLOCBITS_SET(Pjpm_t, Pjpm);  // see above.
        return(Pjpm);

} // j__udyAllocJPM()


FUNCTION Pjbl_t j__udyAllocJBL(Pjpm_t Pjpm)
{
        Word_t Words   = sizeof(jbl_t) / cJU_BYTESPERWORD;
        Pjbl_t PjblRaw = (Pjbl_t) MALLOC(JudyMallocVirtual,
                                         Pjpm->jpm_TotalMemWords, Words);

        assert((Words * cJU_BYTESPERWORD) == sizeof(jbl_t));

        if ((Word_t) PjblRaw > sizeof(Word_t))
        {
            ZEROWORDS(P_JBL(PjblRaw), Words);
            Pjpm->jpm_TotalMemWords += Words;
        }
        else { J__UDYSETALLOCERROR(PjblRaw); }

        TRACE_ALLOC5("0x%x %8lu = j__udyAllocJBL(), Words = %lu\n", PjblRaw,
                     j__udyMemSequence++, Words, (Pjpm->jpm_Pop0) + 2);
        MALLOCBITS_SET(Pjbl_t, PjblRaw);
        return(PjblRaw);

} // j__udyAllocJBL()


FUNCTION Pjbb_t j__udyAllocJBB(Pjpm_t Pjpm)
{
        Word_t Words   = sizeof(jbb_t) / cJU_BYTESPERWORD;
        Pjbb_t PjbbRaw = (Pjbb_t) MALLOC(JudyMallocVirtual,
                                         Pjpm->jpm_TotalMemWords, Words);

        assert((Words * cJU_BYTESPERWORD) == sizeof(jbb_t));

        if ((Word_t) PjbbRaw > sizeof(Word_t))
        {
            ZEROWORDS(P_JBB(PjbbRaw), Words);
            Pjpm->jpm_TotalMemWords += Words;
        }
        else { J__UDYSETALLOCERROR(PjbbRaw); }

        TRACE_ALLOC5("0x%x %8lu = j__udyAllocJBB(), Words = %lu\n", PjbbRaw,
                     j__udyMemSequence++, Words, (Pjpm->jpm_Pop0) + 2);
        MALLOCBITS_SET(Pjbb_t, PjbbRaw);
        return(PjbbRaw);

} // j__udyAllocJBB()


FUNCTION Pjp_t j__udyAllocJBBJP(Word_t NumJPs, Pjpm_t Pjpm)
{
        Word_t Words = JU_BRANCHJP_NUMJPSTOWORDS(NumJPs);
        Pjp_t  PjpRaw;

        PjpRaw = (Pjp_t) MALLOC(JudyMalloc, Pjpm->jpm_TotalMemWords, Words);

        if ((Word_t) PjpRaw > sizeof(Word_t))
        {
            Pjpm->jpm_TotalMemWords += Words;
        }
        else { J__UDYSETALLOCERROR(PjpRaw); }

        TRACE_ALLOC6("0x%x %8lu = j__udyAllocJBBJP(%lu), Words = %lu\n", PjpRaw,
                     j__udyMemSequence++, NumJPs, Words, (Pjpm->jpm_Pop0) + 2);
        MALLOCBITS_SET(Pjp_t, PjpRaw);
        return(PjpRaw);

} // j__udyAllocJBBJP()


FUNCTION Pjbu_t j__udyAllocJBU(Pjpm_t Pjpm)
{
        Word_t Words   = sizeof(jbu_t) / cJU_BYTESPERWORD;
        Pjbu_t PjbuRaw = (Pjbu_t) MALLOC(JudyMallocVirtual,
                                         Pjpm->jpm_TotalMemWords, Words);

        assert((Words * cJU_BYTESPERWORD) == sizeof(jbu_t));

        if ((Word_t) PjbuRaw > sizeof(Word_t))
        {
            Pjpm->jpm_TotalMemWords += Words;
        }
        else { J__UDYSETALLOCERROR(PjbuRaw); }

        TRACE_ALLOC5("0x%x %8lu = j__udyAllocJBU(), Words = %lu\n", PjbuRaw,
                     j__udyMemSequence++, Words, (Pjpm->jpm_Pop0) + 2);
        MALLOCBITS_SET(Pjbu_t, PjbuRaw);
        return(PjbuRaw);

} // j__udyAllocJBU()


#if (defined(JUDYL) || (! defined(JU_64BIT)))

FUNCTION Pjll_t j__udyAllocJLL1(Word_t Pop1, Pjpm_t Pjpm)
{
        Word_t Words = JU_LEAF1POPTOWORDS(Pop1);
        Pjll_t PjllRaw;

        PjllRaw = (Pjll_t) MALLOC(JudyMalloc, Pjpm->jpm_TotalMemWords, Words);

        if ((Word_t) PjllRaw > sizeof(Word_t))
        {
            Pjpm->jpm_TotalMemWords += Words;
        }
        else { J__UDYSETALLOCERROR(PjllRaw); }

        TRACE_ALLOC6("0x%x %8lu = j__udyAllocJLL1(%lu), Words = %lu\n", PjllRaw,
                     j__udyMemSequence++, Pop1, Words, (Pjpm->jpm_Pop0) + 2);
        MALLOCBITS_SET(Pjll_t, PjllRaw);
        return(PjllRaw);

} // j__udyAllocJLL1()

#endif // (JUDYL || (! JU_64BIT))


FUNCTION Pjll_t j__udyAllocJLL2(Word_t Pop1, Pjpm_t Pjpm)
{
        Word_t Words = JU_LEAF2POPTOWORDS(Pop1);
        Pjll_t PjllRaw;

        PjllRaw = (Pjll_t) MALLOC(JudyMalloc, Pjpm->jpm_TotalMemWords, Words);

        if ((Word_t) PjllRaw > sizeof(Word_t))
        {
            Pjpm->jpm_TotalMemWords += Words;
        }
        else { J__UDYSETALLOCERROR(PjllRaw); }

        TRACE_ALLOC6("0x%x %8lu = j__udyAllocJLL2(%lu), Words = %lu\n", PjllRaw,
                     j__udyMemSequence++, Pop1, Words, (Pjpm->jpm_Pop0) + 2);
        MALLOCBITS_SET(Pjll_t, PjllRaw);
        return(PjllRaw);

} // j__udyAllocJLL2()


FUNCTION Pjll_t j__udyAllocJLL3(Word_t Pop1, Pjpm_t Pjpm)
{
        Word_t Words = JU_LEAF3POPTOWORDS(Pop1);
        Pjll_t PjllRaw;

        PjllRaw = (Pjll_t) MALLOC(JudyMalloc, Pjpm->jpm_TotalMemWords, Words);

        if ((Word_t) PjllRaw > sizeof(Word_t))
        {
            Pjpm->jpm_TotalMemWords += Words;
        }
        else { J__UDYSETALLOCERROR(PjllRaw); }

        TRACE_ALLOC6("0x%x %8lu = j__udyAllocJLL3(%lu), Words = %lu\n", PjllRaw,
                     j__udyMemSequence++, Pop1, Words, (Pjpm->jpm_Pop0) + 2);
        MALLOCBITS_SET(Pjll_t, PjllRaw);
        return(PjllRaw);

} // j__udyAllocJLL3()


#ifdef JU_64BIT

FUNCTION Pjll_t j__udyAllocJLL4(Word_t Pop1, Pjpm_t Pjpm)
{
        Word_t Words = JU_LEAF4POPTOWORDS(Pop1);
        Pjll_t PjllRaw;

        PjllRaw = (Pjll_t) MALLOC(JudyMalloc, Pjpm->jpm_TotalMemWords, Words);

        if ((Word_t) PjllRaw > sizeof(Word_t))
        {
            Pjpm->jpm_TotalMemWords += Words;
        }
        else { J__UDYSETALLOCERROR(PjllRaw); }

        TRACE_ALLOC6("0x%x %8lu = j__udyAllocJLL4(%lu), Words = %lu\n", PjllRaw,
                     j__udyMemSequence++, Pop1, Words, (Pjpm->jpm_Pop0) + 2);
        MALLOCBITS_SET(Pjll_t, PjllRaw);
        return(PjllRaw);

} // j__udyAllocJLL4()


FUNCTION Pjll_t j__udyAllocJLL5(Word_t Pop1, Pjpm_t Pjpm)
{
        Word_t Words = JU_LEAF5POPTOWORDS(Pop1);
        Pjll_t PjllRaw;

        PjllRaw = (Pjll_t) MALLOC(JudyMalloc, Pjpm->jpm_TotalMemWords, Words);

        if ((Word_t) PjllRaw > sizeof(Word_t))
        {
            Pjpm->jpm_TotalMemWords += Words;
        }
        else { J__UDYSETALLOCERROR(PjllRaw); }

        TRACE_ALLOC6("0x%x %8lu = j__udyAllocJLL5(%lu), Words = %lu\n", PjllRaw,
                     j__udyMemSequence++, Pop1, Words, (Pjpm->jpm_Pop0) + 2);
        MALLOCBITS_SET(Pjll_t, PjllRaw);
        return(PjllRaw);

} // j__udyAllocJLL5()


FUNCTION Pjll_t j__udyAllocJLL6(Word_t Pop1, Pjpm_t Pjpm)
{
        Word_t Words = JU_LEAF6POPTOWORDS(Pop1);
        Pjll_t PjllRaw;

        PjllRaw = (Pjll_t) MALLOC(JudyMalloc, Pjpm->jpm_TotalMemWords, Words);

        if ((Word_t) PjllRaw > sizeof(Word_t))
        {
            Pjpm->jpm_TotalMemWords += Words;
        }
        else { J__UDYSETALLOCERROR(PjllRaw); }

        TRACE_ALLOC6("0x%x %8lu = j__udyAllocJLL6(%lu), Words = %lu\n", PjllRaw,
                     j__udyMemSequence++, Pop1, Words, (Pjpm->jpm_Pop0) + 2);
        MALLOCBITS_SET(Pjll_t, PjllRaw);
        return(PjllRaw);

} // j__udyAllocJLL6()


FUNCTION Pjll_t j__udyAllocJLL7(Word_t Pop1, Pjpm_t Pjpm)
{
        Word_t Words = JU_LEAF7POPTOWORDS(Pop1);
        Pjll_t PjllRaw;

        PjllRaw = (Pjll_t) MALLOC(JudyMalloc, Pjpm->jpm_TotalMemWords, Words);

        if ((Word_t) PjllRaw > sizeof(Word_t))
        {
            Pjpm->jpm_TotalMemWords += Words;
        }
        else { J__UDYSETALLOCERROR(PjllRaw); }

        TRACE_ALLOC6("0x%x %8lu = j__udyAllocJLL7(%lu), Words = %lu\n", PjllRaw,
                     j__udyMemSequence++, Pop1, Words, (Pjpm->jpm_Pop0) + 2);
        MALLOCBITS_SET(Pjll_t, PjllRaw);
        return(PjllRaw);

} // j__udyAllocJLL7()

#endif // JU_64BIT


// Note:  Root-level leaf addresses are always whole words (Pjlw_t), and unlike
// other j__udyAlloc*() functions, they are returned non-raw, that is, without
// malloc namespace or root pointer type bits (the latter are added later by
// the caller):

FUNCTION Pjlw_t j__udyAllocJLW(Word_t Pop1)
{
        Word_t Words = JU_LEAFWPOPTOWORDS(Pop1);
        Pjlw_t Pjlw  = (Pjlw_t) MALLOC(JudyMalloc, Words, Words);

        TRACE_ALLOC6("0x%x %8lu = j__udyAllocJLW(%lu), Words = %lu\n", Pjlw,
                     j__udyMemSequence++, Pop1, Words, Pop1);
        // MALLOCBITS_SET(Pjlw_t, Pjlw);  // see above.
        return(Pjlw);

} // j__udyAllocJLW()


FUNCTION Pjlb_t j__udyAllocJLB1(Pjpm_t Pjpm)
{
        Word_t Words = sizeof(jlb_t) / cJU_BYTESPERWORD;
        Pjlb_t PjlbRaw;

        PjlbRaw = (Pjlb_t) MALLOC(JudyMalloc, Pjpm->jpm_TotalMemWords, Words);

        assert((Words * cJU_BYTESPERWORD) == sizeof(jlb_t));

        if ((Word_t) PjlbRaw > sizeof(Word_t))
        {
            ZEROWORDS(P_JLB(PjlbRaw), Words);
            Pjpm->jpm_TotalMemWords += Words;
        }
        else { J__UDYSETALLOCERROR(PjlbRaw); }

        TRACE_ALLOC5("0x%x %8lu = j__udyAllocJLB1(), Words = %lu\n", PjlbRaw,
                     j__udyMemSequence++, Words, (Pjpm->jpm_Pop0) + 2);
        MALLOCBITS_SET(Pjlb_t, PjlbRaw);
        return(PjlbRaw);

} // j__udyAllocJLB1()


#ifdef JUDYL

FUNCTION Pjv_t j__udyLAllocJV(Word_t Pop1, Pjpm_t Pjpm)
{
        Word_t Words = JL_LEAFVPOPTOWORDS(Pop1);
        Pjv_t  PjvRaw;

        PjvRaw = (Pjv_t) MALLOC(JudyMalloc, Pjpm->jpm_TotalMemWords, Words);

        if ((Word_t) PjvRaw > sizeof(Word_t))
        {
            Pjpm->jpm_TotalMemWords += Words;
        }
        else { J__UDYSETALLOCERROR(PjvRaw); }

        TRACE_ALLOC6("0x%x %8lu = j__udyLAllocJV(%lu), Words = %lu\n", PjvRaw,
                     j__udyMemSequence++, Pop1, Words, (Pjpm->jpm_Pop0) + 2);
        MALLOCBITS_SET(Pjv_t, PjvRaw);
        return(PjvRaw);

} // j__udyLAllocJV()

#endif // JUDYL


// ****************************************************************************
// FREE FUNCTIONS:
//
// To help the compiler catch coding errors, each function takes a specific
// object type to free.


// Note:  j__udyFreeJPM() receives a root pointer with NO root pointer type
// bits present, that is, they must be stripped by the caller using P_JPM():

FUNCTION void j__udyFreeJPM(Pjpm_t PjpmFree, Pjpm_t PjpmStats)
{
        Word_t Words = (sizeof(jpm_t) + cJU_BYTESPERWORD - 1) / cJU_BYTESPERWORD;

        // MALLOCBITS_TEST(Pjpm_t, PjpmFree);   // see above.
        JudyFree((Pvoid_t) PjpmFree, Words);

        if (PjpmStats != (Pjpm_t) NULL) PjpmStats->jpm_TotalMemWords -= Words;

// Note:  Log PjpmFree->jpm_Pop0, similar to other j__udyFree*() functions, not
// an assumed value of cJU_LEAFW_MAXPOP1, for when the caller is
// Judy*FreeArray(), jpm_Pop0 is set to 0, and the population after the free
// really will be 0, not cJU_LEAFW_MAXPOP1.

        TRACE_FREE6("0x%x %8lu =  j__udyFreeJPM(%lu), Words = %lu\n", PjpmFree,
                    j__udyMemSequence++, Words, Words, PjpmFree->jpm_Pop0);


} // j__udyFreeJPM()


FUNCTION void j__udyFreeJBL(Pjbl_t Pjbl, Pjpm_t Pjpm)
{
        Word_t Words = sizeof(jbl_t) / cJU_BYTESPERWORD;

        MALLOCBITS_TEST(Pjbl_t, Pjbl);
        JudyFreeVirtual((Pvoid_t) Pjbl, Words);

        Pjpm->jpm_TotalMemWords -= Words;

        TRACE_FREE5("0x%x %8lu =  j__udyFreeJBL(), Words = %lu\n", Pjbl,
                    j__udyMemSequence++, Words, Pjpm->jpm_Pop0);


} // j__udyFreeJBL()


FUNCTION void j__udyFreeJBB(Pjbb_t Pjbb, Pjpm_t Pjpm)
{
        Word_t Words = sizeof(jbb_t) / cJU_BYTESPERWORD;

        MALLOCBITS_TEST(Pjbb_t, Pjbb);
        JudyFreeVirtual((Pvoid_t) Pjbb, Words);

        Pjpm->jpm_TotalMemWords -= Words;

        TRACE_FREE5("0x%x %8lu =  j__udyFreeJBB(), Words = %lu\n", Pjbb,
                    j__udyMemSequence++, Words, Pjpm->jpm_Pop0);


} // j__udyFreeJBB()


FUNCTION void j__udyFreeJBBJP(Pjp_t Pjp, Word_t NumJPs, Pjpm_t Pjpm)
{
        Word_t Words = JU_BRANCHJP_NUMJPSTOWORDS(NumJPs);

        MALLOCBITS_TEST(Pjp_t, Pjp);
        JudyFree((Pvoid_t) Pjp, Words);

        Pjpm->jpm_TotalMemWords -= Words;

        TRACE_FREE6("0x%x %8lu =  j__udyFreeJBBJP(%lu), Words = %lu\n", Pjp,
                    j__udyMemSequence++, NumJPs, Words, Pjpm->jpm_Pop0);


} // j__udyFreeJBBJP()


FUNCTION void j__udyFreeJBU(Pjbu_t Pjbu, Pjpm_t Pjpm)
{
        Word_t Words = sizeof(jbu_t) / cJU_BYTESPERWORD;

        MALLOCBITS_TEST(Pjbu_t, Pjbu);
        JudyFreeVirtual((Pvoid_t) Pjbu, Words);

        Pjpm->jpm_TotalMemWords -= Words;

        TRACE_FREE5("0x%x %8lu =  j__udyFreeJBU(), Words = %lu\n", Pjbu,
                    j__udyMemSequence++, Words, Pjpm->jpm_Pop0);


} // j__udyFreeJBU()


#if (defined(JUDYL) || (! defined(JU_64BIT)))

FUNCTION void j__udyFreeJLL1(Pjll_t Pjll, Word_t Pop1, Pjpm_t Pjpm)
{
        Word_t Words = JU_LEAF1POPTOWORDS(Pop1);

        MALLOCBITS_TEST(Pjll_t, Pjll);
        JudyFree((Pvoid_t) Pjll, Words);

        Pjpm->jpm_TotalMemWords -= Words;

        TRACE_FREE6("0x%x %8lu =  j__udyFreeJLL1(%lu), Words = %lu\n", Pjll,
                    j__udyMemSequence++, Pop1, Words, Pjpm->jpm_Pop0);


} // j__udyFreeJLL1()

#endif // (JUDYL || (! JU_64BIT))


FUNCTION void j__udyFreeJLL2(Pjll_t Pjll, Word_t Pop1, Pjpm_t Pjpm)
{
        Word_t Words = JU_LEAF2POPTOWORDS(Pop1);

        MALLOCBITS_TEST(Pjll_t, Pjll);
        JudyFree((Pvoid_t) Pjll, Words);

        Pjpm->jpm_TotalMemWords -= Words;

        TRACE_FREE6("0x%x %8lu =  j__udyFreeJLL2(%lu), Words = %lu\n", Pjll,
                    j__udyMemSequence++, Pop1, Words, Pjpm->jpm_Pop0);


} // j__udyFreeJLL2()


FUNCTION void j__udyFreeJLL3(Pjll_t Pjll, Word_t Pop1, Pjpm_t Pjpm)
{
        Word_t Words = JU_LEAF3POPTOWORDS(Pop1);

        MALLOCBITS_TEST(Pjll_t, Pjll);
        JudyFree((Pvoid_t) Pjll, Words);

        Pjpm->jpm_TotalMemWords -= Words;

        TRACE_FREE6("0x%x %8lu =  j__udyFreeJLL3(%lu), Words = %lu\n", Pjll,
                    j__udyMemSequence++, Pop1, Words, Pjpm->jpm_Pop0);


} // j__udyFreeJLL3()


#ifdef JU_64BIT

FUNCTION void j__udyFreeJLL4(Pjll_t Pjll, Word_t Pop1, Pjpm_t Pjpm)
{
        Word_t Words = JU_LEAF4POPTOWORDS(Pop1);

        MALLOCBITS_TEST(Pjll_t, Pjll);
        JudyFree((Pvoid_t) Pjll, Words);

        Pjpm->jpm_TotalMemWords -= Words;

        TRACE_FREE6("0x%x %8lu =  j__udyFreeJLL4(%lu), Words = %lu\n", Pjll,
                    j__udyMemSequence++, Pop1, Words, Pjpm->jpm_Pop0);


} // j__udyFreeJLL4()


FUNCTION void j__udyFreeJLL5(Pjll_t Pjll, Word_t Pop1, Pjpm_t Pjpm)
{
        Word_t Words = JU_LEAF5POPTOWORDS(Pop1);

        MALLOCBITS_TEST(Pjll_t, Pjll);
        JudyFree((Pvoid_t) Pjll, Words);

        Pjpm->jpm_TotalMemWords -= Words;

        TRACE_FREE6("0x%x %8lu =  j__udyFreeJLL5(%lu), Words = %lu\n", Pjll,
                    j__udyMemSequence++, Pop1, Words, Pjpm->jpm_Pop0);


} // j__udyFreeJLL5()


FUNCTION void j__udyFreeJLL6(Pjll_t Pjll, Word_t Pop1, Pjpm_t Pjpm)
{
        Word_t Words = JU_LEAF6POPTOWORDS(Pop1);

        MALLOCBITS_TEST(Pjll_t, Pjll);
        JudyFree((Pvoid_t) Pjll, Words);

        Pjpm->jpm_TotalMemWords -= Words;

        TRACE_FREE6("0x%x %8lu =  j__udyFreeJLL6(%lu), Words = %lu\n", Pjll,
                    j__udyMemSequence++, Pop1, Words, Pjpm->jpm_Pop0);


} // j__udyFreeJLL6()


FUNCTION void j__udyFreeJLL7(Pjll_t Pjll, Word_t Pop1, Pjpm_t Pjpm)
{
        Word_t Words = JU_LEAF7POPTOWORDS(Pop1);

        MALLOCBITS_TEST(Pjll_t, Pjll);
        JudyFree((Pvoid_t) Pjll, Words);

        Pjpm->jpm_TotalMemWords -= Words;

        TRACE_FREE6("0x%x %8lu =  j__udyFreeJLL7(%lu), Words = %lu\n", Pjll,
                    j__udyMemSequence++, Pop1, Words, Pjpm->jpm_Pop0);


} // j__udyFreeJLL7()

#endif // JU_64BIT


// Note:  j__udyFreeJLW() receives a root pointer with NO root pointer type
// bits present, that is, they are stripped by P_JLW():

FUNCTION void j__udyFreeJLW(Pjlw_t Pjlw, Word_t Pop1, Pjpm_t Pjpm)
{
        Word_t Words = JU_LEAFWPOPTOWORDS(Pop1);

        // MALLOCBITS_TEST(Pjlw_t, Pjlw);       // see above.
        JudyFree((Pvoid_t) Pjlw, Words);

        if (Pjpm) Pjpm->jpm_TotalMemWords -= Words;

        TRACE_FREE6("0x%x %8lu =  j__udyFreeJLW(%lu), Words = %lu\n", Pjlw,
                    j__udyMemSequence++, Pop1, Words, Pop1 - 1);


} // j__udyFreeJLW()


FUNCTION void j__udyFreeJLB1(Pjlb_t Pjlb, Pjpm_t Pjpm)
{
        Word_t Words = sizeof(jlb_t) / cJU_BYTESPERWORD;

        MALLOCBITS_TEST(Pjlb_t, Pjlb);
        JudyFree((Pvoid_t) Pjlb, Words);

        Pjpm->jpm_TotalMemWords -= Words;

        TRACE_FREE5("0x%x %8lu =  j__udyFreeJLB1(), Words = %lu\n", Pjlb,
                    j__udyMemSequence++, Words, Pjpm->jpm_Pop0);


} // j__udyFreeJLB1()


#ifdef JUDYL

FUNCTION void j__udyLFreeJV(Pjv_t Pjv, Word_t Pop1, Pjpm_t Pjpm)
{
        Word_t Words = JL_LEAFVPOPTOWORDS(Pop1);

        MALLOCBITS_TEST(Pjv_t, Pjv);
        JudyFree((Pvoid_t) Pjv, Words);

        Pjpm->jpm_TotalMemWords -= Words;

        TRACE_FREE6("0x%x %8lu = j__udyLFreeJV(%lu), Words = %lu\n", Pjv,
                    j__udyMemSequence++, Pop1, Words, Pjpm->jpm_Pop0);


} // j__udyLFreeJV()

#endif // JUDYL
