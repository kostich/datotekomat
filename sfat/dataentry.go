package sfat

import (
	"fmt"
	"os"
)

type DataBlock struct {
	Content []byte
}

type DataArea struct{}

func (da *DataArea) GetEntry(fsPath string, sb *SuperBlock, entryNo int) (*DataBlock, error) {
	// open the file for reading
	file, err := os.OpenFile(fsPath, os.O_RDONLY, 0640)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// calculate offset
	offset := BOOTLOADER_SIZE + SUPERBLOCK_SIZE +
		(int(sb.TotalFSEntries) * FSENTRY_SIZE) + (int(sb.TotalSectors) * FATENTRY_SIZE)
	for i := 0; i < entryNo; i++ {
		offset += int(sb.BytesPerSector)
	}

	// check if offset within file
	fileInfo, err := os.Stat(fsPath)
	if err != nil {
		return nil, fmt.Errorf("не могу утврдити величину ФС-а: %v", err)
	}

	if int64(offset+int(sb.BytesPerSector)) > fileInfo.Size() {
		return nil, fmt.Errorf("тражени блок податка превазилази величину датотеке")
	}

	// read data entry
	file.Seek(int64(offset), 0)

	buffer := make([]byte, int(sb.BytesPerSector))
	_, err = file.Read(buffer)
	if err != nil {
		return nil, err
	}

	return &DataBlock{Content: buffer}, nil
}

func (da *DataArea) WriteEntry(fs *Filesystem, fatEntryNo uint32, sourcePath string, sourceContents []byte) error {
	// open the fs file for writing
	fsFile, err := os.OpenFile(fs.Path, os.O_WRONLY, 0640)
	if err != nil {
		return err
	}
	defer fsFile.Close()

	if sourcePath == "" {
		// create a temporary directory
		tempDir, err := os.MkdirTemp("", "example")
		if err != nil {
			return fmt.Errorf("не могу направити привремену фасциклу: %v", err)
		}
		defer os.RemoveAll(tempDir)

		tempFile, err := os.CreateTemp(tempDir, "datotekomat-*.bin")
		if err != nil {
			return fmt.Errorf("неуспех при прављењу привремене датотеке: %v", err)
		}

		// write the sourceContents to a file that can be read later on
		_, err = tempFile.Write(sourceContents)
		if err != nil {
			return fmt.Errorf("неуспех при упису у привремену датотеку: %v", err)
		}
		tempFile.Close()

		sourcePath = tempFile.Name()
	}

	// open the source file for reading
	sourceFile, err := os.OpenFile(sourcePath, os.O_RDONLY, 0640)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	// get the fat entries that point to the correct data area
	fatEntries, err := fs.FileAllocationTable.GetEntryChain(uint32(fatEntryNo), fs.Path, fs.SuperBlock)
	if err != nil {
		return err
	}

	// go to the start of the data area
	offset := BOOTLOADER_SIZE + SUPERBLOCK_SIZE +
		(int(fs.SuperBlock.TotalFSEntries) * FSENTRY_SIZE) + (int(fs.SuperBlock.TotalSectors) * FATENTRY_SIZE)

	// write the source file to the data area
	for _, entry := range fatEntries {
		// read the amount from the source file
		buffer := make([]byte, fs.SuperBlock.BytesPerSector)
		_, err := sourceFile.Read(buffer)
		if err != nil {
			return err
		}

		// calc the correct data block
		blockOffset := offset
		for i := 0; i < int(entry); i++ {
			blockOffset += int(fs.SuperBlock.BytesPerSector)
		}

		fsFile.Seek(int64(blockOffset), 0)
		_, err = fsFile.Write(buffer)
		if err != nil {
			return fmt.Errorf("не могу уписати податак у блок: %v", err)
		}
	}

	return nil
}

// ReadFATData reads exactly `size` bytes starting from the FAT chain rooted
// at fatEntryNo. The last sector in the chain may contain stale or padding
// bytes; only the first `size` bytes overall are returned. Returns an error
// if the chain is shorter than requested.
//
// Used by the encrypted path (кшс) where `size` == FSEntry.Size == blob
// length; an exact size lets us drop the AEAD-incompatible padding without
// any plaintext-length leakage.
func (da *DataArea) ReadFATData(fs *Filesystem, fatEntryNo uint32, size uint32) ([]byte, error) {
	chain, err := fs.FileAllocationTable.GetEntryChain(fatEntryNo, fs.Path, fs.SuperBlock)
	if err != nil {
		return nil, err
	}

	bps := int(fs.SuperBlock.BytesPerSector)
	if uint64(len(chain))*uint64(bps) < uint64(size) {
		return nil, fmt.Errorf(
			"ТДД ланац (%d сектора пута %d Б) краћи од тражене величине %d Б",
			len(chain), bps, size,
		)
	}

	out := make([]byte, 0, size)
	remaining := int(size)
	for _, sectorIdx := range chain {
		if remaining <= 0 {
			break
		}
		block, err := da.GetEntry(fs.Path, fs.SuperBlock, int(sectorIdx))
		if err != nil {
			return nil, err
		}
		take := bps
		if take > remaining {
			take = remaining
		}
		out = append(out, block.Content[:take]...)
		remaining -= take
	}

	return out, nil
}

// WriteBlob writes `data` into the FAT chain rooted at fatEntryNo, one
// sector at a time. The final sector is **explicitly zero-padded**: stale
// bytes from previous writes (or from this function's own per-sector buffer)
// never leak onto disk. This is the property the encrypted blob writer
// relies on, the AEAD tag has no slack for trailing garbage.
//
// Returns an error if `data` exceeds what the existing FAT chain can hold;
// callers (e.g. CopyEncryptedFileIn) must AllocateFATEntry with the right
// sector count beforehand.
func (da *DataArea) WriteBlob(fs *Filesystem, fatEntryNo uint32, data []byte) error {
	chain, err := fs.FileAllocationTable.GetEntryChain(fatEntryNo, fs.Path, fs.SuperBlock)
	if err != nil {
		return err
	}

	bps := int(fs.SuperBlock.BytesPerSector)
	if uint64(len(chain))*uint64(bps) < uint64(len(data)) {
		return fmt.Errorf(
			"ТДД ланац (%d сектора пута %d Б) премали за податак од %d Б",
			len(chain), bps, len(data),
		)
	}

	fsFile, err := os.OpenFile(fs.Path, os.O_WRONLY, 0640)
	if err != nil {
		return err
	}
	defer fsFile.Close()

	dataAreaOffset := BOOTLOADER_SIZE + SUPERBLOCK_SIZE +
		(int(fs.SuperBlock.TotalFSEntries) * FSENTRY_SIZE) +
		(int(fs.SuperBlock.TotalSectors) * FATENTRY_SIZE)

	written := 0
	for _, sectorIdx := range chain {
		// Fresh buffer per sector: make() zeroes it, so any leftover slack
		// in the final sector is unambiguously 0x00, not undefined memory
		// like the file-streaming WriteEntry path.
		buf := make([]byte, bps)
		remaining := len(data) - written
		if remaining > 0 {
			n := remaining
			if n > bps {
				n = bps
			}
			copy(buf, data[written:written+n])
			written += n
		}

		blockOffset := dataAreaOffset + int(sectorIdx)*bps
		if _, err := fsFile.Seek(int64(blockOffset), 0); err != nil {
			return err
		}
		if _, err := fsFile.Write(buf); err != nil {
			return fmt.Errorf("не могу уписати податак у блок: %v", err)
		}
	}

	return nil
}

func (da *DataArea) AllocateDataArea(fs *Filesystem) error {
	// open the fs file for writing
	fsFile, err := os.OpenFile(fs.Path, os.O_WRONLY, 0640)
	if err != nil {
		return err
	}
	defer fsFile.Close()

	// go to the start of the data area
	offset := BOOTLOADER_SIZE + SUPERBLOCK_SIZE +
		(int(fs.SuperBlock.TotalFSEntries) * FSENTRY_SIZE) +
		(int(fs.SuperBlock.TotalSectors) * FATENTRY_SIZE)

	buffer := make([]byte, fs.SuperBlock.BytesPerSector)
	for i := 0; i < int(fs.SuperBlock.TotalSectors); i++ {
		fsFile.Seek(int64(offset), 0)
		_, err = fsFile.Write(buffer)
		if err != nil {
			return fmt.Errorf("не могу захватити блок: %v", err)
		}

		offset += int(fs.SuperBlock.BytesPerSector)

	}

	return nil
}
