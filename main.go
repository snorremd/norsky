package main

import (
	"fmt"
	"norsky/cmd"
	"os"

	_ "golang.org/x/crypto/x509roots/fallback"
)

func main() {
	app := cmd.RootApp()
	if err := app.Run(os.Args); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
