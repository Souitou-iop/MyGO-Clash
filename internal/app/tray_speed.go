package app

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"

	"github.com/egoist/mygo/ui"
)

// The traffic beside the tray icon (macOS): the upload over the download,
// in small type of even digits, drawn into the icon's image so that the
// item keeps its width and macOS tints it with the menu bar.

// speedLabelW is the width of the two lines, in points; the icon is
// trayPx device pixels, two to a point.
const speedLabelW = 56

// trayRate is a rate in three digits, as "12.3 KB/s", "123 KB/s" or
// "1.25 MB/s".
func trayRate(b int64) string {
	units := []string{"B/s", "KB/s", "MB/s", "GB/s"}
	f, i := float64(max(b, 0)), 0
	for f >= 999.5 && i < len(units)-1 {
		f /= 1024
		i++
	}
	switch {
	case i == 0 || f >= 99.95:
		return fmt.Sprintf("%.0f %s", f, units[i])
	case f >= 9.995:
		return fmt.Sprintf("%.1f %s", f, units[i])
	}
	return fmt.Sprintf("%.2f %s", f, units[i])
}

// speedIcon is the tray icon with the traffic beside it, as a PNG of
// black on clear for a template image. muted dims the traffic when the
// proxy is off, matching the faint tray icon.
func speedIcon(icon []byte, up, down int64, muted bool) []byte {
	src, err := png.Decode(bytes.NewReader(icon))
	if err != nil {
		return icon
	}
	ih := src.Bounds().Dy()
	textColor := ui.RGBA(0, 0, 0, 1)
	if muted {
		textColor = ui.RGBA(0, 0, 0, 115.0/255.0)
	}
	label := ui.Render(func(c *ui.Context) {
		c.Root().Background(ui.Transparent)
		ui.Column(c).Fill().Justify(ui.Center).Children(func() {
			for _, l := range [][2]string{{"↑", trayRate(up)}, {"↓", trayRate(down)}} {
				ui.Row(c).Height(9).AlignItems(ui.Center).Children(func() {
					ui.Text(c, l[0]).FontSize(8).FontWeight(700).TextColor(textColor)
					ui.Spacer(c)
					ui.Text(c, l[1]).FontSize(9).FontWeight(600).FontFeatures("tnum").TextColor(textColor).SingleLine()
				})
			}
		})
	}, speedLabelW, ih/2, 2)
	gap := 8
	out := image.NewNRGBA(image.Rect(0, 0, ih+gap+label.Bounds().Dx(), ih))
	draw.Draw(out, src.Bounds(), src, image.Point{}, draw.Over)
	draw.Draw(out, label.Bounds().Add(image.Pt(ih+gap, 0)), label, image.Point{}, draw.Over)
	var buf bytes.Buffer
	if png.Encode(&buf, out) != nil {
		return icon
	}
	return buf.Bytes()
}
