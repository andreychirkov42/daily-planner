// genicon рисует иконку приложения: assets/icon.ico (трей и exe), assets/icon.png, web/icon.png.
//
//	go run ./tools/genicon                 иконка по умолчанию (кристалл)
//	go run ./tools/genicon -variant=page   другой вариант
//	go run ./tools/genicon -preview=file   лист со всеми вариантами для сравнения
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
)

const ss = 4 // сглаживание: рисуем в 4 раза крупнее и усредняем

type pt struct{ x, y float64 }

// variants — варианты иконки; у каждого своя функция цвета пикселя в координатах 0..1
var variants = map[string]func(x, y, px float64) color.NRGBA{
	"star":     star,     // гранёная четырёхлучевая звезда-ориентир
	"bookmark": bookmark, // лента-закладка ежедневника
	"sunrise":  sunrise,  // солнце над горизонтом — план на день
	"layers":   layers,   // стопка карточек-задач
	"crystal":  crystal,  // гранёная галочка (прежний вариант)
	"page":     page,
	"glow":     glow,
}
var order = []string{"star", "bookmark", "sunrise", "layers"}

func main() {
	variant := flag.String("variant", "star", "вариант: star, bookmark, sunrise, layers, crystal, page, glow")
	preview := flag.String("preview", "", "сохранить лист сравнения вариантов в этот PNG")
	flag.Parse()

	if *preview != "" {
		must(writePNG(*preview, sheet()))
		log.Println("лист вариантов:", *preview)
		return
	}
	fn, ok := variants[*variant]
	if !ok {
		log.Fatalf("нет варианта %q", *variant)
	}
	sizes := []int{16, 20, 24, 32, 40, 48, 64, 128, 256}
	var pngs [][]byte
	for _, s := range sizes {
		var buf bytes.Buffer
		must(png.Encode(&buf, render(s, fn)))
		pngs = append(pngs, buf.Bytes())
	}
	must(os.MkdirAll("assets", 0o755))
	must(os.WriteFile(filepath.Join("assets", "icon.png"), pngs[len(pngs)-1], 0o644))
	must(os.WriteFile(filepath.Join("web", "icon.png"), pngs[len(pngs)-2], 0o644)) // 128px для страницы
	must(os.WriteFile(filepath.Join("assets", "icon.ico"), ico(sizes, pngs), 0o644))
	log.Printf("готово (%s): assets/icon.ico, assets/icon.png, web/icon.png", *variant)
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func writePNG(path string, img image.Image) error {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// sheet — все варианты в размерах 256, 48, 32, 16 на светлом и тёмном фоне
func sheet() image.Image {
	sizes := []int{256, 48, 32, 16}
	w, h := 40+len(sizes)*0+256+40+48+24+32+24+16+40, 2*(256+40)+20
	img := image.NewNRGBA(image.Rect(0, 0, w*len(order), h))
	for vi, name := range order {
		for row, bg := range []color.NRGBA{{0xf2, 0xf5, 0xfa, 255}, {0x0d, 0x12, 0x1b, 255}} {
			oy := row * (256 + 40)
			draw.Draw(img, image.Rect(vi*w, oy, (vi+1)*w, oy+256+40), &image.Uniform{bg}, image.Point{}, draw.Src)
			x := vi*w + 20
			for _, s := range sizes {
				ic := render(s, variants[name])
				y := oy + 20 + (256-s)/2
				draw.Draw(img, image.Rect(x, y, x+s, y+s), ic, image.Point{}, draw.Over)
				x += s + 24
			}
		}
	}
	return img
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

// render рисует вариант в размере size со сглаживанием.
func render(size int, fn func(x, y, px float64) color.NRGBA) image.Image {
	n := size * ss
	big := image.NewNRGBA(image.Rect(0, 0, n, n))
	px := 1 / float64(n) // размер пикселя в координатах 0..1
	for yy := 0; yy < n; yy++ {
		for xx := 0; xx < n; xx++ {
			big.SetNRGBA(xx, yy, fn((float64(xx)+0.5)/float64(n), (float64(yy)+0.5)/float64(n), px*float64(ss)))
		}
	}
	return downsample(big, size)
}

// ---------- геометрия ----------

func inRoundRect(x, y, x0, y0, x1, y1, r float64) bool {
	cx := math.Max(x0+r, math.Min(x1-r, x))
	cy := math.Max(y0+r, math.Min(y1-r, y))
	return (x-cx)*(x-cx)+(y-cy)*(y-cy) <= r*r
}

func inPoly(x, y float64, p []pt) bool {
	in := false
	for i, j := 0, len(p)-1; i < len(p); j, i = i, i+1 {
		if (p[i].y > y) != (p[j].y > y) && x < (p[j].x-p[i].x)*(y-p[i].y)/(p[j].y-p[i].y)+p[i].x {
			in = !in
		}
	}
	return in
}

func shift(p []pt, dx, dy float64) []pt {
	out := make([]pt, len(p))
	for i, q := range p {
		out[i] = pt{q.x + dx, q.y + dy}
	}
	return out
}

func lerp(a, b color.NRGBA, t float64) color.NRGBA {
	t = math.Max(0, math.Min(1, t))
	f := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t) }
	return color.NRGBA{f(a.R, b.R), f(a.G, b.G), f(a.B, b.B), f(a.A, b.A)}
}

// over накладывает цвет c с прозрачностью a поверх base
func over(base, c color.NRGBA, a float64) color.NRGBA {
	c.A = base.A
	return lerp(base, c, a)
}

func rgb(hex uint32) color.NRGBA {
	return color.NRGBA{uint8(hex >> 16), uint8(hex >> 8), uint8(hex), 255}
}

var transparent = color.NRGBA{}

// ---------- гранёная галочка ----------

// Контур галочки с острыми концами и линией «ребра» посередине:
// верхняя грань светлее, нижняя темнее — получается кристалл.
func checkFacets(scale, ox, oy float64) (upper, lower []pt) {
	m := func(x, y float64) pt { return pt{ox + (x-0.5)*scale + 0.5, oy + (y-0.5)*scale + 0.5} }
	a, b, c := m(0.205, 0.555), m(0.425, 0.775), m(0.835, 0.335)
	d, e, f := m(0.745, 0.245), m(0.425, 0.6), m(0.295, 0.468)
	m1, jc, m2 := m(0.25, 0.5115), m(0.425, 0.6875), m(0.79, 0.29)
	return []pt{m1, f, e, d, m2, jc}, []pt{m1, jc, m2, c, b, a}
}

func drawCheck(base color.NRGBA, x, y, scale, ox, oy float64, light, dark [2]color.NRGBA) (color.NRGBA, bool) {
	up, lo := checkFacets(scale, ox, oy)
	t := (x + y) / 2 // диагональный градиент вдоль галочки
	var c color.NRGBA
	switch {
	case inPoly(x, y, up):
		c = lerp(light[0], light[1], (t-0.3)/0.4)
	case inPoly(x, y, lo):
		c = lerp(dark[0], dark[1], (t-0.3)/0.4)
	default:
		return base, false
	}
	// блик вдоль ребра между гранями
	ridge := math.Min(segDist(x, y, up[0], up[5]), segDist(x, y, up[5], up[4]))
	if w := 0.007 * scale; ridge < w {
		c = lerp(c, rgb(0xeaf6ff), 0.55*(1-ridge/w))
	}
	return c, true
}

func segDist(px, py float64, a, b pt) float64 {
	dx, dy := b.x-a.x, b.y-a.y
	t := math.Max(0, math.Min(1, ((px-a.x)*dx+(py-a.y)*dy)/(dx*dx+dy*dy)))
	return math.Hypot(px-(a.x+t*dx), py-(a.y+t*dy))
}

func shadowOfCheck(base color.NRGBA, x, y, scale, ox, oy, a float64) color.NRGBA {
	up, lo := checkFacets(scale, ox, oy)
	for i, k := range []float64{0.018, 0.03, 0.045} {
		if inPoly(x, y, shift(up, 0, k)) || inPoly(x, y, shift(lo, 0, k)) {
			return over(base, rgb(0x02040a), a*[]float64{1, 0.6, 0.3}[i])
		}
	}
	return base
}

// тёмно-синяя плитка с мягким свечением и светлой кромкой сверху
func darkTile(x, y, px float64) (color.NRGBA, bool) {
	const r = 0.225
	if !inRoundRect(x, y, 0.02, 0.02, 0.98, 0.98, r) {
		return transparent, false
	}
	c := lerp(rgb(0x18243f), rgb(0x0a0f1c), y)
	// свечение за галочкой
	d := math.Hypot(x-0.55, y-0.52)
	c = over(c, rgb(0x2f6bff), 0.38*math.Max(0, 1-d/0.5))
	// тонкая светлая кромка
	if !inRoundRect(x, y, 0.02+px*1.5, 0.02+px*1.5, 0.98-px*1.5, 0.98-px*1.5, r-px*1.5) {
		c = over(c, rgb(0x9fb8ff), 0.18+0.3*(1-y))
	}
	return c, true
}

var (
	lightFacet = [2]color.NRGBA{rgb(0x9fe1ff), rgb(0x5b9bff)}
	darkFacet  = [2]color.NRGBA{rgb(0x3b7bff), rgb(0x1d3fbf)}
)

func crystal(x, y, px float64) color.NRGBA {
	c, ok := darkTile(x, y, px)
	if !ok {
		return c
	}
	c = shadowOfCheck(c, x, y, 1, 0, 0.01, 0.55)
	if cc, hit := drawCheck(c, x, y, 1, 0, 0, lightFacet, darkFacet); hit {
		return cc
	}
	return c
}

func page(x, y, px float64) color.NRGBA {
	c, ok := darkTile(x, y, px)
	if !ok {
		return c
	}
	// листок ежедневника с колечками и линейками
	if inRoundRect(x, y, 0.25, 0.2, 0.75, 0.8, 0.06) {
		c = lerp(rgb(0xf4f7ff), rgb(0xd9e3f7), y)
		for _, ly := range []float64{0.36, 0.48, 0.6} {
			if math.Abs(y-ly) < 0.012 && x > 0.33 && x < 0.67 {
				c = rgb(0xb9c7e3)
			}
		}
	}
	for _, ry := range []float64{0.3, 0.45, 0.6} {
		if math.Hypot(x-0.25, y-ry) < 0.035 {
			c = rgb(0x0f1729)
		}
	}
	c = shadowOfCheck(c, x, y, 0.72, 0.14, 0.17, 0.6)
	if cc, hit := drawCheck(c, x, y, 0.72, 0.14, 0.16, lightFacet, darkFacet); hit {
		return cc
	}
	return c
}

func glow(x, y, px float64) color.NRGBA {
	const r = 0.225
	if !inRoundRect(x, y, 0.02, 0.02, 0.98, 0.98, r) {
		return transparent
	}
	c := lerp(rgb(0x3d8bff), rgb(0x2337c9), (x+y)/2)
	c = over(c, rgb(0xffffff), 0.16*math.Max(0, 1-math.Hypot(x-0.25, y-0.15)/0.6))
	c = shadowOfCheck(c, x, y, 1, 0, 0.01, 0.35)
	white := [2]color.NRGBA{rgb(0xffffff), rgb(0xeaf2ff)}
	ice := [2]color.NRGBA{rgb(0xc7dcff), rgb(0x9db9f5)}
	if cc, hit := drawCheck(c, x, y, 1, 0, 0, white, ice); hit {
		return cc
	}
	return c
}

// ---------- новые символы ----------

// facet красит треугольник: светлая или тёмная грань с диагональным градиентом
func facet(x, y float64, tri []pt, light bool) (color.NRGBA, bool) {
	if !inPoly(x, y, tri) {
		return color.NRGBA{}, false
	}
	t := (x + y - 0.6) / 0.8
	if light {
		return lerp(lightFacet[0], lightFacet[1], t), true
	}
	return lerp(darkFacet[0], darkFacet[1], t), true
}

func withShadow(c color.NRGBA, x, y float64, shapes [][]pt, a float64) color.NRGBA {
	for i, k := range []float64{0.018, 0.03, 0.045} {
		for _, sh := range shapes {
			if inPoly(x, y, shift(sh, 0, k)) {
				return over(c, rgb(0x02040a), a*[]float64{1, 0.6, 0.3}[i])
			}
		}
	}
	return c
}

func star(x, y, px float64) color.NRGBA {
	c, ok := darkTile(x, y, px)
	if !ok {
		return c
	}
	o := pt{0.5, 0.5}
	const arm, waist = 0.34, 0.085
	n, e, s, w := pt{0.5, 0.5 - arm}, pt{0.5 + arm*0.92, 0.5}, pt{0.5, 0.5 + arm}, pt{0.5 - arm*0.92, 0.5}
	ne, se, sw, nw := pt{0.5 + waist, 0.5 - waist}, pt{0.5 + waist, 0.5 + waist}, pt{0.5 - waist, 0.5 + waist}, pt{0.5 - waist, 0.5 - waist}
	tris := [][]pt{{o, n, nw}, {o, n, ne}, {o, e, ne}, {o, e, se}, {o, s, se}, {o, s, sw}, {o, w, sw}, {o, w, nw}}
	c = withShadow(c, x, y, tris, 0.55)
	for i, tr := range tris {
		// у каждого луча одна грань светлая, другая тёмная — как у огранённого камня
		if cc, hit := facet(x, y, tr, i%2 == 0); hit {
			return cc
		}
	}
	return c
}

func bookmark(x, y, px float64) color.NRGBA {
	c, ok := darkTile(x, y, px)
	if !ok {
		return c
	}
	const l, r, top, bot, notch = 0.33, 0.67, 0.18, 0.82, 0.15
	mid := (l + r) / 2
	left := []pt{{l, top}, {mid, top}, {mid, bot - notch}, {l, bot}}
	right := []pt{{mid, top}, {r, top}, {r, bot}, {mid, bot - notch}}
	fold := []pt{{l, top}, {r, top}, {r, top + 0.07}, {l, top + 0.07}}
	c = withShadow(c, x, y, [][]pt{left, right}, 0.55)
	if inPoly(x, y, fold) {
		return lerp(rgb(0xc4ecff), rgb(0x8cc6ff), (x-l)/(r-l))
	}
	if cc, hit := facet(x, y, left, true); hit {
		return cc
	}
	if cc, hit := facet(x, y, right, false); hit {
		return cc
	}
	return c
}

func sunrise(x, y, px float64) color.NRGBA {
	c, ok := darkTile(x, y, px)
	if !ok {
		return c
	}
	cx, hy, r := 0.5, 0.62, 0.2
	sunTop, sunBot := rgb(0xffe08a), rgb(0xff8a3d)
	// лучи
	for i := 0; i < 7; i++ {
		a := math.Pi * (0.08 + 0.84*float64(i)/6)
		dx, dy := -math.Cos(a), -math.Sin(a)
		in, out := r+0.05, r+0.13
		w := 0.022
		p := []pt{
			{cx + dx*in - dy*w, hy + dy*in + dx*w}, {cx + dx*out, hy + dy*out}, {cx + dx*in + dy*w, hy + dy*in - dx*w},
		}
		if inPoly(x, y, p) {
			return lerp(sunTop, sunBot, 0.3)
		}
	}
	if y < hy && math.Hypot(x-cx, y-hy) < r {
		return lerp(sunTop, sunBot, (y-(hy-r))/r)
	}
	// горизонт и отражения
	for i, ly := range []float64{0.66, 0.72, 0.78} {
		half := []float64{0.3, 0.2, 0.11}[i]
		if math.Abs(y-ly) < 0.016 && math.Abs(x-cx) < half {
			return lerp(lightFacet[0], darkFacet[1], float64(i)/2)
		}
	}
	return c
}

func layers(x, y, px float64) color.NRGBA {
	c, ok := darkTile(x, y, px)
	if !ok {
		return c
	}
	rhomb := func(cy float64) []pt {
		return []pt{{0.5, cy - 0.15}, {0.8, cy}, {0.5, cy + 0.15}, {0.2, cy}}
	}
	// снизу вверх: нижние слои темнее, верхний — светлая грань
	shapes := [][]pt{rhomb(0.63), rhomb(0.5), rhomb(0.37)}
	c = withShadow(c, x, y, shapes[:1], 0.5)
	if inPoly(x, y, shapes[2]) {
		// верхняя карточка: левая половина светлее
		if x < 0.5 {
			return lerp(lightFacet[0], lightFacet[1], (y-0.22)/0.3)
		}
		return lerp(rgb(0x7fb6ff), rgb(0x4f86ff), (y-0.22)/0.3)
	}
	// зазор между слоями
	if inPoly(x, y, shift(shapes[2], 0, 0.035)) {
		return c
	}
	if inPoly(x, y, shapes[1]) {
		return lerp(rgb(0x3b7bff), rgb(0x2a5ce0), (x-0.2)/0.6)
	}
	if inPoly(x, y, shift(shapes[1], 0, 0.035)) {
		return c
	}
	if inPoly(x, y, shapes[0]) {
		return lerp(rgb(0x2448c8), rgb(0x1a2f8f), (x-0.2)/0.6)
	}
	return c
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
