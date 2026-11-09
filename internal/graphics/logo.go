package graphics

import (
	"bytes"
	_ "embed"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strings"
)

//go:embed dev-null-logo-dark.png
var darkLogo []byte

//go:embed dev-null-logo-light.png
var lightLogo []byte

//go:embed pastel-de-nata.png
var pastel []byte

// embeddedIcon reports whether an icon ships as artwork with its own colors, so it needs no lane color.
func embeddedIcon(kind string) bool {
	return strings.HasPrefix(kind, "logo-") || kind == "pastel"
}

func prepareLogo(req Request) (*image.NRGBA, error) {
	data, width, height := darkLogo, req.Cols*16, req.Rows*32
	switch strings.TrimSuffix(req.Icon, "-santa") {
	case "logo-light":
		data = lightLogo
	case "pastel":
		// Stats icons share the 96px square canvas of the rasterized stars, which stays sharp on dense displays.
		data, width, height = pastel, 96, 96
	}
	if strings.HasSuffix(req.Icon, "-santa") {
		hatted, err := withSantaHat(data)
		if err != nil {
			return nil, err
		}
		data = hatted
	}
	logo, err := prepare(data, Request{Cols: width / 16, Rows: height / 32})
	if err != nil {
		return nil, err
	}
	canvas := image.NewNRGBA(image.Rect(0, 0, width, height))
	x := (canvas.Bounds().Dx() - logo.Bounds().Dx()) / 2
	y := (canvas.Bounds().Dy() - logo.Bounds().Dy()) / 2
	draw.Draw(canvas, logo.Bounds().Add(image.Pt(x, y)), logo, image.Point{}, draw.Src)
	return canvas, nil
}

// santaHat is the web's santa-hat.tsx pixel art: R is red, W is white, one character per pixel.
var santaHat = []string{
	"........WW",
	"......RRWW",
	"....RRRRR.",
	"..RRRRRRR.",
	".RRRRRRRR.",
	"WWWWWWWWWW",
	"WWWWWWWWWW",
}

// withSantaHat draws the hat over the top right corner of the logo's // icon, as app-logo.tsx does at Christmas.
func withSantaHat(data []byte) ([]byte, error) {
	source, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	canvas := image.NewNRGBA(source.Bounds())
	draw.Draw(canvas, canvas.Bounds(), source, source.Bounds().Min, draw.Src)
	// The icon spans about 220 of the logo's 256 pixel height, so a 12 pixel cell gives the web's 0.85em hat.
	const cell, left, top = 12, 140, 0
	colors := map[byte]color.NRGBA{'R': {0xd6, 0x30, 0x31, 0xff}, 'W': {0xf8, 0xf9, 0xfa, 0xff}}
	for y, row := range santaHat {
		for x := range len(row) {
			if c, ok := colors[row[x]]; ok {
				draw.Draw(canvas, image.Rect(left+x*cell, top+y*cell, left+(x+1)*cell, top+(y+1)*cell), image.NewUniform(c), image.Point{}, draw.Src)
			}
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, canvas); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
