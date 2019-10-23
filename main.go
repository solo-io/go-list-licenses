package main

import (
	"log"

	"github.com/solo-io/go-list-licenses/pkg/license"
)

func main() {
	err := license.PrintLicenses()
	if err != nil {
		log.Fatalf("error: %s\n", err)
	}
}
