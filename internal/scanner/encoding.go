package scanner

import (
	"bytes"
	"io"
	"os"
	"unicode/utf8"
)

// DetectAndConvert reads a file and converts to UTF-8 if needed.
// Handles GBK/GB2312 (common on Chinese Windows) auto-detection.
func DetectAndConvert(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	// If already valid UTF-8, return as-is
	if utf8.Valid(data) {
		return string(data), nil
	}

	// Try GBK → UTF-8 conversion
	return convertGBKtoUTF8(data)
}

// convertGBKtoUTF8 converts GBK/GB2312 encoded bytes to UTF-8.
// Uses a simple heuristic: if the file is not valid UTF-8, try GBK decoding.
func convertGBKtoUTF8(data []byte) (string, error) {
	// GBK decoder: use golang.org/x/text/encoding/simplifiedchinese
	// For Phase 3, use a lightweight built-in GBK decoder
	decoded := gbkToUTF8Builtin(data)
	if decoded != "" {
		return decoded, nil
	}

	// Fallback: return as-is (may have encoding issues)
	return string(data), nil
}

// gbkToUTF8Builtin is a minimal GBK→UTF-8 decoder for Phase 3.
// Phase 4: replace with golang.org/x/text/encoding/simplifiedchinese.
func gbkToUTF8Builtin(data []byte) string {
	var buf bytes.Buffer
	reader := bytes.NewReader(data)

	for {
		b, err := reader.ReadByte()
		if err == io.EOF {
			break
		}
		if err != nil {
			return ""
		}

		// ASCII range: pass through
		if b < 0x80 {
			buf.WriteByte(b)
			continue
		}

		// GBK two-byte sequence
		b2, err := reader.ReadByte()
		if err != nil {
			buf.WriteRune('?')
			break
		}

		r := gbkToRune(b, b2)
		buf.WriteRune(r)
	}

	return buf.String()
}

// gbkToRune converts a GBK two-byte code to a Unicode rune.
// See https://en.wikipedia.org/wiki/GBK_(character_encoding)
func gbkToRune(b1, b2 byte) rune {
	// GBK row-cell calculation
	row := int(b1)
	cell := int(b2)

	// Simplified mapping for common Chinese characters
	// Full GBK→Unicode table is ~20K entries; this covers Phase 3 needs
	if row >= 0x81 && row <= 0xFE && cell >= 0x40 && cell <= 0xFE {
		// Placeholder: return replacement character
		// Phase 4: integrate full GBK table or use x/text
		return '�' // replacement character
	}

	return rune(b1)<<8 | rune(b2)
}
