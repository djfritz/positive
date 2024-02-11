// Copyright 2023 David Fritz
//
// This software may be modified and distributed under the terms of the
// BSD 2-clause license. See the LICENSE file for details.

package main

import (
	"flag"
	"log"
	"fmt"
	"image"
	"image/color"
	_ "image/png"
	"os"
)

const BLACK_POINT uint32 = 32768

var (
	fBW = flag.Bool("bw", false, "set black and white mode (single curve)")
)

func main() {
	flag.Parse()

	input := flag.Arg(0)
	if input == "" {
		fmt.Println("usage: gamma <input file>")
	}

	f, err := os.Open(input)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	m, _, err := image.Decode(f)
	if err != nil {
		log.Fatal(err)
	}

	bounds := m.Bounds()

	if bounds.Max.X != bounds.Max.Y {
		log.Fatal("Input file is not a square!", bounds.Max.X, bounds.Max.Y)
	}

	var red, green, blue []int

OUTER:
	for x := bounds.Min.X; x < bounds.Max.X; x++ {
		// start from the max Y and walk upwards
		// on these curves, it's going to encounter red, green, then
		// blue, in that order.
		y := bounds.Max.Y - 1

		for {
			if y == 0 {
				break
			}
			if black(m.At(x, y)) {
				break
			}
			y--
		}

		// red
		red = append(red, bounds.Max.Y-y)

		// get back to white
		for {
			if !black(m.At(x, y)) {
				break
			}
			y--
		}

		for {
			if y == 0 {
				break
			}
			if black(m.At(x, y)) {
				break
			}
			y--
		}

		if *fBW {
			continue
		}

		// green
		green = append(green, bounds.Max.Y-y)

		// get back to white
		for {
			if !black(m.At(x, y)) {
				break
			}
			y--
		}


		for {
			if y == 0 {
				// we've run out of data -- likely just the rightmost edge of the red curve
				break OUTER
			}
			if black(m.At(x, y)) {
				break
			}
			y--
		}

		// blue
		blue = append(blue, bounds.Max.Y-y)
	}

	if *fBW {
		green = red
		blue = red
	}

	// calculate the slope
	rgamma := slope(red)
	ggamma := slope(green)
	bgamma := slope(blue)

	fmt.Printf("r: %v,\ng: %v,\nb: %v,\n", rgamma, ggamma, bgamma)
}

func black(c color.Color) bool {
	r, b, g, _ := c.RGBA()

	if r < BLACK_POINT && b < BLACK_POINT && g < BLACK_POINT {
		return true
	}
	return false
}

// Calculate the slope of evently distributed points y using linear regression.
func slope(y []int) float64 {
	var meanx, meany float64
	for i, v := range y {
		meanx += float64(i)
		meany += float64(v)
	}
	meanx /= float64(len(y))
	meany /= float64(len(y))

	var n, d float64
	for i, v := range y {
		n += (float64(i) - meanx) * (float64(v) - meany)
		d += (float64(i) - meanx) * (float64(i) - meanx)
	}

	return n / d
}
