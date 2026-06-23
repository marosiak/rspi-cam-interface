package camera

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"
)

const maxReadings = 50

type CameraTimeReading struct {
	Duration  int64 `json:"duration"`
	Timestamp int64 `json:"timestamp"`
}

type CameraTimeStats struct {
	Avg       int64               `json:"avg"`
	Max       int64               `json:"max"`
	Min       int64               `json:"min"`
	Reads     []CameraTimeReading `json:"reads"`
	Timestamp int64               `json:"timestamp"`
}

type readingEntry struct {
	duration time.Duration
	time     time.Time
}

type StatsTracker struct {
	mu       sync.Mutex
	readings []readingEntry
	lastTime time.Time
}

func (st *StatsTracker) Record(d time.Duration) {
	st.mu.Lock()
	defer st.mu.Unlock()
	now := time.Now()
	st.readings = append(st.readings, readingEntry{duration: d, time: now})
	st.lastTime = now
	if len(st.readings) > maxReadings {
		st.readings = st.readings[len(st.readings)-maxReadings:]
	}
}

func (st *StatsTracker) Stats() CameraTimeStats {
	st.mu.Lock()
	defer st.mu.Unlock()

	if len(st.readings) == 0 {
		return CameraTimeStats{Avg: 0, Max: 0, Min: 0, Reads: []CameraTimeReading{}, Timestamp: st.lastTime.UnixMilli()}
	}

	var sum time.Duration
	max := st.readings[0].duration
	min := st.readings[0].duration
	reads := make([]CameraTimeReading, len(st.readings))

	for i, r := range st.readings {
		sum += r.duration
		if r.duration > max {
			max = r.duration
		}
		if r.duration < min {
			min = r.duration
		}
		reads[i] = CameraTimeReading{
			Duration:  r.duration.Milliseconds(),
			Timestamp: r.time.UnixMilli(),
		}
	}

	avg := sum / time.Duration(len(st.readings))
	return CameraTimeStats{
		Avg:       avg.Milliseconds(),
		Max:       max.Milliseconds(),
		Min:       min.Milliseconds(),
		Reads:     reads,
		Timestamp: st.lastTime.UnixMilli(),
	}
}

type Provider interface {
	Start() error
	Stop()
	LatestImage() ([]byte, error)
	SetArgs(args []string)
	SetRate(rate time.Duration)
	Stats() CameraTimeStats
}

type RspiCameraProvider struct {
	args     []string
	rate     time.Duration
	mu       sync.Mutex
	latest   []byte
	stopChan chan struct{}
	rateChan chan time.Duration
	stats    StatsTracker
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

	start := time.Now()
	cmd := exec.Command("rpicam-still", args...)
	data, err := cmd.Output()
	elapsed := time.Since(start)
	if err != nil {
		return
	}

	p.stats.Record(elapsed)

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

func (p *RspiCameraProvider) Stats() CameraTimeStats {
	return p.stats.Stats()
}

type MockCameraProvider struct {
	sourcePath string
	latest     []byte
	rate       time.Duration
	mu         sync.Mutex
	stopChan   chan struct{}
	rateChan   chan time.Duration
	stats      StatsTracker
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
	start := time.Now()
	data, err := os.ReadFile(p.sourcePath)
	elapsed := time.Since(start)
	if err != nil {
		return
	}

	p.stats.Record(elapsed)

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

func (p *MockCameraProvider) Stats() CameraTimeStats {
	return p.stats.Stats()
}
