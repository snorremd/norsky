package main

import (
	"context"
	"fmt"
	"norsky/cmd"
	"os"
	"os/signal"
	"syscall"

	_ "golang.org/x/crypto/x509roots/fallback"
)

func main() {
	// Check if a signal interrupts the process and if so call Done on the context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Listen for interrupt signals
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		<-c
		cancel()
	}()

	app := cmd.RootApp()
	if err := app.RunContext(ctx, os.Args); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
