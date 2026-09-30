package host

import "unicode/utf16"

// UTF16LE encodes s as UTF-16 little endian without a byte order mark, the
// form clip.exe reads.
func UTF16LE(s string) []byte {
	u := utf16.Encode([]rune(s))
	b := make([]byte, 0, 2*len(u))
	for _, c := range u {
		b = append(b, byte(c), byte(c>>8))
	}
	return b
}
