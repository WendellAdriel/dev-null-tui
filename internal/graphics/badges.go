package graphics

import (
	"bytes"
	"embed"
	"image"
	"image/color"
	"image/png"
	"strings"
)

// badgeIcons are the web's achievement icons as white 96px masks: Lucide 1.66.0 (ISC, see badges/LICENSE-lucide)
// and the web's own dragon ball and mushroom cloud, rendered with librsvg.
//
//go:embed badges/*.png
var badgeIcons embed.FS

func knownBadge(kind string) bool {
	name, ok := strings.CutPrefix(kind, "badge-")
	if !ok || name == "" || strings.ContainsAny(name, "/.") {
		return false
	}
	_, err := badgeIcons.Open("badges/" + name + ".png")
	return err == nil
}

// tintedBadge paints a badge mask in the ring color, keeping the mask's alpha.
func tintedBadge(kind, ring string) (*image.NRGBA, error) {
	data, err := badgeIcons.ReadFile("badges/" + strings.TrimPrefix(kind, "badge-") + ".png")
	if err != nil {
		return nil, err
	}
	mask, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	ink := parseRing(ring)
	bounds := mask.Bounds()
	im := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	for y := range bounds.Dy() {
		for x := range bounds.Dx() {
			alpha := color.NRGBAModel.Convert(mask.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.NRGBA).A
			if alpha == 0 {
				continue
			}
			pixel := ink
			pixel.A = alpha
			im.SetNRGBA(x, y, pixel)
		}
	}
	return im, nil
}
