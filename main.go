package main

import (
	"log"

	"xin.dart.zchat/internal/app"
)

func main() {
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
