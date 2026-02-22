package sfat

import (
	"encoding/binary"
	"fmt"
	"path/filepath"
	"strings"
)

var FOLDER_FS_ENTRY_SIZE = 4 // code expects uint32 so only change if you know what you're doing
var FOLDER_SECTOR_AMOUNT = 1 // 1 sector for a new folder, by default

func (fs *Filesystem) RenameFolder(old, new string) error {
	return nil
}

func storeEntries(entries []byte, newEntry []byte) ([]byte, int, error) {
	entryStored := false
	for i := 0; i < len(entries); i += FOLDER_FS_ENTRY_SIZE {
		if entries[i] == 0 && entries[i+1] == 0 &&
			entries[i+2] == 0 && entries[i+3] == 0 {
			// write the new entry to the free space
			entries[i] = newEntry[0]
			entries[i+1] = newEntry[1]
			entries[i+2] = newEntry[2]
			entries[i+3] = newEntry[3]
			entryStored = true
			break
		}
	}

	if !entryStored {
		return entries, 0, fmt.Errorf("нема простора за нову сд ставку")
	}

	// count the amount of used entries
	usedEntries := 0
	for i := 0; i < len(entries); i += FOLDER_FS_ENTRY_SIZE {
		if entries[i] != 0 || entries[i+1] != 0 ||
			entries[i+2] != 0 || entries[i+3] != 0 {
			usedEntries += 1
		}
	}

	return entries, usedEntries, nil
}

func clearEntry(entries []byte, oldEntry []byte) ([]byte, int, error) {
	entryStored := false
	clearedEntries := 0
	for i := 0; i < len(entries); i += FOLDER_FS_ENTRY_SIZE {
		if entries[i] == oldEntry[0] && entries[i+1] == oldEntry[1] &&
			entries[i+2] == oldEntry[2] && entries[i+3] == oldEntry[3] {
			// clear the entry
			entries[i] = 0
			entries[i+1] = 0
			entries[i+2] = 0
			entries[i+3] = 0
			entryStored = true
			clearedEntries += 1
			break
		}
	}

	if !entryStored {
		return entries, clearedEntries, fmt.Errorf("нема простора за нову сд ставку")
	}

	return entries, clearedEntries, nil
}

func (fs *Filesystem) AddEntryToFolder(entryNo, parentNo uint32) error {
	// convert entryNo for storage
	newEntry := make([]byte, 4)
	binary.LittleEndian.PutUint32(newEntry, entryNo)

	// find the parent entry
	parentEntry, err := fs.FSEntries.GetEntry(fs.Path, fs.SuperBlock, int(parentNo))
	if err != nil {
		return err
	}

	// find the parent FAT entries
	fatEntries, err := fs.FileAllocationTable.GetEntryChain(parentEntry.FATEntry, fs.Path, fs.SuperBlock)
	if err != nil {
		return err
	}
	if len(fatEntries) == 0 {
		return fmt.Errorf("унутрашња грешка: нема ТДД ставки за дату родитељску фасциклу %v", parentNo)
	}

	// check if there is space to store the new entry
	allFSEntries := make([]byte, 0)
	for _, entry := range fatEntries {
		block, err := fs.DataArea.GetEntry(fs.Path, fs.SuperBlock, int(entry))
		if err != nil {
			return err
		}

		allFSEntries = append(allFSEntries, block.Content...)
	}

	// store the new entry
	allFSEntries, usedFSEntries, err := storeEntries(allFSEntries, newEntry)
	parentEntry.Size = uint32(usedFSEntries * FOLDER_FS_ENTRY_SIZE)
	fs.FSEntries.WriteEntry(fs.Path, fs.SuperBlock, int(parentNo), parentEntry)
	if err != nil {
		// we have reached the end of the folder's data space and
		// there is no free space to write the entry number

		// check if there is space to recreate folder with double of data area
		availableSpace := int(fs.SuperBlock.AvailableSectors * fs.SuperBlock.BytesPerSector)
		requiredSpace := (FOLDER_SECTOR_AMOUNT * int(fs.SuperBlock.BytesPerSector)) * len(fatEntries)
		if availableSpace-requiredSpace < 0 {
			return fmt.Errorf("недовољно простора у систему датотека")
		}

		// delete existing fat entries
		for _, entry := range fatEntries {
			err := fs.FileAllocationTable.DeleteEntry(entry, fs.Path, fs.SuperBlock)
			if err != nil {
				return err
			}
		}

		// increment available sectors and fs entries
		fs.SuperBlock.AvailableSectors += uint32(len(fatEntries))
		fs.SuperBlock.AvailableFSEntries += 1

		// allocate a new fat entry with the double capacity
		newFatEntry, err := fs.FileAllocationTable.AllocateFATEntry(len(fatEntries)*2, fs.Path, fs.SuperBlock)
		if err != nil {
			return fmt.Errorf("не могу доделити нову ТДД ставку: %v", err)
		}
		parentEntry.FATEntry = uint32(newFatEntry)
		fs.FSEntries.WriteEntry(fs.Path, fs.SuperBlock, int(parentNo), parentEntry)

		// decrement available sectors and fs entries
		fs.SuperBlock.AvailableSectors -= uint32(len(fatEntries) * 2)
		fs.SuperBlock.AvailableFSEntries -= 1

		// persist the superblock
		if err := fs.WriteSuperBlock(); err != nil {
			return err
		}

		// increase the folder size
		parentEntry.Size *= 2
		fs.FSEntries.WriteEntry(fs.Path, fs.SuperBlock, int(parentNo), parentEntry)

		// increase the allFSEntries capacity
		newSize := len(allFSEntries)
		for i := 0; i < newSize; i++ {
			allFSEntries = append(allFSEntries, 0x00)
		}

		// store the previous entries and the new entry in the new data area
		err = nil
		allFSEntries, usedFSEntries, err = storeEntries(allFSEntries, newEntry)
		parentEntry.Size = uint32(usedFSEntries * FOLDER_FS_ENTRY_SIZE)
		if err != nil {
			return fmt.Errorf("не могу сачувати ставке у нови простор података: %v", err)
		}
		fs.FSEntries.WriteEntry(fs.Path, fs.SuperBlock, int(parentNo), parentEntry)
	}

	// write the new data to the data area
	fs.DataArea.WriteEntry(fs, parentEntry.FATEntry, "", allFSEntries)

	// save the new checksum
	parentEntry.Checksum = CalculateCRC32(allFSEntries)
	fs.FSEntries.WriteEntry(fs.Path, fs.SuperBlock, int(parentNo), parentEntry)

	return nil
}

func getParentPath(path string) string {
	parentName := strings.TrimSuffix(path, filepath.Base(path))
	if len(parentName) != 1 {
		parentName = strings.TrimSuffix(parentName, "/")
	}
	if len(parentName) == 0 {
		parentName = "/"
	}

	return parentName
}

func (fs *Filesystem) ParentFolder(folderPath string) (uint32, error) {
	parentNo := uint32(0) // root folder by default

	if filepath.Base(folderPath) != "/" {
		parentPath := getParentPath(folderPath)

		var err error
		parentNo, err = fs.FindFSEntryNumber(parentPath, TYPE_FOLDER)
		if err != nil {
			return parentNo, fmt.Errorf("не могу пронаћи родитеља: %v", err)
		}
	}

	return parentNo, nil
}

func (fs *Filesystem) CreateFolder(folderPath string, timestamp []byte) error {
	// check if the name name is valid
	if err := entryNameValid(folderPath); err != nil {
		return fmt.Errorf("неисправан назив фасцикле: %v", err)
	}

	// check if the name already exists
	if folderPath != "/" {
		if fs.EntryExists(folderPath, TYPE_FOLDER) {
			return fmt.Errorf("фасцикла већ постоји")
		}
	}

	// take one fs entry and one sector to store the data about
	// the files, names and symlinks inside this name
	if fs.SuperBlock.AvailableSectors >= uint32(FOLDER_SECTOR_AMOUNT) {
		fs.SuperBlock.AvailableSectors -= 1

		if fs.SuperBlock.AvailableFSEntries > 0 {
			fs.SuperBlock.AvailableFSEntries -= 1
		} else {
			return fmt.Errorf("недовољно ставки за стварање фасцикле")
		}
	} else {
		return fmt.Errorf("недовољно сектора за стварање фасцикле")
	}

	// persist the superblock
	if err := fs.WriteSuperBlock(); err != nil {
		return err
	}

	// allocate a first free slot
	fatEntry, err := fs.FileAllocationTable.AllocateFATEntry(FOLDER_SECTOR_AMOUNT, fs.Path, fs.SuperBlock)
	if err != nil {
		return err
	}

	// determine the parent folder
	parentNo, err := fs.ParentFolder(folderPath)
	if err != nil {
		return err
	}

	// create the entry
	folderName := filepath.Base(folderPath)
	fullFolderName := make([]byte, FILENAME_LENGTH)
	copy(fullFolderName, []byte(folderName))

	fsentry := FSEntry{
		Name:        fullFolderName,
		FATEntry:    uint32(fatEntry),
		Size:        0,
		ParentEntry: uint32(parentNo),
		Type:        TYPE_FOLDER,
		UserPerm:    0x07,
		GroupPerm:   0x05,
		WorldPerm:   0x00,
		UID:         1000,
		GID:         1000,
		CreatedAt:   timestamp,
		ModifiedAt:  timestamp,
		AccessedAt:  timestamp,
	}

	entryNo, err := fs.AddFSEntry(&fsentry)
	if err != nil {
		return err
	}

	// add the folder to the data area of the parent folder
	if err := fs.AddEntryToFolder(entryNo, fsentry.ParentEntry); err != nil {
		return err
	}

	// populate the blocks in the data area
	folderContents := make([]byte, int(fs.SuperBlock.BytesPerSector))
	err = fs.DataArea.WriteEntry(fs, fsentry.FATEntry, "", folderContents)
	if err != nil {
		return err
	}

	// set the checksum
	fsentry.Checksum = CalculateCRC32(folderContents)
	fs.FSEntries.WriteEntry(fs.Path, fs.SuperBlock, int(entryNo), &fsentry)

	return nil
}
