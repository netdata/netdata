// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostic

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"
)

func CanonicalBytes(value any) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal canonical JSON: %w", err)
	}
	return data, nil
}

func Seal(memberType MemberType, value any) (ContentRef, []byte, error) {
	if err := memberType.Validate(); err != nil {
		return ContentRef{}, nil, err
	}
	data, err := CanonicalBytes(value)
	if err != nil {
		return ContentRef{}, nil, err
	}
	ref, err := ContentRefFor(memberType, data)
	if err != nil {
		return ContentRef{}, nil, err
	}
	return ref, data, nil
}

func ContentRefFor(memberType MemberType, canonical []byte) (ContentRef, error) {
	if err := memberType.Validate(); err != nil {
		return ContentRef{}, err
	}
	if len(canonical) == 0 {
		return ContentRef{}, errors.New("canonical member is empty")
	}
	digest, err := memberDigest(memberType, canonical)
	if err != nil {
		return ContentRef{}, err
	}
	return ContentRef{
		Namespace:        ContentNamespaceV1,
		Kind:             memberType.Kind,
		Schema:           memberType.Schema,
		Canonicalization: CanonicalJSONV1,
		LogicalLength:    uint64(len(canonical)),
		SHA256:           hex.EncodeToString(digest[:]),
	}, nil
}

func VerifyContent(ref ContentRef, canonical []byte) error {
	if err := ref.Validate(); err != nil {
		return err
	}
	if uint64(len(canonical)) != ref.LogicalLength {
		return fmt.Errorf("logical length mismatch: reference=%d actual=%d", ref.LogicalLength, len(canonical))
	}
	digest, err := memberDigest(ref.Type(), canonical)
	if err != nil {
		return err
	}
	if hex.EncodeToString(digest[:]) != ref.SHA256 {
		return errors.New("content digest mismatch")
	}
	return nil
}

func memberDigest(memberType MemberType, canonical []byte) ([sha256.Size]byte, error) {
	if err := memberType.Validate(); err != nil {
		return [sha256.Size]byte{}, err
	}
	h := sha256.New()
	for _, value := range []string{
		ContentNamespaceV1,
		memberType.Kind,
		memberType.Schema,
		CanonicalJSONV1,
	} {
		if err := writeDigestField(h, []byte(value)); err != nil {
			return [sha256.Size]byte{}, err
		}
	}
	if err := binary.Write(h, binary.BigEndian, uint64(len(canonical))); err != nil {
		return [sha256.Size]byte{}, err
	}
	if _, err := h.Write(canonical); err != nil {
		return [sha256.Size]byte{}, err
	}
	var sum [sha256.Size]byte
	copy(sum[:], h.Sum(nil))
	return sum, nil
}

func writeDigestField(w io.Writer, value []byte) error {
	if err := binary.Write(w, binary.BigEndian, uint64(len(value))); err != nil {
		return err
	}
	_, err := w.Write(value)
	return err
}

func DecodeCanonical(data []byte, limits ReaderLimits, dst any) error {
	if err := limits.Validate(); err != nil {
		return err
	}
	if len(data) == 0 {
		return errors.New("empty JSON member")
	}
	if uint64(len(data)) > limits.MaxMemberBytes {
		return fmt.Errorf("member bytes %d exceeds limit %d", len(data), limits.MaxMemberBytes)
	}
	if !utf8.Valid(data) {
		return errors.New("member is not valid UTF-8")
	}
	if err := scanJSON(data, limits); err != nil {
		return err
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("decode member: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return err
	}
	canonical, err := CanonicalBytes(dst)
	if err != nil {
		return err
	}
	if !bytes.Equal(data, canonical) {
		return errors.New("member JSON is not canonical")
	}
	return nil
}

type jsonScanBudget struct {
	limits      ReaderLimits
	tokens      uint64
	stringBytes uint64
}

func scanJSON(data []byte, limits ReaderLimits) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	b := jsonScanBudget{limits: limits}
	if err := b.scanValue(decoder, 1); err != nil {
		return err
	}
	return requireJSONEOF(decoder)
}

func (b *jsonScanBudget) scanValue(decoder *json.Decoder, depth uint64) error {
	if depth > b.limits.MaxNestingDepth {
		return fmt.Errorf("JSON nesting depth %d exceeds limit %d", depth, b.limits.MaxNestingDepth)
	}
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("read JSON token: %w", err)
	}
	if err := b.addToken(token); err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}

	switch delim {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return fmt.Errorf("read JSON object key: %w", err)
			}
			if err := b.addToken(keyToken); err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("JSON object key is not a string")
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("duplicate JSON object key %q", key)
			}
			seen[key] = struct{}{}
			if err := b.scanValue(decoder, depth+1); err != nil {
				return err
			}
		}
		return b.consumeClosing(decoder, '}')
	case '[':
		for decoder.More() {
			if err := b.scanValue(decoder, depth+1); err != nil {
				return err
			}
		}
		return b.consumeClosing(decoder, ']')
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delim)
	}
}

func (b *jsonScanBudget) consumeClosing(decoder *json.Decoder, want json.Delim) error {
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("read JSON closing token: %w", err)
	}
	if err := b.addToken(token); err != nil {
		return err
	}
	if token != want {
		return fmt.Errorf("unexpected JSON closing token %v", token)
	}
	return nil
}

func (b *jsonScanBudget) addToken(token json.Token) error {
	var err error
	if b.tokens, err = checkedAdd(b.tokens, 1); err != nil {
		return err
	}
	if b.tokens > b.limits.MaxJSONTokens {
		return fmt.Errorf("JSON tokens %d exceeds limit %d", b.tokens, b.limits.MaxJSONTokens)
	}
	if value, ok := token.(string); ok {
		if b.stringBytes, err = checkedAdd(b.stringBytes, uint64(len(value))); err != nil {
			return err
		}
		if b.stringBytes > b.limits.MaxStringBytes {
			return fmt.Errorf("JSON string bytes %d exceeds limit %d", b.stringBytes, b.limits.MaxStringBytes)
		}
	}
	return nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	if _, err := decoder.Token(); err == io.EOF {
		return nil
	} else if err != nil {
		return fmt.Errorf("read trailing JSON: %w", err)
	}
	return errors.New("multiple JSON values are not supported")
}
