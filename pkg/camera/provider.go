package camera

import (
	"fmt"
	"os"
	"os/exec"
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
	args     []string
	rate     time.Duration
	mu       sync.Mutex
	latest   []byte
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
		args:     baseArgs,
		rate:     rate,
		rateChan: make(chan time.Duration),
	}
}

func (p *RspiCameraProvider) Start() error {
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
	p.mu.Lock()
	args := append([]string{}, p.args...)
	p.mu.Unlock()
	args = append(args, "--output", "-")

	cmd := exec.Command("rpicam-still", args...)
	data, err := cmd.Output()
	if err != nil {
		return
	}

	p.mu.Lock()
	p.latest = data
	p.mu.Unlock()
}

func (p *RspiCameraProvider) LatestImage() ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.latest == nil {
		return nil, fmt.Errorf("no image available yet")
	}

	data := make([]byte, len(p.latest))
	copy(data, p.latest)
	return data, nil
}

type MockCameraProvider struct {
	sourcePath string
	latest     []byte
	rate       time.Duration
	mu         sync.Mutex
	stopChan   chan struct{}
	rateChan   chan time.Duration
}

func NewMockCameraProvider(rate time.Duration) *MockCameraProvider {
	if rate <= 0 {
		rate = 1 * time.Second
	}
	return &MockCameraProvider{
		sourcePath: "placeholder.jpg",
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
	data, err := os.ReadFile(p.sourcePath)
	if err != nil {
		return
	}

	p.mu.Lock()
	p.latest = data
	p.mu.Unlock()
}

func (p *MockCameraProvider) LatestImage() ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.latest == nil {
		return nil, fmt.Errorf("no image available yet")
	}

	data := make([]byte, len(p.latest))
	copy(data, p.latest)
	return data, nil
}
