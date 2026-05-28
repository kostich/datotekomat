package sfat

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// dataentryFixture spins up a fresh on-disk filesystem and pre-allocates a
// FAT chain of the requested length. Returns the Filesystem (re-read from
// disk so it has accurate superblock counters) and the FAT entry index that
// roots the allocated chain.
//
// We use 512 B sectors (matches the кшу/кшс minimum) for these tests
// because they exercise the same path encrypted files will take.
func dataentryFixture(t *testing.T, sectorsToAllocate int) (*Filesystem, uint32) {
	t.Helper()

	tempDir := t.TempDir()
	imgPath := filepath.Join(tempDir, "dataentry.img")

	if _, err := New(8, 512, 4, "ТЕСТ", imgPath, TestTimestamp); err != nil {
		t.Fatalf("New: %v", err)
	}

	fs, err := Read(imgPath)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	root, err := fs.FileAllocationTable.AllocateFATEntry(sectorsToAllocate, fs.Path, fs.SuperBlock)
	if err != nil {
		t.Fatalf("AllocateFATEntry(%d): %v", sectorsToAllocate, err)
	}
	return fs, uint32(root)
}

// TestWriteBlob_RoundTrip verifies that WriteBlob followed by ReadFATData
// returns exactly the input bytes for a range of sizes, including the
// boundary cases (zero bytes, exact sector multiple, one byte short of
// sector, one byte over sector).
func TestWriteBlob_RoundTrip(t *testing.T) {
	cases := []struct {
		name    string
		size    int
		sectors int
	}{
		{"empty in 1 sector", 0, 1},
		{"one byte", 1, 1},
		{"under one sector", 200, 1},
		{"exactly one sector", 512, 1},
		{"one byte over sector", 513, 2},
		{"under two sectors", 1000, 2},
		{"exactly two sectors", 1024, 2},
		{"three sectors with remainder", 1500, 3},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs, root := dataentryFixture(t, tc.sectors)

			data := bytes.Repeat([]byte{0xAB}, tc.size)
			for i := range data {
				data[i] = byte(i % 251) // deterministic, non-uniform
			}

			if err := fs.DataArea.WriteBlob(fs, root, data); err != nil {
				t.Fatalf("WriteBlob: %v", err)
			}

			got, err := fs.DataArea.ReadFATData(fs, root, uint32(tc.size))
			if err != nil {
				t.Fatalf("ReadFATData: %v", err)
			}
			if !bytes.Equal(got, data) {
				t.Fatalf("округли тест: дужина %d, очекивано % x, добијено % x",
					tc.size, data[:min(16, len(data))], got[:min(16, len(got))])
			}
		})
	}
}

// TestWriteBlob_ZeroPadsFinalSector is the regression test for the bug
// called out in the plan: WriteEntry leaves stale buffer bytes in the last
// sector when the source isn't sector-aligned. WriteBlob must not.
//
// Strategy: write a sector worth of 0xFF bytes first (poisoning the data
// area), then call WriteBlob with a shorter payload and read the underlying
// image bytes directly, the slack must be 0x00.
func TestWriteBlob_ZeroPadsFinalSector(t *testing.T) {
	fs, root := dataentryFixture(t, 2)
	bps := int(fs.SuperBlock.BytesPerSector)

	// Poison both sectors with 0xFF so we can prove WriteBlob overwrote
	// them rather than leaving leftovers from a previous tenant.
	poison := bytes.Repeat([]byte{0xFF}, 2*bps)
	if err := fs.DataArea.WriteBlob(fs, root, poison); err != nil {
		t.Fatalf("seed WriteBlob: %v", err)
	}

	payload := []byte("ШФ-test-payload") // 17 B, shorter than a sector
	if err := fs.DataArea.WriteBlob(fs, root, payload); err != nil {
		t.Fatalf("WriteBlob: %v", err)
	}

	// Inspect the raw image to check the slack region of the final
	// used sector (= sector 0 since payload < bps) is all zero.
	img, err := os.ReadFile(fs.Path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	chain, err := fs.FileAllocationTable.GetEntryChain(root, fs.Path, fs.SuperBlock)
	if err != nil {
		t.Fatalf("GetEntryChain: %v", err)
	}
	dataAreaOffset := BOOTLOADER_SIZE + SUPERBLOCK_SIZE +
		(int(fs.SuperBlock.TotalFSEntries) * FSENTRY_SIZE) +
		(int(fs.SuperBlock.TotalSectors) * FATENTRY_SIZE)

	sector0Off := dataAreaOffset + int(chain[0])*bps
	sector0 := img[sector0Off : sector0Off+bps]
	if !bytes.Equal(sector0[:len(payload)], payload) {
		t.Fatalf("почетак сектора не одговара отвореном тексту: % x", sector0[:len(payload)])
	}
	for i := len(payload); i < bps; i++ {
		if sector0[i] != 0x00 {
			t.Fatalf("стари бајт је остао на позицији %d: 0x%02x (мора бити 0x00)",
				i, sector0[i])
		}
	}

	// And the second (unused) sector must be entirely zero too.
	sector1Off := dataAreaOffset + int(chain[1])*bps
	sector1 := img[sector1Off : sector1Off+bps]
	for i, b := range sector1 {
		if b != 0x00 {
			t.Fatalf("други сектор није нула на позицији %d: 0x%02x", i, b)
		}
	}
}

// TestWriteBlob_TooBig refuses to silently truncate.
func TestWriteBlob_TooBig(t *testing.T) {
	fs, root := dataentryFixture(t, 1)
	// 1 sector = 512 B; ask to write 513.
	data := bytes.Repeat([]byte{0xCC}, 513)
	if err := fs.DataArea.WriteBlob(fs, root, data); err == nil {
		t.Fatal("очекивана грешка због прекорачења ТДД ланца, али WriteBlob је успео")
	}
}

// TestReadFATData_ShortChain refuses to over-read.
func TestReadFATData_ShortChain(t *testing.T) {
	fs, root := dataentryFixture(t, 1)
	if _, err := fs.DataArea.ReadFATData(fs, root, 513); err == nil {
		t.Fatal("очекивана грешка због тражења више бајтова него што ланац садржи")
	}
}

// TestReadFATData_ZeroSize handles the edge case of size==0 (e.g. an empty
// encrypted plaintext is still 61 B on disk, but the public-facing
// "plaintext length" view of ReadFATData with size==0 should return an
// empty slice without touching the chain).
func TestReadFATData_ZeroSize(t *testing.T) {
	fs, root := dataentryFixture(t, 1)
	got, err := fs.DataArea.ReadFATData(fs, root, 0)
	if err != nil {
		t.Fatalf("ReadFATData(0): %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("очекиван празан исечак, добијено % x", got)
	}
}
