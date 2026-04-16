package sandbox

import (
	"strings"
	"unicode"
)

// SanitizeInput strips dangerous Unicode characters from untrusted text
// to defend against steganographic prompt injection.
func SanitizeInput(s string) string {
	return strings.Map(func(r rune) rune {
		// Tag characters (U+E0000-U+E007F)
		if r >= 0xE0000 && r <= 0xE007F {
			return -1
		}
		// Zero-width characters
		switch r {
		case 0x200B, 0x200C, 0x200D, 0xFEFF:
			return -1
		}
		// Bidirectional overrides (U+202A-U+202E)
		if r >= 0x202A && r <= 0x202E {
			return -1
		}
		// Bidirectional isolates (U+2066-U+2069)
		if r >= 0x2066 && r <= 0x2069 {
			return -1
		}
		// Strip other control characters except common whitespace
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return -1
		}
		return r
	}, s)
}
