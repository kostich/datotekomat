package sfat

import (
	"hash/crc32"
	"io"
	"os"
)

// CalculateCRC32File calculates the CRC32 checksum of the file at the given path
// without loading the entire file into memory.
func CalculateCRC32File(filePath string) (uint32, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	hash := crc32.New(crc32.IEEETable)

	// create a buffer to read chunks of the file
	buf := make([]byte, 4096) // 4KB buffer size

	// read the file in chunks and update the hash
	for {
		n, err := file.Read(buf)
		if err != nil && err != io.EOF {
			return 0, err
		}
		if n == 0 {
			break
		}

		hash.Write(buf[:n])
	}

	return hash.Sum32(), nil
}

// CalculateCRC32 calculates the CRC32 checksum of the given byte slice
func CalculateCRC32(data []byte) uint32 {
	hash := crc32.New(crc32.IEEETable)
	hash.Write(data)

	return hash.Sum32()
}
