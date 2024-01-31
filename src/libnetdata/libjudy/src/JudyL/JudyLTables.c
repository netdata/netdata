// @(#) From generation tool: $Revision: 4.37 $ $Source: /judy/src/JudyCommon/JudyTables.c $
// Pregenerated and modified by hand. Do not overwrite!

#include "JudyL.h"
// Leave the malloc() sizes readable in the binary (via strings(1)):
#ifdef JU_64BIT
const char * JudyLMallocSizes = "JudyLMallocSizes = 3, 5, 7, 11, 15, 23, 32, 47, 64, Leaf1 = 13";
#else // JU_32BIT
const char * JudyLMallocSizes = "JudyLMallocSizes = 3, 5, 7, 11, 15, 23, 32, 47, 64, Leaf1 = 25";
#endif // JU_64BIT

#ifdef JU_64BIT
//	object uses 64 words
//	cJU_BITSPERSUBEXPB = 32
const uint8_t
j__L_BranchBJPPopToWords[cJU_BITSPERSUBEXPB + 1] =
{
	 0,
	 3,  5,  7, 11, 11, 15, 15, 23,
	23, 23, 23, 32, 32, 32, 32, 32,
	47, 47, 47, 47, 47, 47, 47, 64,
	64, 64, 64, 64, 64, 64, 64, 64
};

//	object uses 15 words
//	cJL_LEAF1_MAXPOP1 = 13
const uint8_t
j__L_Leaf1PopToWords[cJL_LEAF1_MAXPOP1 + 1] =
{
	 0,
	 3,  3,  5,  5,  7,  7, 11, 11,
	11, 15, 15, 15, 15
};
const uint8_t
j__L_Leaf1Offset[cJL_LEAF1_MAXPOP1 + 1] =
{
	 0,
	 1,  1,  1,  1,  1,  1,  2,  2,
	 2,  2,  2,  2,  2
};

//	object uses 64 words
//	cJL_LEAF2_MAXPOP1 = 51
const uint8_t
j__L_Leaf2PopToWords[cJL_LEAF2_MAXPOP1 + 1] =
{
	 0,
	 3,  3,  5,  5,  7, 11, 11, 11,
	15, 15, 15, 15, 23, 23, 23, 23,
	23, 23, 32, 32, 32, 32, 32, 32,
	32, 47, 47, 47, 47, 47, 47, 47,
	47, 47, 47, 47, 47, 64, 64, 64,
	64, 64, 64, 64, 64, 64, 64, 64,
	64, 64, 64
};
const uint8_t
j__L_Leaf2Offset[cJL_LEAF2_MAXPOP1 + 1] =
{
	 0,
	 1,  1,  1,  1,  2,  3,  3,  3,
	 3,  3,  3,  3,  5,  5,  5,  5,
	 5,  5,  7,  7,  7,  7,  7,  7,
	 7, 10, 10, 10, 10, 10, 10, 10,
	10, 10, 10, 10, 10, 13, 13, 13,
	13, 13, 13, 13, 13, 13, 13, 13,
	13, 13, 13
};

//	object uses 64 words
//	cJL_LEAF3_MAXPOP1 = 46
const uint8_t
j__L_Leaf3PopToWords[cJL_LEAF3_MAXPOP1 + 1] =
{
	 0,
	 3,  3,  5,  7,  7, 11, 11, 11,
	15, 15, 23, 23, 23, 23, 23, 23,
	32, 32, 32, 32, 32, 32, 32, 47,
	47, 47, 47, 47, 47, 47, 47, 47,
	47, 47, 64, 64, 64, 64, 64, 64,
	64, 64, 64, 64, 64, 64
};
const uint8_t
j__L_Leaf3Offset[cJL_LEAF3_MAXPOP1 + 1] =
{
	 0,
	 1,  1,  2,  2,  2,  3,  3,  3,
	 4,  4,  6,  6,  6,  6,  6,  6,
	 9,  9,  9,  9,  9,  9,  9, 13,
	13, 13, 13, 13, 13, 13, 13, 13,
	13, 13, 18, 18, 18, 18, 18, 18,
	18, 18, 18, 18, 18, 18
};

//	object uses 63 words
//	cJL_LEAF4_MAXPOP1 = 42
const uint8_t
j__L_Leaf4PopToWords[cJL_LEAF4_MAXPOP1 + 1] =
{
	 0,
	 3,  3,  5,  7, 11, 11, 11, 15,
	15, 15, 23, 23, 23, 23, 23, 32,
	32, 32, 32, 32, 32, 47, 47, 47,
	47, 47, 47, 47, 47, 47, 47, 63,
	63, 63, 63, 63, 63, 63, 63, 63,
	63, 63
};
const uint8_t
j__L_Leaf4Offset[cJL_LEAF4_MAXPOP1 + 1] =
{
	 0,
	 1,  1,  2,  2,  4,  4,  4,  5,
	 5,  5,  8,  8,  8,  8,  8, 11,
	11, 11, 11, 11, 11, 16, 16, 16,
	16, 16, 16, 16, 16, 16, 16, 21,
	21, 21, 21, 21, 21, 21, 21, 21,
	21, 21
};

//	object uses 64 words
//	cJL_LEAF5_MAXPOP1 = 39
const uint8_t
j__L_Leaf5PopToWords[cJL_LEAF5_MAXPOP1 + 1] =
{
	 0,
	 3,  5,  5,  7, 11, 11, 15, 15,
	15, 23, 23, 23, 23, 23, 32, 32,
	32, 32, 32, 47, 47, 47, 47, 47,
	47, 47, 47, 47, 64, 64, 64, 64,
	64, 64, 64, 64, 64, 64, 64
};
const uint8_t
j__L_Leaf5Offset[cJL_LEAF5_MAXPOP1 + 1] =
{
	 0,
	 2,  2,  2,  3,  4,  4,  6,  6,
	 6,  9,  9,  9,  9,  9, 12, 12,
	12, 12, 12, 18, 18, 18, 18, 18,
	18, 18, 18, 18, 25, 25, 25, 25,
	25, 25, 25, 25, 25, 25, 25
};

//	object uses 63 words
//	cJL_LEAF6_MAXPOP1 = 36
const uint8_t
j__L_Leaf6PopToWords[cJL_LEAF6_MAXPOP1 + 1] =
{
	 0,
	 3,  5,  7,  7, 11, 11, 15, 15,
	23, 23, 23, 23, 23, 32, 32, 32,
	32, 32, 47, 47, 47, 47, 47, 47,
	47, 47, 63, 63, 63, 63, 63, 63,
	63, 63, 63, 63
};
const uint8_t
j__L_Leaf6Offset[cJL_LEAF6_MAXPOP1 + 1] =
{
	 0,
	 1,  3,  3,  3,  5,  5,  6,  6,
	10, 10, 10, 10, 10, 14, 14, 14,
	14, 14, 20, 20, 20, 20, 20, 20,
	20, 20, 27, 27, 27, 27, 27, 27,
	27, 27, 27, 27
};

//	object uses 64 words
//	cJL_LEAF7_MAXPOP1 = 34
const uint8_t
j__L_Leaf7PopToWords[cJL_LEAF7_MAXPOP1 + 1] =
{
	 0,
	 3,  5,  7, 11, 11, 15, 15, 15,
	23, 23, 23, 23, 32, 32, 32, 32,
	32, 47, 47, 47, 47, 47, 47, 47,
	47, 64, 64, 64, 64, 64, 64, 64,
	64, 64
};
const uint8_t
j__L_Leaf7Offset[cJL_LEAF7_MAXPOP1 + 1] =
{
	 0,
	 1,  3,  3,  5,  5,  7,  7,  7,
	11, 11, 11, 11, 15, 15, 15, 15,
	15, 22, 22, 22, 22, 22, 22, 22,
	22, 30, 30, 30, 30, 30, 30, 30,
	30, 30
};

//	object uses 63 words
//	cJL_LEAFW_MAXPOP1 = 31
const uint8_t
j__L_LeafWPopToWords[cJL_LEAFW_MAXPOP1 + 1] =
{
	 0,
	 3,  5,  7, 11, 11, 15, 15, 23,
	23, 23, 23, 32, 32, 32, 32, 47,
	47, 47, 47, 47, 47, 47, 47, 63,
	63, 63, 63, 63, 63, 63, 63
};
const uint8_t
j__L_LeafWOffset[cJL_LEAFW_MAXPOP1 + 1] =
{
	 0,
	 2,  3,  4,  6,  6,  8,  8, 12,
	12, 12, 12, 16, 16, 16, 16, 24,
	24, 24, 24, 24, 24, 24, 24, 32,
	32, 32, 32, 32, 32, 32, 32
};

//	object uses 64 words
//	cJU_BITSPERSUBEXPL = 64
const uint8_t
j__L_LeafVPopToWords[cJU_BITSPERSUBEXPL + 1] =
{
	 0,
	 3,  3,  3,  5,  5,  7,  7, 11,
	11, 11, 11, 15, 15, 15, 15, 23,
	23, 23, 23, 23, 23, 23, 23, 32,
	32, 32, 32, 32, 32, 32, 32, 32,
	47, 47, 47, 47, 47, 47, 47, 47,
	47, 47, 47, 47, 47, 47, 47, 64,
	64, 64, 64, 64, 64, 64, 64, 64,
	64, 64, 64, 64, 64, 64, 64, 64
};
#else // JU_32BIT
//	object uses 64 words
//	cJU_BITSPERSUBEXPB = 32
const uint8_t
j__L_BranchBJPPopToWords[cJU_BITSPERSUBEXPB + 1] =
{
	 0,
	 3,  5,  7, 11, 11, 15, 15, 23,
	23, 23, 23, 32, 32, 32, 32, 32,
	47, 47, 47, 47, 47, 47, 47, 64,
	64, 64, 64, 64, 64, 64, 64, 64
};

//	object uses 32 words
//	cJL_LEAF1_MAXPOP1 = 25
const uint8_t
j__L_Leaf1PopToWords[cJL_LEAF1_MAXPOP1 + 1] =
{
	 0,
	 3,  3,  5,  5,  7, 11, 11, 11,
	15, 15, 15, 15, 23, 23, 23, 23,
	23, 23, 32, 32, 32, 32, 32, 32,
	32
};
const uint8_t
j__L_Leaf1Offset[cJL_LEAF1_MAXPOP1 + 1] =
{
	 0,
	 1,  1,  1,  1,  2,  3,  3,  3,
	 3,  3,  3,  3,  5,  5,  5,  5,
	 5,  5,  7,  7,  7,  7,  7,  7,
	 7
};

//	object uses 63 words
//	cJL_LEAF2_MAXPOP1 = 42
const uint8_t
j__L_Leaf2PopToWords[cJL_LEAF2_MAXPOP1 + 1] =
{
	 0,
	 3,  3,  5,  7, 11, 11, 11, 15,
	15, 15, 23, 23, 23, 23, 23, 32,
	32, 32, 32, 32, 32, 47, 47, 47,
	47, 47, 47, 47, 47, 47, 47, 63,
	63, 63, 63, 63, 63, 63, 63, 63,
	63, 63
};
const uint8_t
j__L_Leaf2Offset[cJL_LEAF2_MAXPOP1 + 1] =
{
	 0,
	 1,  1,  2,  2,  4,  4,  4,  5,
	 5,  5,  8,  8,  8,  8,  8, 11,
	11, 11, 11, 11, 11, 16, 16, 16,
	16, 16, 16, 16, 16, 16, 16, 21,
	21, 21, 21, 21, 21, 21, 21, 21,
	21, 21
};

//	object uses 63 words
//	cJL_LEAF3_MAXPOP1 = 36
const uint8_t
j__L_Leaf3PopToWords[cJL_LEAF3_MAXPOP1 + 1] =
{
	 0,
	 3,  5,  7,  7, 11, 11, 15, 15,
	23, 23, 23, 23, 23, 32, 32, 32,
	32, 32, 47, 47, 47, 47, 47, 47,
	47, 47, 63, 63, 63, 63, 63, 63,
	63, 63, 63, 63
};
const uint8_t
j__L_Leaf3Offset[cJL_LEAF3_MAXPOP1 + 1] =
{
	 0,
	 1,  3,  3,  3,  5,  5,  6,  6,
	10, 10, 10, 10, 10, 14, 14, 14,
	14, 14, 20, 20, 20, 20, 20, 20,
	20, 20, 27, 27, 27, 27, 27, 27,
	27, 27, 27, 27
};

//	object uses 63 words
//	cJL_LEAFW_MAXPOP1 = 31
const uint8_t
j__L_LeafWPopToWords[cJL_LEAFW_MAXPOP1 + 1] =
{
	 0,
	 3,  5,  7, 11, 11, 15, 15, 23,
	23, 23, 23, 32, 32, 32, 32, 47,
	47, 47, 47, 47, 47, 47, 47, 63,
	63, 63, 63, 63, 63, 63, 63
};
const uint8_t
j__L_LeafWOffset[cJL_LEAFW_MAXPOP1 + 1] =
{
	 0,
	 2,  3,  4,  6,  6,  8,  8, 12,
	12, 12, 12, 16, 16, 16, 16, 24,
	24, 24, 24, 24, 24, 24, 24, 32,
	32, 32, 32, 32, 32, 32, 32
};

//	object uses 32 words
//	cJU_BITSPERSUBEXPL = 32
const uint8_t
j__L_LeafVPopToWords[cJU_BITSPERSUBEXPL + 1] =
{
	 0,
	 3,  3,  3,  5,  5,  7,  7, 11,
	11, 11, 11, 15, 15, 15, 15, 23,
	23, 23, 23, 23, 23, 23, 23, 32,
	32, 32, 32, 32, 32, 32, 32, 32
};
#endif // JU_64BIT
