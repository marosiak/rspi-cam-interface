package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"rspi-cam-interface/templates"

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
		Period  Duration `yaml:"period,omitempty" json:"period,omitempty"`
		Name    string   `yaml:"name,omitempty" json:"name,omitempty"`
		Counter int      `yaml:"counter,omitempty" json:"counter,omitempty"`
	} `yaml:"timelapse,omitempty" json:"timelapse,omitempty"`
	CameraRefreshRate Duration `yaml:"camera_refresh_rate,omitempty" json:"camera_refresh_rate,omitempty"`
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
	return os.WriteFile(path, data, 0o644)
}

func saveCameraConfig(path string, cfg camera.CameraConfig) error {
	wrapper := struct {
		Camera camera.CameraConfig `yaml:"camera"`
	}{Camera: cfg}
	data, err := yaml.Marshal(wrapper)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
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

	if err := os.MkdirAll(packagesDir, 0o755); err != nil {
		return err
	}

	entries, err := os.ReadDir(timelapseDir)
	if err != nil {
		return err
	}

	var photos []string
	prefix := timelapseName + "_"
	now := time.Now()
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".jpg") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		isOld := info.ModTime().Before(now.Add(-5 * time.Second))
		isFuture := info.ModTime().After(now)
		if strings.HasPrefix(name, prefix) && (isOld || isFuture) {
			photos = append(photos, filepath.Join(timelapseDir, name))
		} else if !strings.HasPrefix(name, prefix) && isOld {
			os.Remove(filepath.Join(timelapseDir, name))
		}
	}

	if len(photos) == 0 {
		return nil
	}

	sort.Strings(photos)

	nextNum := nextPackageNumber(packagesDir, timelapseName)

	const batchSize = 5
	var packaged []string

	for i := 0; i < len(photos); i += batchSize {
		end := i + batchSize
		if end > len(photos) {
			end = len(photos)
		}
		batch := photos[i:end]

		packageName := fmt.Sprintf("timelapse_%s_%06d.tar.gz", timelapseName, nextNum)
		packagePath := filepath.Join(packagesDir, packageName)

		file, err := os.Create(packagePath)
		if err != nil {
			return err
		}

		gzw := gzip.NewWriter(file)
		tw := tar.NewWriter(gzw)

		success := true
		for _, photo := range batch {
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

		if err := tw.Close(); err != nil {
			success = false
		}
		if err := gzw.Close(); err != nil {
			success = false
		}
		if err := file.Close(); err != nil {
			success = false
		}

		if success {
			packaged = append(packaged, batch...)
			nextNum++
		} else {
			os.Remove(packagePath)
			return fmt.Errorf("failed to package one or more photos")
		}
	}

	for _, photo := range packaged {
		os.Remove(photo)
	}

	return nil
}

func startTimelapse(provider camera.Provider, stopChan <-chan struct{}) {
	cfgMu.RLock()
	period := time.Duration(cfg.Timelapse.Period)
	cfgMu.RUnlock()

	ticker := time.NewTicker(period)
	defer ticker.Stop()

	os.MkdirAll("./timelapse", 0o755)

	for {
		select {
		case <-ticker.C:
			data, err := provider.LatestImage()
			if err != nil {
				log.Printf("timelapse failed to get latest image: %v", err)
				continue
			}
			cfgMu.Lock()
			name := cfg.Timelapse.Name
			counter := cfg.Timelapse.Counter
			cfg.Timelapse.Counter++
			if err := saveConfig(cfgPath, cfg); err != nil {
				log.Printf("timelapse failed to save config: %v", err)
			}
			cfgMu.Unlock()
			filename := fmt.Sprintf("%s_%d.jpg", name, counter)
			outputPath := filepath.Join("./timelapse", filename)
			if err := os.WriteFile(outputPath, data, 0o644); err != nil {
				log.Printf("timelapse failed to write image: %v", err)
			}
		case <-stopChan:
			return
		}
	}
}

func startPackager(stopChan <-chan struct{}) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cfgMu.RLock()
			name := cfg.Timelapse.Name
			cfgMu.RUnlock()
			if err := packagePhotos(name); err != nil {
				log.Printf("packaging failed: %v", err)
			}
		case <-stopChan:
			return
		}
	}
}

func parseTimelapseName(packageName string) (string, bool) {
	if !strings.HasPrefix(packageName, "timelapse_") || !strings.HasSuffix(packageName, ".tar.gz") {
		return "", false
	}
	trimmed := strings.TrimPrefix(packageName, "timelapse_")
	trimmed = strings.TrimSuffix(trimmed, ".tar.gz")
	// trimmed is now "name_NN" where NN is a number; we need to strip the last _NN
	lastUnderscore := strings.LastIndex(trimmed, "_")
	if lastUnderscore <= 0 {
		return "", false
	}
	name := trimmed[:lastUnderscore]
	return name, true
}

type TimelapseGroup struct {
	Name            string        `json:"name"`
	PackageCount    int           `json:"package_count"`
	TotalSize       int64         `json:"total_size"`
	TotalSizeStr    string        `json:"total_size_str"`
	EarliestTime    time.Time     `json:"earliest_time"`
	LatestTime      time.Time     `json:"latest_time"`
	EarliestTimeStr string        `json:"earliest_time_str"`
	LatestTimeStr   string        `json:"latest_time_str"`
	Duration        time.Duration `json:"duration"`
	DurationStr     string        `json:"duration_str"`
}

func listTimelapseGroups(sortBy string) ([]TimelapseGroup, error) {
	packagesDir := "./packages"
	entries, err := os.ReadDir(packagesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []TimelapseGroup{}, nil
		}
		return nil, err
	}

	groups := make(map[string]*TimelapseGroup)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		timelapseName, ok := parseTimelapseName(name)
		if !ok {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		g, exists := groups[timelapseName]
		if !exists {
			g = &TimelapseGroup{
				Name:         timelapseName,
				EarliestTime: info.ModTime(),
				LatestTime:   info.ModTime(),
			}
			groups[timelapseName] = g
		}
		g.PackageCount++
		g.TotalSize += info.Size()
		if info.ModTime().Before(g.EarliestTime) {
			g.EarliestTime = info.ModTime()
		}
		if info.ModTime().After(g.LatestTime) {
			g.LatestTime = info.ModTime()
		}
	}

	result := make([]TimelapseGroup, 0, len(groups))
	for _, g := range groups {
		g.TotalSizeStr = formatBytes(g.TotalSize)
		g.EarliestTimeStr = g.EarliestTime.Format("2006-01-02 15:04")
		g.LatestTimeStr = g.LatestTime.Format("2006-01-02 15:04")
		g.Duration = g.LatestTime.Sub(g.EarliestTime)
		g.DurationStr = formatDuration(g.Duration)
		result = append(result, *g)
	}

	switch sortBy {
	case "latest":
		sort.Slice(result, func(i, j int) bool {
			return result[i].LatestTime.After(result[j].LatestTime)
		})
	default:
		sort.Slice(result, func(i, j int) bool {
			return result[i].Duration > result[j].Duration
		})
	}
	return result, nil
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	if d < time.Hour {
		m := int(d.Minutes())
		s := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm %ds", m, s)
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh %dm", h, m)
}

func deleteTimelapse(name string) (int, error) {
	packagesDir := "./packages"
	entries, err := os.ReadDir(packagesDir)
	if err != nil {
		return 0, err
	}

	deleted := 0
	prefix := fmt.Sprintf("timelapse_%s_", name)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		fname := entry.Name()
		if strings.HasPrefix(fname, prefix) && strings.HasSuffix(fname, ".tar.gz") {
			if err := os.Remove(filepath.Join(packagesDir, fname)); err == nil {
				deleted++
			}
		}
	}
	return deleted, nil
}

func main() {
	mock := flag.Bool("mock", false, "use mock camera provider with placeholder.jpg")
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
	if cfg.CameraRefreshRate <= 0 {
		slog.Warn("camera refresh rate must be positive, defaulting to 10s")
		cfg.CameraRefreshRate = Duration(time.Second * 10)
	}

	if _, err := os.Stat(cameraCfgPath); err == nil {
		camCfg, err = loadCameraConfig(cameraCfgPath)
		if err != nil {
			log.Fatalf("failed to load camera config: %v", err)
		}
	}

	var provider camera.Provider
	if *mock {
		provider = camera.NewMockCameraProvider(time.Duration(cfg.CameraRefreshRate))
	} else {
		provider = camera.NewRspiCameraProvider(camera.ArgsFromConfig(camCfg), time.Duration(cfg.CameraRefreshRate))
	}
	if err := provider.Start(); err != nil {
		log.Fatalf("failed to start camera provider: %v", err)
	}
	defer provider.Stop()

	stopChan := make(chan struct{})
	defer close(stopChan)

	go startTimelapse(provider, stopChan)
	go startPackager(stopChan)

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

	app.Get("/api/v1/health", func(c fiber.Ctx) error {
		return c.SendString("OK")
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
		if body.Config.CameraRefreshRate <= 0 {
			return c.Status(400).JSON(fiber.Map{"error": "camera refresh rate must be positive"})
		}

		cfgMu.Lock()
		defer cfgMu.Unlock()

		oldCounter := cfg.Timelapse.Counter
		cfg = body.Config
		cfg.Timelapse.Counter = oldCounter
		camCfg = body.Camera

		if err := saveConfig(cfgPath, cfg); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("failed to save config: %v", err)})
		}
		if err := saveCameraConfig(cameraCfgPath, camCfg); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("failed to save camera config: %v", err)})
		}

		provider.SetArgs(camera.ArgsFromConfig(camCfg))
		provider.SetRate(time.Duration(body.Config.CameraRefreshRate))

		return c.JSON(fiber.Map{"status": "ok"})
	})

	app.Get("/api/v1/timelapse/groups", func(c fiber.Ctx) error {
		sortBy := c.Query("sort", "duration")
		if sortBy != "duration" && sortBy != "latest" {
			sortBy = "duration"
		}
		groups, err := listTimelapseGroups(sortBy)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("failed to list groups: %v", err)})
		}
		return c.JSON(fiber.Map{"groups": groups})
	})

	app.Delete("/api/v1/timelapse/:name", func(c fiber.Ctx) error {
		name := c.Params("name")
		if name == "" {
			return c.Status(400).JSON(fiber.Map{"error": "name is required"})
		}
		deleted, err := deleteTimelapse(name)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("failed to delete: %v", err)})
		}
		return c.JSON(fiber.Map{"deleted": deleted})
	})

	log.Fatal(app.Listen(":80"))
}
