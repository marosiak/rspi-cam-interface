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
	SetArgs(args []string)
	SetRate(rate time.Duration)
}

type RspiCameraProvider struct {
	tmpDir   string
	args     []string
	rate     time.Duration
	mu       sync.Mutex
	latest   string
	stopChan chan struct{}
	rateChan chan time.Duration
}

func (p *RspiCameraProvider) SetArgs(args []string) {
	p.mu.Lock()
	p.args = args
	p.mu.Unlock()
}

func (p *RspiCameraProvider) SetRate(rate time.Duration) {
	p.mu.Lock()
	p.rate = rate
	p.mu.Unlock()
	select {
	case p.rateChan <- rate:
	default:
	}
}

func NewRspiCameraProvider(baseArgs []string, rate time.Duration) *RspiCameraProvider {
	if rate <= 0 {
		rate = 1 * time.Second
	}
	return &RspiCameraProvider{
		tmpDir:   "./tmp",
		args:     baseArgs,
		rate:     rate,
		rateChan: make(chan time.Duration),
	}
}

func (p *RspiCameraProvider) Start() error {
	if err := os.MkdirAll(p.tmpDir, 0755); err != nil {
		return err
	}

	p.stopChan = make(chan struct{})
	go p.worker()

	return nil
}

func (p *RspiCameraProvider) Stop() {
	close(p.stopChan)
}

func (p *RspiCameraProvider) worker() {
	p.mu.Lock()
	rate := p.rate
	p.mu.Unlock()

	ticker := time.NewTicker(rate)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.capture()
		case newRate := <-p.rateChan:
			ticker.Stop()
			ticker = time.NewTicker(newRate)
		case <-p.stopChan:
			return
		}
	}
}

func (p *RspiCameraProvider) capture() {
	timestamp := time.Now().UnixNano()
	filename := fmt.Sprintf("%d.jpg", timestamp)
	outputPath := filepath.Join(p.tmpDir, filename)

	p.mu.Lock()
	args := append([]string{}, p.args...)
	p.mu.Unlock()
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

	type fileInfo struct {
		name    string
		modTime time.Time
	}
	var files []fileInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) != ".jpg" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, fileInfo{name: name, modTime: info.ModTime()})
	}

	if len(files) <= 10 {
		return
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.Before(files[j].modTime)
	})

	p.mu.Lock()
	latest := p.latest
	p.mu.Unlock()

	for _, f := range files[:len(files)-10] {
		path := filepath.Join(p.tmpDir, f.name)
		if path == latest {
			continue
		}
		os.Remove(path)
	}
}

func (p *RspiCameraProvider) LatestImage() ([]byte, error) {
	p.mu.Lock()
	latest := p.latest
	p.mu.Unlock()

	if latest == "" {
		return nil, fmt.Errorf("no image available yet")
	}

	data, err := os.ReadFile(latest)
	if err != nil {
		return nil, err
	}
	return data, nil
}

type MockCameraProvider struct {
	sourcePath string
	tmpDir     string
	rate       time.Duration
	mu         sync.Mutex
	latest     string
	stopChan   chan struct{}
	rateChan   chan time.Duration
}

func NewMockCameraProvider(rate time.Duration) *MockCameraProvider {
	if rate <= 0 {
		rate = 1 * time.Second
	}
	return &MockCameraProvider{
		sourcePath: "placeholder.jpg",
		tmpDir:     "./tmp",
		rate:       rate,
		rateChan:   make(chan time.Duration),
	}
}

func (p *MockCameraProvider) SetArgs(args []string) {}

func (p *MockCameraProvider) SetRate(rate time.Duration) {
	p.mu.Lock()
	p.rate = rate
	p.mu.Unlock()
	select {
	case p.rateChan <- rate:
	default:
	}
}

func (p *MockCameraProvider) Start() error {
	if _, err := os.Stat(p.sourcePath); err != nil {
		return fmt.Errorf("placeholder image not found: %w", err)
	}
	if err := os.MkdirAll(p.tmpDir, 0755); err != nil {
		return err
	}
	p.stopChan = make(chan struct{})
	go p.worker()
	return nil
}

func (p *MockCameraProvider) Stop() {
	close(p.stopChan)
}

func (p *MockCameraProvider) worker() {
	p.mu.Lock()
	rate := p.rate
	p.mu.Unlock()

	ticker := time.NewTicker(rate)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.capture()
		case newRate := <-p.rateChan:
			ticker.Stop()
			ticker = time.NewTicker(newRate)
		case <-p.stopChan:
			return
		}
	}
}

func (p *MockCameraProvider) capture() {
	timestamp := time.Now().UnixNano()
	filename := fmt.Sprintf("%d.jpg", timestamp)
	outputPath := filepath.Join(p.tmpDir, filename)

	data, err := os.ReadFile(p.sourcePath)
	if err != nil {
		return
	}
	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return
	}

	p.mu.Lock()
	p.latest = outputPath
	p.mu.Unlock()

	p.cleanup()
}

func (p *MockCameraProvider) cleanup() {
	entries, err := os.ReadDir(p.tmpDir)
	if err != nil {
		return
	}

	type fileInfo struct {
		name    string
		modTime time.Time
	}
	var files []fileInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) != ".jpg" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, fileInfo{name: name, modTime: info.ModTime()})
	}

	if len(files) <= 10 {
		return
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.Before(files[j].modTime)
	})

	p.mu.Lock()
	latest := p.latest
	p.mu.Unlock()

	for _, f := range files[:len(files)-10] {
		path := filepath.Join(p.tmpDir, f.name)
		if path == latest {
			continue
		}
		os.Remove(path)
	}
}

func (p *MockCameraProvider) LatestImage() ([]byte, error) {
	p.mu.Lock()
	latest := p.latest
	p.mu.Unlock()

	if latest == "" {
		return nil, fmt.Errorf("no image available yet")
	}

	data, err := os.ReadFile(latest)
	if err != nil {
		return nil, err
	}
	return data, nil
}
