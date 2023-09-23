package server

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cache"
)

// Returns a fiber.App instance to be used as an HTTP server for the norsky feed
func Server() *fiber.App {

	app := fiber.New()

	app.Use(cache.New())

	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString("Hello, World!")
	})

	return app
}
