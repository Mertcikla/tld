package main

import (
	"fmt"
	"os"

	assets "github.com/mertcikla/tld"
)

func main() {
	target := "./public"
	if len(os.Args) > 1 && os.Args[1] != "" {
		target = os.Args[1]
	}

	if err := assets.ExtractIcons(target); err != nil {
		fmt.Fprintf(os.Stderr, "extract icons: %v\n", err)
		os.Exit(1)
	}
}
