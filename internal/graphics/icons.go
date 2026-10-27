package graphics

import (
	"image"
	"image/color"
	"math"

	"golang.org/x/image/vector"
)

func validIcon(kind string) bool {
	switch kind {
	case "starred", "followed", "approved", "rejected", "stats-star", "stats-starred":
		return true
	}
	return false
}

func notificationIcon(kind, lane string) *image.NRGBA {
	im := image.NewNRGBA(image.Rect(0, 0, 96, 96))
	ink := parseRing(lane)
	if kind == "stats-star" || kind == "stats-starred" {
		return statsStar(im, ink, kind == "stats-starred")
	}
	for y := range 96 {
		for x := range 96 {
			px, py := float64(x)+0.5, float64(y)+0.5
			coverage := max(0, min(1, 2.5-math.Abs(math.Hypot(px-48, py-48)-44)))
			if kind == "followed" {
				head := 2 - math.Abs(math.Hypot(px-38, py-33)-8)
				shoulders := 2 - math.Abs(math.Hypot(px-38, py-61)-15)
				if py > 61 {
					shoulders = 0
				}
				coverage = max(coverage, min(1, max(head, shoulders)))
			}
			if coverage > 0 {
				pixel := ink
				pixel.A = uint8(255 * coverage)
				im.SetNRGBA(x, y, pixel)
			}
		}
	}
	z := vector.NewRasterizer(96, 96)
	switch kind {
	case "starred":
		for i := range 10 {
			radius := 21.0
			if i%2 == 1 {
				radius = 9
			}
			angle := float64(i)*math.Pi/5 - math.Pi/2
			x, y := float32(48+radius*math.Cos(angle)), float32(48+radius*math.Sin(angle))
			if i == 0 {
				z.MoveTo(x, y)
			} else {
				z.LineTo(x, y)
			}
		}
		z.ClosePath()
	case "followed":
		iconStroke(z, 66, 38, 66, 54, 3)
		iconStroke(z, 58, 46, 74, 46, 3)
		iconStroke(z, 23, 60, 23, 66, 3)
		iconStroke(z, 53, 60, 53, 66, 3)
	case "approved":
		iconStroke(z, 29, 48, 42, 61, 5)
		iconStroke(z, 42, 61, 68, 34, 5)
	case "rejected":
		iconStroke(z, 32, 32, 64, 64, 5)
		iconStroke(z, 32, 64, 64, 32, 5)
	}
	z.Draw(im, im.Bounds(), image.NewUniform(ink), image.Point{})
	return im
}

func iconStroke(z *vector.Rasterizer, x1, y1, x2, y2, width float64) {
	length := math.Hypot(x2-x1, y2-y1)
	dx, dy := (y2-y1)*width/(2*length), (x1-x2)*width/(2*length)
	z.MoveTo(float32(x1+dx), float32(y1+dy))
	z.LineTo(float32(x2+dx), float32(y2+dy))
	z.LineTo(float32(x2-dx), float32(y2-dy))
	z.LineTo(float32(x1-dx), float32(y1-dy))
	z.ClosePath()
}

// statsStar fills the cell with a star, hollowed by a reverse-wound inner star when unfilled.
func statsStar(im *image.NRGBA, ink color.NRGBA, filled bool) *image.NRGBA {
	z := vector.NewRasterizer(96, 96)
	starPath(z, 46, false)
	if !filled {
		starPath(z, 46*0.6, true)
	}
	z.Draw(im, im.Bounds(), image.NewUniform(ink), image.Point{})
	return im
}

func starPath(z *vector.Rasterizer, radius float64, reverse bool) {
	for i := range 10 {
		step := i
		if reverse {
			step = 10 - i
		}
		r := radius
		if step%2 == 1 {
			r = radius * 0.42
		}
		angle := float64(step)*math.Pi/5 - math.Pi/2
		x, y := float32(48+r*math.Cos(angle)), float32(52+r*math.Sin(angle))
		if i == 0 {
			z.MoveTo(x, y)
		} else {
			z.LineTo(x, y)
		}
	}
	z.ClosePath()
}
