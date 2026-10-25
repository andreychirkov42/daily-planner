// genicon рисует иконку приложения: assets/icon.ico (для трея и exe) и assets/icon.png.
//
//	go run ./tools/genicon
package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
)

const ss = 4 // сглаживание: рисуем в 4 раза крупнее и усредняем

func main() {
	sizes := []int{16, 20, 24, 32, 40, 48, 64, 128, 256}
	var pngs [][]byte
	for _, s := range sizes {
		var buf bytes.Buffer
		if err := png.Encode(&buf, render(s)); err != nil {
			log.Fatal(err)
		}
		pngs = append(pngs, buf.Bytes())
	}
	must(os.MkdirAll("assets", 0o755))
	must(os.WriteFile(filepath.Join("assets", "icon.png"), pngs[len(pngs)-1], 0o644))
	must(os.WriteFile(filepath.Join("web", "icon.png"), pngs[len(pngs)-2], 0o644)) // 128px для страницы
	must(os.WriteFile(filepath.Join("assets", "icon.ico"), ico(sizes, pngs), 0o644))
	log.Println("готово: assets/icon.ico, assets/icon.png")
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

// ico собирает ICO-файл из PNG-картинок (поддерживается с Windows Vista).
func ico(sizes []int, pngs [][]byte) []byte {
	var b bytes.Buffer
	le := func(v any) { _ = binary.Write(&b, binary.LittleEndian, v) }
	le(uint16(0))
	le(uint16(1))
	le(uint16(len(sizes)))
	offset := 6 + 16*len(sizes)
	for i, s := range sizes {
		dim := uint8(s)
		if s >= 256 {
			dim = 0
		}
		le(dim)
		le(dim)
		le(uint8(0))
		le(uint8(0))
		le(uint16(1))
		le(uint16(32))
		le(uint32(len(pngs[i])))
		le(uint32(offset))
		offset += len(pngs[i])
	}
	for _, p := range pngs {
		b.Write(p)
	}
	return b.Bytes()
}

func render(size int) image.Image {
	n := size * ss
	big := image.NewNRGBA(image.Rect(0, 0, n, n))
	fn := float64(n)
	radius := fn * 0.24
	// галочка
	pts := [][2]float64{{0.27, 0.53}, {0.43, 0.68}, {0.74, 0.35}}
	stroke := fn * 0.105
	if size <= 20 {
		stroke = fn * 0.14 // на маленьких размерах линия толще, иначе не видно
	}
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			if !inRoundRect(px, py, fn, radius) {
				continue
			}
			// диагональный градиент от голубого к синему
			t := (px + py) / (2 * fn)
			c := lerp(color.NRGBA{0x3d, 0xc8, 0xff, 255}, color.NRGBA{0x24, 0x52, 0xf0, 255}, t)
			// лёгкий блик сверху
			if py < fn*0.5 {
				k := (fn*0.5 - py) / (fn * 0.5) * 0.12
				c = lerp(c, color.NRGBA{255, 255, 255, 255}, k)
			}
			for i := 0; i+1 < len(pts); i++ {
				if segDist(px, py, pts[i][0]*fn, pts[i][1]*fn, pts[i+1][0]*fn, pts[i+1][1]*fn) < stroke/2 {
					c = color.NRGBA{255, 255, 255, 255}
				}
			}
			big.SetNRGBA(x, y, c)
		}
	}
	return downsample(big, size)
}

func inRoundRect(x, y, s, r float64) bool {
	cx := math.Max(r, math.Min(s-r, x))
	cy := math.Max(r, math.Min(s-r, y))
	return (x-cx)*(x-cx)+(y-cy)*(y-cy) <= r*r
}

func segDist(px, py, ax, ay, bx, by float64) float64 {
	dx, dy := bx-ax, by-ay
	t := ((px-ax)*dx + (py-ay)*dy) / (dx*dx + dy*dy)
	t = math.Max(0, math.Min(1, t))
	return math.Hypot(px-(ax+t*dx), py-(ay+t*dy))
}

func lerp(a, b color.NRGBA, t float64) color.NRGBA {
	t = math.Max(0, math.Min(1, t))
	f := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t) }
	return color.NRGBA{f(a.R, b.R), f(a.G, b.G), f(a.B, b.B), 255}
}

// downsample усредняет блоки ss×ss с учётом прозрачности.
func downsample(src *image.NRGBA, size int) image.Image {
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var r, g, b, a float64
			for j := 0; j < ss; j++ {
				for i := 0; i < ss; i++ {
					c := src.NRGBAAt(x*ss+i, y*ss+j)
					al := float64(c.A)
					r += float64(c.R) * al
					g += float64(c.G) * al
					b += float64(c.B) * al
					a += al
				}
			}
			if a > 0 {
				dst.SetNRGBA(x, y, color.NRGBA{uint8(r / a), uint8(g / a), uint8(b / a), uint8(a / ss / ss)})
			}
		}
	}
	return dst
}
