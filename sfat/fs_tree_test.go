package sfat

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"testing"
)

// TestListTree tests the tree display of the filesystem hierarchy
func TestListTree(t *testing.T) {
	tempDir := t.TempDir()
	tempFSFile, err := os.CreateTemp(tempDir, "testfs-*.img")
	if err != nil {
		t.Fatalf("не могу направити привремену датотеку: %v", err)
	}
	tempFSFile.Close()

	// create a filesystem with enough space
	_, err = generateFS(12, 16, 8, "СТАБЛО-1", tempFSFile.Name())
	if err != nil {
		t.Fatalf("не могу створити систем датотека: %v", err)
	}

	// create folders: /фас1/ and /фас2/
	testfs, err := Read(tempFSFile.Name())
	if err != nil {
		t.Fatalf("не могу прочитати систем датотека: %v", err)
	}
	if err := testfs.CreateFolder("/фас1", TestTimestamp); err != nil {
		t.Fatalf("не могу створити фас1: %v", err)
	}

	testfs, err = Read(tempFSFile.Name())
	if err != nil {
		t.Fatalf("не могу прочитати систем датотека: %v", err)
	}
	if err := testfs.CreateFolder("/фас1/подфас", TestTimestamp); err != nil {
		t.Fatalf("не могу створити подфас: %v", err)
	}

	testfs, err = Read(tempFSFile.Name())
	if err != nil {
		t.Fatalf("не могу прочитати систем датотека: %v", err)
	}
	if err := testfs.CreateFolder("/фас2", TestTimestamp); err != nil {
		t.Fatalf("не могу створити фас2: %v", err)
	}

	// copy a file into /фас1/подфас/
	fileContents := []byte{0x41, 0x42, 0x43, 0x44, 0x45, 0x46, 0x47, 0x0A}
	tempTestFilePath := tempDir + "/д.ткт"
	if err := os.WriteFile(tempTestFilePath, fileContents, 0640); err != nil {
		t.Fatalf("не могу уписати пробну датотеку: %v", err)
	}
	if err := os.Chmod(tempTestFilePath, fs.FileMode(0640)); err != nil {
		t.Fatalf("не могу поставити овлашћења: %v", err)
	}
	if err := CopyFileIn(tempTestFilePath, "/фас1/подфас/", tempFSFile.Name(), TestTimestamp); err != nil {
		t.Fatalf("не могу копирати датотеку: %v", err)
	}

	// test full tree from root
	testfs, err = Read(tempFSFile.Name())
	if err != nil {
		t.Fatalf("не могу прочитати систем датотека: %v", err)
	}

	// redirect stdout to a pipe so we can read what ListTree prints
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("не могу направити цев: %v", err)
	}
	os.Stdout = w

	if err := testfs.ListTree("/"); err != nil {
		os.Stdout = oldStdout
		t.Fatalf("ListTree грешка: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	expectedFull := "/ (8.00Б)\n" +
		"├── фас1/ (4.00Б)\n" +
		"│   └── подфас/ (4.00Б)\n" +
		"│       └── д.ткт (8.00Б)\n" +
		"└── фас2/ (0.00Б)\n"

	if output != expectedFull {
		t.Fatalf("неочекиван излаз стабла:\nочекивано:\n%v\nдобијено:\n%v", expectedFull, output)
	}

	// test subtree from /фас1/
	testfs, err = Read(tempFSFile.Name())
	if err != nil {
		t.Fatalf("не могу прочитати систем датотека: %v", err)
	}

	r, w, err = os.Pipe()
	if err != nil {
		t.Fatalf("не могу направити цев: %v", err)
	}
	os.Stdout = w

	if err := testfs.ListTree("/фас1"); err != nil {
		os.Stdout = oldStdout
		t.Fatalf("ListTree грешка: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	buf.Reset()
	io.Copy(&buf, r)
	output = buf.String()

	expectedSubtree := "фас1/ (4.00Б)\n" +
		"└── подфас/ (4.00Б)\n" +
		"    └── д.ткт (8.00Б)\n"

	if output != expectedSubtree {
		t.Fatalf("неочекиван излаз подстабла:\nочекивано:\n%v\nдобијено:\n%v", expectedSubtree, output)
	}
}

// TestListTreeEmpty tests the tree display of an empty filesystem
func TestListTreeEmpty(t *testing.T) {
	tempDir := t.TempDir()
	tempFSFile, err := os.CreateTemp(tempDir, "testfs-*.img")
	if err != nil {
		t.Fatalf("не могу направити привремену датотеку: %v", err)
	}
	tempFSFile.Close()

	_, err = generateFS(4, 16, 4, "СТАБЛО-2", tempFSFile.Name())
	if err != nil {
		t.Fatalf("не могу створити систем датотека: %v", err)
	}

	testfs, err := Read(tempFSFile.Name())
	if err != nil {
		t.Fatalf("не могу прочитати систем датотека: %v", err)
	}

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("не могу направити цев: %v", err)
	}
	os.Stdout = w

	if err := testfs.ListTree("/"); err != nil {
		os.Stdout = oldStdout
		t.Fatalf("ListTree грешка: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	expected := "/ (0.00Б)\n"

	if output != expected {
		t.Fatalf("неочекиван излаз празног стабла:\nочекивано:\n%v\nдобијено:\n%v", expected, output)
	}
}
