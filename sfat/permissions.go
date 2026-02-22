package sfat

import (
	"fmt"
	"io/fs"
	"strconv"
	"strings"
	"time"
)

func encodeFilePermissions(perms []byte) fs.FileMode {
	fileMode := fs.FileMode(0)
	fileMode |= fs.FileMode(perms[0]) << 6 // user permissions (shift left by 6 bits)
	fileMode |= fs.FileMode(perms[1]) << 3 // group permissions (shift left by 3 bits)
	fileMode |= fs.FileMode(perms[2])      // world permissions

	return fileMode
}

func parseFilePermissions(input string) ([]byte, error) {
	if len(input) != 3 {
		return nil, fmt.Errorf("неисправно овлашћење „%v“", input)
	}

	byteArray := make([]byte, 3)
	for i, char := range input {
		// check if the character is a digit
		if char < '0' || char > '9' {
			return nil, fmt.Errorf("неисправна цифра: %c", char)
		}

		// convert the character to a byte and store it in the byte array
		byteArray[i] = byte(char - '0')
	}

	return byteArray, nil
}

func parseUIDGID(input string) ([]uint16, error) {
	parts := strings.Split(input, ":")
	if len(parts) != 2 {
		return nil, fmt.Errorf("неисправан КИБ/ГИБ")
	}

	parsed := []uint16{}
	for _, part := range parts {
		val, err := strconv.Atoi(part)
		if err != nil {
			return nil, err
		}

		if val < 0 || val > 65535 {
			return nil, fmt.Errorf("КИБ/ГИБ вредност ван опсега 0-65535")
		}
		parsed = append(parsed, uint16(val))
	}

	return parsed, nil
}

func (fs *Filesystem) ChangeEntryMode(filePath, mode string) error {
	// convert to a byte array
	perms, err := parseFilePermissions(mode)
	if err != nil {
		return err
	}

	desiredType := TYPE_FILE
	if strings.HasSuffix(filePath, "/") {
		// we need to change mode on a folder
		desiredType = TYPE_FOLDER
	}

	entryNo, err := fs.FindFSEntryNumber(filePath, desiredType)
	if err != nil {
		// maybe it's a link?
		_, err = fs.FindFSEntryNumber(filePath, TYPE_LINK)
		if err != nil {
			return fmt.Errorf("не могу наћи ставку зарад промене приступа: %v", err)
		} else {
			// we are trying to change mode on a link
			// links are always 777, don't change it
			return nil
		}
	}

	fsEntry, err := fs.FSEntries.GetEntry(fs.Path, fs.SuperBlock, int(entryNo))
	if err != nil {
		return err
	}

	err = fsEntry.ChangeMode(perms)
	if err != nil {
		return fmt.Errorf("не могу променити овлашћење: %v", err)
	}
	fs.FSEntries.WriteEntry(fs.Path, fs.SuperBlock, int(entryNo), fsEntry)

	return nil
}

func ParseTimestamp(input string) (time.Time, error) {
	t, err := time.ParseInLocation("02.01.2006-15:04:05", input, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("неисправан формат времена „%v“, очекиван: дд.мм.гггг-чч:мм:сс", input)
	}
	return t, nil
}

func (fs *Filesystem) ChangeTimestamp(entryPath string, which string, newTime time.Time) error {
	timestamp := TimeToBytes(newTime)

	desiredType := TYPE_FILE
	if strings.HasSuffix(entryPath, "/") {
		desiredType = TYPE_FOLDER
	}

	entryNo, err := fs.FindFSEntryNumber(entryPath, desiredType)
	if err != nil {
		// try link
		entryNo, err = fs.FindFSEntryNumber(entryPath, TYPE_LINK)
		if err != nil {
			return fmt.Errorf("не могу наћи ставку зарад промене времена: %v", err)
		}
	}

	fsEntry, err := fs.FSEntries.GetEntry(fs.Path, fs.SuperBlock, int(entryNo))
	if err != nil {
		return err
	}

	err = fsEntry.ChangeTimestamp(which, timestamp)
	if err != nil {
		return fmt.Errorf("не могу променити временску ознаку: %v", err)
	}
	fs.FSEntries.WriteEntry(fs.Path, fs.SuperBlock, int(entryNo), fsEntry)

	return nil
}

func (fs *Filesystem) ChangeUIDGID(filePath, id string) error {
	// convert to a byte array
	perms, err := parseUIDGID(id)
	if err != nil {
		return err
	}

	desiredType := TYPE_FILE
	if strings.HasSuffix(filePath, "/") {
		// we need to change mode on a folder
		desiredType = TYPE_FOLDER
	}

	entryNo, err := fs.FindFSEntryNumber(filePath, desiredType)
	if err != nil {
		return fmt.Errorf("не могу наћи ставку зарад КИБ/ГИБ промене: %v", err)
	}

	fsEntry, err := fs.FSEntries.GetEntry(fs.Path, fs.SuperBlock, int(entryNo))
	if err != nil {
		return err
	}

	err = fsEntry.ChangeUIDGID(perms)
	if err != nil {
		return fmt.Errorf("не могу променити КИБ/ГИБ: %v", err)
	}
	fs.FSEntries.WriteEntry(fs.Path, fs.SuperBlock, int(entryNo), fsEntry)

	return nil
}
