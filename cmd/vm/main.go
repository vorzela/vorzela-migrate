// Command vm is the `go install`-friendly entry point:
//
//	go install github.com/vorzela/vorzela-migrate/v2/cmd/vm@v2.3.3
//
// It installs a binary named "vm" into GOBIN (or GOPATH/bin) with no root.
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
