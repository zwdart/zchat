package main

import (
	_ "embed"
	"log"

	"xin.dart.zchat/internal/app"
)

// The app icons live in resources/, outside internal/app, so the root
// package embeds them and hands them to the app for the tray.
//
//go:embed resources/icon.png
var appIcon []byte

//go:embed resources/icon-dev.png
var appIconDev []byte

func main() {
	app.SetTrayIcons(appIcon, appIconDev)
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
