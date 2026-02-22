package sfat

import (
	"encoding/binary"
	"fmt"
	"os"
	"strings"
)

var BOOTLOADER_SIZE = 4096
var SUPERBLOCK_RESERVED = 425
var SUPERBLOCK_SIZE = FILENAME_LENGTH + SUPERBLOCK_RESERVED + 25

type SuperBlock struct {
	TotalSectors       uint32 // 4 bytes
	AvailableSectors   uint32 // 4 bytes
	SectorsPerCluster  uint32 // 4 bytes, reserved for v2 when we implement sector clusters
	BytesPerSector     uint32 // 4 bytes
	TotalFSEntries     uint32 // 4 bytes
	AvailableFSEntries uint32 // 4 bytes
	FsType             byte   // 1 byte
	Reserved           []byte // 425 bytes for future use (also makes the superblock one sector large)
	Label              string // 62 bytes
}

func (fs *Filesystem) WriteSuperBlock() error {
	offset := int64(BOOTLOADER_SIZE)

	// open the file for writing
	file, err := os.OpenFile(fs.Path, os.O_WRONLY, 0640)
	if err != nil {
		return fmt.Errorf("не могу направити нову или обрисати садржај постојеће датотеке: %v", err)
	}
	defer file.Close()

	// write the superblock total sectors
	buffer := make([]byte, 4)
	binary.LittleEndian.PutUint32(buffer, fs.SuperBlock.TotalSectors)
	_, err = file.WriteAt(buffer, offset)
	if err != nil {
		return fmt.Errorf("не могу уписати укупно сектора супер-блока: %v", err)
	}
	offset += int64(len(buffer))

	// write the superblock available sectors
	binary.LittleEndian.PutUint32(buffer, fs.SuperBlock.AvailableSectors)
	_, err = file.WriteAt(buffer, offset)
	if err != nil {
		return fmt.Errorf("не могу уписати доступне секторе супер-блока: %v", err)
	}
	offset += int64(len(buffer))

	// write the superblock sectors per cluster
	binary.LittleEndian.PutUint32(buffer, fs.SuperBlock.SectorsPerCluster)
	_, err = file.WriteAt(buffer, offset)
	if err != nil {
		return fmt.Errorf("не могу уписати секторе по кластеру супер-блока: %v", err)
	}
	offset += int64(len(buffer))

	// write the superblock bytes per sector
	binary.LittleEndian.PutUint32(buffer, fs.SuperBlock.BytesPerSector)
	_, err = file.WriteAt(buffer, offset)
	if err != nil {
		return fmt.Errorf("не могу уписати бајтове по сектору супер-блока: %v", err)
	}
	offset += int64(len(buffer))

	// write the superblock total fs entries
	binary.LittleEndian.PutUint32(buffer, fs.SuperBlock.TotalFSEntries)
	_, err = file.WriteAt(buffer, offset)
	if err != nil {
		return fmt.Errorf("не могу уписати укупне ставке супер-блока: %v", err)
	}
	offset += int64(len(buffer))

	// write the superblock available fs entries
	binary.LittleEndian.PutUint32(buffer, fs.SuperBlock.AvailableFSEntries)
	_, err = file.WriteAt(buffer, offset)
	if err != nil {
		return fmt.Errorf("не могу уписати доступне ставке супер-блока: %v", err)
	}
	offset += int64(len(buffer))

	// write the superblock fs type
	_, err = file.WriteAt([]byte{fs.SuperBlock.FsType}, offset)
	if err != nil {
		return fmt.Errorf("не могу уписати доступне тип система датотека супер-блока: %v", err)
	}
	offset += 1

	// write the superblock reserved space
	_, err = file.WriteAt(fs.SuperBlock.Reserved, offset)
	if err != nil {
		return fmt.Errorf("не могу уписати резервни простор супер-блока: %v", err)
	}
	offset += int64(len(fs.SuperBlock.Reserved))

	// write the superblock label
	label := make([]byte, FILENAME_LENGTH)
	copy(label, []byte(fs.SuperBlock.Label))
	_, err = file.WriteAt(label, offset)
	if err != nil {
		return fmt.Errorf("не могу уписати резервни простор супер-блока: %v", err)
	}
	offset += int64(len(label))

	return nil
}

func (fs *Filesystem) ReadSuperBlock() error {
	// open the file for reading
	file, err := os.OpenFile(fs.Path, os.O_RDONLY, 0640)
	if err != nil {
		return fmt.Errorf("не могу отворити датотеку за читање супер-блока: %v", err)
	}
	defer file.Close()

	file.Seek(int64(BOOTLOADER_SIZE), 0) // skip the bootloader area

	buffer4b := make([]byte, 4)
	_, err = file.Read(buffer4b)
	if err != nil {
		return fmt.Errorf("не могу прочитати укупно сектора: %v", err)
	}
	fs.SuperBlock.TotalSectors = binary.LittleEndian.Uint32(buffer4b)

	_, err = file.Read(buffer4b)
	if err != nil {
		return fmt.Errorf("не могу прочитати доступне секторе: %v", err)
	}
	fs.SuperBlock.AvailableSectors = binary.LittleEndian.Uint32(buffer4b)

	_, err = file.Read(buffer4b)
	if err != nil {
		return fmt.Errorf("не могу прочитати секторе по кластеру: %v", err)
	}
	fs.SuperBlock.SectorsPerCluster = binary.LittleEndian.Uint32(buffer4b)

	_, err = file.Read(buffer4b)
	if err != nil {
		return fmt.Errorf("не могу прочитати бајтове по сектору: %v", err)
	}
	fs.SuperBlock.BytesPerSector = binary.LittleEndian.Uint32(buffer4b)

	_, err = file.Read(buffer4b)
	if err != nil {
		return fmt.Errorf("не могу прочитати укупних сд ставки: %v", err)
	}
	fs.SuperBlock.TotalFSEntries = binary.LittleEndian.Uint32(buffer4b)

	_, err = file.Read(buffer4b)
	if err != nil {
		return fmt.Errorf("не могу прочитати доступних сд ставки: %v", err)
	}
	fs.SuperBlock.AvailableFSEntries = binary.LittleEndian.Uint32(buffer4b)

	buffer1b := make([]byte, 1)
	_, err = file.Read(buffer1b)
	if err != nil {
		return fmt.Errorf("не могу прочитати тип система датотека: %v", err)
	}
	fs.SuperBlock.FsType = buffer1b[0]

	bufferRes := make([]byte, SUPERBLOCK_RESERVED)
	_, err = file.Read(bufferRes)
	if err != nil {
		return fmt.Errorf("не могу прочитати резервисани простор: %v", err)
	}
	fs.SuperBlock.Reserved = bufferRes

	bufferLabel := make([]byte, FILENAME_LENGTH)
	_, err = file.Read(bufferLabel)
	if err != nil {
		return fmt.Errorf("не могу прочитати етикету: %v", err)
	}
	fs.SuperBlock.Label = strings.TrimRight(string(bufferLabel), "\x00")

	return nil
}

func (fs *Filesystem) RenameLabel(newLabel string) error {
	if newLabel == "" {
		return fmt.Errorf("назив етикете не може бити празан")
	}

	if !entrySizeValid(newLabel, FILENAME_LENGTH) {
		return fmt.Errorf("назив етикете је предугачак")
	}

	fs.SuperBlock.Label = newLabel
	return fs.WriteSuperBlock()
}

func (sb *SuperBlock) Details() string {
	details := fmt.Sprintf("укупно сектора: %v\nдоступних сектора: %v\n", sb.TotalSectors, sb.AvailableSectors)
	details += fmt.Sprintf("бајтова по сектору: %v\nсектора по кластеру: %v\n", sb.BytesPerSector, sb.SectorsPerCluster)
	details += fmt.Sprintf("укупно ставки: %v\nдоступних ставки: %v\n", sb.TotalFSEntries, sb.AvailableFSEntries)
	details += fmt.Sprintf("тип система датотека: %X\nрезервисано: %v\n", sb.FsType, len(sb.Reserved))
	details += fmt.Sprintf("етикета: „%v“", sb.Label)
	return details
}
