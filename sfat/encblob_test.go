package sfat

import (
	"bytes"
	"encoding/hex"
	"testing"
)

// goldenSalt and goldenNonce are reused across the deterministic tests.
// Picking obviously-fake values (0x01..0x10 / 0x10..0x27) makes the hex
// dumps in ENCRYPTION.md self-describing.
var (
	goldenSalt  = mustDecodeHex("0102030405060708090a0b0c0d0e0f10")
	goldenNonce = mustDecodeHex("101112131415161718191a1b1c1d1e1f2021222324252627")
)

// TestEncblob_GoldenVector locks the on-disk byte layout of an encrypted blob.
// If this test fails after a code change, the disk format has changed and
// every existing encrypted file in the wild becomes unreadable, bump
// EncVersion and add a migration path before changing anything here.
//
// Parameters:
//
//	plaintext  = "Здраво!"  (13 bytes UTF-8)
//	passphrase = "тест"
//	salt       = 01..10
//	nonce      = 10..27
//
// Expected blob layout (45 B header + 13 B ciphertext + 16 B tag = 74 B):
//
//	0xD0 0xA8 0xD0 0xA4                        // magic: Ш, Ф (4 B)
//	0x01                                       // ver
//	0102030405060708090a0b0c0d0e0f10           // salt (16)
//	101112131415161718191a1b1c1d1e1f2021222324252627  // nonce (24)
//	d4a86cf80cdab900139aaeb13c                 // ciphertext (13)
//	3dd365ca3442cb8713ff3934848d1cdd           // Poly1305 tag (16)
func TestEncblob_GoldenVector(t *testing.T) {
	wantHex := "d0a8d0a401" +
		"0102030405060708090a0b0c0d0e0f10" +
		"101112131415161718191a1b1c1d1e1f2021222324252627" +
		"d4a86cf80cdab900139aaeb13c3dd365ca3442cb8713ff3934848d1cdd"
	want := mustDecodeHex(wantHex)

	got, err := encryptWithSaltNonce([]byte("Здраво!"), "тест", goldenSalt, goldenNonce)
	if err != nil {
		t.Fatalf("неочекивана грешка приликом шифровања: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("golden vector promenjen!\n  got  = %s\n  want = %s",
			hex.EncodeToString(got), hex.EncodeToString(want))
	}

	// Round-trip: decrypting the golden blob must recover plaintext.
	pt, err := Decrypt(got, "тест")
	if err != nil {
		t.Fatalf("неуспело дешифровање златног блока: %v", err)
	}
	if string(pt) != "Здраво!" {
		t.Fatalf("дешифровани отворени текст не одговара: %q", pt)
	}
}

// TestEncblob_RoundTrip uses random salt/nonce (the production path) and
// verifies that Decrypt recovers the exact plaintext for a few sizes,
// including the empty plaintext and a payload spanning multiple ChaCha20
// blocks (>64 B).
func TestEncblob_RoundTrip(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"single byte", []byte{0x42}},
		{"ascii", []byte("The quick brown fox jumps over the lazy dog")},
		{"cyrillic", []byte("Брза смеђа лисица прескаче лењог пса.")},
		{"multi-block", bytes.Repeat([]byte("ABCDEFGH"), 32)}, // 256 B
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			blob, err := Encrypt(tc.data, "лозинка")
			if err != nil {
				t.Fatalf("Encrypt: %v", err)
			}
			if len(blob) != EncHeaderSize+len(tc.data)+EncTagSize {
				t.Fatalf("неочекивана дужина блока: got %d, want %d",
					len(blob), EncHeaderSize+len(tc.data)+EncTagSize)
			}
			if !bytes.Equal(blob[:EncMagicSize], EncMagic) {
				t.Fatalf("маркер на почетку блока није ШФ: got % x", blob[:EncMagicSize])
			}
			if blob[EncMagicSize] != EncVersion {
				t.Fatalf("неочекивано издање: 0x%02x", blob[EncMagicSize])
			}

			got, err := Decrypt(blob, "лозинка")
			if err != nil {
				t.Fatalf("Decrypt: %v", err)
			}
			if !bytes.Equal(got, tc.data) {
				t.Fatalf("округли тест: got %q, want %q", got, tc.data)
			}
		})
	}
}

// TestEncblob_WrongPassphrase ensures Decrypt fails (AEAD authentication)
// rather than returning garbage when the key is wrong.
func TestEncblob_WrongPassphrase(t *testing.T) {
	blob, err := Encrypt([]byte("тајна"), "права-лозинка")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if _, err := Decrypt(blob, "погрешна-лозинка"); err == nil {
		t.Fatal("очекивана грешка при погрешној лозинки, али је Decrypt успео")
	}
}

// TestEncblob_TamperedHeader covers two corruption modes:
//
//	(a) wrong magic   -> rejected before AEAD work
//	(b) flipped byte  -> AEAD authentication fails
func TestEncblob_TamperedHeader(t *testing.T) {
	blob, err := Encrypt([]byte("Hello"), "pass")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	t.Run("wrong magic", func(t *testing.T) {
		tampered := bytes.Clone(blob)
		tampered[0] ^= 0xFF
		if _, err := Decrypt(tampered, "pass"); err == nil {
			t.Fatal("очекивана грешка због погрешног маркера")
		}
	})

	t.Run("flipped salt byte", func(t *testing.T) {
		tampered := bytes.Clone(blob)
		tampered[EncMagicSize+EncVerSize] ^= 0x01
		if _, err := Decrypt(tampered, "pass"); err == nil {
			t.Fatal("очекивана грешка због измењене соли")
		}
	})

	t.Run("flipped ciphertext byte", func(t *testing.T) {
		tampered := bytes.Clone(blob)
		tampered[EncHeaderSize] ^= 0x01
		if _, err := Decrypt(tampered, "pass"); err == nil {
			t.Fatal("очекивана грешка због измењеног шифрата")
		}
	})

	t.Run("truncated below min", func(t *testing.T) {
		short := blob[:EncMinBlobSize-1]
		if _, err := Decrypt(short, "pass"); err == nil {
			t.Fatal("очекивана грешка због прекратког блока")
		}
	})

	t.Run("unsupported version", func(t *testing.T) {
		tampered := bytes.Clone(blob)
		tampered[EncMagicSize] = 0x02 // bump version
		if _, err := Decrypt(tampered, "pass"); err == nil {
			t.Fatal("очекивана грешка због неподржане верзије")
		}
	})
}

// TestEncblob_MagicNotMutated guards against future regressions where some
// caller accidentally `append`s onto EncMagic, corrupting the package-level
// constant.
func TestEncblob_MagicNotMutated(t *testing.T) {
	before := bytes.Clone(EncMagic)
	if _, err := Encrypt([]byte("x"), "y"); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, EncMagic) {
		t.Fatalf("EncMagic је измењен! before=% x after=% x", before, EncMagic)
	}
}

// TestEncblob_MinBPS is a tiny smoke test pinning the 512 B policy; if this
// changes the ENCRYPTION.md note and CLI gate must change together.
func TestEncblob_MinBPS(t *testing.T) {
	if got := MinBytesPerSectorForEncryption(); got != 512 {
		t.Fatalf("очекивано 512, добијено %d", got)
	}
}

func mustDecodeHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}
