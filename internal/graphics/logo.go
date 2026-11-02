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

func prepareLogo(req Request) (*image.NRGBA, error) {
	data := darkLogo
	if req.Icon == "logo-light" {
		data = lightLogo
	}
	logo, err := prepare(data, Request{Cols: req.Cols, Rows: req.Rows})
	if err != nil {
		return nil, err
	}
	canvas := image.NewNRGBA(image.Rect(0, 0, req.Cols*16, req.Rows*32))
	x := (canvas.Bounds().Dx() - logo.Bounds().Dx()) / 2
	y := (canvas.Bounds().Dy() - logo.Bounds().Dy()) / 2
	draw.Draw(canvas, logo.Bounds().Add(image.Pt(x, y)), logo, image.Point{}, draw.Src)
	return canvas, nil
}
