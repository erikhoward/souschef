package main

import "fmt"

// version is overridden at build time via -ldflags.
var version = "0.1.0-dev"

func main() {
	fmt.Printf("souschef %s\n", version)
}
