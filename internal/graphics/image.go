package graphics

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"math"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

func prepare(data []byte, req Request) (*image.NRGBA, error) {
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 || uint64(config.Width)*uint64(config.Height) > 8_000_000 || (format != "png" && format != "jpeg" && format != "webp") {
		return nil, errors.New("image unavailable")
	}
	source, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, errors.New("image unavailable")
	}
	bounds := source.Bounds()
	width, height := min(config.Width, req.Cols*16), min(config.Height, req.Rows*32)
	if req.Ring != "" {
		width, height = 96, 96
		side := min(bounds.Dx(), bounds.Dy())
		bounds = image.Rect(bounds.Min.X+(bounds.Dx()-side)/2, bounds.Min.Y+(bounds.Dy()-side)/2, bounds.Min.X+(bounds.Dx()+side)/2, bounds.Min.Y+(bounds.Dy()+side)/2)
	} else {
		scale := min(float64(width)/float64(config.Width), float64(height)/float64(config.Height))
		width, height = max(1, int(float64(config.Width)*scale)), max(1, int(float64(config.Height)*scale))
	}
	output := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(output, output.Bounds(), source, bounds, draw.Src, nil)
	if req.Ring != "" {
		ring := parseRing(req.Ring)
		for y := range height {
			for x := range width {
				distance := math.Hypot(float64(x)+0.5-48, float64(y)+0.5-48)
				switch {
				case distance > 47.5:
					output.SetNRGBA(x, y, color.NRGBA{})
				case distance > 43:
					ring.A = byte(255 * min(1, 47.5-distance))
					output.SetNRGBA(x, y, ring)
				case distance > 40:
					output.SetNRGBA(x, y, color.NRGBA{})
				}
			}
		}
	}
	return output, nil
}
