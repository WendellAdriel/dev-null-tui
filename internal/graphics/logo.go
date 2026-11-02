package graphics

import (
	_ "embed"
	"image"
	"image/draw"
)

//go:embed dev-null-logo-dark.png
var darkLogo []byte

//go:embed dev-null-logo-light.png
var lightLogo []byte

//go:embed pastel-de-nata.png
var pastel []byte

// embeddedIcon reports whether an icon ships as artwork with its own colors, so it needs no lane color.
func embeddedIcon(kind string) bool {
	return kind == "logo-dark" || kind == "logo-light" || kind == "pastel"
}

func prepareLogo(req Request) (*image.NRGBA, error) {
	data, width, height := darkLogo, req.Cols*16, req.Rows*32
	switch req.Icon {
	case "logo-light":
		data = lightLogo
	case "pastel":
		// Stats icons share the 96px square canvas of the rasterized stars, which stays sharp on dense displays.
		data, width, height = pastel, 96, 96
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
