// genicon рисует иконку приложения: assets/icon.ico (трей и exe), assets/icon.png, web/icon.png
// и значки приложения Android (android/app/src/main/res/mipmap-*).
//
//	go run ./tools/genicon                 иконка по умолчанию (часы одной линией на тёмно-фиолетовом)
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
	"line-violet":      lineHourglass(rgb(0x7c5cff), 0.066), // часы одной линией на фиолетовом
	"line-deep":        lineHourglass(rgb(0x5b3fd6), 0.066),
	"line-lilac":       lineHourglass(rgb(0x9a7bff), 0.066),
	"line-violet-bold": lineHourglass(rgb(0x7c5cff), 0.075),
	"hourglass-white":  flatHourglass(flatLight), // плоские песочные часы: белое стекло, оранжевый песок
	"hourglass-blue":   flatHourglass(flatBlue),  // плоские песочные часы: синее стекло
	"monogram":         monogram,                 // буква «Е» из строк списка и тёплая точка «сегодня»
	"stripes":          stripes,                  // ретро-солнце с вырезанными полосами
	"hourglass":        hourglass,                // гранёные песочные часы с тёплым песком
	"gem":              gem,                      // огранённый камень с искрой
	"star":             star,                     // гранёная четырёхлучевая звезда-ориентир
	"bookmark":         bookmark,                 // лента-закладка ежедневника
	"sunrise":          sunrise,                  // солнце над горизонтом — план на день
	"layers":           layers,                   // стопка карточек-задач
	"crystal":          crystal,                  // гранёная галочка (прежний вариант)
	"page":             page,
	"glow":             glow,
}
var order = []string{"line-violet", "line-deep", "line-lilac"}

func main() {
	variant := flag.String("variant", "line-deep", "вариант: line-deep, line-violet, line-lilac, hourglass-white, hourglass-blue, monogram, stripes, hourglass, gem, star, bookmark, sunrise, layers, crystal, page, glow")
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
	// значок приложения Android во всех плотностях экрана
	for dir, s := range map[string]int{"mdpi": 48, "hdpi": 72, "xhdpi": 96, "xxhdpi": 144, "xxxhdpi": 192} {
		res := filepath.Join("android", "app", "src", "main", "res", "mipmap-"+dir)
		must(os.MkdirAll(res, 0o755))
		must(writePNG(filepath.Join(res, "ic_launcher.png"), render(s, fn)))
	}
	log.Printf("готово (%s): assets/icon.ico, assets/icon.png, web/icon.png, android/.../mipmap-*/ic_launcher.png", *variant)
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

// ---------- второй круг: сильный силуэт, объём от света ----------

var (
	warmTop = rgb(0xffd66b)
	warmBot = rgb(0xff6a5a)
)

// capsule — расстояние до отрезка с закруглёнными концами
func inCapsule(x, y float64, a, b pt, r float64) bool { return segDist(x, y, a, b) <= r }

// blueBody — объёмная заливка: светлее сверху-слева, с бликом у верхнего края фигуры
func blueBody(x, y, top float64) color.NRGBA {
	c := lerp(rgb(0x8fd0ff), rgb(0x2f5fe8), (x*0.4+y*0.6-0.15)/0.7)
	if y-top < 0.03 {
		c = lerp(c, rgb(0xe6f5ff), 0.45*(1-(y-top)/0.03))
	}
	return c
}

func glowAt(c color.NRGBA, x, y, cx, cy, r float64, col color.NRGBA, a float64) color.NRGBA {
	return over(c, col, a*math.Max(0, 1-math.Hypot(x-cx, y-cy)/r))
}

func monogram(x, y, px float64) color.NRGBA {
	c, ok := darkTile(x, y, px)
	if !ok {
		return c
	}
	const r = 0.068
	sx := 0.32
	bars := [][2]pt{
		{{sx, 0.27}, {sx, 0.73}},   // вертикаль
		{{sx, 0.27}, {0.69, 0.27}}, // верхняя строка
		{{sx, 0.5}, {0.55, 0.5}},   // средняя — короче, рядом точка «сегодня»
		{{sx, 0.73}, {0.69, 0.73}}, // нижняя строка
	}
	dot := pt{0.715, 0.5}
	c = glowAt(c, x, y, dot.x, dot.y, 0.22, rgb(0xff8a4a), 0.35)
	// тень
	for _, b := range bars {
		if inCapsule(x, y-0.025, b[0], b[1], r) {
			c = over(c, rgb(0x02040a), 0.5)
		}
	}
	for _, b := range bars {
		if inCapsule(x, y, b[0], b[1], r) {
			return blueBody(x, y, math.Min(b[0].y, b[1].y)-r)
		}
	}
	if d := math.Hypot(x-dot.x, y-dot.y); d < 0.072 {
		cc := lerp(warmTop, warmBot, (y-dot.y+0.072)/0.144)
		if d < 0.03 && y < dot.y {
			cc = lerp(cc, rgb(0xfff6d8), 0.5)
		}
		return cc
	}
	return c
}

func stripes(x, y, px float64) color.NRGBA {
	c, ok := darkTile(x, y, px)
	if !ok {
		return c
	}
	cx, cy, r := 0.5, 0.52, 0.29
	c = glowAt(c, x, y, cx, cy, 0.48, rgb(0xff7a59), 0.28)
	if math.Hypot(x-cx, y-cy) > r {
		return c
	}
	// вырезанные полосы в нижней половине: чем ниже, тем шире
	for _, band := range [][2]float64{{0.585, 0.012}, {0.645, 0.02}, {0.705, 0.028}, {0.765, 0.036}} {
		if y > band[0] && y < band[0]+band[1] {
			return c
		}
	}
	col := lerp(warmTop, rgb(0xff4f7a), (y-(cy-r))/(2*r))
	// мягкий блик сверху-слева
	return glowAt(col, x, y, cx-0.1, cy-0.15, 0.2, rgb(0xfff3c4), 0.45)
}

func hourglass(x, y, px float64) color.NRGBA {
	c, ok := darkTile(x, y, px)
	if !ok {
		return c
	}
	topBar := [2]pt{{0.3, 0.2}, {0.7, 0.2}}
	botBar := [2]pt{{0.3, 0.8}, {0.7, 0.8}}
	upL := []pt{{0.34, 0.235}, {0.5, 0.235}, {0.5, 0.5}}
	upR := []pt{{0.5, 0.235}, {0.66, 0.235}, {0.5, 0.5}}
	loL := []pt{{0.5, 0.5}, {0.5, 0.765}, {0.34, 0.765}}
	loR := []pt{{0.5, 0.5}, {0.66, 0.765}, {0.5, 0.765}}
	c = glowAt(c, x, y, 0.5, 0.66, 0.3, rgb(0xff8a4a), 0.25)
	c = withShadow(c, x, y, [][]pt{upL, upR, loL, loR}, 0.5)
	for _, b := range [][2]pt{topBar, botBar} {
		if inCapsule(x, y, b[0], b[1], 0.04) {
			return blueBody(x, y, b[0].y-0.04)
		}
	}
	sand := func(cc color.NRGBA) color.NRGBA { return lerp(warmTop, warmBot, (y-0.4)/0.4) }
	inside := inPoly(x, y, upL) || inPoly(x, y, upR) || inPoly(x, y, loL) || inPoly(x, y, loR)
	if !inside {
		return c
	}
	// песок: горка внизу и остаток наверху, тонкая струйка посередине
	if (y > 0.66 && inPoly(x, y, append(loL[:0:0], loL...))) || (y > 0.66 && inPoly(x, y, loR)) ||
		(y > 0.4 && y < 0.5 && (inPoly(x, y, upL) || inPoly(x, y, upR))) || (math.Abs(x-0.5) < 0.008 && y > 0.5 && y < 0.67) {
		return sand(c)
	}
	glass := lerp(rgb(0x6fb6ff), rgb(0x2a55d6), (y-0.2)/0.6)
	if x < 0.5 {
		glass = lerp(glass, rgb(0xd8efff), 0.28)
	}
	return over(c, glass, 0.78)
}

func gem(x, y, px float64) color.NRGBA {
	c, ok := darkTile(x, y, px)
	if !ok {
		return c
	}
	// классическая огранка: «корона» сверху и «павильон» снизу
	a, b, cc, d := pt{0.25, 0.37}, pt{0.38, 0.22}, pt{0.62, 0.22}, pt{0.75, 0.37}
	bot := pt{0.5, 0.8}
	m1, m2 := pt{0.44, 0.37}, pt{0.56, 0.37}
	tris := [][]pt{
		{a, b, m1}, {b, cc, m1}, {m1, cc, m2}, {cc, d, m2}, // корона
		{a, m1, bot}, {m1, m2, bot}, {m2, d, bot}, // павильон
	}
	tone := []color.NRGBA{rgb(0xb8e6ff), rgb(0x8cc8ff), rgb(0xd2f0ff), rgb(0x5e9bff), rgb(0x4f86ff), rgb(0x7fb4ff), rgb(0x1f45c6)}
	c = glowAt(c, x, y, 0.5, 0.5, 0.45, rgb(0x2f6bff), 0.25)
	c = withShadow(c, x, y, tris, 0.55)
	for i, tr := range tris {
		if inPoly(x, y, tr) {
			col := lerp(tone[i], rgb(0x1b3aa8), (y-0.22)/0.9)
			return col
		}
	}
	// тёплая искра справа сверху
	sp := pt{0.73, 0.24}
	if dx, dy := math.Abs(x-sp.x), math.Abs(y-sp.y); dx*dy < 0.0004 && dx < 0.07 && dy < 0.07 {
		return lerp(rgb(0xfff2c0), warmTop, (dx+dy)/0.1)
	}
	return c
}

// ---------- плоские песочные часы: три сплошных цвета, без градиентов ----------

var (
	flatTile  = rgb(0x131b2e)
	flatLight = rgb(0xeef2fa)
	flatBlue  = rgb(0x4f86ff)
	flatSand  = rgb(0xff8a3d)
)

func flatHourglass(glass color.NRGBA) func(x, y, px float64) color.NRGBA {
	// inBulb — выпуклая колба: у рамки чуть уже, посередине раздувается, к горлышку сходится
	const y0, neck, y1, wmax, wneck = 0.27, 0.5, 0.73, 0.2, 0.022
	inBulb := func(x, y float64) bool {
		var t float64
		switch {
		case y >= y0 && y <= neck:
			t = (y - y0) / (neck - y0)
		case y > neck && y <= y1:
			t = (y1 - y) / (y1 - neck)
		default:
			return false
		}
		w := wneck + (wmax-wneck)*math.Sin(math.Pi*(0.2+0.8*t))
		return math.Abs(x-0.5) <= w
	}
	return func(x, y, px float64) color.NRGBA {
		if !inRoundRect(x, y, 0.02, 0.02, 0.98, 0.98, 0.225) {
			return transparent
		}
		for _, by := range []float64{0.235, 0.765} {
			if inCapsule(x, y, pt{0.3, by}, pt{0.7, by}, 0.048) {
				return flatLight
			}
		}
		if !inBulb(x, y) {
			return flatTile
		}
		// песок: горка в нижней колбе и остаток у горлышка наверху
		if y > 0.63 || (y > 0.43 && y < neck) {
			return flatSand
		}
		return glass
	}
}

// ---------- песочные часы одной линией с перехлёстом (по эскизу) ----------

// path — ломаная, в которую раскладываются отрезки, дуги и кривые Безье
type path struct{ pts []pt }

func (p *path) to(q pt) { p.pts = append(p.pts, q) }

func (p *path) cubic(c1, c2, end pt) {
	s := p.pts[len(p.pts)-1]
	for i := 1; i <= 32; i++ {
		t := float64(i) / 32
		u := 1 - t
		p.to(pt{
			u*u*u*s.x + 3*u*u*t*c1.x + 3*u*t*t*c2.x + t*t*t*end.x,
			u*u*u*s.y + 3*u*u*t*c1.y + 3*u*t*t*c2.y + t*t*t*end.y,
		})
	}
}

// arc — дуга окружности от угла a0 до a1 (градусы, ось y вниз)
func (p *path) arc(c pt, r, a0, a1 float64) {
	for i := 1; i <= 16; i++ {
		a := (a0 + (a1-a0)*float64(i)/16) * math.Pi / 180
		p.to(pt{c.x + r*math.Cos(a), c.y + r*math.Sin(a)})
	}
}

// hourglassLine строит контур в координатах эскиза: рамка 78..197 × 57..258,
// одна линия от разрыва вокруг всей фигуры обратно к разрыву.
func hourglassLine() []pt {
	const L, R, T, B, r = 78.0, 197.0, 57.0, 258.0, 18.0
	p := &path{}
	p.to(pt{116, 178}) // нижний конец разрыва
	p.to(pt{105, 190})
	p.cubic(pt{93, 202}, pt{L, 220}, pt{L, B - r})
	p.arc(pt{L + r, B - r}, r, 180, 90)
	p.to(pt{R - r, B})
	p.arc(pt{R - r, B - r}, r, 90, 0)
	p.cubic(pt{R, 220}, pt{187, 207}, pt{175, 195})
	p.to(pt{100, 120}) // диагональ, проходящая «сверху» через центр
	p.cubic(pt{88, 108}, pt{L, 95}, pt{L, T + r})
	p.arc(pt{L + r, T + r}, r, 180, 270)
	p.to(pt{R - r, T})
	p.arc(pt{R - r, T + r}, r, 270, 360)
	p.cubic(pt{R, 95}, pt{182, 113}, pt{170, 125})
	p.to(pt{156, 140}) // верхний конец разрыва
	// в координаты иконки: высота фигуры 0.6, центр в середине плитки
	k := 0.6 / (B - T)
	out := make([]pt, len(p.pts))
	for i, q := range p.pts {
		out[i] = pt{0.5 + (q.x-137.5)*k, 0.5 + (q.y-157.5)*k}
	}
	return out
}

var hourglassPts = hourglassLine()

func lineHourglass(bg color.NRGBA, width float64) func(x, y, px float64) color.NRGBA {
	half := width / 2
	return func(x, y, px float64) color.NRGBA {
		if !inRoundRect(x, y, 0.02, 0.02, 0.98, 0.98, 0.225) {
			return transparent
		}
		if x < 0.29 || x > 0.71 || y < 0.17 || y > 0.83 {
			return bg
		}
		for i := 1; i < len(hourglassPts); i++ {
			if segDist(x, y, hourglassPts[i-1], hourglassPts[i]) <= half {
				return rgb(0xffffff)
			}
		}
		return bg
	}
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
