package camera

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type Provider interface {
	Start() error
	Stop()
	LatestImage() ([]byte, error)
}

type RspiCameraProvider struct {
	tmpDir   string
	args     []string
	mu       sync.Mutex
	latest   string
	stopChan chan struct{}
	ticker   *time.Ticker
}

func NewRspiCameraProvider(baseArgs []string) *RspiCameraProvider {
	return &RspiCameraProvider{
		tmpDir: "./tmp",
		args:   baseArgs,
	}
}

func (p *RspiCameraProvider) Start() error {
	if err := os.MkdirAll(p.tmpDir, 0755); err != nil {
		return err
	}

	p.stopChan = make(chan struct{})
	p.ticker = time.NewTicker(1 * time.Second)

	go p.worker()

	return nil
}

func (p *RspiCameraProvider) Stop() {
	if p.ticker != nil {
		p.ticker.Stop()
	}
	close(p.stopChan)
}

func (p *RspiCameraProvider) worker() {
	for {
		select {
		case <-p.ticker.C:
			p.capture()
		case <-p.stopChan:
			return
		}
	}
}

func (p *RspiCameraProvider) capture() {
	timestamp := time.Now().UnixNano()
	filename := fmt.Sprintf("%d.jpg", timestamp)
	outputPath := filepath.Join(p.tmpDir, filename)

	args := append([]string{}, p.args...)
	args = append(args, "--output", outputPath)

	cmd := exec.Command("rpicam-jpeg", args...)
	if err := cmd.Run(); err != nil {
		return
	}

	p.mu.Lock()
	p.latest = outputPath
	p.mu.Unlock()

	p.cleanup()
}

func (p *RspiCameraProvider) cleanup() {
	entries, err := os.ReadDir(p.tmpDir)
	if err != nil {
		return
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) == ".jpg" {
			files = append(files, name)
		}
	}

	if len(files) <= 10 {
		return
	}

	sort.Strings(files)

	for _, f := range files[:len(files)-10] {
		os.Remove(filepath.Join(p.tmpDir, f))
	}
}

func (p *RspiCameraProvider) LatestImage() ([]byte, error) {
	p.mu.Lock()
	latest := p.latest
	p.mu.Unlock()

	if latest == "" {
		return nil, fmt.Errorf("no image available yet")
	}

	return os.ReadFile(latest)
}
