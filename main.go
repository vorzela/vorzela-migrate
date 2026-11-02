package main

import (
	"log"

	"github.com/vorzela/vorzela-migrate/v2/internal/app"
)

func main() {
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
