//go:build (linux || freebsd) && amd64

#include "textflag.h"

// func spinPause()
// CPU PAUSE hint for spin loops — reduces pipeline contention.
TEXT ·spinPause(SB),NOSPLIT,$0-0
	PAUSE
	RET
