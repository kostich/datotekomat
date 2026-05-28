package sfat

import (
	"crypto/rand"
	"errors"
	"fmt"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/scrypt"
)

// EncMagic is the on-disk marker for encrypted blobs: UTF-8 "ШФ" (Ш + Ф).
// Treat as a constant, never append onto its backing array, copy first.
var EncMagic = []byte{0xD0, 0xA8, 0xD0, 0xA4}

// On-disk blob layout (see FORMAT.md):
//
//	off  size  field
//	  0    4   magic ("ШФ")
//	  4    1   ver   (EncVersion)
//	  5   16   salt  (scrypt)
//	 21   24   nonce (XChaCha20)
//	 45   N+16 payload (ciphertext || Poly1305 tag)
const (
	EncVersion     = byte(0x01)
	EncMagicSize   = 4
	EncVerSize     = 1
	EncSaltSize    = 16
	EncNonceSize   = 24                                                     // chacha20poly1305.NonceSizeX
	EncTagSize     = 16                                                     // chacha20poly1305.Overhead
	EncHeaderSize  = EncMagicSize + EncVerSize + EncSaltSize + EncNonceSize // 45
	EncMinBlobSize = EncHeaderSize + EncTagSize                             // 61

	// scrypt parameters (Percival recommended interactive set
	// N=2^15, r=8, p=1, derived key = 32 B.
	ScryptN     = 32768
	ScryptR     = 8
	ScryptP     = 1
	ScryptDKLen = 32
)

// MinBytesPerSectorForEncryption is the minimum sector size required to use
// кшу/кшс. 512 leaves enough room for the 45 B header + AEAD tag in one
// sector even for tiny files.
func MinBytesPerSectorForEncryption() uint32 { return 512 }

// Encrypt builds an on-disk encrypted blob from plaintext using a fresh
// random salt and nonce. The returned blob is exactly:
//
//	magic || ver || salt || nonce || Seal(plaintext, AAD=header)
func Encrypt(plaintext []byte, passphrase string) ([]byte, error) {
	salt := make([]byte, EncSaltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("неуспело генерисање соли: %w", err)
	}

	nonce := make([]byte, EncNonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("неуспело генерисање нонса: %w", err)
	}

	return encryptWithSaltNonce(plaintext, passphrase, salt, nonce)
}

// encryptWithSaltNonce is the deterministic variant used by tests for golden
// vectors. Not exported, callers outside tests must use Encrypt.
func encryptWithSaltNonce(plaintext []byte, passphrase string, salt, nonce []byte) ([]byte, error) {
	if len(salt) != EncSaltSize {
		return nil, fmt.Errorf("со мора имати тачно %d бајтова", EncSaltSize)
	}
	if len(nonce) != EncNonceSize {
		return nil, fmt.Errorf("нонс мора имати тачно %d бајтова", EncNonceSize)
	}

	key, err := scrypt.Key([]byte(passphrase), salt, ScryptN, ScryptR, ScryptP, ScryptDKLen)
	if err != nil {
		return nil, fmt.Errorf("неуспела деривација кључа: %w", err)
	}

	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, fmt.Errorf("неуспело постављање XChaCha20-Poly1305: %w", err)
	}

	header := make([]byte, 0, EncHeaderSize)
	header = append(header, EncMagic...)
	header = append(header, EncVersion)
	header = append(header, salt...)
	header = append(header, nonce...)

	payload := aead.Seal(nil, nonce, plaintext, header)

	blob := make([]byte, 0, len(header)+len(payload))
	blob = append(blob, header...)
	blob = append(blob, payload...)
	return blob, nil
}

// Decrypt validates the header of a blob, derives the key from passphrase,
// and AEAD-opens the payload. Returns plaintext on success.
func Decrypt(blob []byte, passphrase string) ([]byte, error) {
	if len(blob) < EncMinBlobSize {
		return nil, errors.New("неисправан шифровани блок (премали)")
	}

	if !bytesEqual(blob[:EncMagicSize], EncMagic) {
		return nil, errors.New("неисправан шифровани блок (очекиван маркер ШФ)")
	}

	ver := blob[EncMagicSize]
	if ver != EncVersion {
		return nil, fmt.Errorf("неподржано издање шифрованог формата: 0x%02X", ver)
	}

	salt := blob[EncMagicSize+EncVerSize : EncMagicSize+EncVerSize+EncSaltSize]
	nonce := blob[EncMagicSize+EncVerSize+EncSaltSize : EncHeaderSize]
	header := blob[:EncHeaderSize]
	payload := blob[EncHeaderSize:]

	key, err := scrypt.Key([]byte(passphrase), salt, ScryptN, ScryptR, ScryptP, ScryptDKLen)
	if err != nil {
		return nil, fmt.Errorf("неуспела деривација кључа: %w", err)
	}

	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, fmt.Errorf("неуспело постављање XChaCha20-Poly1305: %w", err)
	}

	plaintext, err := aead.Open(nil, nonce, payload, header)
	if err != nil {
		return nil, errors.New("погрешна лозинка или оштећен садржај")
	}

	return plaintext, nil
}

// bytesEqual is a tiny dependency-free byte comparison used inside this
// package. It avoids pulling in bytes just for one call site.
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
