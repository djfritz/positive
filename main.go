// Copyright 2023 David Fritz
//
// This software may be modified and distributed under the terms of the
// BSD 2-clause license. See the LICENSE file for details.

package main

import (
	"flag"
	"golang.org/x/image/tiff"
	"image"
	"image/color"
	"log"
	"math"
	"os"
)

type gamma struct {
	r float64
	g float64
	b float64
}

// Gamma correction map. Values are generated by the included gamma tool.
var gammaMap = map[string]gamma{
	"none": {
		r: 1.0,
		g: 1.0,
		b: 1.0,
	},
	"ektar100": {
		r: 0.5733379896124348,
		g: 0.5737822736392102,
		b: 0.6624829032379945,
	},
	"portra800": {
		r: 0.5228012326204643,
		g: 0.536735995403697,
		b: 0.6114420242779521,
	},
}

var (
	fInvert    = flag.Bool("invert", true, "Invert the image before setting levels")
	fGamma     = flag.String("gamma", "", "Apply the given gamma profile")
	fNormalize = flag.Bool("normalize", true, "Normalize the image by channel")
	fBorder    = flag.Int("border", 10, "Percentage border to ignore when calculating normalization")
	fBase      = flag.String("base", "", "Path to mask film sample for mask correction")
	fUpper     = flag.Int("tupper", 10, "Pixel count upper threshold for normalization")
	fLower     = flag.Int("tlower", 10, "Pixel count lower threshold for normalization")
)

func main() {
	flag.Parse()

	if _, ok := gammaMap[*fGamma]; !ok {
		log.Println("must specify gamma profile. Options are:")
		for k, _ := range gammaMap {
			log.Println(k)
		}
		return
	}

	// open the image
	input := flag.Arg(0)
	output := flag.Arg(1)
	f, err := os.Open(input)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	m, err := tiff.Decode(f)
	if err != nil {
		log.Fatal(err)
	}

	// remove film mask
	if *fBase == "" {
		log.Println("not removing film mask!")
	} else {
		s, err := sample(*fBase)
		if err != nil {
			log.Fatal(err)
		}
		m = removeCast(m, s)
	}

	// apply γ
	g := gammaMap[*fGamma]
	m = applyGamma(m, 1/g.r, 1/g.g, 1/g.b)

	// normalize levels
	if *fNormalize {
		m = normalize(m, *fUpper, *fLower)
	}

	// invert
	if *fInvert {
		m = invert(m)
	}

	// output
	fout, err := os.Create(output)
	if err != nil {
		log.Fatal(err)
	}

	defer fout.Close()
	tiff.Encode(fout, m, nil)
}

// applies a 0,1 bound gamma correction
func applyGamma(m image.Image, rg, gg, bg float64) image.Image {
	ret := image.NewRGBA64(image.Rect(0, 0, m.Bounds().Max.X, m.Bounds().Max.Y))
	for x := 0; x < m.Bounds().Max.X; x++ {
		for y := 0; y < m.Bounds().Max.Y; y++ {
			r, g, b, _ := m.At(x, y).RGBA()
			r = uint32(math.Pow(float64(r)/float64(65535), rg) * 65535)
			g = uint32(math.Pow(float64(g)/float64(65535), gg) * 65535)
			b = uint32(math.Pow(float64(b)/float64(65535), bg) * 65535)
			ret.Set(x, y, color.RGBA64{R: uint16(r), G: uint16(g), B: uint16(b), A: 0xffff})
		}
	}
	return ret
}

// simple image invert
func invert(m image.Image) image.Image {
	ret := image.NewRGBA64(image.Rect(0, 0, m.Bounds().Max.X, m.Bounds().Max.Y))
	for x := 0; x < m.Bounds().Max.X; x++ {
		for y := 0; y < m.Bounds().Max.Y; y++ {
			r, g, b, _ := m.At(x, y).RGBA()
			r = uint32(0xffff) - r
			g = uint32(0xffff) - g
			b = uint32(0xffff) - b
			ret.Set(x, y, color.RGBA64{R: uint16(r), G: uint16(g), B: uint16(b), A: 0xffff})
		}
	}
	return ret
}

// Level normalization. This is done by evaluating a rectangle -border
// percentage smaller than the source image (to account for film edges if
// present). Per channel min/max values are determined and then the entire
// output channel color space is scaled. -tupper and -tlower can be used to
// provide some amount of hysteresis, which allows for overcoming light/dark
// spots of dust, etc.
func normalize(m image.Image, tUpper, tLower int) image.Image {
	// sample from the given border percentage by creating a subimage
	upper := (100.0 - float64(*fBorder)) / 100.0
	lower := float64(*fBorder) / 100.0

	interior := image.Rect(
		int(float64(m.Bounds().Max.X)*lower),
		int(float64(m.Bounds().Max.Y)*lower),
		int(float64(m.Bounds().Max.X)*upper),
		int(float64(m.Bounds().Max.Y)*upper))

	// find the min and max of each channel
	rh := make(map[uint32]int)
	gh := make(map[uint32]int)
	bh := make(map[uint32]int)
	for x := interior.Min.X; x < interior.Max.X; x++ {
		for y := interior.Min.Y; y < interior.Bounds().Max.Y; y++ {
			r, g, b, _ := m.At(x, y).RGBA()
			rh[r]++
			gh[g]++
			bh[b]++
		}
	}

	rmin := uint32(0xffff)
	gmin := uint32(0xffff)
	bmin := uint32(0xffff)
	rmax := uint32(0)
	gmax := uint32(0)
	bmax := uint32(0)
	for i := uint32(0); i < 0xffff; i++ {
		if rmin == 0xffff && rh[i] > tLower {
			rmin = i
		}
		if gmin == 0xffff && gh[i] > tLower {
			gmin = i
		}
		if bmin == 0xffff && bh[i] > tLower {
			bmin = i
		}
		if rmin != 0xffff && gmin != 0xffff && bmin != 0xffff {
			break
		}
	}
	for i := uint32(0xffff) - 1; i > 0; i-- {
		if rmax == 0 && rh[i] > tUpper {
			rmax = i
		}
		if gmax == 0 && gh[i] > tUpper {
			gmax = i
		}
		if bmax == 0 && bh[i] > tUpper {
			bmax = i
		}
		if rmax != 0 && gmax != 0 && bmax != 0 {
			break
		}
	}

	rw := 0xffff / float64(rmax-rmin)
	gw := 0xffff / float64(gmax-gmin)
	bw := 0xffff / float64(bmax-bmin)

	// walk each pixel again and normalize
	ret := image.NewRGBA64(image.Rect(0, 0, m.Bounds().Max.X, m.Bounds().Max.Y))
	for x := 0; x < m.Bounds().Max.X; x++ {
		for y := 0; y < m.Bounds().Max.Y; y++ {
			r, g, b, _ := m.At(x, y).RGBA()

			rmod := (float64(r) - float64(rmin)) * rw
			gmod := (float64(g) - float64(gmin)) * gw
			bmod := (float64(b) - float64(bmin)) * bw

			if rmod < 0 {
				r = 0
			} else if rmod > 0xffff {
				r = 0xffff
			} else {
				r = uint32(rmod)
			}

			if gmod < 0 {
				g = 0
			} else if gmod > 0xffff {
				g = 0xffff
			} else {
				g = uint32(gmod)
			}

			if bmod < 0 {
				b = 0
			} else if bmod > 0xffff {
				b = 0xffff
			} else {
				b = uint32(bmod)
			}
			ret.Set(x, y, color.RGBA64{R: uint16(r), G: uint16(g), B: uint16(b), A: 0xffff})
		}
	}
	return ret
}

// Calculates the average r,g,b colors of the given image
func sample(sample string) (color.Color, error) {
	f, err := os.Open(sample)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	m, err := tiff.Decode(f)
	if err != nil {
		return nil, err
	}

	var r, g, b uint64

	for x := 0; x < m.Bounds().Max.X; x++ {
		for y := 0; y < m.Bounds().Max.Y; y++ {
			c := m.At(x, y)
			dr, dg, db, _ := c.RGBA()
			r += uint64(dr)
			g += uint64(dg)
			b += uint64(db)
		}
	}
	size := uint64(m.Bounds().Max.X * m.Bounds().Max.Y)
	return color.RGBA64{R: uint16(r / size), G: uint16(g / size), B: uint16(b / size), A: 0xffff}, nil
}

// Removes (in negative color space, so adds the inverted sample) the color
// cast determined by the provided mask sample.
func removeCast(m image.Image, s color.Color) image.Image {
	r, g, b, _ := s.RGBA()

	r = uint32(0xffff - uint16(r))
	g = uint32(0xffff - uint16(g))
	b = uint32(0xffff - uint16(b))

	ret := image.NewRGBA64(image.Rect(0, 0, m.Bounds().Max.X, m.Bounds().Max.Y))
	for x := 0; x < m.Bounds().Max.X; x++ {
		for y := 0; y < m.Bounds().Max.Y; y++ {
			c := m.At(x, y)
			dr, dg, db, _ := c.RGBA()

			nr := dr + r
			ng := dg + g
			nb := db + b
			if nr > 0x0000ffff {
				nr = 0xffff
			}
			if ng > 0x0000ffff {
				ng = 0xffff
			}
			if nb > 0x0000ffff {
				nb = 0xffff
			}
			ret.Set(x, y, color.RGBA64{R: uint16(nr), G: uint16(ng), B: uint16(nb), A: 0xffff})
		}
	}
	return ret
}
