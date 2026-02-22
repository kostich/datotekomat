package sfat

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"strings"
)

var FILENAME_LENGTH = 62
var FSENTRY_SIZE = FILENAME_LENGTH + 4*3 + 1*4 + 2*2 + 4 + 14*3

var TYPE_FILE = byte(0x01)
var TYPE_FOLDER = byte(0x02)
var TYPE_LINK = byte(0x03)
var TYPE_ANY = byte(0xFF)

type FSEntry struct { // 128 bytes in total
	Name []byte // 62 bytes (up to 31 UTF-8 characters or 62 ASCII ones)

	FATEntry    uint32 // 4 bytes
	Size        uint32 // 4 bytes
	ParentEntry uint32 // 4 bytes (parent entry of type folder)

	Type      byte // 1 byte (file, folder, link, etc.)
	UserPerm  byte // 1 byte (rwx for user)
	GroupPerm byte // 1 byte (rwx for group)
	WorldPerm byte // 1 byte (rwx for world)

	UID uint16 // 2 bytes (user ID of the owner)
	GID uint16 // 2 bytes (group ID of the owner)

	Checksum uint32 // 4 bytes (CRC32 checksum for bitrot detection)

	// year 3 bytes, month 1, day 1, hour 1, minute 1, second 1, nanosecond 4, timezone 2
	CreatedAt  []byte // 14 bytes
	ModifiedAt []byte // 14 bytes
	AccessedAt []byte // 14 bytes
}

type FSEntries struct{}

func humanReadableFileType(ftype byte) (res string) {
	switch ftype {
	case TYPE_FILE:
		res = "датотека"
	case TYPE_FOLDER:
		res = "фасцикла"
	case TYPE_LINK:
		res = "веза"
	default:
		res = "непознато"
	}

	return res
}

func (fse *FSEntry) Details() string {
	var details string
	if string(bytes.TrimRight(fse.Name, "\x00")) != "" {
		details = fmt.Sprintf(
			"%v „%v“ (ТДД %v, %v, родитељ: %v, сума: %x),",
			humanReadableFileType(fse.Type),
			string(bytes.TrimRight(fse.Name, "\x00")),
			fse.FATEntry,
			HumanReadableUnit(float64(fse.Size)),
			fse.ParentEntry,
			fse.Checksum,
		)
	}

	return details
}

func (fses *FSEntries) GetEntry(fsPath string, sb *SuperBlock, entryNo int) (*FSEntry, error) {
	// open the file for reading
	file, err := os.OpenFile(fsPath, os.O_RDONLY, 0640)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// calculate offset
	offset := BOOTLOADER_SIZE + SUPERBLOCK_SIZE
	for i := 0; i < entryNo; i++ {
		offset += FSENTRY_SIZE
	}

	// check if offset within file
	fileInfo, err := os.Stat(fsPath)
	if err != nil {
		return nil, fmt.Errorf("не могу утврдити величину ФС-а: %v", err)
	}

	if int64(offset+FSENTRY_SIZE) > fileInfo.Size() {
		return nil, fmt.Errorf("тражени сд индекс превазилази величину датотеке")
	}

	// read fsentry
	file.Seek(int64(offset), 0)

	buffer1b := make([]byte, 1)
	buffer2b := make([]byte, 2)
	buffer4b := make([]byte, 4)
	buffer14b := make([]byte, 14)
	bufferName := make([]byte, FILENAME_LENGTH)
	fsentry := FSEntry{}

	_, err = file.Read(bufferName)
	if err != nil {
		return nil, err
	}
	fsentry.Name = bufferName

	_, err = file.Read(buffer4b)
	if err != nil {
		return nil, err
	}
	fsentry.FATEntry = binary.LittleEndian.Uint32(buffer4b)

	_, err = file.Read(buffer4b)
	if err != nil {
		return nil, err
	}
	fsentry.Size = binary.LittleEndian.Uint32(buffer4b)

	_, err = file.Read(buffer4b)
	if err != nil {
		return nil, err
	}
	fsentry.ParentEntry = binary.LittleEndian.Uint32(buffer4b)

	_, err = file.Read(buffer1b)
	if err != nil {
		return nil, err
	}
	fsentry.Type = buffer1b[0]

	_, err = file.Read(buffer1b)
	if err != nil {
		return nil, err
	}
	fsentry.UserPerm = buffer1b[0]

	_, err = file.Read(buffer1b)
	if err != nil {
		return nil, err
	}
	fsentry.GroupPerm = buffer1b[0]

	_, err = file.Read(buffer1b)
	if err != nil {
		return nil, err
	}
	fsentry.WorldPerm = buffer1b[0]

	_, err = file.Read(buffer2b)
	if err != nil {
		return nil, err
	}
	fsentry.UID = binary.LittleEndian.Uint16(buffer2b)

	_, err = file.Read(buffer2b)
	if err != nil {
		return nil, err
	}
	fsentry.GID = binary.LittleEndian.Uint16(buffer2b)

	_, err = file.Read(buffer4b)
	if err != nil {
		return nil, err
	}
	fsentry.Checksum = binary.LittleEndian.Uint32(buffer4b)

	_, err = file.Read(buffer14b)
	if err != nil {
		return nil, err
	}
	fsentry.CreatedAt = buffer14b

	_, err = file.Read(buffer14b)
	if err != nil {
		return nil, err
	}
	fsentry.ModifiedAt = buffer14b

	_, err = file.Read(buffer14b)
	if err != nil {
		return nil, err
	}
	fsentry.AccessedAt = buffer14b

	return &fsentry, nil
}

func (fses *FSEntries) WriteEntry(fsPath string, sb *SuperBlock, entryNo int, entry *FSEntry) error {
	// open the file for writing
	file, err := os.OpenFile(fsPath, os.O_WRONLY, 0640)
	if err != nil {
		return err
	}
	defer file.Close()

	// calculate offset
	offset := BOOTLOADER_SIZE + SUPERBLOCK_SIZE
	for i := 0; i < entryNo; i++ {
		offset += FSENTRY_SIZE
	}

	// go to fsentry location
	file.Seek(int64(offset), 0)

	// write the values to the file
	buffer2b := make([]byte, 2)
	buffer4b := make([]byte, 4)

	_, err = file.Write(entry.Name)
	if err != nil {
		return fmt.Errorf("не могу уписати назив сд ставке: %v", err)
	}

	binary.LittleEndian.PutUint32(buffer4b, entry.FATEntry)
	_, err = file.Write(buffer4b)
	if err != nil {
		return fmt.Errorf("не могу уписати ТДД број сд ставке: %v", err)
	}

	binary.LittleEndian.PutUint32(buffer4b, entry.Size)
	_, err = file.Write(buffer4b)
	if err != nil {
		return fmt.Errorf("не могу уписати величину сд ставке: %v", err)
	}

	binary.LittleEndian.PutUint32(buffer4b, entry.ParentEntry)
	_, err = file.Write(buffer4b)
	if err != nil {
		return fmt.Errorf("не могу уписати број родитеља сд ставке: %v", err)
	}

	_, err = file.Write([]byte{entry.Type})
	if err != nil {
		return err
	}

	_, err = file.Write([]byte{entry.UserPerm})
	if err != nil {
		return err
	}

	_, err = file.Write([]byte{entry.GroupPerm})
	if err != nil {
		return err
	}

	_, err = file.Write([]byte{entry.WorldPerm})
	if err != nil {
		return err
	}

	binary.LittleEndian.PutUint16(buffer2b, uint16(entry.UID))
	_, err = file.Write(buffer2b)
	if err != nil {
		return err
	}

	binary.LittleEndian.PutUint16(buffer2b, uint16(entry.GID))
	_, err = file.Write(buffer2b)
	if err != nil {
		return err
	}

	binary.LittleEndian.PutUint32(buffer4b, uint32(entry.Checksum))
	_, err = file.Write(buffer4b)
	if err != nil {
		return err
	}

	_, err = file.Write(entry.CreatedAt)
	if err != nil {
		return err
	}
	_, err = file.Write(entry.ModifiedAt)
	if err != nil {
		return err
	}

	_, err = file.Write(entry.AccessedAt)
	if err != nil {
		return err
	}

	return nil
}

func (fs *Filesystem) AddFSEntry(fsentry *FSEntry) (uint32, error) {
	if strings.Contains(string(fsentry.Name), "?") {
		return 0, fmt.Errorf("%v", "назив датотеке не може садржати знак упитника")
	}

	for i := 0; i < int(fs.SuperBlock.TotalFSEntries); i++ {
		entry, err := fs.FSEntries.GetEntry(fs.Path, fs.SuperBlock, i)
		if err != nil {
			return 0, err
		}

		if filenameEmpty(entry.Name) {
			// fsentry is available
			fs.FSEntries.WriteEntry(fs.Path, fs.SuperBlock, i, fsentry)

			return uint32(i), nil
		}
	}

	return 0, fmt.Errorf("не могу пронаћи празну ставку")
}

func (fs *Filesystem) FindFSEntryNumber(entryPath string, ftype byte) (uint32, error) {
	// root folder always has fs entry number 0
	if entryPath == "/" {
		return 0, nil
	} else if entryPath == "" {
		return 0, fmt.Errorf("путања не може бити празна")
	}

	// trim the / suffix
	entryPath = strings.TrimSuffix(entryPath, "/")

	splitPath := strings.Split(entryPath, "/")
	var members []string
	for _, member := range splitPath {
		if member != "" {
			members = append(members, member)
		}
	}

	// start traversing from root
	currentPath := "/"  // start reading from the root folder
	currentFSEntry := 0 // root folder is at index 0
	origType := ftype
	origMemberCount := len(members)
	for currentPath != entryPath {
		// get the current entry
		fsEntry, err := fs.FSEntries.GetEntry(fs.Path, fs.SuperBlock, currentFSEntry)
		if err != nil {
			return 0, err
		}

		// find the entries for the current fs entry
		fatEntries, err := fs.FileAllocationTable.GetEntryChain(fsEntry.FATEntry, fs.Path, fs.SuperBlock)
		if err != nil {
			return 0, err
		}

		data := []byte{}
		for _, entry := range fatEntries {
			block, err := fs.DataArea.GetEntry(fs.Path, fs.SuperBlock, int(entry))
			if err != nil {
				return 0, err
			}
			data = append(data, block.Content...)
		}
		fsEntries := []int{}
		for i := 0; i < len(data); i += FOLDER_FS_ENTRY_SIZE {
			value := int(binary.LittleEndian.Uint32([]byte{data[i], data[i+1], data[i+2], data[i+3]}))
			if value != 0 {
				fsEntries = append(fsEntries, value)
			}
		}

		// check if any of the entries match
		entryFound := false
		for _, entry := range fsEntries {
			// if it's not the last member, it must be a folder since files
			// cannot act as folders for other files
			if origMemberCount > 1 && len(members) > 1 {
				ftype = TYPE_FOLDER
			} else if origMemberCount > 1 && len(members) == 1 {
				ftype = origType
			}

			checkEntry, err := fs.FSEntries.GetEntry(fs.Path, fs.SuperBlock, entry)
			if err != nil {
				return 0, err
			}

			if string(bytes.TrimRight(checkEntry.Name, "\x00")) == members[0] &&
				(checkEntry.Type == ftype || ftype == TYPE_ANY) {
				// on match, save the current fsentry index
				currentFSEntry = entry

				// and then read the next member
				if currentPath != "/" {
					currentPath += "/" + members[0]
				} else {
					currentPath += members[0]
				}

				if len(members) > 0 {
					members = members[1:]
				}

				entryFound = true
				break
			}
		}

		if !entryFound {
			// if no match, invalid path so break
			switch ftype {
			case TYPE_FILE, TYPE_LINK:
				return 0, fmt.Errorf("непостојећа датотека или веза „%v“", entryPath)
			case TYPE_FOLDER:
				return 0, fmt.Errorf("непостојећа фасцикла „%v“", entryPath)
			default:
				return 0, fmt.Errorf("непостојећа путања „%v“", entryPath)
			}
		}

	}

	return uint32(currentFSEntry), nil
}

func (fse *FSEntry) ChangeMode(perms []byte) error {
	fse.UserPerm = perms[0]
	fse.GroupPerm = perms[1]
	fse.WorldPerm = perms[2]

	return nil
}

func (fse *FSEntry) ChangeUIDGID(perms []uint16) error {
	fse.UID = perms[0]
	fse.GID = perms[1]

	return nil
}

func (fse *FSEntry) ChangeTimestamp(which string, newTime []byte) error {
	switch which {
	case "н":
		fse.CreatedAt = newTime
	case "и":
		fse.ModifiedAt = newTime
	case "п":
		fse.AccessedAt = newTime
	default:
		return fmt.Errorf("непозната временска ознака „%v“", which)
	}
	return nil
}

func entrySizeValid(name string, length int) bool {
	nameBinary := make([]byte, length+2)
	copy(nameBinary, name)

	lblLength := 0
	for _, b := range nameBinary {
		if b != byte(0x00) {
			lblLength += 1
		}
	}

	return lblLength <= length
}
