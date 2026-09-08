package message

import (
	"errors"
	"testing"
)

// webpVP8X builds the header of an extended WebP, the variant an animated sticker always uses.
// Dimensions go in minus one, 24 bits each, which is exactly the trap the parser has to get right.
func webpVP8X(width, height uint32, animated bool) []byte {
	data := make([]byte, riffHeaderMinBytes)
	copy(data[0:4], "RIFF")
	copy(data[8:12], "WEBP")
	copy(data[12:16], "VP8X")
	if animated {
		data[20] = 0x02
	}
	w, h := width-1, height-1
	data[24], data[25], data[26] = byte(w), byte(w>>8), byte(w>>16)
	data[27], data[28], data[29] = byte(h), byte(h>>8), byte(h>>16)
	return data
}

// webpVP8L builds a lossless WebP: both dimensions packed as 14 bits, minus one.
func webpVP8L(width, height uint32) []byte {
	data := make([]byte, riffHeaderMinBytes)
	copy(data[0:4], "RIFF")
	copy(data[8:12], "WEBP")
	copy(data[12:16], "VP8L")
	bits := (width - 1) | ((height - 1) << 14)
	data[21], data[22] = byte(bits), byte(bits>>8)
	data[23], data[24] = byte(bits>>16), byte(bits>>24)
	return data
}

// webpVP8 builds a simple lossy WebP, whose dimensions sit after the 0x9D012A start code.
func webpVP8(width, height uint16) []byte {
	data := make([]byte, riffHeaderMinBytes)
	copy(data[0:4], "RIFF")
	copy(data[8:12], "WEBP")
	copy(data[12:16], "VP8 ")
	copy(data[23:26], "\x9d\x01\x2a")
	data[26], data[27] = byte(width), byte(width>>8)
	data[28], data[29] = byte(height), byte(height>>8)
	return data
}

func TestInspectStickerAcceptsEveryWebPVariantAt512(t *testing.T) {
	cases := map[string][]byte{
		"VP8X": webpVP8X(512, 512, false),
		"VP8L": webpVP8L(512, 512),
		"VP8 ": webpVP8(512, 512),
	}
	for name, data := range cases {
		info, err := inspectSticker(data)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if info.width != 512 || info.height != 512 {
			t.Fatalf("%s: got %dx%d", name, info.width, info.height)
		}
	}
}

// The animation flag decides IsAnimated on the wire. Getting it wrong makes the sticker arrive
// frozen on the recipient's phone, with nothing reporting the problem.
func TestInspectStickerReadsTheAnimationFlag(t *testing.T) {
	still, err := inspectSticker(webpVP8X(512, 512, false))
	if err != nil || still.animated {
		t.Fatalf("parado: animated=%v err=%v", still.animated, err)
	}
	moving, err := inspectSticker(webpVP8X(512, 512, true))
	if err != nil || !moving.animated {
		t.Fatalf("animado: animated=%v err=%v", moving.animated, err)
	}
}

// Off-by-one in the minus-one encoding is the likeliest bug here: 511 stored means 512 real.
func TestInspectStickerRejectsWrongDimensions(t *testing.T) {
	for _, data := range [][]byte{
		webpVP8X(511, 512, false),
		webpVP8L(512, 256),
		webpVP8(1024, 1024),
	} {
		if _, err := inspectSticker(data); !errors.Is(err, ErrStickerDimensions) {
			t.Fatalf("esperava ErrStickerDimensions, veio %v", err)
		}
	}
}

func TestInspectStickerRejectsNonWebP(t *testing.T) {
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, riffHeaderMinBytes)...)
	if _, err := inspectSticker(png); !errors.Is(err, ErrStickerNotWebP) {
		t.Fatalf("PNG: esperava ErrStickerNotWebP, veio %v", err)
	}
	// A RIFF that is not WEBP (a WAV, say) must not slip through on the container alone.
	wav := make([]byte, riffHeaderMinBytes)
	copy(wav[0:4], "RIFF")
	copy(wav[8:12], "WAVE")
	if _, err := inspectSticker(wav); !errors.Is(err, ErrStickerNotWebP) {
		t.Fatalf("WAV: esperava ErrStickerNotWebP, veio %v", err)
	}
	// Truncated: shorter than the header it claims to have.
	if _, err := inspectSticker([]byte("RIFF")); !errors.Is(err, ErrStickerNotWebP) {
		t.Fatalf("truncado: esperava ErrStickerNotWebP, veio %v", err)
	}
}

// The size check comes BEFORE parsing: an oversized file is refused for its size, and the caller
// gets the reason that actually matters instead of a header complaint.
func TestInspectStickerRejectsOversizedBeforeParsing(t *testing.T) {
	big := make([]byte, stickerMaxBytes+1)
	copy(big[0:4], "RIFF")
	copy(big[8:12], "WEBP")
	copy(big[12:16], "VP8X")
	if _, err := inspectSticker(big); !errors.Is(err, ErrStickerTooLarge) {
		t.Fatalf("esperava ErrStickerTooLarge, veio %v", err)
	}
}
