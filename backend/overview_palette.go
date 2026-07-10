package main

import "math"

const overviewColorBase = 20

var overviewColorLevels = [...]uint8{0, 96, 160, 208, 232, 255}

var overviewPaletteRGB = func() [256][3]uint8 {
	palette := [256][3]uint8{
		0:  {0xc0, 0xc0, 0xc0},
		1:  {0x20, 0x20, 0x20},
		2:  {0xe0, 0xe0, 0xe0},
		3:  {0xe9, 0xec, 0xff},
		4:  {0xe9, 0xff, 0xea},
		5:  {0xff, 0xe9, 0xea},
		6:  {0xec, 0xec, 0xff},
		7:  {0xff, 0xf0, 0xea},
		8:  {0xd4, 0xff, 0xf2},
		9:  {0xf0, 0xe4, 0xff},
		10: {0xdc, 0xdc, 0xdc},
		11: {0xe3, 0x1c, 0x1c},
		12: {0x22, 0xc5, 0x5e},
		13: {0x21, 0x66, 0xf3},
		14: {0xf5, 0x9e, 0x0b},
		15: {0xff, 0x6b, 0x2c},
		16: {0xa8, 0x55, 0xf7},
		17: {0x06, 0xb6, 0xd4},
		18: {0xff, 0x69, 0xb4},
		19: {0x00, 0x00, 0x00},
	}
	for r, red := range overviewColorLevels {
		for g, green := range overviewColorLevels {
			for b, blue := range overviewColorLevels {
				palette[overviewColorBase+r*36+g*6+b] = [3]uint8{red, green, blue}
			}
		}
	}
	return palette
}()

var overviewSRGBToLinear = func() [256]uint32 {
	var table [256]uint32
	for i := range table {
		value := float64(i) / 255
		if value <= 0.04045 {
			value /= 12.92
		} else {
			value = math.Pow((value+0.055)/1.055, 2.4)
		}
		table[i] = uint32(math.Round(value * 4095))
	}
	return table
}()

var overviewLinearToSRGB = func() [4096]uint8 {
	var table [4096]uint8
	for i := range table {
		value := float64(i) / 4095
		if value <= 0.0031308 {
			value *= 12.92
		} else {
			value = 1.055*math.Pow(value, 1/2.4) - 0.055
		}
		table[i] = uint8(math.Round(value * 255))
	}
	return table
}()

func nearestOverviewLevel(value uint8) int {
	best := 0
	bestDistance := 256
	for i, level := range overviewColorLevels {
		distance := int(value) - int(level)
		if distance < 0 {
			distance = -distance
		}
		if distance < bestDistance {
			best = i
			bestDistance = distance
		}
	}
	return best
}

func quantizeOverviewColor(red, green, blue uint8) byte {
	r := nearestOverviewLevel(red)
	g := nearestOverviewLevel(green)
	b := nearestOverviewLevel(blue)
	return byte(overviewColorBase + r*36 + g*6 + b)
}

func downsampleOverviewColor(full []byte, lod int) []byte {
	block := ChunkSize / lod
	output := make([]byte, lod*lod)
	for y := 0; y < lod; y++ {
		for x := 0; x < lod; x++ {
			first := full[(y*block)*ChunkSize+x*block]
			uniform := true
			var red, green, blue uint32
			for dy := 0; dy < block; dy++ {
				base := (y*block+dy)*ChunkSize + x*block
				for dx := 0; dx < block; dx++ {
					index := full[base+dx]
					uniform = uniform && index == first
					color := overviewPaletteRGB[index]
					red += overviewSRGBToLinear[color[0]]
					green += overviewSRGBToLinear[color[1]]
					blue += overviewSRGBToLinear[color[2]]
				}
			}
			if uniform {
				output[y*lod+x] = first
				continue
			}
			count := uint32(block * block)
			output[y*lod+x] = quantizeOverviewColor(
				overviewLinearToSRGB[red/count],
				overviewLinearToSRGB[green/count],
				overviewLinearToSRGB[blue/count],
			)
		}
	}
	return output
}
