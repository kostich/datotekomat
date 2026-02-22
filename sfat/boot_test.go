package sfat

import (
	"os"
	"testing"
)

// TestBootloaderWrite tests the writing of a bootloader to the boot area
func TestBootloaderWrite(t *testing.T) {
	type bootTest struct {
		testname   string
		bootloader []byte
	}

	// Case 1: bootloader smaller than 4096 bytes (512 bytes)
	smallBoot := make([]byte, 512)
	smallBoot[0] = 0xEB // JMP short
	smallBoot[1] = 0x3C
	smallBoot[2] = 0x90 // NOP
	smallBoot[510] = 0x55
	smallBoot[511] = 0xAA // boot signature

	// Case 2: bootloader exactly 4096 bytes
	fullBoot := make([]byte, BOOTLOADER_SIZE)
	fullBoot[0] = 0xEB
	fullBoot[1] = 0x3C
	fullBoot[2] = 0x90
	fullBoot[510] = 0x55
	fullBoot[511] = 0xAA
	fullBoot[4094] = 0xDE
	fullBoot[4095] = 0xAD // end marker

	tests := []bootTest{
		{
			testname:   "TestBootloaderWrite512B",
			bootloader: smallBoot,
		},
		{
			testname:   "TestBootloaderWrite4096B",
			bootloader: fullBoot,
		},
	}

	for _, tt := range tests {
		tempDir := t.TempDir()

		// create a temp filesystem image
		tempFSFile, err := os.CreateTemp(tempDir, "testfs-*.img")
		if err != nil {
			t.Fatalf("%v: %v", tt.testname, err)
		}
		tempFSFile.Close()

		_, err = generateFS(8, 16, 4, "BLTEST", tempFSFile.Name())
		if err != nil {
			t.Fatalf("%v: %v", tt.testname, err)
		}

		// write the dummy bootloader to a temp file
		tempBootFile, err := os.CreateTemp(tempDir, "bootloader-*.bin")
		if err != nil {
			t.Fatalf("%v: %v", tt.testname, err)
		}
		if _, err := tempBootFile.Write(tt.bootloader); err != nil {
			t.Fatalf("%v: %v", tt.testname, err)
		}
		tempBootFile.Close()

		// write the bootloader to the filesystem image
		testfs, err := Read(tempFSFile.Name())
		if err != nil {
			t.Fatalf("%v: %v", tt.testname, err)
		}

		err = testfs.WriteBootArea(tempBootFile.Name())
		if err != nil {
			t.Fatalf("%v: %v", tt.testname, err)
		}

		// read back the first BOOTLOADER_SIZE bytes from the image
		fsFile, err := os.Open(tempFSFile.Name())
		if err != nil {
			t.Fatalf("%v: %v", tt.testname, err)
		}

		actual := make([]byte, BOOTLOADER_SIZE)
		_, err = fsFile.Read(actual)
		fsFile.Close()
		if err != nil {
			t.Fatalf("%v: %v", tt.testname, err)
		}

		// build the expected 4096-byte boot area (zero-padded if shorter)
		expected := make([]byte, BOOTLOADER_SIZE)
		copy(expected, tt.bootloader)

		// compare byte by byte
		err = checkBinary(expected, actual)
		if err != nil {
			t.Fatalf("%v: %v", tt.testname, err)
		}
	}
}
