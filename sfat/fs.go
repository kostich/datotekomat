package sfat

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Filesystem struct {
	Path                string
	BootArea            *BootArea
	SuperBlock          *SuperBlock
	FSEntries           *FSEntries
	FileAllocationTable *FileAllocationTable
	DataArea            *DataArea
}

func New(totalSectors, bytesPerSector, totalFSEntries int, label, path string, timestamp []byte) (*Filesystem, error) {
	// check if the provided values make sense
	if totalSectors <= 0 {
		return nil, fmt.Errorf("неисправна вредност укупних сектора")
	}

	if bytesPerSector <= 0 {
		return nil, fmt.Errorf("неисправна вредност бајтова по сектору")
	}

	if totalFSEntries <= 0 {
		return nil, fmt.Errorf("неисправна вредност укупних сд ставки")
	}

	if totalSectors == 1 {
		fmt.Println("УПОЗОРЕЊЕ: затражили сте један сектор што не оставља простора за податке")
	}

	if totalFSEntries == 1 {
		fmt.Println("УПОЗОРЕЊЕ: затражили сте једну сд ставку што не оставља простора за податке")
	}

	if totalSectors < totalFSEntries {
		fmt.Println("УПОЗОРЕЊЕ: имате мање сектора него сд ставки, вероватно нећете имати довољно простора за све ставке")
	}

	if !entrySizeValid(label, FILENAME_LENGTH) {
		return nil, fmt.Errorf("назив система датотека је предугачак")
	}

	if !folderExists(filepath.Dir(path)) {
		return nil, fmt.Errorf("дата је непостојећа путања „%v“", path)
	}

	fs := &Filesystem{
		Path:                path,
		BootArea:            &BootArea{},
		SuperBlock:          &SuperBlock{},
		FSEntries:           &FSEntries{},
		FileAllocationTable: &FileAllocationTable{},
		DataArea:            &DataArea{},
	}

	// allocate space for the bootloader
	err := fs.BootArea.AllocateBootArea(fs.Path)
	if err != nil {
		return nil, fmt.Errorf("не могу доделити простор за подизача система: %v", err)
	}

	// write the superblock
	fs.SuperBlock.TotalSectors = uint32(totalSectors)
	fs.SuperBlock.AvailableSectors = uint32(totalSectors)
	fs.SuperBlock.SectorsPerCluster = 1 // reserved for v2 when we implement sector clusters
	fs.SuperBlock.BytesPerSector = uint32(bytesPerSector)
	fs.SuperBlock.TotalFSEntries = uint32(totalFSEntries)
	fs.SuperBlock.AvailableFSEntries = uint32(totalFSEntries)
	fs.SuperBlock.FsType = 0xE1
	fs.SuperBlock.Reserved = make([]byte, SUPERBLOCK_RESERVED)
	fs.SuperBlock.Label = label

	if err := fs.WriteSuperBlock(); err != nil {
		return fs, err
	}

	// allocate space for fs entries
	for i := 0; i < int(fs.SuperBlock.TotalFSEntries); i++ {
		filename := make([]byte, FILENAME_LENGTH)
		fsentry := FSEntry{
			Name:        filename,
			FATEntry:    0,
			Size:        0,
			ParentEntry: 0,
			Type:        TYPE_FILE,
			UserPerm:    0x06,
			GroupPerm:   0x04,
			WorldPerm:   0x00,
			UID:         1000,
			GID:         1000,
			Checksum:    0,
			CreatedAt:   timestamp,
			ModifiedAt:  timestamp,
			AccessedAt:  timestamp,
		}
		fs.FSEntries.WriteEntry(fs.Path, fs.SuperBlock, i, &fsentry)
	}

	// allocate space for FAT
	err = fs.FileAllocationTable.AllocateFAT(fs)
	if err != nil {
		return fs, err
	}

	// allocate space for data
	err = fs.DataArea.AllocateDataArea(fs)
	if err != nil {
		return fs, err
	}

	// create the root folder
	err = fs.CreateFolder("/", timestamp)
	if err != nil {
		fmt.Printf("датотекомат: неуспешно стварање система датотека: %v.\n", err)
		os.Exit(1)
	}

	return fs, nil
}

func Read(fsPath string) (*Filesystem, error) {
	fs := &Filesystem{
		Path:                fsPath,
		BootArea:            &BootArea{},
		SuperBlock:          &SuperBlock{},
		FSEntries:           &FSEntries{},
		FileAllocationTable: &FileAllocationTable{},
		DataArea:            &DataArea{},
	}
	if err := fs.ReadSuperBlock(); err != nil {
		return fs, fmt.Errorf("не могу прочитати супер-блок: %v", err)
	}

	return fs, nil
}

func filenameEmpty(fname []byte) bool {
	if len(fname) != FILENAME_LENGTH {
		return false
	}

	// if the first filename char is ?, the file was deleted
	// in the past so we can use fsentry
	if fname[0] == byte('?') {
		return true
	}

	for _, b := range fname {
		if b != 0 {
			return false
		}
	}

	return true
}

func entryDeleted(fname string) bool {
	return strings.HasPrefix(filepath.Base(fname), "?")
}

func humanReadablePermission(perm byte) string {
	// TODO: redo this properly by converting to binary
	// and looking through every bit
	readablePerm := "???"
	switch int(perm) {
	case 0:
		readablePerm = "---"
	case 1:
		readablePerm = "--ш"
	case 2:
		readablePerm = "-п-"
	case 3:
		readablePerm = "-пш"
	case 4:
		readablePerm = "ч--"
	case 5:
		readablePerm = "ч-ш"
	case 6:
		readablePerm = "чп-"
	case 7:
		readablePerm = "чпш"
	}

	return readablePerm
}

func (fs *Filesystem) ListEntries(path string) error {
	// determine the given path fat entry
	fsEntryNo, err := fs.FindFSEntryNumber(path, TYPE_FOLDER)
	if err != nil {
		return err
	}

	// read the fs entry
	fsEntry, err := fs.FSEntries.GetEntry(fs.Path, fs.SuperBlock, int(fsEntryNo))
	if err != nil {
		return err
	}

	// read the fat chain
	fatEntries, err := fs.FileAllocationTable.GetEntryChain(fsEntry.FATEntry, fs.Path, fs.SuperBlock)
	if err != nil {
		return err
	}

	// read the data from the data area
	data := []byte{}
	for _, entry := range fatEntries {
		block, err := fs.DataArea.GetEntry(fs.Path, fs.SuperBlock, int(entry))
		if err != nil {
			return err
		}

		data = append(data, block.Content...)
	}

	for i := 0; i < len(data); i += FOLDER_FS_ENTRY_SIZE {
		if data[i] != 0 || data[i+1] != 0 || data[i+2] != 0 || data[i+3] != 0 {
			// if the FOLDER_FS_ENTRY_SIZE
			fsEntryNo := int(binary.LittleEndian.Uint32([]byte{data[i], data[i+1], data[i+2], data[i+3]}))

			// read the fs entry
			fsentry, err := fs.FSEntries.GetEntry(fs.Path, fs.SuperBlock, int(fsEntryNo))
			if err != nil {
				return err
			}

			// determine the entry type
			var etype string
			switch fsentry.Type {
			case TYPE_FILE:
				etype = "-" // file
			case TYPE_FOLDER:
				etype = "ф" // f (fascikla), folder
			case TYPE_LINK:
				etype = "в" // v (veza), link
			default:
				etype = "?" // unknown (probably an error)
			}

			timestamp, err := BytesToTime(fsentry.CreatedAt)
			if err != nil {
				return err
			}

			linkDest := ""
			if fsentry.Type == TYPE_LINK {
				// determine the destination path

				destEntry, err := fs.ReadLinkDest(fsEntryNo)
				if err != nil {
					return fmt.Errorf("не могу прочитати одредиште везе: %v", err)
				}

				// determine the full path on the filesystem
				destPath, err := fs.GetFullPath(destEntry)
				if err != nil {
					return fmt.Errorf("не могу одредити путању везе: %v", err)
				}

				linkDest = " -> " + destPath

			}

			detailedFileInfo := fmt.Sprintf(
				"%v%v%v%v %v %v\t%v\t%v  %v%v",
				etype,
				humanReadablePermission(fsentry.UserPerm),
				humanReadablePermission(fsentry.GroupPerm),
				humanReadablePermission(fsentry.WorldPerm),
				fsentry.UID,
				fsentry.GID,
				HumanReadableUnit(float64(fsentry.Size)),
				formatTimestamp(timestamp),
				string(bytes.TrimRight(fsentry.Name, "\x00")),
				linkDest,
			)
			fmt.Println(detailedFileInfo)
		}
	}

	return nil
}

func (fs *Filesystem) ListTree(path string) error {
	// determine the given path fs entry
	fsEntryNo, err := fs.FindFSEntryNumber(path, TYPE_FOLDER)
	if err != nil {
		return err
	}

	// read the fs entry
	fsEntry, err := fs.FSEntries.GetEntry(fs.Path, fs.SuperBlock, int(fsEntryNo))
	if err != nil {
		return err
	}

	// print the root of the tree
	name := string(bytes.TrimRight(fsEntry.Name, "\x00"))
	if fsEntry.Type == TYPE_FOLDER && name != "/" {
		name += "/"
	}
	fmt.Printf("%v (%v)\n", name, HumanReadableUnit(float64(fsEntry.Size)))

	// recurse into children
	return fs.listTreeRecursive(int(fsEntryNo), "")
}

func (fs *Filesystem) listTreeRecursive(fsEntryNo int, prefix string) error {
	// read the fs entry for this folder
	fsEntry, err := fs.FSEntries.GetEntry(fs.Path, fs.SuperBlock, fsEntryNo)
	if err != nil {
		return err
	}

	// read the fat chain
	fatEntries, err := fs.FileAllocationTable.GetEntryChain(fsEntry.FATEntry, fs.Path, fs.SuperBlock)
	if err != nil {
		return err
	}

	// read the data from the data area
	data := []byte{}
	for _, entry := range fatEntries {
		block, err := fs.DataArea.GetEntry(fs.Path, fs.SuperBlock, int(entry))
		if err != nil {
			return err
		}
		data = append(data, block.Content...)
	}

	// collect non-zero, non-deleted child entries
	type childInfo struct {
		entryNo int
		entry   *FSEntry
	}
	children := []childInfo{}
	for i := 0; i < len(data); i += FOLDER_FS_ENTRY_SIZE {
		if data[i] != 0 || data[i+1] != 0 || data[i+2] != 0 || data[i+3] != 0 {
			childNo := int(binary.LittleEndian.Uint32([]byte{data[i], data[i+1], data[i+2], data[i+3]}))
			childFSEntry, err := fs.FSEntries.GetEntry(fs.Path, fs.SuperBlock, childNo)
			if err != nil {
				return err
			}

			childName := string(bytes.TrimRight(childFSEntry.Name, "\x00"))
			if entryDeleted(childName) {
				continue
			}

			children = append(children, childInfo{entryNo: childNo, entry: childFSEntry})
		}
	}

	// print each child with appropriate tree connector
	for i, child := range children {
		isLast := i == len(children)-1

		connector := "├── "
		childPrefix := prefix + "│   "
		if isLast {
			connector = "└── "
			childPrefix = prefix + "    "
		}

		childName := string(bytes.TrimRight(child.entry.Name, "\x00"))
		if child.entry.Type == TYPE_FOLDER {
			childName += "/"
		}

		fmt.Printf("%v%v%v (%v)\n", prefix, connector, childName, HumanReadableUnit(float64(child.entry.Size)))

		// if it's a folder, recurse into it
		if child.entry.Type == TYPE_FOLDER {
			if err := fs.listTreeRecursive(child.entryNo, childPrefix); err != nil {
				return err
			}
		}
	}

	return nil
}

func (fs *Filesystem) RenameEntry(oldPath, newFilename string) error {
	// only accept a new name, not the full path
	if strings.Contains(newFilename, "/") {
		return fmt.Errorf("неисправан нови назив, путање нису дозвољене")
	}

	desiredType := TYPE_FILE
	if strings.HasSuffix(oldPath, "/") {
		// we need to rename a folder
		desiredType = TYPE_FOLDER
	}

	if entryDeleted(oldPath) {
		if desiredType == TYPE_FILE {
			return fmt.Errorf("неисправан назив датотеке")
		} else {
			return fmt.Errorf("неисправан назив фасцикле")
		}
	}

	// check if the given file or folder exists
	if !fs.EntryExists(oldPath, TYPE_ANY) {
		return fmt.Errorf("непостојећа изворишна ставка")
	}

	// check if the new name is not already taken
	if fs.EntryExists(newFilename, TYPE_ANY) {
		return fmt.Errorf("одредишна ставка већ постоји")
	}

	// rename the folder or a file
	fsEntryNo, err := fs.FindFSEntryNumber(oldPath, desiredType)
	if err != nil {
		return err
	}

	newName := make([]byte, FILENAME_LENGTH)
	copy(newName, []byte(filepath.Base(newFilename)))

	fsEntry, err := fs.FSEntries.GetEntry(fs.Path, fs.SuperBlock, int(fsEntryNo))
	if err != nil {
		return err
	}

	fsEntry.Name = newName
	fs.FSEntries.WriteEntry(fs.Path, fs.SuperBlock, int(fsEntryNo), fsEntry)

	return nil
}

func FileProperties(filePath string) (int64, []byte, error) {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return 0, []byte{0, 0, 0}, err
	}
	fileMode := fileInfo.Mode()
	userMode := (fileMode & 0700) >> 6
	groupMode := (fileMode & 0070) >> 3
	worldMode := (fileMode & 0007)

	return fileInfo.Size(),
		[]byte{byte(userMode), byte(groupMode), byte(worldMode)},
		nil
}

func (fs *Filesystem) DeleteEntry(filePath string) error {
	// don't allow the deletion of the root folder
	if filePath == "/" {
		return fmt.Errorf("није дозвољено брисање корене фасцикле")
	}

	desiredType := TYPE_FILE
	if strings.HasSuffix(filePath, "/") {
		// we need to delete a folder
		desiredType = TYPE_FOLDER
	}

	// get the fs index
	fsEntryNo, err := fs.FindFSEntryNumber(filePath, desiredType)
	if err != nil {
		// try to find the fsentry for a link instead and err out if that fails as well
		desiredType = TYPE_LINK
		fsEntryNo, err = fs.FindFSEntryNumber(filePath, desiredType)
		if err != nil {
			return fmt.Errorf("не могу наћи ставку: %v", err)
		}
	}

	// read the fs entry
	fsEntry, err := fs.FSEntries.GetEntry(fs.Path, fs.SuperBlock, int(fsEntryNo))
	if err != nil {
		return err
	}

	// check if the entry is already marked as deleted
	entryName := string(bytes.TrimRight(fsEntry.Name, "\x00"))
	if entryDeleted(entryName) {
		switch desiredType {
		case TYPE_FILE:
			return fmt.Errorf("неисправан назив датотеке")
		case TYPE_FOLDER:
			return fmt.Errorf("неисправан назив фасцикле")
		case TYPE_LINK:
			return fmt.Errorf("неисправан назив везе")
		default:
			return fmt.Errorf("неисправна ставка врсте „%v“", desiredType)
		}
	}

	// if it's a folder and has a non-zero size, err out
	if desiredType == TYPE_FOLDER && fsEntry.Size != 0 {
		return fmt.Errorf("фасцикла мора бити празна")
	}

	// set the first char of the current name to ?
	// to mark it as deleted
	runeSlice := []rune(entryName)
	runeSlice[0] = '?'
	newName := make([]byte, FILENAME_LENGTH)
	copy(newName, []byte(string(runeSlice)))
	fsEntry.Name = newName
	fs.FSEntries.WriteEntry(fs.Path, fs.SuperBlock, int(fsEntryNo), fsEntry)

	// enumerate FAT entries of the file
	fatEntries, err := fs.FileAllocationTable.GetEntryChain(fsEntry.FATEntry, fs.Path, fs.SuperBlock)
	if err != nil {
		return err
	}

	// delete fat entries
	for _, entry := range fatEntries {
		fs.FileAllocationTable.DeleteEntry(entry, fs.Path, fs.SuperBlock)
	}

	// increment available sectors and fs entries
	fs.SuperBlock.AvailableSectors += uint32(len(fatEntries))
	fs.SuperBlock.AvailableFSEntries += 1

	// persist the superblock
	if err := fs.WriteSuperBlock(); err != nil {
		return err
	}

	// read the parent entry
	parentEntry, err := fs.FSEntries.GetEntry(fs.Path, fs.SuperBlock, int(fsEntry.ParentEntry))
	if err != nil {
		return err
	}

	// delete the fs entry from the parent folder
	// find the parent FAT entries
	parentFatEntries, err := fs.FileAllocationTable.GetEntryChain(parentEntry.FATEntry, fs.Path, fs.SuperBlock)
	if err != nil {
		return err
	}

	// get the currently stored entries
	allFSEntries := make([]byte, 0)
	for _, entry := range parentFatEntries {
		block, err := fs.DataArea.GetEntry(fs.Path, fs.SuperBlock, int(entry))
		if err != nil {
			return err
		}

		allFSEntries = append(allFSEntries, block.Content...)
	}

	// clear the fs entry in the parent
	currentEntry := make([]byte, 4)
	binary.LittleEndian.PutUint32(currentEntry, fsEntryNo)
	allFSEntries, clearedEntries, err := clearEntry(allFSEntries, currentEntry)
	if err != nil {
		return err
	}
	parentEntry.Size -= uint32(clearedEntries * FOLDER_FS_ENTRY_SIZE)
	fs.FSEntries.WriteEntry(fs.Path, fs.SuperBlock, int(fsEntry.ParentEntry), parentEntry)

	// write the cleared data to the parent
	fs.DataArea.WriteEntry(fs, parentEntry.FATEntry, "", allFSEntries)

	// update the checksum
	parentEntry.Checksum = CalculateCRC32(allFSEntries)
	fs.FSEntries.WriteEntry(fs.Path, fs.SuperBlock, int(fsEntry.ParentEntry), parentEntry)

	return nil
}

func (fs *Filesystem) EntryExists(entryPath string, ftype byte) bool {
	_, err := fs.FindFSEntryNumber(entryPath, ftype)
	return err == nil
}

func (fs *Filesystem) GetFullPath(entry *FSEntry) (string, error) {
	entryType := entry.Type // need this for later

	// determine all the parents in the chain
	path := []string{}
	for {
		path = append(path, string(bytes.TrimRight(entry.Name, "\x00")))

		if entry.ParentEntry == 0 {
			break
		}

		var err error
		entry, err = fs.FSEntries.GetEntry(fs.Path, fs.SuperBlock, int(entry.ParentEntry))
		if err != nil {
			return "", err
		}
	}

	// construct the path
	fullPath := "/"
	for i := len(path) - 1; i >= 0; i-- {
		if i != 0 {
			fullPath += path[i] + "/"
		} else {
			fullPath += path[i]

			// if the type is a folder, add additional / to show that
			if entryType == TYPE_FOLDER {
				fullPath += "/"
			}
		}
	}

	return fullPath, nil
}

func HumanReadableUnit(sizeInBytes float64) (humanReadable string) {
	bUnit := "Б"
	kbUnit := "КБ"
	mbUnit := "МБ"
	gbUnit := "ГБ"
	tbUnit := "ТБ"

	unit := bUnit
	convertedSize := sizeInBytes

	if convertedSize > 1024*1024*1024*1024 {
		convertedSize = convertedSize / (1024 * 1024 * 1024 * 1024)
		unit = tbUnit
	} else if convertedSize > 1024*1024*1024 {
		convertedSize = convertedSize / (1024 * 1024 * 1024)
		unit = gbUnit
	} else if convertedSize > 1024*1024 {
		convertedSize = convertedSize / (1024 * 1024)
		unit = mbUnit
	} else if convertedSize > 1024 {
		convertedSize = convertedSize / 1024
		unit = kbUnit
	}

	return fmt.Sprintf("%.2f%v", convertedSize, unit)
}

func folderExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}

	return info.IsDir()
}
