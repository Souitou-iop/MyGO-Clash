// Genicon makes the app's icons from its Icon Composer document, with the
// tools of Xcode 26 or later:
//
//	go run ./cmd/genicon path/to/MyGo-Clash.icon
//
// It writes
//
//   - resources/darwin/Assets.car, the document compiled by actool, which
//     macOS 26 and later show in the system's appearance: light, dark,
//     clear or tinted (mygo.json names it with CFBundleIconName);
//   - resources/icon.png, its default rendition on the grid of macOS app
//     icons (824 of 1024 pixels, over a soft shadow), which mygo build
//     makes the icon of Windows, Linux and earlier macOS;
//   - src/assets/icon.png and icon-dark.png, its light and dark renditions
//     for the About page;
//   - src/assets/mark.png, its badge (the first layer) for the sidebar.
package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/image/draw"
)

// iconName is the name of the icon in Assets.car: CFBundleIconName.
const iconName = "AppIcon"

func main() {
	if len(os.Args) != 2 || !strings.HasSuffix(strings.TrimSuffix(os.Args[1], "/"), ".icon") {
		fmt.Fprintln(os.Stderr, "usage: genicon DOCUMENT.icon")
		os.Exit(2)
	}
	doc := strings.TrimSuffix(os.Args[1], "/")
	tmp, err := os.MkdirTemp("", "genicon")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	compile(doc, tmp)
	light, dark := render(doc, tmp, "Default"), render(doc, tmp, "Dark")
	save("resources/icon.png", appIcon(light))
	save("src/assets/icon.png", scale(light, 192))
	save("src/assets/icon-dark.png", scale(dark, 192))
	save("src/assets/mark.png", scale(trim(load(badge(doc))), 192))
}

// compile builds Assets.car with actool, from a copy of the document named
// for iconName.
func compile(doc, tmp string) {
	src := filepath.Join(tmp, iconName+".icon")
	run("cp", "-R", doc, src)
	out := filepath.Join(tmp, "car")
	if err := os.MkdirAll(out, 0o755); err != nil {
		log.Fatal(err)
	}
	run("xcrun", "actool", src, "--compile", out, "--platform", "macosx",
		"--minimum-deployment-target", "12.0", "--target-device", "mac", "--app-icon", iconName,
		"--output-partial-info-plist", filepath.Join(tmp, "partial.plist"),
		"--errors", "--warnings", "--output-format", "human-readable-text")
	if err := os.MkdirAll("resources/darwin", 0o755); err != nil {
		log.Fatal(err)
	}
	run("cp", filepath.Join(out, "Assets.car"), "resources/darwin/Assets.car")
}

// render exports a rendition of the document for macOS at 1024 pixels with
// Icon Composer's ictool.
func render(doc, tmp, rendition string) image.Image {
	dev, err := exec.Command("xcode-select", "-p").Output()
	if err != nil {
		log.Fatal("xcode-select: ", err)
	}
	ictool := filepath.Join(strings.TrimSpace(string(dev)), "../Applications/Icon Composer.app/Contents/Executables/ictool")
	out := filepath.Join(tmp, rendition+".png")
	run(ictool, doc, "--export-image", "--output-file", out, "--platform", "macOS",
		"--rendition", rendition, "--width", "1024", "--height", "1024", "--scale", "1")
	return load(out)
}

// badge is the image of the document's first layer.
func badge(doc string) string {
	b, err := os.ReadFile(filepath.Join(doc, "icon.json"))
	if err != nil {
		log.Fatal(err)
	}
	var d struct {
		Groups []struct {
			Layers []struct {
				Image string `json:"image-name"`
			} `json:"layers"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(b, &d); err != nil {
		log.Fatal("icon.json: ", err)
	}
	if len(d.Groups) == 0 || len(d.Groups[0].Layers) == 0 || d.Groups[0].Layers[0].Image == "" {
		log.Fatal("icon.json: no layer with an image")
	}
	return filepath.Join(doc, "Assets", d.Groups[0].Layers[0].Image)
}

func run(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("%s: %v", filepath.Base(name), err)
	}
}

// appIcon lays a rendition out as macOS draws app icons: 824 pixels of
// 1024, centered, over a shadow 10 pixels down with a 10-pixel blur at 30%.
func appIcon(src image.Image) image.Image {
	const size, body, offset, blur, opacity = 1024, 824, 10, 5.0, 0.3
	at := (size - body) / 2
	icon := image.NewNRGBA(image.Rect(0, 0, size, size))
	draw.CatmullRom.Scale(icon, image.Rect(at, at, at+body, at+body), src, src.Bounds(), draw.Over, nil)
	alpha := make([]float64, size*size)
	for y := offset; y < size; y++ {
		for x := 0; x < size; x++ {
			alpha[y*size+x] = float64(icon.NRGBAAt(x, y-offset).A) / 255
		}
	}
	alpha = gaussian(alpha, size, blur)
	out := image.NewNRGBA(icon.Bounds())
	for i, a := range alpha {
		out.Pix[4*i+3] = uint8(math.Round(255 * opacity * a))
	}
	draw.Draw(out, out.Bounds(), icon, image.Point{}, draw.Over)
	return out
}

// gaussian blurs a square of size×size values, along the rows and then
// along the columns.
func gaussian(v []float64, size int, sigma float64) []float64 {
	r := int(math.Ceil(3 * sigma))
	k := make([]float64, 2*r+1)
	var sum float64
	for i := range k {
		d := float64(i - r)
		k[i] = math.Exp(-d * d / (2 * sigma * sigma))
		sum += k[i]
	}
	for i := range k {
		k[i] /= sum
	}
	pass := func(src []float64, at func(x, y int) int) []float64 {
		dst := make([]float64, len(src))
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				var s float64
				for i, w := range k {
					if xx := x + i - r; xx >= 0 && xx < size {
						s += w * src[at(xx, y)]
					}
				}
				dst[at(x, y)] = s
			}
		}
		return dst
	}
	rows := func(x, y int) int { return y*size + x }
	cols := func(x, y int) int { return x*size + y }
	return pass(pass(v, rows), cols)
}

// trim crops img to the square around what it draws.
func trim(img image.Image) image.Image {
	b := img.Bounds()
	box := image.Rectangle{Min: b.Max, Max: b.Min}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a > 0x0400 {
				box = box.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	side := max(box.Dx(), box.Dy())
	cx, cy := (box.Min.X+box.Max.X)/2, (box.Min.Y+box.Max.Y)/2
	sq := image.Rect(cx-side/2, cy-side/2, cx-side/2+side, cy-side/2+side)
	out := image.NewNRGBA(image.Rect(0, 0, side, side))
	draw.Draw(out, out.Bounds(), &image.Uniform{color.Transparent}, image.Point{}, draw.Src)
	draw.Draw(out, out.Bounds(), img, sq.Min, draw.Over)
	return out
}

func scale(img image.Image, px int) image.Image {
	out := image.NewNRGBA(image.Rect(0, 0, px, px))
	draw.CatmullRom.Scale(out, out.Bounds(), img, img.Bounds(), draw.Over, nil)
	return out
}

func load(path string) image.Image {
	f, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		log.Fatalf("%s: %v", path, err)
	}
	return img
}

func save(path string, img image.Image) {
	f, err := os.Create(path)
	if err != nil {
		log.Fatal(err)
	}
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(f, img); err != nil {
		log.Fatal(err)
	}
	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
}
