package sfat

import (
	"encoding/binary"
	"fmt"
	"time"
)

// consistent timestamp for all test files and folders
var TestTimestamp = []byte{0x00, 0x07, 0xE8, 0x07, 0x11, 0x06, 0x12, 0x31, 0x01, 0x03, 0x08, 0x09, 0x00, 0x00}

// TimeToBytes converts a time.Time to a 14-byte representation that
// we can use to set the created, modified and accessed properties of a file
func TimeToBytes(t time.Time) []byte {
	timestamp := make([]byte, 14)
	year := t.Year()                                         // year (3 bytes)
	binary.BigEndian.PutUint32(timestamp[0:4], uint32(year)) // write as 4 bytes first
	copy(timestamp[0:3], timestamp[1:4])                     // use only 3 bytes
	timestamp[3] = byte(t.Month())                           // month
	timestamp[4] = byte(t.Day())                             // day
	timestamp[5] = byte(t.Hour())                            // hour
	timestamp[6] = byte(t.Minute())                          // minute
	timestamp[7] = byte(t.Second())                          // second
	nanosecond := t.Nanosecond()
	binary.BigEndian.PutUint32(timestamp[8:12], uint32(nanosecond)) // nanosecond (4 bytes)
	_, offset := t.Zone()                                           // timezone (2 bytes)
	binary.BigEndian.PutUint16(timestamp[12:14], uint16(offset/60)) // store timezone offset in minutes

	return timestamp
}

// BytesToTime converts a 14-byte representation to a time.Time
func BytesToTime(timestamp []byte) (time.Time, error) {
	if len(timestamp) != 14 {
		return time.Time{}, fmt.Errorf("неисправна количина бајтова датума и времена: %d", len(timestamp))
	}

	// year (3 bytes)
	year := int(binary.BigEndian.Uint32(append([]byte{0}, timestamp[0:3]...)))
	month := time.Month(timestamp[3])                           // month
	day := int(timestamp[4])                                    // day
	hour := int(timestamp[5])                                   // hour
	minute := int(timestamp[6])                                 // minute
	second := int(timestamp[7])                                 // second
	nanosecond := int(binary.BigEndian.Uint32(timestamp[8:12])) // nanosecond (4 bytes)
	// timezone (2 bytes)
	offset := int(binary.BigEndian.Uint16(timestamp[12:14])) * 60 // offset in seconds
	// create time.Time with the given timezone offset
	loc := time.FixedZone("", offset)

	return time.Date(year, month, day, hour, minute, second, nanosecond, loc), nil
}

func formatTimestamp(t time.Time) string {
	// format the timestamp to the required format
	timeString := t.Format("02.01.2006 15:04:05.000 -0700")
	// extract the nanoseconds
	nano := t.Nanosecond() / 1e6 // convert to milliseconds and truncate to int
	// format the nanoseconds to always be three digits
	formattedNano := fmt.Sprintf("%03d", nano)
	// replace the ".000" placeholder with the formatted nanoseconds
	return timeString[:20] + formattedNano + timeString[23:]
}
