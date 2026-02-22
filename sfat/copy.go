package sfat

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

func entryNameValid(name string) error {
	fileName := filepath.Base(name)
	if strings.Contains(fileName, "?") {
		return fmt.Errorf("назив не може садржати упитник")
	}

	if (strings.Contains(fileName, "/") ||
		strings.Contains(fileName, "\\")) && fileName != "/" {
		return fmt.Errorf("назив не може садржати косе црте")
	}

	if !entrySizeValid(filepath.Base(name), FILENAME_LENGTH) {
		return fmt.Errorf("назив је предугачак")
	}

	return nil
}

func CopyFileIn(filePath, folderPath, fsPath string, timestamp []byte) error {
	// check if the filename is valid
	if err := entryNameValid(filePath); err != nil {
		return fmt.Errorf("неисправан назив датотеке: %v", err)
	}

	// read the filesystem
	copyfs, err := Read(fsPath)
	if err != nil {
		return err
	}

	// check if the given file can fit into our filesystem
	fileSize, perms, err := FileProperties(filePath)
	if err != nil {
		return err
	}

	fileSectorSize := uint32(math.Ceil(float64(fileSize) / float64(copyfs.SuperBlock.BytesPerSector)))

	// check if the filename is unique
	fileName := filepath.Base(filePath)
	if !strings.HasSuffix(folderPath, "/") {
		folderPath += "/"
	}
	if copyfs.EntryExists(folderPath+fileName, TYPE_FILE) {
		return fmt.Errorf("датотека већ постоји")
	}

	// decrement AvailableSectors and AvailableFSEntries
	if copyfs.SuperBlock.AvailableSectors >= fileSectorSize {
		copyfs.SuperBlock.AvailableSectors -= fileSectorSize

		if copyfs.SuperBlock.AvailableFSEntries > 0 {
			copyfs.SuperBlock.AvailableFSEntries -= 1
		} else {
			return fmt.Errorf("недовољно ставки за упис датотеке")
		}
	} else {
		return fmt.Errorf("недовољно сектора за упис датотеке")
	}
	if err := copyfs.WriteSuperBlock(); err != nil {
		return err
	}

	// allocate a first free slot
	fatEntry, err := copyfs.FileAllocationTable.AllocateFATEntry(int(fileSectorSize), copyfs.Path, copyfs.SuperBlock)
	if err != nil {
		return err
	}

	// determine the parent folder
	var fullPath string
	if folderPath == "/" {
		fullPath += filepath.Base(filePath)
	} else {
		fullPath = folderPath + "/" + filepath.Base(filePath)
	}

	parentNo, err := copyfs.ParentFolder(fullPath)
	if err != nil {
		return err
	}

	// add the fsentry
	fullFileName := make([]byte, FILENAME_LENGTH)
	copy(fullFileName, []byte(fileName))

	checksum, err := CalculateCRC32File(filePath)
	if err != nil {
		return err
	}

	fsentry := FSEntry{
		Name:        fullFileName,
		FATEntry:    uint32(fatEntry),
		Size:        uint32(fileSize),
		ParentEntry: uint32(parentNo),
		Type:        TYPE_FILE,
		UserPerm:    perms[0],
		GroupPerm:   perms[1],
		WorldPerm:   perms[2],
		UID:         1000,
		GID:         1000,
		Checksum:    checksum,
		CreatedAt:   timestamp,
		ModifiedAt:  timestamp,
		AccessedAt:  timestamp,
	}
	entryNo, err := copyfs.AddFSEntry(&fsentry)
	if err != nil {
		return err
	}

	// add the file to the data area of the parent folder
	if err := copyfs.AddEntryToFolder(entryNo, fsentry.ParentEntry); err != nil {
		return err
	}

	// populate the blocks in the data area
	copyfs.DataArea.WriteEntry(copyfs, fsentry.FATEntry, filePath, []byte{})

	return nil
}

func CopyFileOut(internalPath, externalPath, fsPath string) error {
	// check if the filename is valid
	fileName := filepath.Base(internalPath)
	if err := entryNameValid(internalPath); err != nil {
		return fmt.Errorf("неисправан назив датотеке: %v", err)
	}

	// check if the file exists on the host filesystem already
	if _, err := os.Stat(fileName); err == nil {
		return fmt.Errorf("датотека већ постоји на вашем систему")
	}

	// read the filesystem
	copyfs, err := Read(fsPath)
	if err != nil {
		return err
	}

	// check if the file exists in the filesystem
	entryNo, err := copyfs.FindFSEntryNumber(internalPath, TYPE_FILE)
	if err != nil {
		return fmt.Errorf("не могу наћи датотеку: %v", err)
	}
	fsEntry, err := copyfs.FSEntries.GetEntry(copyfs.Path, copyfs.SuperBlock, int(entryNo))
	if err != nil {
		return err
	}
	startingPoint := fsEntry.FATEntry
	fileSize := int(fsEntry.Size)

	// write the internal file to the host filesystem
	perms := []byte{
		fsEntry.UserPerm,
		fsEntry.GroupPerm,
		fsEntry.WorldPerm,
	}

	hostFile, err := os.Create(externalPath)
	if err != nil {
		return fmt.Errorf("не могу отворити датотеку за упис: %v", err)
	}
	defer hostFile.Close()

	// set the correct permission
	err = os.Chmod(externalPath, encodeFilePermissions(perms))
	if err != nil {
		return fmt.Errorf("не могу поставити овлашћења датотеке: %v", err)
	}

	// read the blocks, one by one, and write them to the host file
	fatEntries, err := copyfs.FileAllocationTable.GetEntryChain(startingPoint, copyfs.Path, copyfs.SuperBlock)
	if err != nil {
		return err
	}

	bytesWritten := 0
	for _, entry := range fatEntries {
		block, err := copyfs.DataArea.GetEntry(copyfs.Path, copyfs.SuperBlock, int(entry))
		if err != nil {
			return err
		}

		bytesToWrite := len(block.Content)
		if bytesWritten+bytesToWrite <= fileSize {
			_, err := hostFile.Write(block.Content)
			if err != nil {
				return fmt.Errorf("не могу уписати податке у датотеку: %v", err)
			}
			bytesWritten += bytesToWrite
		} else {
			// don't write zero bytes that were not in the original file
			remainder := make([]byte, fileSize-bytesWritten)
			copy(remainder, block.Content)
			_, err := hostFile.Write(remainder)
			if err != nil {
				return fmt.Errorf("не могу уписати податке у датотеку: %v", err)
			}
			bytesWritten += fileSize - bytesWritten
		}
	}

	return nil
}
