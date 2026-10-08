// Winsetup builds the Windows installer of the app, replacing the one that
// `mygo build` makes. It runs after the build, once per Windows build
// directory:
//
//	go run ./tools/winsetup build/windows-amd64 build/windows-arm64
//
// For each directory it compiles packaging/windows/installer.nsi with
// makensis, from the executable and files that mygo build left there, and
// writes "<name> Setup <version>.exe" over mygo's installer, under the same
// name, so that whatever renames and uploads that file keeps working. The
// installer keeps mygo's layout (per-user, the same uninstall entry and
// folder), so the app's own updates and installs of both kinds keep working.
//
// It needs NSIS 3.08 or later (makensis, from the nsis package on Debian
// and Ubuntu). The installer's icon and bitmaps are made from the app's
// icon, so that nothing binary lives in the repository.
//
// To sign, set WINSETUP_SIGN_CMD to a command that signs the file that
// replaces %1, such as `signtool sign /f cert.pfx /fd sha256 %1`: it signs
// the uninstaller, and then the installer.
package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// brand is the blue of MyGO!!!!!'s official art.
var brand = color.NRGBA{0x0B, 0x88, 0xBB, 0xFF}

// brandDeep is brand, darker, for the bottom of the welcome bitmap.
var brandDeep = color.NRGBA{0x07, 0x62, 0x88, 0xFF}

type config struct {
	Name       string   `json:"name"`
	Identifier string   `json:"identifier"`
	Version    string   `json:"version"`
	Copyright  string   `json:"copyright"`
	Icon       string   `json:"icon"`
	URLSchemes []string `json:"urlSchemes"`
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("winsetup: ")
	if len(os.Args) < 2 {
		log.Fatal("usage: winsetup <build/windows-amd64> [<build/windows-arm64>...]")
	}
	root, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	c, err := loadConfig(filepath.Join(root, "mygo.json"))
	if err != nil {
		log.Fatal(err)
	}
	for _, dir := range os.Args[1:] {
		if err := build(root, c, dir); err != nil {
			log.Fatalf("%s: %v", dir, err)
		}
	}
}

func loadConfig(path string) (*config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if c.Name == "" || c.Identifier == "" || c.Version == "" {
		return nil, fmt.Errorf("%s: name, identifier and version are required", path)
	}
	if c.Icon == "" {
		c.Icon = "resources/icon.png"
	}
	return &c, nil
}

// slug turns the app's name into the name its service uses, as the app's
// paths.Slug does: "MyGO-Clash" -> "mygo-clash".
func slug(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "-"):
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// fsName makes s fit in a file name, as mygo build does.
func fsName(s string) string {
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || r < ' ' {
			return '-'
		}
		return r
	}, s)
}

// excluded are the files of a build directory that are not the app:
// installers, update manifests and archives.
func excluded(name string) bool {
	switch {
	case strings.HasSuffix(name, ".exe") && strings.Contains(name, " Setup "):
		return true
	case strings.HasPrefix(name, "update-") && strings.HasSuffix(name, ".json"):
		return true
	case strings.HasSuffix(name, ".tar.gz"), strings.HasSuffix(name, ".delta"), name == "install.sh":
		return true
	}
	return false
}

var versionRe = regexp.MustCompile(`^v?(\d+)(?:\.(\d+))?(?:\.(\d+))?`)

// numericVersion is version as the four numbers of a file version.
func numericVersion(v string) (string, error) {
	m := versionRe.FindStringSubmatch(v)
	if m == nil {
		return "", fmt.Errorf("version %q does not start with a number", v)
	}
	parts := make([]string, 4)
	for i := range parts {
		parts[i] = "0"
		if i < 3 && m[i+1] != "" {
			parts[i] = m[i+1]
		}
	}
	return strings.Join(parts, "."), nil
}

// nsisString quotes s for an NSIS script.
func nsisString(s string) string {
	return `"` + strings.NewReplacer(`$`, `$$`, `"`, `$\"`, "\n", `$\n`, "\r", `$\r`, "\t", `$\t`).Replace(s) + `"`
}

func build(root string, c *config, dir string) error {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	base := filepath.Base(dir)
	arch, ok := strings.CutPrefix(base, "windows-")
	if !ok {
		return fmt.Errorf("%q is not a windows-<arch> build directory", base)
	}
	exe := c.Name + ".exe"
	if _, err := os.Stat(filepath.Join(dir, exe)); err != nil {
		return fmt.Errorf("the build has no %s: %w", exe, err)
	}
	makensis, err := findMakensis()
	if err != nil {
		return err
	}
	vi, err := numericVersion(c.Version)
	if err != nil {
		return err
	}

	work, err := os.MkdirTemp("", "winsetup-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)

	// The files of the app: all of the directory but the installers and
	// update files, as mygo installs them.
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var files strings.Builder
	var size int64
	for _, e := range entries {
		if excluded(e.Name()) {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if e.IsDir() {
			fmt.Fprintf(&files, "File /r %s\n", nsisString(p))
		} else {
			fmt.Fprintf(&files, "File %s\n", nsisString(p))
		}
		err := filepath.WalkDir(p, func(_ string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				if fi, err := d.Info(); err == nil {
					size += fi.Size()
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	if err := writeFile(filepath.Join(work, "files.nsh"), files.String()); err != nil {
		return err
	}

	// The URL schemes, registered as mygo's installer does: only ours is
	// removed again, when it still opens this app.
	open := `'"$INSTDIR\` + strings.ReplaceAll(exe, `$`, `$$`) + `" "%1"'`
	var schemes strings.Builder
	schemes.WriteString("!macro RegisterSchemes\n")
	for _, s := range c.URLSchemes {
		key := `Software\Classes\` + s
		fmt.Fprintf(&schemes, "  WriteRegStr HKCU %s \"\" %s\n", nsisString(key), nsisString("URL:"+c.Name))
		fmt.Fprintf(&schemes, "  WriteRegStr HKCU %s \"URL Protocol\" \"\"\n", nsisString(key))
		fmt.Fprintf(&schemes, "  WriteRegStr HKCU %s \"\" %s\n", nsisString(key+`\shell\open\command`), open)
	}
	if len(c.URLSchemes) > 0 {
		schemes.WriteString("  System::Call 'shell32::SHChangeNotify(i 0x08000000, i 0, p 0, p 0)'\n")
	}
	schemes.WriteString("!macroend\n!macro UnregisterSchemes\n")
	for i, s := range c.URLSchemes {
		key := `Software\Classes\` + s
		fmt.Fprintf(&schemes, "  ReadRegStr $0 HKCU %s \"\"\n", nsisString(key+`\shell\open\command`))
		fmt.Fprintf(&schemes, "  StrCmp $0 %s 0 scheme_%d_done\n", open, i)
		fmt.Fprintf(&schemes, "    DeleteRegKey HKCU %s\n", nsisString(key))
		fmt.Fprintf(&schemes, "  scheme_%d_done:\n", i)
	}
	if len(c.URLSchemes) > 0 {
		schemes.WriteString("  System::Call 'shell32::SHChangeNotify(i 0x08000000, i 0, p 0, p 0)'\n")
	}
	schemes.WriteString("!macroend\n")
	if err := writeFile(filepath.Join(work, "schemes.nsh"), schemes.String()); err != nil {
		return err
	}

	// Branding, from the app's icon.
	src, err := readPNG(filepath.Join(root, c.Icon))
	if err != nil {
		return err
	}
	ico := filepath.Join(work, "installer.ico")
	if err := os.WriteFile(ico, makeICO(src), 0o644); err != nil {
		return err
	}
	header := filepath.Join(work, "header.bmp")
	if err := os.WriteFile(header, encodeBMP(headerImage(src)), 0o644); err != nil {
		return err
	}
	welcome := filepath.Join(work, "welcome.bmp")
	if err := os.WriteFile(welcome, encodeBMP(welcomeImage(src)), 0o644); err != nil {
		return err
	}

	out := filepath.Join(dir, fsName(c.Name)+" Setup "+fsName(c.Version)+".exe")
	signCmd := os.Getenv("WINSETUP_SIGN_CMD")
	defs := [][2]string{
		{"PRODUCT_NAME", c.Name},
		{"IDENTIFIER", c.Identifier},
		{"VERSION", c.Version},
		{"VI_VERSION", vi},
		{"ARCH", arch},
		{"MAIN_EXE", exe},
		{"SERVICE_SLUG", slug(c.Name)},
		{"SERVICE_NAME", slug(c.Name) + "-service"},
		{"COPYRIGHT", c.Copyright},
		{"PUBLISHER", c.Name},
		{"ESTIMATED_KB", strconv.FormatInt(size/1024, 10)},
		{"OUTFILE", out},
		{"WORKDIR", work},
		{"ICON", ico},
		{"HEADER_BMP", header},
		{"WELCOME_BMP", welcome},
	}
	if signCmd != "" {
		defs = append(defs, [2]string{"SIGN_CMD", signCmd})
	}
	verbosity := "-V2"
	if v := os.Getenv("WINSETUP_VERBOSITY"); v != "" {
		verbosity = "-V" + v
	}
	args := []string{verbosity, "-INPUTCHARSET", "UTF8"}
	for _, d := range defs {
		args = append(args, "-D"+d[0]+"="+d[1])
	}
	args = append(args, filepath.Join(root, "packaging", "windows", "installer.nsi"))

	log.Printf("creating %s", filepath.Base(out))
	cmd := exec.Command(makensis, args...)
	cmd.Dir = filepath.Join(root, "packaging", "windows")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("makensis: %v\n%s", err, output)
	}
	if bytes.Contains(output, []byte("warning")) || os.Getenv("WINSETUP_VERBOSITY") != "" {
		// Warnings are not fatal, but nobody should miss them.
		log.Printf("makensis warnings:\n%s", output)
	}
	if signCmd != "" {
		if err := runSign(signCmd, out); err != nil {
			return err
		}
	}
	return nil
}

func writeFile(path, s string) error {
	// NSIS reads UTF-8 only with a byte order mark.
	return os.WriteFile(path, append([]byte{0xEF, 0xBB, 0xBF}, s...), 0o644)
}

func findMakensis() (string, error) {
	if p := os.Getenv("MAKENSIS"); p != "" {
		return p, nil
	}
	p, err := exec.LookPath("makensis")
	if err != nil {
		return "", fmt.Errorf("makensis not found: install NSIS (apt install nsis, brew install makensis)")
	}
	return p, nil
}

func runSign(command, file string) error {
	command = strings.ReplaceAll(command, "%1", `"`+file+`"`)
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", command)
	} else {
		cmd = exec.Command("sh", "-c", command)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("signing %s: %v\n%s", filepath.Base(file), err, out)
	}
	return nil
}

// ---- Images ----

func readPNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return img, nil
}

// resize scales img to a size x size square: by averaging the pixels that a
// target pixel covers, with the colors weighted by their alpha so that
// transparent pixels do not darken the edges.
func resize(img image.Image, size int) *image.NRGBA {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	sx, sy := float64(w)/float64(size), float64(h)/float64(size)
	for y := range size {
		for x := range size {
			x0, x1 := int(float64(x)*sx), max(int(float64(x+1)*sx), int(float64(x)*sx)+1)
			y0, y1 := int(float64(y)*sy), max(int(float64(y+1)*sy), int(float64(y)*sy)+1)
			var r, g, bl, a, n float64
			for yy := y0; yy < y1 && yy < h; yy++ {
				for xx := x0; xx < x1 && xx < w; xx++ {
					c := color.NRGBAModel.Convert(img.At(b.Min.X+xx, b.Min.Y+yy)).(color.NRGBA)
					al := float64(c.A)
					r += float64(c.R) * al
					g += float64(c.G) * al
					bl += float64(c.B) * al
					a += al
					n++
				}
			}
			if a == 0 || n == 0 {
				continue
			}
			dst.SetNRGBA(x, y, color.NRGBA{uint8(r/a + 0.5), uint8(g/a + 0.5), uint8(bl/a + 0.5), uint8(a/n + 0.5)})
		}
	}
	return dst
}

// makeICO is img as an icon of several sizes. They are bitmaps rather than
// PNGs, which every NSIS reads.
func makeICO(img image.Image) []byte {
	sizes := []int{16, 24, 32, 48, 64, 128, 256}
	var out bytes.Buffer
	_ = binary.Write(&out, binary.LittleEndian, [3]uint16{0, 1, uint16(len(sizes))})
	var images [][]byte
	for _, s := range sizes {
		images = append(images, dib(resize(img, s)))
	}
	offset := 6 + 16*len(sizes)
	for i, s := range sizes {
		dim := byte(s)
		if s >= 256 {
			dim = 0
		}
		out.Write([]byte{dim, dim, 0, 0})
		_ = binary.Write(&out, binary.LittleEndian, [2]uint16{1, 32})
		_ = binary.Write(&out, binary.LittleEndian, [2]uint32{uint32(len(images[i])), uint32(offset)})
		offset += len(images[i])
	}
	for _, p := range images {
		out.Write(p)
	}
	return out.Bytes()
}

// dib is img as the bitmap of an icon: 32 bits per pixel, bottom row first,
// followed by an empty mask (the alpha channel does the masking).
func dib(img *image.NRGBA) []byte {
	s := img.Bounds().Dx()
	var out bytes.Buffer
	_ = binary.Write(&out, binary.LittleEndian, struct {
		Size                 uint32
		Width, Height        int32
		Planes, Bits         uint16
		Compression, ImgSize uint32
		XPels, YPels         int32
		Used, Important      uint32
	}{40, int32(s), int32(2 * s), 1, 32, 0, 0, 0, 0, 0, 0})
	for y := s - 1; y >= 0; y-- {
		for x := range s {
			c := img.NRGBAAt(x, y)
			out.Write([]byte{c.B, c.G, c.R, c.A})
		}
	}
	out.Write(make([]byte, (s+31)/32*4*s))
	return out.Bytes()
}

// encodeBMP is img as a 24-bit BMP, which is what NSIS shows.
func encodeBMP(img *image.RGBA) []byte {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	row := (w*3 + 3) &^ 3
	var out bytes.Buffer
	out.WriteString("BM")
	_ = binary.Write(&out, binary.LittleEndian, struct {
		FileSize       uint32
		Reserved       uint32
		DataOffset     uint32
		HeaderSize     uint32
		Width, Height  int32
		Planes, Bits   uint16
		Compression    uint32
		ImageSize      uint32
		XPels, YPels   int32
		Colors, Import uint32
	}{uint32(54 + row*h), 0, 54, 40, int32(w), int32(h), 1, 24, 0, uint32(row * h), 2835, 2835, 0, 0})
	pad := make([]byte, row-w*3)
	for y := h - 1; y >= 0; y-- {
		for x := range w {
			c := img.RGBAAt(x, y)
			out.Write([]byte{c.B, c.G, c.R})
		}
		out.Write(pad)
	}
	return out.Bytes()
}

// headerImage is the bitmap at the right of the page header: 150x57 on
// white, the app's icon on it.
func headerImage(icon image.Image) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 150, 57))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	const s = 52
	draw.Draw(img, image.Rect(150-s-6, (57-s)/2, 150-6, (57-s)/2+s), resize(icon, s), image.Point{}, draw.Over)
	return img
}

// welcomeImage is the bitmap at the left of the welcome and finish pages:
// 164x314, the brand blue fading to a deeper one, the icon above the middle.
func welcomeImage(icon image.Image) *image.RGBA {
	const w, h = 164, 314
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		t := float64(y) / float64(h-1)
		mix := func(a, b uint8) uint8 { return uint8(float64(a)*(1-t) + float64(b)*t + 0.5) }
		c := color.RGBA{mix(brand.R, brandDeep.R), mix(brand.G, brandDeep.G), mix(brand.B, brandDeep.B), 0xFF}
		for x := range w {
			img.SetRGBA(x, y, c)
		}
	}
	const s = 112
	draw.Draw(img, image.Rect((w-s)/2, 64, (w-s)/2+s, 64+s), resize(icon, s), image.Point{}, draw.Over)
	return img
}
