package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"image/color"
	"io"
	"math"
	"strings"

	staticMap "github.com/Luzifer/go-staticmaps"
	"github.com/fogleman/gg"
	"github.com/golang/geo/s2"
	"github.com/pkg/errors"
	"golang.org/x/image/font/basicfont"
)

//nolint:mnd // these are the "constant" definitions
var markerColors = map[string]color.Color{
	"black":  color.RGBA{R: 145, G: 145, B: 145, A: 0xff},
	"brown":  color.RGBA{R: 178, G: 154, B: 123, A: 0xff},
	"green":  color.RGBA{R: 168, G: 196, B: 68, A: 0xff},
	"purple": color.RGBA{R: 177, G: 150, B: 191, A: 0xff},
	"yellow": color.RGBA{R: 237, G: 201, B: 107, A: 0xff},
	"blue":   color.RGBA{R: 163, G: 196, B: 253, A: 0xff},
	"gray":   color.RGBA{R: 204, G: 204, B: 204, A: 0xff},
	"orange": color.RGBA{R: 229, G: 165, B: 68, A: 0xff},
	"red":    color.RGBA{R: 246, G: 118, B: 112, A: 0xff},
	"white":  color.RGBA{R: 245, G: 244, B: 241, A: 0xff},
}

type markerSize float64

//nolint:mnd // these are the "constant" definitions
var markerSizes = map[string]markerSize{
	"tiny":  10,
	"mid":   15,
	"small": 20,
}

type marker struct {
	pos   s2.LatLng
	color color.Color
	size  markerSize
	label string
}

func (m marker) String() string {
	r, g, b, a := m.color.RGBA()
	return fmt.Sprintf("%s|%.0f|%d,%d,%d,%d|%s", m.pos.String(), m.size, r, g, b, a, m.label)
}

type mapPath struct {
	positions []s2.LatLng
	color     color.Color
}

type generateMapConfig struct {
	Center             s2.LatLng
	Zoom               int
	Markers            []marker
	Paths              []mapPath
	Width              int
	Height             int
	DisableAttribution bool
	Overlays           []*staticMap.TileProvider
}

func (g generateMapConfig) getCacheKey() string {
	markerString := []string{}
	for _, m := range g.Markers {
		markerString = append(markerString, m.String())
	}
	pathString := []string{}
	for _, path := range g.Paths {
		parts := []string{}
		for _, position := range path.positions {
			parts = append(parts, position.String())
		}
		r, green, b, a := path.color.RGBA()
		pathString = append(pathString, fmt.Sprintf("%d,%d,%d,%d:%s", r, green, b, a, strings.Join(parts, ",")))
	}

	overlayString := []string{}
	for _, o := range g.Overlays {
		overlayString = append(overlayString, o.URLPattern)
	}

	hashString := fmt.Sprintf("%s:::%s|%d|%s|%s|%dx%d|%v|%s",
		version,
		g.Center.String(),
		g.Zoom,
		strings.Join(markerString, "+"),
		strings.Join(pathString, "+"),
		g.Width,
		g.Height,
		g.DisableAttribution,
		fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(overlayString, "::")))),
	)

	return fmt.Sprintf("%x", sha256.Sum256([]byte(hashString)))
}

func generateMap(opts generateMapConfig) (io.Reader, error) {
	ctx := staticMap.NewContext()
	ctx.SetUserAgent(fmt.Sprintf("Mozilla/5.0+(compatible; staticmap/%s; https://github.com/Luzifer/staticmap)", version))

	ctx.SetSize(opts.Width, opts.Height)
	ctx.SetCenter(opts.Center)
	ctx.SetZoom(opts.Zoom)

	if opts.DisableAttribution {
		ctx.OverrideAttribution("")
	}

	if opts.Markers != nil {
		for _, m := range opts.Markers {
			ctx.AddObject(staticMap.NewMarker(m.pos, m.color, float64(m.size)))
		}
	}

	if opts.Overlays != nil {
		for _, o := range opts.Overlays {
			ctx.AddOverlay(o)
		}
	}

	img, err := ctx.Render()
	if err != nil {
		return nil, errors.Wrap(err, "rendering context")
	}

	pngCtx := gg.NewContextForImage(img)
	for _, path := range opts.Paths {
		if len(path.positions) < 2 {
			continue
		}
		pngCtx.SetColor(path.color)
		pngCtx.SetLineWidth(3)
		for i := 1; i < len(path.positions); i++ {
			x1, y1 := coordinateToPixel(path.positions[i-1], opts)
			x2, y2 := coordinateToPixel(path.positions[i], opts)
			pngCtx.DrawLine(x1, y1, x2, y2)
			pngCtx.Stroke()
		}
	}
	pngCtx.SetFontFace(basicfont.Face7x13)
	for _, marker := range opts.Markers {
		if marker.label == "" {
			continue
		}
		x, y := coordinateToPixel(marker.pos, opts)
		pngCtx.SetColor(color.Black)
		pngCtx.DrawString(marker.label, x+8, y-8)
	}
	pngBuf := new(bytes.Buffer)
	return pngBuf, errors.Wrap(pngCtx.EncodePNG(pngBuf), "encoding to PNG")
}

func coordinateToPixel(position s2.LatLng, opts generateMapConfig) (float64, float64) {
	const tileSize = 256.0
	worldSize := tileSize * math.Pow(2, float64(opts.Zoom))
	project := func(ll s2.LatLng) (float64, float64) {
		lat := math.Max(-85.05112878, math.Min(85.05112878, ll.Lat.Degrees()))
		return (ll.Lng.Degrees() + 180) / 360 * worldSize,
			(1 - math.Log(math.Tan(lat*math.Pi/180)+1/math.Cos(lat*math.Pi/180))/math.Pi) / 2 * worldSize
	}
	x, y := project(position)
	cx, cy := project(opts.Center)
	dx := x - cx
	if dx > worldSize/2 {
		dx -= worldSize
	} else if dx < -worldSize/2 {
		dx += worldSize
	}
	return float64(opts.Width)/2 + dx, float64(opts.Height)/2 + y - cy
}
