package ui

import (
	"fmt"
	"image"
	"image/color"
)

// decodeTGA handles the true-colour, grayscale and colour-mapped TGA variants
// used by the game's screen art, including RLE and all four image origins.
func DecodeTGA(data []byte) (image.Image, error) {
	if len(data) < 18 {
		return nil, fmt.Errorf("TGA header is truncated")
	}
	idLen, cmapType, imageType := int(data[0]), data[1], data[2]
	w, h := int(data[12])|int(data[13])<<8, int(data[14])|int(data[15])<<8
	depth, descriptor := int(data[16]), data[17]
	if w < 1 || h < 1 || w > 8192 || h > 8192 || w*h > 16*1024*1024 {
		return nil, fmt.Errorf("invalid TGA dimensions %dx%d", w, h)
	}
	mapped := imageType == 1 || imageType == 9
	gray := imageType == 3 || imageType == 11
	rle := imageType == 9 || imageType == 10 || imageType == 11
	if !mapped && !gray && imageType != 2 && imageType != 10 {
		return nil, fmt.Errorf("unsupported TGA image type %d", imageType)
	}
	if mapped && cmapType != 1 || !mapped && cmapType != 0 {
		return nil, fmt.Errorf("unsupported TGA colour map type %d", cmapType)
	}
	if mapped && depth != 8 || gray && depth != 8 || !mapped && !gray && depth != 16 && depth != 24 && depth != 32 {
		return nil, fmt.Errorf("unsupported TGA pixel depth %d", depth)
	}
	p := 18 + idLen
	if p > len(data) {
		return nil, fmt.Errorf("TGA image id is truncated")
	}
	paletteFirst, paletteEnd := 0, 0
	palette := []color.NRGBA(nil)
	if mapped {
		paletteFirst = int(data[3]) | int(data[4])<<8
		paletteEnd = paletteFirst + int(data[5]) + int(data[6])<<8
		paletteDepth := int(data[7])
		bytesPerColour := (paletteDepth + 7) / 8
		if paletteEnd <= paletteFirst || paletteEnd > 65536 || (paletteDepth != 16 && paletteDepth != 24 && paletteDepth != 32) || p+paletteEnd*bytesPerColour > len(data) {
			return nil, fmt.Errorf("invalid or truncated TGA colour map")
		}
		palette = make([]color.NRGBA, paletteEnd)
		for i := paletteFirst; i < paletteEnd; i++ {
			c, err := tgaColour(data[p+i*bytesPerColour:p+(i+1)*bytesPerColour], paletteDepth, descriptor&15)
			if err != nil {
				return nil, err
			}
			palette[i] = c
		}
		p += paletteEnd * bytesPerColour
	}
	bytesPerPixel := (depth + 7) / 8
	read := func() (color.NRGBA, error) {
		if p+bytesPerPixel > len(data) {
			return color.NRGBA{}, fmt.Errorf("TGA pixels are truncated")
		}
		b := data[p : p+bytesPerPixel]
		p += bytesPerPixel
		if mapped {
			i := int(b[0])
			if i < paletteFirst || i >= paletteEnd {
				return color.NRGBA{}, fmt.Errorf("TGA palette index out of range")
			}
			return palette[i], nil
		}
		if gray {
			return color.NRGBA{R: b[0], G: b[0], B: b[0], A: 255}, nil
		}
		return tgaColour(b, depth, descriptor&15)
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h; {
		count, repeat := 1, false
		if rle {
			if p >= len(data) {
				return nil, fmt.Errorf("TGA RLE stream is truncated")
			}
			header := data[p]
			p++
			count, repeat = int(header&127)+1, header&128 != 0
			if count > w*h-i {
				return nil, fmt.Errorf("TGA RLE packet exceeds image")
			}
		}
		var repeated color.NRGBA
		var err error
		if repeat {
			repeated, err = read()
			if err != nil {
				return nil, err
			}
		}
		for n := 0; n < count; n++ {
			c := repeated
			if !repeat {
				c, err = read()
				if err != nil {
					return nil, err
				}
			}
			sx, sy := i%w, i/w
			if descriptor&0x10 != 0 {
				sx = w - 1 - sx
			}
			if descriptor&0x20 == 0 {
				sy = h - 1 - sy
			}
			img.SetNRGBA(sx, sy, c)
			i++
		}
	}
	return img, nil
}

func tgaColour(b []byte, depth int, attributeBits uint8) (color.NRGBA, error) {
	switch depth {
	case 16:
		if len(b) < 2 {
			return color.NRGBA{}, fmt.Errorf("TGA colour is truncated")
		}
		v := uint16(b[0]) | uint16(b[1])<<8
		expand := func(x uint16) uint8 { return uint8((x << 3) | (x >> 2)) }
		a := uint8(255)
		if attributeBits > 0 && v&0x8000 == 0 {
			a = 0
		}
		return color.NRGBA{R: expand((v >> 10) & 31), G: expand((v >> 5) & 31), B: expand(v & 31), A: a}, nil
	case 24:
		if len(b) < 3 {
			return color.NRGBA{}, fmt.Errorf("TGA colour is truncated")
		}
		return color.NRGBA{R: b[2], G: b[1], B: b[0], A: 255}, nil
	case 32:
		if len(b) < 4 {
			return color.NRGBA{}, fmt.Errorf("TGA colour is truncated")
		}
		a := uint8(255)
		if attributeBits > 0 {
			a = b[3]
		}
		return color.NRGBA{R: b[2], G: b[1], B: b[0], A: a}, nil
	default:
		return color.NRGBA{}, fmt.Errorf("unsupported TGA colour depth %d", depth)
	}
}
