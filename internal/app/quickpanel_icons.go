package app

import "github.com/egoist/mygo/ui"

// The icons of the quick panel (Lucide, ISC), drawn in the color of their
// text.
func panelIcon(paths string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` + paths + `</svg>`))
}

var (
	iconWindow   = panelIcon(`<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M3 9h18"/>`)
	iconSettings = panelIcon(`<path d="M21 4h-7M10 4H3M21 12h-9M8 12H3M21 20h-5M12 20H3M14 2v4M8 10v4M16 18v4"/>`)
	iconPower    = panelIcon(`<path d="M12 2v10"/><path d="M18.4 6.6a9 9 0 1 1-12.8 0"/>`)
	iconGlobe    = panelIcon(`<circle cx="12" cy="12" r="10"/><path d="M2 12h20"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/>`)
	iconShield   = panelIcon(`<path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/>`)
	iconCheck    = panelIcon(`<path d="M20 6 9 17l-5-5"/>`)
	iconUp       = panelIcon(`<path d="m5 12 7-7 7 7"/><path d="M12 19V5"/>`)
	iconDown     = panelIcon(`<path d="M12 5v14"/><path d="m19 12-7 7-7-7"/>`)
	iconZap      = panelIcon(`<path d="M4 14a1 1 0 0 1-.78-1.63l9.9-10.2a.5.5 0 0 1 .86.46l-1.92 6.02A1 1 0 0 0 13 10h7a1 1 0 0 1 .78 1.63l-9.9 10.2a.5.5 0 0 1-.86-.46l1.92-6.02A1 1 0 0 0 11 14z"/>`)
)
