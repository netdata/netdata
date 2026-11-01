// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"bytes"
	"context"
	"crypto"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
	openpgp "github.com/ProtonMail/go-crypto/openpgp/v2"
	"github.com/netdata/netdata/go/plugins/pkg/safefile"
)

// These bounds apply before parsing attacker-controlled OpenPGP data. Public
// package key history fits within the metadata bound; signatures contain one packet.
const ReleaseKeyringLimit = 1 << 20
const ReleaseSignatureLimit = 64 << 10
const ReleaseKeyringURL = "https://repository.netdata.cloud/netdatabot.gpg.key"

// ReleaseTrust is scoped to one preparation attempt, never persisted or refreshed
// by execution/status paths. Parsing is repeated for verification because the
// OpenPGP library caches certificate validation in mutable entities.
type ReleaseTrust struct{ binary []byte }

func validateReleaseTrust(trusted *ReleaseTrust) error {
	if trusted == nil || len(trusted.binary) == 0 {
		return errors.New("no runtime release verification key is configured")
	}
	return nil
}

// ParseReleaseKeyring accepts only public key packets, identities and signatures.
// Expired/revoked historical keys are retained; validity is checked for the signer.
func ParseReleaseKeyring(data []byte) (*ReleaseTrust, error) {
	if len(data) == 0 || len(data) > ReleaseKeyringLimit {
		return nil, errors.New("runtime public keyring exceeds protocol size limit or is empty")
	}
	binary := data
	trimmed := bytes.TrimSpace(data)
	if bytes.HasPrefix(trimmed, []byte("-----BEGIN")) {
		const begin = "-----BEGIN PGP PUBLIC KEY BLOCK-----"
		const end = "-----END PGP PUBLIC KEY BLOCK-----"
		if !bytes.HasPrefix(trimmed, []byte(begin)) || !bytes.HasSuffix(trimmed, []byte(end)) || bytes.Count(trimmed, []byte(begin)) != 1 || bytes.Count(trimmed, []byte(end)) != 1 {
			return nil, errors.New("expected one public OpenPGP armor block")
		}
		block, err := armor.Decode(bytes.NewReader(trimmed))
		if err != nil || block.Type != openpgp.PublicKeyType {
			return nil, errors.New("invalid public OpenPGP armor")
		}
		binary, err = io.ReadAll(io.LimitReader(block.Body, ReleaseKeyringLimit+1))
		if err != nil || len(binary) > ReleaseKeyringLimit {
			return nil, errors.New("invalid public OpenPGP armor body")
		}
	}
	reader := bytes.NewReader(binary)
	for reader.Len() > 0 {
		p, err := readReleasePacket(reader)
		if err != nil {
			return nil, errors.New("invalid or unsupported public OpenPGP packet")
		}
		switch p.(type) {
		case *packet.PublicKey, *packet.UserId, *packet.UserAttribute:
		case *packet.Signature:
			switch p.(*packet.Signature).SigType {
			case packet.SigTypeGenericCert, packet.SigTypePersonaCert, packet.SigTypeCasualCert, packet.SigTypePositiveCert,
				packet.SigTypeSubkeyBinding, packet.SigTypePrimaryKeyBinding, packet.SigTypeDirectSignature,
				packet.SigTypeKeyRevocation, packet.SigTypeSubkeyRevocation, packet.SigTypeCertificationRevocation:
			default:
				return nil, errors.New("non-key signature material in public OpenPGP keyring")
			}
		default:
			return nil, errors.New("unsupported material in public OpenPGP keyring")
		}
	}
	entities, err := openpgp.ReadKeyRing(bytes.NewReader(binary))
	if err != nil || len(entities) == 0 {
		return nil, errors.New("invalid public OpenPGP keyring")
	}
	return &ReleaseTrust{binary: bytes.Clone(binary)}, nil
}

func ReadReleaseKeyring(path string) (*ReleaseTrust, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("offline runtime keyring requires an absolute path")
	}
	file, err := safefile.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return readReleaseKeyring(file)
}

func readReleaseKeyring(reader io.Reader) (*ReleaseTrust, error) {
	data, err := io.ReadAll(io.LimitReader(reader, ReleaseKeyringLimit+1))
	if err != nil {
		return nil, err
	}
	return ParseReleaseKeyring(data)
}

// FetchReleaseKeyring always uses the canonical HTTPS authority. There is no
// descriptor-supplied URL, keyserver, persistent cache or failed-refresh fallback.
func FetchReleaseKeyring(ctx context.Context) (*ReleaseTrust, error) {
	body, err := openDownload(ctx, ReleaseKeyringURL, downloadIdleTimeout)
	if err != nil {
		return nil, fmt.Errorf("runtime public keyring: %w", err)
	}
	defer body.Close()
	return readReleaseKeyring(body)
}

func verifyReleaseSignature(payload, signature []byte, trust *ReleaseTrust) error {
	if err := validateReleaseTrust(trust); err != nil {
		return err
	}
	if len(signature) == 0 || len(signature) > ReleaseSignatureLimit {
		return errors.New("invalid runtime release signature size")
	}
	reader := bytes.NewReader(signature)
	p, err := readReleasePacket(reader)
	if err != nil || reader.Len() != 0 {
		return errors.New("runtime release requires exactly one binary OpenPGP signature packet")
	}
	sig, ok := p.(*packet.Signature)
	if !ok || sig.SigType != packet.SigTypeBinary || sig.IssuerKeyId == nil {
		return errors.New("runtime release requires a binary detached signature")
	}
	switch sig.Hash {
	case crypto.SHA256, crypto.SHA384, crypto.SHA512, crypto.SHA3_256, crypto.SHA3_512:
	default:
		return errors.New("runtime release signature requires SHA256 or stronger")
	}
	entities, err := openpgp.ReadKeyRing(bytes.NewReader(trust.binary))
	if err != nil {
		return errors.New("invalid runtime release verification keyring")
	}
	now := time.Now()
	config := &packet.Config{Time: func() time.Time { return now }}
	_, signer, err := openpgp.VerifyDetachedSignature(entities, bytes.NewReader(payload), bytes.NewReader(signature), config)
	if err != nil {
		return fmt.Errorf("runtime release is not signed by a trusted key: %w", err)
	}
	// Verification at signature time permits historical signatures. Preparation
	// additionally requires the exact signing key and its primary to be valid now.
	if _, ok := signer.SigningKeyById(now, *sig.IssuerKeyId, config); !ok || sig.SigExpired(now) {
		return errors.New("runtime release signer or signature is expired or revoked")
	}
	return nil
}

// Keyrings and detached signatures use definite packet lengths. Checking framing
// here prevents packet parsers from accepting truncated length declarations or
// silently treating material inside a packet as a second packet.
func readReleasePacket(reader *bytes.Reader) (packet.Packet, error) {
	first, err := reader.ReadByte()
	if err != nil || first&0x80 == 0 {
		return nil, errors.New("invalid OpenPGP packet header")
	}
	header := []byte{first}
	var size uint32
	if first&0x40 == 0 {
		kind := first & 3
		if kind == 3 {
			return nil, errors.New("indeterminate OpenPGP packet length")
		}
		length := make([]byte, 1<<kind)
		if _, err := io.ReadFull(reader, length); err != nil {
			return nil, err
		}
		header = append(header, length...)
		for _, b := range length {
			size = size<<8 | uint32(b)
		}
	} else {
		length, err := reader.ReadByte()
		if err != nil {
			return nil, err
		}
		header = append(header, length)
		switch {
		case length < 192:
			size = uint32(length)
		case length < 224:
			next, err := reader.ReadByte()
			if err != nil {
				return nil, err
			}
			header = append(header, next)
			size = (uint32(length)-192)*256 + uint32(next) + 192
		case length == 255:
			var lengthBytes [4]byte
			if _, err := io.ReadFull(reader, lengthBytes[:]); err != nil {
				return nil, err
			}
			header = append(header, lengthBytes[:]...)
			size = binary.BigEndian.Uint32(lengthBytes[:])
		default:
			return nil, errors.New("partial OpenPGP packet length")
		}
	}
	if uint64(size) > uint64(reader.Len()) {
		return nil, io.ErrUnexpectedEOF
	}
	frame := make([]byte, len(header)+int(size))
	copy(frame, header)
	if _, err := io.ReadFull(reader, frame[len(header):]); err != nil {
		return nil, err
	}
	framed := bytes.NewReader(frame)
	p, err := packet.Read(framed)
	if err != nil || framed.Len() != 0 {
		return nil, errors.New("invalid or trailing OpenPGP packet material")
	}
	return p, nil
}
