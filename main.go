package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/static"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Timelapse struct {
		Period time.Duration `yaml:"period"`
		Name   string        `yaml:"name"`
	} `yaml:"timelapse"`
}

func loadConfig(path string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	err = yaml.Unmarshal(data, &cfg)
	return cfg, err
}

func capturePhoto(outputPath string) error {
	cmd := exec.Command("rpicam-jpeg", "--output", outputPath)
	return cmd.Run()
}

func nextPackageNumber(packagesDir, timelapseName string) int {
	entries, err := os.ReadDir(packagesDir)
	if err != nil {
		return 1
	}

	maxNum := 0
	prefix := fmt.Sprintf("timelapse_%s_", timelapseName)
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".tar.gz") {
			numStr := strings.TrimPrefix(name, prefix)
			numStr = strings.TrimSuffix(numStr, ".tar.gz")
			var num int
			if _, err := fmt.Sscanf(numStr, "%d", &num); err == nil && num > maxNum {
				maxNum = num
			}
		}
	}
	return maxNum + 1
}

func packagePhotos(timelapseName string) error {
	timelapseDir := "./timelapse"
	packagesDir := "./packages"

	if err := os.MkdirAll(packagesDir, 0755); err != nil {
		return err
	}

	entries, err := os.ReadDir(timelapseDir)
	if err != nil {
		return err
	}

	var photos []string
	prefix := timelapseName + "_"
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".jpg") {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if time.Since(info.ModTime()) > 5*time.Second {
				photos = append(photos, filepath.Join(timelapseDir, name))
			}
		}
	}

	if len(photos) == 0 {
		return nil
	}

	nextNum := nextPackageNumber(packagesDir, timelapseName)
	packageName := fmt.Sprintf("timelapse_%s_%02d.tar.gz", timelapseName, nextNum)
	packagePath := filepath.Join(packagesDir, packageName)

	file, err := os.Create(packagePath)
	if err != nil {
		return err
	}

	gzw := gzip.NewWriter(file)
	tw := tar.NewWriter(gzw)

	success := true
	for _, photo := range photos {
		f, err := os.Open(photo)
		if err != nil {
			success = false
			continue
		}
		info, err := f.Stat()
		if err != nil {
			f.Close()
			success = false
			continue
		}
		hdr, err := tar.FileInfoHeader(info, info.Name())
		if err != nil {
			f.Close()
			success = false
			continue
		}
		hdr.Name = filepath.Base(photo)
		if err := tw.WriteHeader(hdr); err != nil {
			f.Close()
			success = false
			continue
		}
		if _, err := io.Copy(tw, f); err != nil {
			f.Close()
			success = false
			continue
		}
		f.Close()
	}

	tw.Close()
	gzw.Close()
	file.Close()

	if success {
		for _, photo := range photos {
			os.Remove(photo)
		}
	} else {
		os.Remove(packagePath)
	}

	return nil
}

func startTimelapse(cfg Config, stopChan <-chan struct{}) {
	ticker := time.NewTicker(cfg.Timelapse.Period)
	defer ticker.Stop()

	os.MkdirAll("./timelapse", 0755)

	for {
		select {
		case <-ticker.C:
			id := time.Now().UnixNano()
			filename := fmt.Sprintf("%s_%d.jpg", cfg.Timelapse.Name, id)
			outputPath := filepath.Join("./timelapse", filename)
			if err := capturePhoto(outputPath); err != nil {
				log.Printf("timelapse capture failed: %v", err)
			}
		case <-stopChan:
			return
		}
	}
}

func startPackager(cfg Config, stopChan <-chan struct{}) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := packagePhotos(cfg.Timelapse.Name); err != nil {
				log.Printf("packaging failed: %v", err)
			}
		case <-stopChan:
			return
		}
	}
}

func main() {
	cfg, err := loadConfig("config.yaml")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	if cfg.Timelapse.Period <= 0 {
		log.Fatal("timelapse period must be positive")
	}
	if cfg.Timelapse.Name == "" {
		log.Fatal("timelapse name must not be empty")
	}

	stopChan := make(chan struct{})
	defer close(stopChan)

	go startTimelapse(cfg, stopChan)
	go startPackager(cfg, stopChan)

	app := fiber.New()

	app.Use("/static/*", static.New("./packages"))

	app.Get("/", func(c fiber.Ctx) error {
		return c.SendString(`<a href="/api/v1/preview">Preview</a><br><a href="/api/v1/photo">Photo</a><br><a href="/api/v1/timelapse">Timelapse Packages</a>`)
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

	app.Get("/api/v1/timelapse", func(c fiber.Ctx) error {
		packagesDir := "./packages"
		entries, err := os.ReadDir(packagesDir)
		if err != nil {
			return c.JSON(fiber.Map{"packages": []string{}})
		}

		var urls []string
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if strings.HasPrefix(name, "timelapse_") && strings.HasSuffix(name, ".tar.gz") {
				urls = append(urls, "/static/"+name)
			}
		}
		sort.Strings(urls)
		return c.JSON(fiber.Map{"packages": urls})
	})

	log.Fatal(app.Listen(":8080"))
}
