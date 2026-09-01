// SPDX-License-Identifier: GPL-3.0-or-later

package probe

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

const PayloadBytes = 4096

type Generator struct {
	Prefix  string
	OwnerID string
	Now     func() time.Time
	Random  io.Reader
}

type Object struct {
	Key     string
	Payload []byte
	Digest  string
}

func (g Generator) Next() (Object, error) {
	switch {
	case g.Prefix == "" || !strings.HasSuffix(g.Prefix, "/"):
		return Object{}, errors.New("probe prefix must end with '/'")
	case len(g.OwnerID) < 16:
		return Object{}, errors.New("probe owner identity is invalid")
	}
	now := time.Now
	if g.Now != nil {
		now = g.Now
	}
	random := g.Random
	if random == nil {
		random = rand.Reader
	}
	nonce := make([]byte, 16)
	if _, err := io.ReadFull(random, nonce); err != nil {
		return Object{}, fmt.Errorf("generate probe nonce: %w", err)
	}
	payload := make([]byte, PayloadBytes)
	if _, err := io.ReadFull(random, payload); err != nil {
		return Object{}, fmt.Errorf("generate probe payload: %w", err)
	}
	sum := sha256.Sum256(payload)
	key := fmt.Sprintf(
		"%s%s/probe-%d-%s.bin",
		g.Prefix,
		g.OwnerID[:16],
		now().UTC().UnixNano(),
		hex.EncodeToString(nonce),
	)
	return Object{Key: key, Payload: payload, Digest: hex.EncodeToString(sum[:])}, nil
}

func Digest(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
