// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"sync"
)

// structuralID is a fixed-size internal identity. User-visible chart identities,
// labels, and AWS dimensions remain exact strings.
type structuralID [sha256.Size]byte

const maxPooledStructuralIDBytes = 64 << 10

var structuralIDBufferPool = sync.Pool{New: func() any { return &bytes.Buffer{} }}

type structuralIDBuilder struct {
	buffer *bytes.Buffer
	length [8]byte
}

func newStructuralIDBuilder(domain string) structuralIDBuilder {
	buffer := structuralIDBufferPool.Get().(*bytes.Buffer)
	buffer.Reset()
	b := structuralIDBuilder{buffer: buffer}
	b.addString(domain)
	return b
}

func (b *structuralIDBuilder) addString(value string) {
	binary.BigEndian.PutUint64(b.length[:], uint64(len(value)))
	_, _ = b.buffer.Write(b.length[:])
	_, _ = b.buffer.WriteString(value)
}

func (b *structuralIDBuilder) addInt64(value int64) {
	binary.BigEndian.PutUint64(b.length[:], uint64(value))
	_, _ = b.buffer.Write(b.length[:])
}

func (b *structuralIDBuilder) addID(value structuralID) {
	_, _ = b.buffer.Write(value[:])
}

func (b *structuralIDBuilder) sum() structuralID {
	id := sha256.Sum256(b.buffer.Bytes())
	if b.buffer.Cap() <= maxPooledStructuralIDBytes {
		clear(b.buffer.Bytes())
		b.buffer.Reset()
		structuralIDBufferPool.Put(b.buffer)
	}
	b.buffer = nil
	return id
}

func structuralIDFromStrings(domain string, values ...string) structuralID {
	b := newStructuralIDBuilder(domain)
	for _, value := range values {
		b.addString(value)
	}
	return b.sum()
}
