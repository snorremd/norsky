package main

import (
	_ "embed"
	"norsky/cmd"

	_ "golang.org/x/crypto/x509roots/fallback" // We need this to make TLS work in scratch containers
)

func main() {
	cmd.Execute()
}
