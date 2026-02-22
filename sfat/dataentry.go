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
