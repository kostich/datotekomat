package sfat

import (
	"encoding/binary"
	"fmt"
	"os"
)

var FATENTRY_SIZE = 4
var FATENTRY_END = uint32(0xFFFFFFFF)

type FileAllocationTableEntry struct {
	DataEntry uint32
}

type FileAllocationTable struct{}

func (fate *FileAllocationTableEntry) Details() string {
	details := fmt.Sprintf(
		"податак: 0x%X,",
		fate.DataEntry,
	)

	return details
}

func (fat *FileAllocationTable) AllocateFAT(fs *Filesystem) error {
	// open the fs file for writing
	fsFile, err := os.OpenFile(fs.Path, os.O_WRONLY, 0640)
	if err != nil {
		return err
	}
	defer fsFile.Close()

	// go to the start of the FAT
	offset := BOOTLOADER_SIZE + SUPERBLOCK_SIZE +
		(int(fs.SuperBlock.TotalFSEntries) * FSENTRY_SIZE)

	buffer := make([]byte, FATENTRY_SIZE)
	for i := 0; i < int(fs.SuperBlock.TotalSectors); i++ {
		fsFile.Seek(int64(offset), 0)
		_, err = fsFile.Write(buffer)
		if err != nil {
			return fmt.Errorf("не могу захватити ТДД ставку: %v", err)
		}

		offset += FATENTRY_SIZE

	}

	return nil
}

func (fat *FileAllocationTable) WriteEntry(fsPath string, sb *SuperBlock, entryNo int, entry *FileAllocationTableEntry) error {
	// open the file for writing
	file, err := os.OpenFile(fsPath, os.O_WRONLY, 0640)
	if err != nil {
		return err
	}
	defer file.Close()

	// calculate offset
	offset := BOOTLOADER_SIZE + SUPERBLOCK_SIZE + (int(sb.TotalFSEntries) * FSENTRY_SIZE)
	for i := 0; i < entryNo; i++ {
		offset += FATENTRY_SIZE
	}

	// go to fsentry location
	file.Seek(int64(offset), 0)

	// write the FAT entry to the file
	buffer := make([]byte, FATENTRY_SIZE)
	binary.LittleEndian.PutUint32(buffer, entry.DataEntry)
	_, err = file.Write(buffer)
	if err != nil {
		return fmt.Errorf("не могу уписати назив сд ставке: %v", err)
	}

	return nil
}

func (fat *FileAllocationTable) GetEntry(fsPath string, sb *SuperBlock, entryNo int) (*FileAllocationTableEntry, error) {
	// open the file for reading
	file, err := os.OpenFile(fsPath, os.O_RDONLY, 0640)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// calculate offset
	offset := BOOTLOADER_SIZE + SUPERBLOCK_SIZE + (int(sb.TotalFSEntries) * FSENTRY_SIZE)
	for i := 0; i < entryNo; i++ {
		offset += FATENTRY_SIZE
	}

	// check if offset within file
	fileInfo, err := os.Stat(fsPath)
	if err != nil {
		return nil, fmt.Errorf("не могу утврдити величину ФС-а: %v", err)
	}

	if int64(offset+FATENTRY_SIZE) > fileInfo.Size() {
		return nil, fmt.Errorf("тражени ТДД индекс превазилази величину датотеке")
	}

	// read fat entry
	file.Seek(int64(offset), 0)

	buffer := make([]byte, FATENTRY_SIZE)
	entry := FileAllocationTableEntry{}
	_, err = file.Read(buffer)
	if err != nil {
		return nil, err
	}
	entry.DataEntry = binary.LittleEndian.Uint32(buffer)

	return &entry, nil
}

func (fat *FileAllocationTable) FindFreeFATEntry(startPoint int, fsPath string, sb *SuperBlock) (int, error) {
	if startPoint > int(sb.TotalSectors) {
		return 0, fmt.Errorf("нема слободних сектора")
	}

	for i := startPoint; i < int(sb.TotalSectors); i++ {
		entry, err := fat.GetEntry(fsPath, sb, i)
		if err != nil {
			return 0, err
		}
		if entry.DataEntry == 0 {
			return i, nil
		}
	}

	return 0, fmt.Errorf("грешка приликом тражења слободног сектора")
}

func (fat *FileAllocationTable) AllocateFATEntry(sectorSize int, fsPath string, sb *SuperBlock) (int, error) {
	multiSectorFile := false
	firstSector := 0

	for i := 0; i < int(sb.TotalSectors); i++ {
		entry, err := fat.GetEntry(fsPath, sb, i)
		if err != nil {
			return 0, err
		}

		if entry.DataEntry == 0 {
			if sectorSize == 1 {
				// denote end of the block with FATENTRY_END
				entry.DataEntry = FATENTRY_END
				if err := fat.WriteEntry(fsPath, sb, i, entry); err != nil {
					return 0, err
				}

				if multiSectorFile {
					return firstSector, nil
				}

				return i, nil
			} else {
				if !multiSectorFile {
					multiSectorFile = true
					firstSector = i
				}

				// write the location of the next sector in chain
				nextFreeSector, err := fat.FindFreeFATEntry(i+1, fsPath, sb)
				if err != nil {
					return 0, err
				}
				entry.DataEntry = uint32(nextFreeSector)
				if err := fat.WriteEntry(fsPath, sb, i, entry); err != nil {
					return 0, err
				}
				sectorSize -= 1
			}
		}
	}

	return 0, fmt.Errorf("не могу наћи слободну ТДД-ставку")
}

func RemoveFirstNBytes(data []byte, byteAmount int) []byte {
	if len(data) <= byteAmount {
		block := make([]byte, byteAmount)
		copy(block, data)
		return block
	}

	return data[byteAmount:]
}

func (fat *FileAllocationTable) GetEntryChain(fatEntryNo uint32, fsPath string, sb *SuperBlock) ([]uint32, error) {
	entries := []uint32{fatEntryNo}

	entry, err := fat.GetEntry(fsPath, sb, int(fatEntryNo))
	if err != nil {
		return nil, err
	}
	for entry.DataEntry != FATENTRY_END {
		entries = append(entries, uint32(entry.DataEntry))

		entry, err = fat.GetEntry(fsPath, sb, int(entry.DataEntry))
		if err != nil {
			return nil, err
		}
	}

	return entries, nil
}

func (fat *FileAllocationTable) DeleteEntry(entryNo uint32, fsPath string, sb *SuperBlock) error {
	for i := 0; i < int(sb.TotalSectors); i++ {
		if i == int(entryNo) {
			entry, err := fat.GetEntry(fsPath, sb, i)
			if err != nil {
				return err
			}

			entry.DataEntry = 0
			fat.WriteEntry(fsPath, sb, i, entry)
		}
	}

	return nil
}
