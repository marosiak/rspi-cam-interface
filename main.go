package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/gofiber/fiber/v3"
)

func main() {
	app := fiber.New()
	app.Get("/", func(c fiber.Ctx) error {
		return c.SendString(`<a href="/api/v1/preview">Preview</a><br><a href="/api/v1/photo">Photo</a>`)
	})

	app.Get("/api/v1/preview", func(c fiber.Ctx) error {
		id := fmt.Sprintf("%d", time.Now().UnixNano())
		filename := fmt.Sprintf("preview_%s.jpg", id)

		cmd := exec.Command("rpicam-jpeg", "--output", filename, "--timeout", "2000", "--width", "640", "--height", "480")
		if err := cmd.Run(); err != nil {
			return c.Status(500).SendString(fmt.Sprintf("camera capture failed: %v", err))
		}
		defer os.Remove(filename)

		data, err := os.ReadFile(filename)
		if err != nil {
			return c.Status(500).SendString(fmt.Sprintf("failed to read image: %v", err))
		}

		c.Set("Content-Type", "image/jpeg")
		return c.Send(data)
	})

	app.Get("/api/v1/photo", func(c fiber.Ctx) error {
		id := fmt.Sprintf("%d", time.Now().UnixNano())
		filename := fmt.Sprintf("photo_%s.jpg", id)

		cmd := exec.Command("rpicam-jpeg", "--output", filename)
		if err := cmd.Run(); err != nil {
			return c.Status(500).SendString(fmt.Sprintf("camera capture failed: %v", err))
		}
		defer os.Remove(filename)

		data, err := os.ReadFile(filename)
		if err != nil {
			return c.Status(500).SendString(fmt.Sprintf("failed to read image: %v", err))
		}

		c.Set("Content-Type", "image/jpeg")
		return c.Send(data)
	})

	log.Fatal(app.Listen(":80"))
}
