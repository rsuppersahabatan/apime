package message

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// WhatsApp's limits for a sticker. They are the client's, not ours: a file outside them is accepted
// by the server and then fails to render on the recipient's phone, which is the worst outcome
// because nothing reports it. Rejecting here turns a silent failure into a 400 the caller can act on.
const (
	stickerSide        = 512
	stickerMaxBytes    = 500 * 1024
	stickerMimeType    = "image/webp"
	riffHeaderMinBytes = 30
)

var (
	// ErrStickerNotWebP is returned for any container that is not RIFF/WEBP. WhatsApp takes WebP
	// only: PNG, JPEG and GIF are refused, so converting is the caller's job, not ours.
	ErrStickerNotWebP = errors.New("figurinha deve ser WebP")
	// ErrStickerTooLarge guards the 500 KB ceiling.
	ErrStickerTooLarge = errors.New("figurinha deve ter no máximo 500 KB")
	// ErrStickerDimensions guards the mandatory 512x512.
	ErrStickerDimensions = errors.New("figurinha deve ser 512x512")
)

// stickerInfo is what the sticker branch needs to know about the file it is about to upload.
type stickerInfo struct {
	width    uint32
	height   uint32
	animated bool
}

// inspectSticker reads the WebP header to validate the file and discover whether it is animated.
//
// Parsed by hand instead of decoding the image: the project has no image dependency, decoding a
// 500 KB frame per send would be wasted work, and everything needed sits in the first 30 bytes.
//
// WebP is a RIFF container: "RIFF" <size> "WEBP" followed by chunks. The chunk that comes first
// says which of the three variants it is, and each stores its dimensions differently:
//
//	VP8   simple lossy, 14-bit dimensions after a 3-byte start code and the 3-byte signature
//	VP8L  lossless, 14-bit dimensions packed into 4 little-endian bytes, minus one
//	VP8X  extended, 24-bit dimensions minus one, and the only one that can be animated
//
// An animated sticker is always VP8X with the animation bit set, so `animated` comes from the flag
// and not from guessing by size.
func inspectSticker(data []byte) (stickerInfo, error) {
	if len(data) > stickerMaxBytes {
		return stickerInfo{}, fmt.Errorf("%w (%d bytes)", ErrStickerTooLarge, len(data))
	}
	if len(data) < riffHeaderMinBytes {
		return stickerInfo{}, ErrStickerNotWebP
	}
	if string(data[0:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return stickerInfo{}, ErrStickerNotWebP
	}

	var info stickerInfo
	switch string(data[12:16]) {
	case "VP8X":
		// Bit 1 of the flags byte marks animation. The dimensions are stored minus one, 24 bits each.
		info.animated = data[20]&0x02 != 0
		info.width = (uint32(data[24]) | uint32(data[25])<<8 | uint32(data[26])<<16) + 1
		info.height = (uint32(data[27]) | uint32(data[28])<<8 | uint32(data[29])<<16) + 1
	case "VP8L":
		// 14 bits for each dimension, stored minus one, right after the 0x2F signature byte.
		bits := binary.LittleEndian.Uint32(data[21:25])
		info.width = (bits & 0x3FFF) + 1
		info.height = ((bits >> 14) & 0x3FFF) + 1
	case "VP8 ":
		// The 0x9D012A start code precedes the dimensions; the top two bits of each are the scale.
		if string(data[23:26]) != "\x9d\x01\x2a" {
			return stickerInfo{}, ErrStickerNotWebP
		}
		info.width = uint32(binary.LittleEndian.Uint16(data[26:28]) & 0x3FFF)
		info.height = uint32(binary.LittleEndian.Uint16(data[28:30]) & 0x3FFF)
	default:
		return stickerInfo{}, ErrStickerNotWebP
	}

	if info.width != stickerSide || info.height != stickerSide {
		return stickerInfo{}, fmt.Errorf("%w (recebido %dx%d)", ErrStickerDimensions, info.width, info.height)
	}
	return info, nil
}
