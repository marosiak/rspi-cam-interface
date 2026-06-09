package main

import (
	"archive/tar"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Timelapse struct {
		Period time.Duration `yaml:"period"`
		Name   string        `yaml:"name"`
	} `yaml:"timelapse"`
	Camera struct {
		VFlip bool `yaml:"vflip"`
		HFlip bool `yaml:"hflip"`
	} `yaml:"camera"`
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

func cameraArgs(cfg Config, base ...string) []string {
	args := append([]string{}, base...)
	if cfg.Camera.VFlip {
		args = append(args, "--vflip")
	}
	if cfg.Camera.HFlip {
		args = append(args, "--hflip")
	}
	return args
}

func capturePhoto(cfg Config, outputPath string) error {
	args := cameraArgs(cfg, "--output", outputPath)
	cmd := exec.Command("rpicam-jpeg", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", cmd.String(), err)
	}
	return nil
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
			if err := capturePhoto(cfg, outputPath); err != nil {
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
	var cfgPath string
	flag.StringVar(&cfgPath, "cfg", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := loadConfig(cfgPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	if cfg.Timelapse.Period <= 0 {
		log.Fatal("timelapse period must be positive")
	}
	if cfg.Timelapse.Name == "" {
		log.Fatal("timelapse name must not be empty")
	}

	os.MkdirAll("./videos", 0755)

	stopChan := make(chan struct{})
	defer close(stopChan)

	go startTimelapse(cfg, stopChan)
	go startPackager(cfg, stopChan)

	app := fiber.New()

	app.Get("/static/*", func(c fiber.Ctx) error {
		path := c.Params("*")
		if strings.Contains(path, "..") {
			return c.Status(400).SendString("invalid path")
		}

		fullPath := filepath.Join("./packages", path)
		if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
			return c.SendFile(fullPath)
		}

		fullPath = filepath.Join("./videos", path)
		if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
			return c.SendFile(fullPath)
		}

		return c.Status(404).SendString("not found")
	})

	app.Get("/", func(c fiber.Ctx) error {
		return c.SendString(`<a href="/api/v1/preview">Preview</a><br><a href="/api/v1/photo">Photo</a><br><a href="/api/v1/timelapse">Timelapse Packages</a><br><a href="/api/v1/video?t=10">Video (10s)</a><br><a href="/api/v1/videos">Videos</a>`)
	})

	app.Get("/api/v1/preview", func(c fiber.Ctx) error {
		id := fmt.Sprintf("%d", time.Now().UnixNano())
		filename := fmt.Sprintf("preview_%s.jpg", id)

		args := cameraArgs(cfg, "--output", filename, "--timeout", "2000", "--width", "640", "--height", "480")
		cmd := exec.Command("rpicam-jpeg", args...)
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

		args := cameraArgs(cfg, "--output", filename)
		cmd := exec.Command("rpicam-jpeg", args...)
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

	app.Get("/api/v1/video", func(c fiber.Ctx) error {
		tStr := c.Query("t", "10")
		t, err := strconv.Atoi(tStr)
		if err != nil || t <= 0 {
			return c.Status(400).SendString("invalid duration")
		}

		id := fmt.Sprintf("%d", time.Now().UnixNano())
		filename := fmt.Sprintf("video_%s.mp4", id)

		async := c.Query("async", "false") == "true"

		var outputPath string
		if async {
			outputPath = filepath.Join("./videos", filename)
		} else {
			outputPath = filename
		}

		cmd := exec.Command("rpicam-vid", "-t", fmt.Sprintf("%d", t), "--codec", "libav", "-o", outputPath)
		if cfg.Camera.VFlip {
			cmd.Args = append(cmd.Args, "--vflip")
		}
		if cfg.Camera.HFlip {
			cmd.Args = append(cmd.Args, "--hflip")
		}
		if err := cmd.Run(); err != nil {
			return c.Status(500).SendString(fmt.Sprintf("video capture failed: %v", err))
		}

		if async {
			c.Set("Content-Type", "application/json")
			return c.JSON(fiber.Map{"url": "/static/" + filename})
		}

		defer os.Remove(filename)

		data, err := os.ReadFile(filename)
		if err != nil {
			return c.Status(500).SendString(fmt.Sprintf("failed to read video: %v", err))
		}

		c.Set("Content-Type", "video/mp4")
		return c.Send(data)
	})

	app.Get("/api/v1/videos", func(c fiber.Ctx) error {
		videosDir := "./videos"
		entries, err := os.ReadDir(videosDir)
		if err != nil {
			return c.JSON(fiber.Map{"videos": []string{}})
		}

		var urls []string
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if strings.HasSuffix(name, ".mp4") {
				urls = append(urls, "/static/"+name)
			}
		}
		sort.Strings(urls)
		return c.JSON(fiber.Map{"videos": urls})
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

	log.Fatal(app.Listen(":80"))
}
