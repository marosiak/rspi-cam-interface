package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"rspi-cam-interface/templates"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/template/html/v2"

	"github.com/gofiber/fiber/v3"
	"gopkg.in/yaml.v3"

	"rspi-cam-interface/pkg/camera"
)

type Duration time.Duration

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

func (d *Duration) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(dur)
	return nil
}

func (d Duration) MarshalYAML() (interface{}, error) {
	return time.Duration(d).String(), nil
}

func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	var s string
	if err := node.Decode(&s); err != nil {
		return err
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(dur)
	return nil
}

type Config struct {
	Timelapse struct {
		Period Duration `yaml:"period,omitempty" json:"period,omitempty"`
		Name   string   `yaml:"name,omitempty" json:"name,omitempty"`
	} `yaml:"timelapse,omitempty" json:"timelapse,omitempty"`
}

var (
	cfg           Config
	camCfg        camera.CameraConfig
	cfgPath       string
	cameraCfgPath string
	cfgMu         sync.RWMutex
)

func loadConfig(path string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	err = yaml.Unmarshal(data, &cfg)
	return cfg, err
}

func loadCameraConfig(path string) (camera.CameraConfig, error) {
	var wrapper struct {
		Camera camera.CameraConfig `yaml:"camera"`
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return wrapper.Camera, err
	}
	err = yaml.Unmarshal(data, &wrapper)
	return wrapper.Camera, err
}

func saveConfig(path string, cfg Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func saveCameraConfig(path string, cfg camera.CameraConfig) error {
	wrapper := struct {
		Camera camera.CameraConfig `yaml:"camera"`
	}{Camera: cfg}
	data, err := yaml.Marshal(wrapper)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
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

func startTimelapse(provider camera.Provider, cfg Config, stopChan <-chan struct{}) {
	ticker := time.NewTicker(time.Duration(cfg.Timelapse.Period))
	defer ticker.Stop()

	os.MkdirAll("./timelapse", 0755)

	for {
		select {
		case <-ticker.C:
			data, err := provider.LatestImage()
			if err != nil {
				log.Printf("timelapse failed to get latest image: %v", err)
				continue
			}
			id := time.Now().UnixNano()
			filename := fmt.Sprintf("%s_%d.jpg", cfg.Timelapse.Name, id)
			outputPath := filepath.Join("./timelapse", filename)
			if err := os.WriteFile(outputPath, data, 0644); err != nil {
				log.Printf("timelapse failed to write image: %v", err)
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
	flag.StringVar(&cfgPath, "cfg", "config.yaml", "path to config file")
	flag.StringVar(&cameraCfgPath, "camera-cfg", "camera.yaml", "path to camera config file")
	flag.Parse()

	var err error
	cfg, err = loadConfig(cfgPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	if cfg.Timelapse.Period <= 0 {
		log.Fatal("timelapse period must be positive")
	}
	if cfg.Timelapse.Name == "" {
		log.Fatal("timelapse name must not be empty")
	}

	if _, err := os.Stat(cameraCfgPath); err == nil {
		camCfg, err = loadCameraConfig(cameraCfgPath)
		if err != nil {
			log.Fatalf("failed to load camera config: %v", err)
		}
	}

	provider := camera.NewRspiCameraProvider(camera.ArgsFromConfig(camCfg))
	if err := provider.Start(); err != nil {
		log.Fatalf("failed to start camera provider: %v", err)
	}
	defer provider.Stop()

	stopChan := make(chan struct{})
	defer close(stopChan)

	go startTimelapse(provider, cfg, stopChan)
	go startPackager(cfg, stopChan)

	engine := html.NewFileSystem(http.FS(templates.FS), ".gohtml")
	app := fiber.New(fiber.Config{
		Views: engine,
	})

	app.Get("/static/*", func(c fiber.Ctx) error {
		path := c.Params("*")
		if strings.Contains(path, "..") {
			return c.Status(400).SendString("invalid path")
		}

		fullPath := filepath.Join("./packages", path)
		if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
			return c.SendFile(fullPath)
		}

		return c.Status(404).SendString("not found")
	})

	app.Get("/", func(c fiber.Ctx) error {
		return c.Render("home", fiber.Map{
			"Title": "Timelaps dashboard",
		})
	})

	app.Get("/api/v1/photo", func(c fiber.Ctx) error {
		data, err := provider.LatestImage()
		if err != nil {
			return c.Status(500).SendString(fmt.Sprintf("failed to get latest image: %v", err))
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

	app.Get("/api/v1/config", func(c fiber.Ctx) error {
		cfgMu.RLock()
		defer cfgMu.RUnlock()
		return c.JSON(fiber.Map{
			"config": cfg,
			"camera": camCfg,
		})
	})

	app.Post("/api/v1/config", func(c fiber.Ctx) error {
		var body struct {
			Config Config              `json:"config"`
			Camera camera.CameraConfig `json:"camera"`
		}
		if err := c.Bind().Body(&body); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("invalid body: %v", err)})
		}

		if body.Config.Timelapse.Period <= 0 {
			return c.Status(400).JSON(fiber.Map{"error": "timelapse period must be positive"})
		}
		if body.Config.Timelapse.Name == "" {
			return c.Status(400).JSON(fiber.Map{"error": "timelapse name must not be empty"})
		}

		cfgMu.Lock()
		defer cfgMu.Unlock()

		cfg = body.Config
		camCfg = body.Camera

		if err := saveConfig(cfgPath, cfg); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("failed to save config: %v", err)})
		}
		if err := saveCameraConfig(cameraCfgPath, camCfg); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("failed to save camera config: %v", err)})
		}

		provider.SetArgs(camera.ArgsFromConfig(camCfg))

		return c.JSON(fiber.Map{"status": "ok"})
	})

	log.Fatal(app.Listen(":80"))
}
