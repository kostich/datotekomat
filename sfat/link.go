package sfat

import (
	"encoding/binary"
	"fmt"
	"path/filepath"
	"strings"
)

var LINK_SECTOR_AMOUNT = 1 // 1 sector for a new link, by default

func (fs *Filesystem) CreateLink(destPath, linkPath string, timestamp []byte) error {
	// check if the name is valid
	if err := entryNameValid(destPath); err != nil {
		return fmt.Errorf("неисправан назив одредишта: %v", err)
	}
	if err := entryNameValid(linkPath); err != nil {
		return fmt.Errorf("неисправан назив везе: %v", err)
	}

	// check if the linkPath already exists
	if fs.EntryExists(linkPath, TYPE_LINK) {
		return fmt.Errorf("веза већ постоји")
	}

	// check if the destPath already exists
	if !fs.EntryExists(destPath, TYPE_ANY) {
		return fmt.Errorf("непостојеће одредиште „%v“", destPath)
	}

	// take one fs entry and one sector to store the destination
	if fs.SuperBlock.AvailableSectors >= uint32(LINK_SECTOR_AMOUNT) {
		fs.SuperBlock.AvailableSectors -= 1

		if fs.SuperBlock.AvailableFSEntries > 0 {
			fs.SuperBlock.AvailableFSEntries -= 1
		} else {
			return fmt.Errorf("недовољно ставки за стварање везе")
		}
	} else {
		return fmt.Errorf("недовољно сектора за стварање везе")
	}

	// persist the superblock
	if err := fs.WriteSuperBlock(); err != nil {
		return err
	}

	// allocate a first free slot
	fatEntry, err := fs.FileAllocationTable.AllocateFATEntry(LINK_SECTOR_AMOUNT, fs.Path, fs.SuperBlock)
	if err != nil {
		return err
	}

	// determine the parent
	parentNo, err := fs.ParentFolder(linkPath)
	if err != nil {
		return err
	}

	// create the entry
	linkName := filepath.Base(linkPath)
	fullLinkName := make([]byte, FILENAME_LENGTH)
	copy(fullLinkName, []byte(linkName))

	fsentry := FSEntry{
		Name:        fullLinkName,
		FATEntry:    uint32(fatEntry),
		Size:        4, // single destination that takes 4 bytes (uint32)
		ParentEntry: uint32(parentNo),
		Type:        TYPE_LINK,
		UserPerm:    0x07,
		GroupPerm:   0x07,
		WorldPerm:   0x07,
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

	// add the link to the data area of the parent folder
	if err := fs.AddEntryToFolder(entryNo, fsentry.ParentEntry); err != nil {
		return err
	}

	// determine the fs entry no of the destination
	desiredType := TYPE_FILE
	if strings.HasSuffix(destPath, "/") {
		desiredType = TYPE_FOLDER
	}

	destPathEntryNo, err := fs.FindFSEntryNumber(destPath, desiredType)
	if err != nil {
		// another link?
		destPathEntryNo, err = fs.FindFSEntryNumber(destPath, TYPE_LINK)
		if err != nil {
			return fmt.Errorf("не могу наћи број ставке одредишта: %v", err)
		}
	}

	// add the destPath fsentry no to the data area
	linkContents := make([]byte, int(fs.SuperBlock.BytesPerSector))
	binary.LittleEndian.PutUint32(linkContents, destPathEntryNo)
	err = fs.DataArea.WriteEntry(fs, fsentry.FATEntry, "", linkContents)
	if err != nil {
		return err
	}

	// set the checksum
	fsentry.Checksum = CalculateCRC32(linkContents)
	fs.FSEntries.WriteEntry(fs.Path, fs.SuperBlock, int(entryNo), &fsentry)

	return nil
}

func (fs *Filesystem) ReadLinkDest(linkNo int) (*FSEntry, error) {
	// read the link entry
	linkEntry, err := fs.FSEntries.GetEntry(fs.Path, fs.SuperBlock, linkNo)
	if err != nil {
		return &FSEntry{}, err
	}

	// read the fat chain
	fatEntries, err := fs.FileAllocationTable.GetEntryChain(linkEntry.FATEntry, fs.Path, fs.SuperBlock)
	if err != nil {
		return &FSEntry{}, err
	}

	// read the data from the data area
	data := []byte{}
	for _, entry := range fatEntries {
		block, err := fs.DataArea.GetEntry(fs.Path, fs.SuperBlock, int(entry))
		if err != nil {
			return &FSEntry{}, err
		}
		data = append(data, block.Content...)
	}

	// first 4 bytes are dest
	destUint32 := []byte{data[0], data[1], data[2], data[3]}
	destEntryNo := int(binary.LittleEndian.Uint32(destUint32))

	destEntry, err := fs.FSEntries.GetEntry(fs.Path, fs.SuperBlock, destEntryNo)
	if err != nil {
		return &FSEntry{}, err
	}

	return destEntry, nil
}
