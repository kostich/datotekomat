package sfat

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// encFixture builds a fresh image with BPS=512 and totalSectors=8, then
// returns the image path plus a host directory the test can put inputs
// into. 8 sectors × 512 B = 4096 B of data area, enough for several
// encrypted files plus the root folder's directory sector.
func encFixture(t *testing.T, bps int) string {
	t.Helper()
	tempDir := t.TempDir()
	img := filepath.Join(tempDir, "enc.img")
	if _, err := New(8, bps, 6, "ШИФР", img, TestTimestamp); err != nil {
		t.Fatalf("New: %v", err)
	}
	return img
}

// writeHost is a tiny helper to put a plaintext file on the host so we can
// hand it to CopyEncryptedFileIn.
func writeHost(t *testing.T, dir, name string, content []byte, mode os.FileMode) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, content, mode); err != nil {
		t.Fatalf("WriteFile %v: %v", p, err)
	}
	if err := os.Chmod(p, mode); err != nil {
		t.Fatalf("Chmod %v: %v", p, err)
	}
	return p
}

// TestCopyEncrypted_RoundTrip is the happy path: write, list, read back,
// confirm the FSEntry uses TYPE_ENCRYPTED + blob-CRC + blob-length, and
// the raw blob in the data area starts with the ШФ magic.
func TestCopyEncrypted_RoundTrip(t *testing.T) {
	img := encFixture(t, 512)
	hostDir := t.TempDir()
	src := writeHost(t, hostDir, "tajna.txt",
		[]byte("Здраво, ово је тајна порука!\n"), 0640)

	pass := "лозинка-1"

	if err := CopyEncryptedFileIn(src, "/", img, pass, TestTimestamp); err != nil {
		t.Fatalf("CopyEncryptedFileIn: %v", err)
	}

	fs, err := Read(img)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	// Locate the entry. кпс must not find it; кшс via TYPE_ENCRYPTED must.
	if _, err := fs.FindFSEntryNumber("/tajna.txt", TYPE_FILE); err == nil {
		t.Fatal("TYPE_FILE је нашао шифровану датотеку, кпс не сме")
	}
	entryNo, err := fs.FindFSEntryNumber("/tajna.txt", TYPE_ENCRYPTED)
	if err != nil {
		t.Fatalf("FindFSEntryNumber(TYPE_ENCRYPTED): %v", err)
	}
	entry, err := fs.FSEntries.GetEntry(fs.Path, fs.SuperBlock, int(entryNo))
	if err != nil {
		t.Fatalf("GetEntry: %v", err)
	}

	if entry.Type != TYPE_ENCRYPTED {
		t.Fatalf("Type = 0x%02X, очекивано 0x%02X", entry.Type, TYPE_ENCRYPTED)
	}
	// Size on disk = plaintext + 45 header + 16 tag.
	plaintextLen := uint32(len([]byte("Здраво, ово је тајна порука!\n")))
	if entry.Size != plaintextLen+uint32(EncHeaderSize)+uint32(EncTagSize) {
		t.Fatalf("Size = %d, очекивано %d", entry.Size,
			plaintextLen+uint32(EncHeaderSize)+uint32(EncTagSize))
	}

	// CRC on entry must match CRC of the blob, not of plaintext.
	blob, err := fs.DataArea.ReadFATData(fs, entry.FATEntry, entry.Size)
	if err != nil {
		t.Fatalf("ReadFATData: %v", err)
	}
	if got := CalculateCRC32(blob); got != entry.Checksum {
		t.Fatalf("CRC blob 0x%08X, ставка 0x%08X, мора се поклапати", got, entry.Checksum)
	}
	if bytes.Equal(blob[:4], []byte("ШФ")[:4]) == false &&
		!bytes.Equal(blob[:4], EncMagic) {
		t.Fatalf("блок не почиње маркером ШФ: % x", blob[:4])
	}

	// Round-trip via CopyEncryptedFileOut.
	outPath := filepath.Join(hostDir, "decrypted.txt")
	if err := CopyEncryptedFileOut("/tajna.txt", outPath, img, pass); err != nil {
		t.Fatalf("CopyEncryptedFileOut: %v", err)
	}
	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(got, []byte("Здраво, ово је тајна порука!\n")) {
		t.Fatalf("дешифровани садржај се не поклапа: %q", got)
	}

	// Permissions transferred. We wrote with 0640 so user=rw, group=r,
	// world=0; encoded as on FS the perm bytes are 0x06/0x04/0x00.
	if entry.UserPerm != 0x06 || entry.GroupPerm != 0x04 || entry.WorldPerm != 0x00 {
		t.Fatalf("очекивано 6/4/0, добијено %d/%d/%d",
			entry.UserPerm, entry.GroupPerm, entry.WorldPerm)
	}
}

// TestCopyEncrypted_BPSGate ensures кшу/кшс refuse to run on a small-sector
// image. Default tests use bps=16; we exercise it directly here.
func TestCopyEncrypted_BPSGate(t *testing.T) {
	img := encFixture(t, 16) // below the 512-byte threshold
	hostDir := t.TempDir()
	src := writeHost(t, hostDir, "x.txt", []byte("xyz"), 0644)

	err := CopyEncryptedFileIn(src, "/", img, "pw", TestTimestamp)
	if err == nil {
		t.Fatal("очекивана грешка због бпс<512, али CopyEncryptedFileIn је успео")
	}
}

// TestCopyEncrypted_WrongPassphrase exercises the AEAD failure path.
func TestCopyEncrypted_WrongPassphrase(t *testing.T) {
	img := encFixture(t, 512)
	hostDir := t.TempDir()
	src := writeHost(t, hostDir, "x.txt", []byte("payload"), 0644)

	if err := CopyEncryptedFileIn(src, "/", img, "right", TestTimestamp); err != nil {
		t.Fatalf("CopyEncryptedFileIn: %v", err)
	}
	outPath := filepath.Join(hostDir, "dec.txt")
	if err := CopyEncryptedFileOut("/x.txt", outPath, img, "wrong"); err == nil {
		t.Fatal("очекивана грешка због погрешне лозинке")
	}
	if _, err := os.Stat(outPath); err == nil {
		// We created the file before Decrypt; if decrypt failed, the
		// file may have been created but should be empty. This is not
		// a critical guarantee, the test passes either way; only the
		// "no plaintext written" property matters and that is implied
		// by the Decrypt-then-Write order.
		got, _ := os.ReadFile(outPath)
		if len(got) > 0 {
			t.Fatalf("отворен текст је исцурео у %v: %q", outPath, got)
		}
	}
}

// TestCopyEncrypted_BitRot flips one byte in the data area and confirms
// the CRC32 guard fires before AEAD.
func TestCopyEncrypted_BitRot(t *testing.T) {
	img := encFixture(t, 512)
	hostDir := t.TempDir()
	src := writeHost(t, hostDir, "x.txt", []byte("payload payload"), 0644)

	if err := CopyEncryptedFileIn(src, "/", img, "pw", TestTimestamp); err != nil {
		t.Fatalf("CopyEncryptedFileIn: %v", err)
	}

	// Find the blob's first sector on disk and flip a byte inside the
	// ciphertext (offset 50, well past header+magic).
	fs, _ := Read(img)
	entryNo, _ := fs.FindFSEntryNumber("/x.txt", TYPE_ENCRYPTED)
	entry, _ := fs.FSEntries.GetEntry(fs.Path, fs.SuperBlock, int(entryNo))
	chain, _ := fs.FileAllocationTable.GetEntryChain(entry.FATEntry, fs.Path, fs.SuperBlock)
	dataAreaOffset := BOOTLOADER_SIZE + SUPERBLOCK_SIZE +
		(int(fs.SuperBlock.TotalFSEntries) * FSENTRY_SIZE) +
		(int(fs.SuperBlock.TotalSectors) * FATENTRY_SIZE)
	sector0 := dataAreaOffset + int(chain[0])*int(fs.SuperBlock.BytesPerSector)

	f, err := os.OpenFile(img, os.O_RDWR, 0640)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	if _, err := f.Seek(int64(sector0+50), 0); err != nil {
		t.Fatalf("Seek: %v", err)
	}
	buf := make([]byte, 1)
	if _, err := f.Read(buf); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if _, err := f.Seek(int64(sector0+50), 0); err != nil {
		t.Fatalf("Seek2: %v", err)
	}
	buf[0] ^= 0xFF
	if _, err := f.Write(buf); err != nil {
		t.Fatalf("Write: %v", err)
	}
	f.Close()

	outPath := filepath.Join(hostDir, "dec.txt")
	if err := CopyEncryptedFileOut("/x.txt", outPath, img, "pw"); err == nil {
		t.Fatal("очекивана грешка због оштећења блока (ЦРЦ32 проверa)")
	}
}

// TestCopyEncrypted_PlaintextCopyOutRejects confirms кпс (CopyFileOut)
// cannot extract an encrypted entry, it must look up TYPE_FILE only.
func TestCopyEncrypted_PlaintextCopyOutRejects(t *testing.T) {
	img := encFixture(t, 512)
	hostDir := t.TempDir()
	src := writeHost(t, hostDir, "x.txt", []byte("hi"), 0644)

	if err := CopyEncryptedFileIn(src, "/", img, "pw", TestTimestamp); err != nil {
		t.Fatalf("CopyEncryptedFileIn: %v", err)
	}
	outPath := filepath.Join(hostDir, "kpc-out.txt")
	if err := CopyFileOut("/x.txt", outPath, img); err == nil {
		t.Fatal("кпс је успео на TYPE_ENCRYPTED, мора одбити")
	}
}

// TestCopyEncrypted_PlaintextCopyInMakesFile is the symmetric check: кпу
// must only create TYPE_FILE entries, never TYPE_ENCRYPTED.
func TestCopyEncrypted_PlaintextCopyInMakesFile(t *testing.T) {
	img := encFixture(t, 512)
	hostDir := t.TempDir()
	src := writeHost(t, hostDir, "p.txt", []byte("plain"), 0644)

	if err := CopyFileIn(src, "/", img, TestTimestamp); err != nil {
		t.Fatalf("CopyFileIn: %v", err)
	}
	fs, _ := Read(img)
	if _, err := fs.FindFSEntryNumber("/p.txt", TYPE_FILE); err != nil {
		t.Fatalf("очекиван TYPE_FILE: %v", err)
	}
	if _, err := fs.FindFSEntryNumber("/p.txt", TYPE_ENCRYPTED); err == nil {
		t.Fatal("кпу не сме створити TYPE_ENCRYPTED ставку")
	}
}

// TestCopyEncrypted_NameCollision_AllTypes confirms кшу refuses a path
// that already exists as any other entry type.
func TestCopyEncrypted_NameCollision_AllTypes(t *testing.T) {
	hostDir := t.TempDir()
	src := writeHost(t, hostDir, "name.txt", []byte("data"), 0644)

	// Each subtest reformats a fresh image so collisions are isolated.
	t.Run("vs plain file", func(t *testing.T) {
		img := encFixture(t, 512)
		if err := CopyFileIn(src, "/", img, TestTimestamp); err != nil {
			t.Fatalf("CopyFileIn: %v", err)
		}
		if err := CopyEncryptedFileIn(src, "/", img, "pw", TestTimestamp); err == nil {
			t.Fatal("очекивана грешка због колизије с TYPE_FILE")
		}
	})

	t.Run("vs folder", func(t *testing.T) {
		img := encFixture(t, 512)
		fs, _ := Read(img)
		if err := fs.CreateFolder("/name.txt", TestTimestamp); err != nil {
			t.Fatalf("CreateFolder: %v", err)
		}
		if err := CopyEncryptedFileIn(src, "/", img, "pw", TestTimestamp); err == nil {
			t.Fatal("очекивана грешка због колизије с TYPE_FOLDER")
		}
	})

	t.Run("vs encrypted (re-import)", func(t *testing.T) {
		img := encFixture(t, 512)
		if err := CopyEncryptedFileIn(src, "/", img, "pw", TestTimestamp); err != nil {
			t.Fatalf("first CopyEncryptedFileIn: %v", err)
		}
		if err := CopyEncryptedFileIn(src, "/", img, "pw2", TestTimestamp); err == nil {
			t.Fatal("очекивана грешка због колизије с TYPE_ENCRYPTED")
		}
	})
}

// TestCopyEncrypted_DeleteThenList covers the "metadata-only" lifecycle:
// делете must work on TYPE_ENCRYPTED without a passphrase.
func TestCopyEncrypted_DeleteThenList(t *testing.T) {
	img := encFixture(t, 512)
	hostDir := t.TempDir()
	src := writeHost(t, hostDir, "ш.txt", []byte("tajno"), 0644)

	if err := CopyEncryptedFileIn(src, "/", img, "pw", TestTimestamp); err != nil {
		t.Fatalf("CopyEncryptedFileIn: %v", err)
	}
	fs, _ := Read(img)
	if err := fs.DeleteEntry("/ш.txt"); err != nil {
		t.Fatalf("DeleteEntry: %v", err)
	}
	// Re-read; entry must no longer be findable.
	fs2, _ := Read(img)
	if _, err := fs2.FindFSEntryNumber("/ш.txt", TYPE_ENCRYPTED); err == nil {
		t.Fatal("брисана ставка је и даље присутна")
	}
}
