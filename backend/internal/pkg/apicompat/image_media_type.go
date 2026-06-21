package apicompat

import (
	"encoding/base64"
)

// detectImageMediaType inspects the first few bytes of base64-encoded image data
// to determine the actual image format. Returns the correct MIME type, or ""
// if the format cannot be determined.
//
// Magic bytes:
//   - JPEG: starts with FF D8 FF (base64: "/9j/")
//   - PNG:  starts with 89 50 4E 47 (base64: "iVBOR")
//   - GIF:  starts with 47 49 46 38 (base64: "R0lG")
//   - WebP: starts with 52 49 46 46 ... 57 45 42 50 (base64: "UklGR")
func DetectImageMediaType(b64Data string) string {
	if len(b64Data) < 8 {
		return ""
	}

	// Fast path: check base64 prefix (avoids full decode)
	switch {
	case len(b64Data) >= 4 && b64Data[:4] == "/9j/":
		return "image/jpeg"
	case len(b64Data) >= 5 && b64Data[:5] == "iVBOR":
		return "image/png"
	case len(b64Data) >= 4 && b64Data[:4] == "R0lG":
		return "image/gif"
	case len(b64Data) >= 5 && b64Data[:5] == "UklGR":
		// Could be WebP - need to check further
		return detectWebP(b64Data)
	}

	// Slow path: decode first 16 bytes and check magic bytes
	prefix := b64Data
	if len(prefix) > 24 {
		prefix = prefix[:24]
	}
	decoded, err := base64.StdEncoding.DecodeString(prefix + "==")
	if err != nil {
		// Try RawStdEncoding
		decoded, err = base64.RawStdEncoding.DecodeString(prefix)
		if err != nil {
			return ""
		}
	}

	if len(decoded) < 3 {
		return ""
	}

	switch {
	case decoded[0] == 0xFF && decoded[1] == 0xD8 && decoded[2] == 0xFF:
		return "image/jpeg"
	case decoded[0] == 0x89 && decoded[1] == 0x50 && decoded[2] == 0x4E:
		return "image/png"
	case decoded[0] == 0x47 && decoded[1] == 0x49 && decoded[2] == 0x46:
		return "image/gif"
	}

	return ""
}

// detectWebP checks if RIFF data is actually WebP format
func detectWebP(b64Data string) string {
	if len(b64Data) < 16 {
		return ""
	}
	decoded, err := base64.StdEncoding.DecodeString(b64Data[:16] + "==")
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(b64Data[:16])
		if err != nil {
			return ""
		}
	}
	if len(decoded) >= 12 &&
		decoded[8] == 'W' && decoded[9] == 'E' && decoded[10] == 'B' && decoded[11] == 'P' {
		return "image/webp"
	}
	return ""
}
