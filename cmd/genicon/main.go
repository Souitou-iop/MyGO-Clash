// Genicon draws the app's icon into resources/icon.png.
//
//	go run ./cmd/genicon
package main

import (
	"log"
	"os"

	"github.com/mygo-clash/mygo-clash/internal/art"
)

func main() {
	if err := os.WriteFile("resources/icon.png", art.AppIcon(1024), 0o644); err != nil {
		log.Fatal(err)
	}
}
