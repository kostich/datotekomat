package sfat

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// requireEncryptionSectorSize is the BPS gate for кшу/кшс. Encryption
// requires room for a 45 B header + 16 B AEAD tag (61 B minimum blob) in
// the first sector, plus enough slack to keep small files practical. 512 B
// matches the design and prevents formatting an image with the default
// 16 B sector from being silently usable for encryption.
func requireEncryptionSectorSize(sb *SuperBlock) error {
	if sb.BytesPerSector < MinBytesPerSectorForEncryption() {
		return fmt.Errorf(
			"шифровање захтева бар %d бајтова по сектору (тренутно: %d); поново форматирајте систем са већим бпс",
			MinBytesPerSectorForEncryption(), sb.BytesPerSector,
		)
	}
	return nil
}

// CopyEncryptedFileIn (кшу) reads the host file at filePath, encrypts it
// with the provided passphrase, and stores the resulting blob inside
// fsPath at folderPath/<basename>. FSEntry.Size is the **blob length**
// (header + ciphertext + tag), and FSEntry.Checksum is CRC32 of the blob
// , never of plaintext. timestamp matches the CopyFileIn contract.
//
// Failure modes (in order of evaluation):
//
//   - invalid filename             → "неисправан назив датотеке"
//   - read filesystem              → wrapped from Read
//   - BPS < 512                    → requireEncryptionSectorSize
//   - cannot read host file        → wrapped from os.ReadFile
//   - encryption failure           → wrapped from Encrypt
//   - name collision (any type)    → "ставка с тим именом већ постоји"
//   - not enough sectors/FS entries → existing superblock messages
//   - FAT / FSEntry / folder I/O   → wrapped from underlying calls
//   - WriteBlob failure            → wrapped
func CopyEncryptedFileIn(filePath, folderPath, fsPath, passphrase string, timestamp []byte) error {
	if err := entryNameValid(filePath); err != nil {
		return fmt.Errorf("неисправан назив датотеке: %v", err)
	}

	copyfs, err := Read(fsPath)
	if err != nil {
		return err
	}

	if err := requireEncryptionSectorSize(copyfs.SuperBlock); err != nil {
		return err
	}

	// Load the entire host file into RAM. AEAD requires the plaintext in
	// one buffer to compute the tag; for the file sizes this CLI targets
	// (educational FS, tiny images) this is acceptable. If we ever need
	// streaming, the format is intentionally non-chunked so we'd switch
	// to a different framing.
	plaintext, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("не могу прочитати изворну датотеку: %v", err)
	}

	// Preserve UNIX mode bits the same way CopyFileIn does so chmod after
	// decryption (in кшс) restores the original permissions. We don't
	// need fileSize separately, Size on disk is the blob length.
	_, perms, err := FileProperties(filePath)
	if err != nil {
		return err
	}

	blob, err := Encrypt(plaintext, passphrase)
	if err != nil {
		return fmt.Errorf("не могу шифровати датотеку: %v", err)
	}

	fileName := filepath.Base(filePath)
	if !strings.HasSuffix(folderPath, "/") {
		folderPath += "/"
	}

	// Name collision must cover TYPE_ANY (file, folder, link, encrypted
	//, same path may not refer to two different things), matching the
	// plan's acceptance criterion.
	if copyfs.EntryExists(folderPath+fileName, TYPE_ANY) {
		return fmt.Errorf("ставка с тим именом већ постоји")
	}

	// Sector budget is computed from the blob length, not host file size.
	fileSectorSize := uint32(math.Ceil(float64(len(blob)) / float64(copyfs.SuperBlock.BytesPerSector)))

	if copyfs.SuperBlock.AvailableSectors >= fileSectorSize {
		copyfs.SuperBlock.AvailableSectors -= fileSectorSize

		if copyfs.SuperBlock.AvailableFSEntries > 0 {
			copyfs.SuperBlock.AvailableFSEntries -= 1
		} else {
			return fmt.Errorf("недовољно ставки за упис шифроване датотеке")
		}
	} else {
		return fmt.Errorf("недовољно сектора за упис шифроване датотеке")
	}
	if err := copyfs.WriteSuperBlock(); err != nil {
		return err
	}

	fatEntry, err := copyfs.FileAllocationTable.AllocateFATEntry(int(fileSectorSize), copyfs.Path, copyfs.SuperBlock)
	if err != nil {
		return err
	}

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

	fullFileName := make([]byte, FILENAME_LENGTH)
	copy(fullFileName, []byte(fileName))

	checksum := CalculateCRC32(blob) // checksum the blob so we don't leak metadata

	fsentry := FSEntry{
		Name:        fullFileName,
		FATEntry:    uint32(fatEntry),
		Size:        uint32(len(blob)),
		ParentEntry: uint32(parentNo),
		Type:        TYPE_ENCRYPTED,
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

	if err := copyfs.AddEntryToFolder(entryNo, fsentry.ParentEntry); err != nil {
		return err
	}

	if err := copyfs.DataArea.WriteBlob(copyfs, fsentry.FATEntry, blob); err != nil {
		return err
	}

	return nil
}

// CopyEncryptedFileOut (кшс) reads the encrypted entry at internalPath
// inside fsPath, decrypts it with passphrase, and writes plaintext to
// externalPath. The host file inherits the entry's UNIX mode bits.
//
// CRC32 of the on-disk blob is checked against FSEntry.Checksum **before**
// AEAD decryption, bitrot is reported with a dedicated error so the user
// can distinguish "the image is corrupt" from "you typed the wrong
// password". An AEAD failure after a CRC pass means: wrong passphrase or
// header tampering (which AEAD also catches, but CRC catches it sooner
// and without the scrypt round-trip).
func CopyEncryptedFileOut(internalPath, externalPath, fsPath, passphrase string) error {
	fileName := filepath.Base(internalPath)
	if err := entryNameValid(internalPath); err != nil {
		return fmt.Errorf("неисправан назив датотеке: %v", err)
	}

	if _, err := os.Stat(fileName); err == nil {
		// Mirror CopyFileOut's quirk of guarding by basename in the
		// current dir. CLI passes an explicit externalPath; this check
		// stays for back-compat with how CopyFileOut behaves.
		return fmt.Errorf("датотека већ постоји на вашем систему")
	}

	copyfs, err := Read(fsPath)
	if err != nil {
		return err
	}

	if err := requireEncryptionSectorSize(copyfs.SuperBlock); err != nil {
		return err
	}

	entryNo, err := copyfs.FindFSEntryNumber(internalPath, TYPE_ENCRYPTED)
	if err != nil {
		return fmt.Errorf("не могу наћи шифровану датотеку: %v", err)
	}
	fsEntry, err := copyfs.FSEntries.GetEntry(copyfs.Path, copyfs.SuperBlock, int(entryNo))
	if err != nil {
		return err
	}

	blob, err := copyfs.DataArea.ReadFATData(copyfs, fsEntry.FATEntry, fsEntry.Size)
	if err != nil {
		return fmt.Errorf("не могу прочитати шифровани блок: %v", err)
	}

	if got := CalculateCRC32(blob); got != fsEntry.Checksum {
		return fmt.Errorf(
			"оштећен шифровани блок (ЦРЦ32 0x%08X не одговара ставки 0x%08X)",
			got, fsEntry.Checksum,
		)
	}

	plaintext, err := Decrypt(blob, passphrase)
	if err != nil {
		return fmt.Errorf("не могу дешифровати: %v", err)
	}

	perms := []byte{fsEntry.UserPerm, fsEntry.GroupPerm, fsEntry.WorldPerm}

	hostFile, err := os.Create(externalPath)
	if err != nil {
		return fmt.Errorf("не могу отворити датотеку за упис: %v", err)
	}
	defer hostFile.Close()

	if _, err := hostFile.Write(plaintext); err != nil {
		return fmt.Errorf("не могу уписати податке у датотеку: %v", err)
	}

	if err := os.Chmod(externalPath, encodeFilePermissions(perms)); err != nil {
		return fmt.Errorf("не могу поставити овлашћења датотеке: %v", err)
	}

	return nil
}
