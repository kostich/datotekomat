package sfat

import (
	"os"
	"strings"
	"testing"
)

func TestRenameLabel(t *testing.T) {
	tempDir := t.TempDir()
	tempFSFile, err := os.CreateTemp(tempDir, "testfs-*.img")
	if err != nil {
		t.Fatalf("TestRenameLabel: %v", err)
	}
	tempFSFile.Close()

	// create a filesystem with an initial label
	_, err = generateFS(8, 16, 4, "СтараЕтк", tempFSFile.Name())
	if err != nil {
		t.Fatalf("TestRenameLabel: %v", err)
	}

	// read the filesystem back
	testfs, err := Read(tempFSFile.Name())
	if err != nil {
		t.Fatalf("TestRenameLabel: %v", err)
	}

	// verify the initial label
	initialLabel := strings.TrimRight(testfs.SuperBlock.Label, "\x00")
	if initialLabel != "СтараЕтк" {
		t.Fatalf("TestRenameLabel: очекивана почетна етикета СтараЕтк, добијена %v", initialLabel)
	}

	// rename the label
	err = testfs.RenameLabel("НоваЕтк")
	if err != nil {
		t.Fatalf("TestRenameLabel: %v", err)
	}

	// read the filesystem again to verify persistence
	testfs2, err := Read(tempFSFile.Name())
	if err != nil {
		t.Fatalf("TestRenameLabel: %v", err)
	}

	newLabel := strings.TrimRight(testfs2.SuperBlock.Label, "\x00")
	if newLabel != "НоваЕтк" {
		t.Fatalf("TestRenameLabel: очекивана нова етикета НоваЕтк, добијена %v", newLabel)
	}
}

func TestRenameLabelTooLong(t *testing.T) {
	tempDir := t.TempDir()
	tempFSFile, err := os.CreateTemp(tempDir, "testfs-*.img")
	if err != nil {
		t.Fatalf("TestRenameLabelTooLong: %v", err)
	}
	tempFSFile.Close()

	_, err = generateFS(8, 16, 4, "Тест", tempFSFile.Name())
	if err != nil {
		t.Fatalf("TestRenameLabelTooLong: %v", err)
	}

	testfs, err := Read(tempFSFile.Name())
	if err != nil {
		t.Fatalf("TestRenameLabelTooLong: %v", err)
	}

	// create a label that exceeds FILENAME_LENGTH (62 bytes)
	longLabel := strings.Repeat("А", 63)
	err = testfs.RenameLabel(longLabel)
	if err == nil {
		t.Fatalf("TestRenameLabelTooLong: очекивана грешка за предугачку етикету, а није је било")
	}
}

func TestRenameLabelEmpty(t *testing.T) {
	tempDir := t.TempDir()
	tempFSFile, err := os.CreateTemp(tempDir, "testfs-*.img")
	if err != nil {
		t.Fatalf("TestRenameLabelEmpty: %v", err)
	}
	tempFSFile.Close()

	_, err = generateFS(8, 16, 4, "Тест", tempFSFile.Name())
	if err != nil {
		t.Fatalf("TestRenameLabelEmpty: %v", err)
	}

	testfs, err := Read(tempFSFile.Name())
	if err != nil {
		t.Fatalf("TestRenameLabelEmpty: %v", err)
	}

	err = testfs.RenameLabel("")
	if err == nil {
		t.Fatalf("TestRenameLabelEmpty: очекивана грешка за празну етикету, а није је било")
	}
}
