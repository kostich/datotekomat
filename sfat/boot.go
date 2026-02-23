package sfat

import (
	"fmt"
	"os"
)

type BootArea struct{}

func (fs *Filesystem) ShowBootloader() {
	fmt.Printf("Подизач система: ")
	// read the bootloader to memory
	fsFile, err := os.OpenFile(fs.Path, os.O_RDONLY, 0640)
	if err != nil {
		fmt.Printf("грешка приликом читања датотеке система датотека: %v.\n", err)
		return
	}
	defer fsFile.Close()

	bootLoader := make([]byte, BOOTLOADER_SIZE)
	_, err = fsFile.Read(bootLoader)
	if err != nil {
		fmt.Printf("грешка приликом читања подизача система: %v.\n", err)
		return
	}

	// show the bootloader properties
	bootSectorUsedBytes := 0
	bootTotalUsedBytes := 0
	for bi, bc := range bootLoader {

		if bc != 0 {
			bootTotalUsedBytes += 1

			// useful to count and know the amount of used bytes in the first sector
			// since we need that space to use BIOS calls to read all other code
			// from other sectors of our bootloader
			if bi < 512 {
				bootSectorUsedBytes += 1
			}
		}
	}
	if bootTotalUsedBytes == 0 {
		fmt.Println("подизач система није уписан.")
	} else {
		fmt.Printf(
			"%v/512 бајтова у првом сектору, %v/%v укупно заузето.\n",
			bootSectorUsedBytes, bootTotalUsedBytes, BOOTLOADER_SIZE,
		)
	}
}

func (fs *Filesystem) WriteBootArea(blPath string) error {
	// read the bootloader to memory
	bootFile, err := os.OpenFile(blPath, os.O_RDONLY, 0640)
	if err != nil {
		return err
	}
	defer bootFile.Close()

	bootLoader := make([]byte, BOOTLOADER_SIZE)
	_, err = bootFile.Read(bootLoader)
	if err != nil {
		return err
	}

	// write the bootloader to the boot area
	fsFile, err := os.OpenFile(fs.Path, os.O_WRONLY, 0640)
	if err != nil {
		return err
	}
	defer fsFile.Close()

	count, err := fsFile.Write(bootLoader)
	if err != nil {
		return fmt.Errorf("не могу уписати подизача система: %v", err)
	}

	fmt.Printf("уписао %vБ у датотеку %v\n", count, fs.Path)

	return nil
}

func (ba *BootArea) AllocateBootArea(fsPath string) error {
	file, err := os.OpenFile(fsPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0640)
	if err != nil {
		return fmt.Errorf("cannot open file system file: %v", err)
	}
	defer file.Close()

	bootloader := make([]byte, BOOTLOADER_SIZE)
	if _, err := file.Write(bootloader); err != nil {
		return fmt.Errorf("error writing bootloader area: %v", err)
	}

	return nil
}
