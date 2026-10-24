//go:build windows && amd64

#include "textflag.h"

// func spinPause()
// CPU PAUSE hint for SHM spin loops.
TEXT ·spinPause(SB),NOSPLIT,$0-0
	PAUSE
	RET
