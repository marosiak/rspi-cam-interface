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
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

type State struct {
	Packages map[string]bool `json:"packages"`
}

func loadState(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &State{Packages: make(map[string]bool)}, nil
		}
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	if s.Packages == nil {
		s.Packages = make(map[string]bool)
	}
	return &s, nil
}

func (s *State) save(path string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// --- Model ---

type model struct {
	server     string
	outputDir  string
	workDir    string
	keep       bool
	stateFile  string
	state      *State
	groups     map[string][]string

	// Processing
	spinner        spinner.Model
	progressModel  progress.Model
	selectedGroups []string
	fps            int
	processingIdx  int
	doneGroups     []string
	errGroups      map[string]error
	currentGroup   string
	progressCh     chan tea.Msg
	progressState  map[string]progressInfo
	groupStartTime time.Time
}

type progressInfo struct {
	stage       string
	current     int
	total       int
	description string
}

type processProgressMsg struct {
	group       string
	stage       string
	current     int
	total       int
	description string
}

type processDoneMsg struct {
	group string
	err   error
}

// --- Init ---

type startProcessingMsg struct{}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, func() tea.Msg { return startProcessingMsg{} })
}

// --- Update ---

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.progressModel.SetWidth(msg.Width - 4)
		return m, nil

	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			return m, tea.Quit
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
		m.progressModel, cmd = m.progressModel.Update(msg)
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)

	case startProcessingMsg:
		var cmd tea.Cmd
		m, cmd = m.processNext()
		cmds = append(cmds, cmd)

	case processProgressMsg:
		m.progressState[msg.group] = progressInfo{
			stage:       msg.stage,
			current:     msg.current,
			total:       msg.total,
			description: msg.description,
		}
		if m.progressCh != nil {
			return m, func() tea.Msg {
				return <-m.progressCh
			}
		}
		return m, nil

	case processDoneMsg:
		m.processingIdx++
		if msg.err != nil {
			m.errGroups[msg.group] = msg.err
		} else {
			m.doneGroups = append(m.doneGroups, msg.group)
		}
		if m.processingIdx < len(m.selectedGroups) {
			m.currentGroup = m.selectedGroups[m.processingIdx]
		} else {
			m.currentGroup = ""
		}
		var cmd tea.Cmd
		m, cmd = m.processNext()
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)
	}

	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	cmds = append(cmds, cmd)
	m.progressModel, cmd = m.progressModel.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func (m model) processNext() (model, tea.Cmd) {
	if m.processingIdx >= len(m.selectedGroups) {
		return m, nil
	}
	group := m.selectedGroups[m.processingIdx]

	m.progressCh = make(chan tea.Msg, 10)
	m.groupStartTime = time.Now()
	m.currentGroup = group

	go func() {
		err := processGroupWorkWithProgress(m.server, m.outputDir, m.workDir, m.keep, m.state, m.stateFile, group, m.groups[group], m.fps, func(stage string, current, total int, desc string) {
			m.progressCh <- processProgressMsg{
				group:       group,
				stage:       stage,
				current:     current,
				total:       total,
				description: desc,
			}
		})
		m.progressCh <- processDoneMsg{group: group, err: err}
	}()

	return m, func() tea.Msg {
		return <-m.progressCh
	}
}

// --- View ---

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#EE6FF8"))
	subtitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00B4D8"))
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#777777"))
	successStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#04B575"))
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B"))
)

func stageIcon(stage string) string {
	switch stage {
	case "download":
		return "↓"
	case "extract":
		return "📦"
	case "encode":
		return "🎬"
	default:
		return "•"
	}
}

func humanizeBytes(b int64) string {
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

func (m model) View() tea.View {
	var s strings.Builder
	s.WriteString(titleStyle.Render("Processing timelapse videos") + "\n\n")

	// Overall progress
	totalGroups := len(m.selectedGroups)
	doneGroups := len(m.doneGroups)
	if totalGroups > 0 {
		s.WriteString(subtitleStyle.Render("Overall Progress") + "\n")
		s.WriteString(m.progressModel.ViewAs(float64(doneGroups) / float64(totalGroups)) + "\n")
		s.WriteString(fmt.Sprintf("Groups: %d/%d completed", doneGroups, totalGroups) + "\n")
	}

	if m.currentGroup != "" {
		info := m.progressState[m.currentGroup]
		s.WriteString("\n" + subtitleStyle.Render("Current: "+m.currentGroup) + "\n")

		if info.description != "" {
			s.WriteString(info.description + "\n")
		}

		if info.total > 0 {
			progress := float64(info.current) / float64(info.total)
			s.WriteString(m.progressModel.ViewAs(progress) + "\n")
			s.WriteString(fmt.Sprintf("%s %d/%d", stageIcon(info.stage), info.current, info.total) + "\n")
		}

		elapsed := time.Since(m.groupStartTime)
		s.WriteString(fmt.Sprintf("Elapsed: %s", elapsed.Round(time.Second)))
		if info.total > info.current && info.current > 0 {
			rate := elapsed / time.Duration(info.current)
			remaining := time.Duration(info.total-info.current) * rate
			s.WriteString(fmt.Sprintf(" • ETA: %s", remaining.Round(time.Second)))
		}
		s.WriteString("\n")

		s.WriteString("\n" + m.spinner.View() + "\n")
	} else if totalGroups > 0 {
		s.WriteString("\n" + successStyle.Render("All done!") + "\n")
	}

	if len(m.errGroups) > 0 {
		s.WriteString("\nErrors:\n")
		for _, group := range m.selectedGroups {
			if err, ok := m.errGroups[group]; ok {
				s.WriteString(errorStyle.Render(fmt.Sprintf("  • %s: %v", group, err)) + "\n")
			}
		}
	}

	if m.processingIdx >= len(m.selectedGroups) && len(m.selectedGroups) > 0 {
		s.WriteString("\n" + helpStyle.Render("All done! Press q to quit."))
	} else {
		s.WriteString("\n" + helpStyle.Render("Press q to quit."))
	}

	v := tea.NewView(s.String())
	v.AltScreen = true
	return v
}

// --- Main ---

func main() {
	serverURL := flag.String("server", "http://raspberrypi.local/", "Server base URL")
	outputDir := flag.String("output-dir", ".", "Output directory for generated videos")
	workDir := flag.String("work-dir", "./timelapse_work", "Working directory for downloads and frames")
	fpsFlag := flag.Int("fps", 60, "Frames per second for output video")
	keep := flag.Bool("keep", false, "Keep working directory after encoding")
	stateFile := flag.String("state", "timelapse_state.json", "Path to state file tracking downloaded packages")
	flag.Parse()

	server := strings.TrimSuffix(*serverURL, "/")

	if err := os.MkdirAll(*workDir, 0o755); err != nil {
		log.Fatalf("failed to create work dir: %v", err)
	}

	state, err := loadState(*stateFile)
	if err != nil {
		log.Fatalf("failed to load state: %v", err)
	}

	packages, err := fetchPackageList(server)
	if err != nil {
		log.Fatalf("failed to fetch package list: %v", err)
	}
	if len(packages) == 0 {
		fmt.Println("no packages found")
		return
	}

	groups := groupPackages(packages)

	groupNames := make([]string, 0, len(groups))
	for name := range groups {
		groupNames = append(groupNames, name)
	}
	sort.Strings(groupNames)

	// Redirect logs to file so they don't interfere with TUI
	logFile, err := os.OpenFile("client.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		log.Fatalf("failed to open log file: %v", err)
	}
	defer logFile.Close()
	log.SetOutput(logFile)

	// Build group options for huh
	groupOptions := make([]huh.Option[string], 0, len(groupNames))
	for _, name := range groupNames {
		count := len(groups[name])
		newCount := 0
		for _, pkg := range groups[name] {
			if !state.Packages[pkg] {
				newCount++
			}
		}
		var label string
		if newCount == 0 {
			label = fmt.Sprintf("%s (%d packages, all done)", name, count)
		} else {
			label = fmt.Sprintf("%s (%d packages, %d new)", name, count, newCount)
		}
		opt := huh.NewOption(label, name).Selected(newCount > 0)
		groupOptions = append(groupOptions, opt)
	}

	var selectedGroups []string
	var selectedFPS int

	// Custom theme: use a filled circle for all cursors so MultiSelect and Select match
	customTheme := huh.ThemeFunc(func(isDark bool) *huh.Styles {
		s := huh.ThemeCharm(isDark)
		s.Focused.SelectSelector = s.Focused.SelectSelector.SetString("● ")
		s.Focused.MultiSelectSelector = s.Focused.MultiSelectSelector.SetString("● ")
		s.Blurred.SelectSelector = s.Blurred.SelectSelector.SetString("● ")
		return s
	})

	fpsOptions := []huh.Option[int]{
		huh.NewOption("30 FPS", 30),
		huh.NewOption("60 FPS (default)", 60),
		huh.NewOption("120 FPS", 120),
		huh.NewOption("240 FPS", 240),
	}

	// Huh form for group selection and FPS
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Key("groups").
				Title("Select timelapse groups to process").
				Description("Space to toggle, ↑↓ to navigate, enter to confirm").
				Options(groupOptions...).
				Filterable(true).
				Height(len(groupOptions) + 2).
				Value(&selectedGroups),
		),
		huh.NewGroup(
			huh.NewSelect[int]().
				Key("fps").
				Title("Select FPS for output video").
				Description("↑↓ to navigate, enter to confirm").
				Options(fpsOptions...).
				Height(len(fpsOptions) + 2).
				Value(&selectedFPS),
		),
	).WithTheme(customTheme)

	// Set default FPS
	selectedFPS = *fpsFlag

	if err := form.Run(); err != nil {
		if err == huh.ErrUserAborted {
			fmt.Println("Aborted.")
			return
		}
		log.Fatalf("form error: %v", err)
	}

	// Validate at least one group selected
	if len(selectedGroups) == 0 {
		fmt.Println("no groups selected")
		return
	}

	// Progress
	progressModel := progress.New(progress.WithDefaultBlend(), progress.WithWidth(40))

	// Spinner
	sp := spinner.New()
	sp.Spinner = spinner.Dot

	m := model{
		server:         server,
		outputDir:      *outputDir,
		workDir:        *workDir,
		keep:           *keep,
		stateFile:      *stateFile,
		state:          state,
		groups:         groups,
		selectedGroups: selectedGroups,
		fps:            selectedFPS,
		progressModel:  progressModel,
		spinner:        sp,
		progressState:  make(map[string]progressInfo),
		errGroups:      make(map[string]error),
	}
	if len(m.selectedGroups) > 0 {
		m.currentGroup = m.selectedGroups[0]
	}

	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
		os.Exit(1)
	}

	if !*keep {
		if err := os.RemoveAll(*workDir); err != nil {
			log.Printf("failed to clean up work dir: %v", err)
		}
	}
}

// --- Work functions ---

func processGroupWorkWithProgress(server, outputDir, workDir string, keep bool, state *State, stateFile, group string, packages []string, fps int, sendProgress func(stage string, current, total int, desc string)) error {
	framesDir := filepath.Join(workDir, "frames", group)
	if err := os.MkdirAll(framesDir, 0o755); err != nil {
		return fmt.Errorf("failed to create frames dir: %w", err)
	}

	newPackages := make([]string, 0, len(packages))
	for _, pkg := range packages {
		if !state.Packages[pkg] {
			newPackages = append(newPackages, pkg)
		}
	}

	outputPath := filepath.Join(outputDir, group+".mp4")

	if len(newPackages) == 0 {
		entries, err := os.ReadDir(framesDir)
		if err != nil || len(entries) == 0 {
			return fmt.Errorf("no frames for %s, skipping encoding", group)
		}
		if _, err := os.Stat(outputPath); err == nil {
			return nil
		}
	}

	var newFramePaths []string
	for i, pkg := range newPackages {
		filename := path.Base(pkg)

		lastUpdate := time.Now()
		sendProgress("download", i, len(newPackages), fmt.Sprintf("Downloading %s...", filename))
		archivePath, err := downloadFile(server, pkg, workDir, func(downloaded, total int64) {
			if time.Since(lastUpdate) > 200*time.Millisecond {
				if total > 0 {
					pct := float64(downloaded) / float64(total) * 100
					sendProgress("download", i, len(newPackages), fmt.Sprintf("Downloading %s (%.0f%%)", filename, pct))
				} else {
					sendProgress("download", i, len(newPackages), fmt.Sprintf("Downloading %s (%s)", filename, humanizeBytes(downloaded)))
				}
				lastUpdate = time.Now()
			}
		})
		if err != nil {
			return fmt.Errorf("failed to download %s: %w", pkg, err)
		}
		sendProgress("download", i+1, len(newPackages), fmt.Sprintf("Downloaded %s", filename))

		sendProgress("extract", i, len(newPackages), fmt.Sprintf("Extracting %s...", filename))
		extracted, err := extractTarGz(archivePath, framesDir)
		if err != nil {
			os.Remove(archivePath)
			return fmt.Errorf("failed to extract %s: %w", pkg, err)
		}
		os.Remove(archivePath)
		newFramePaths = append(newFramePaths, extracted...)
		sendProgress("extract", i+1, len(newPackages), fmt.Sprintf("Extracted %s", filename))

		state.Packages[pkg] = true
		if err := state.save(stateFile); err != nil {
			return fmt.Errorf("failed to save state: %w", err)
		}
	}

	// Count frames
	entries, err := os.ReadDir(framesDir)
	if err != nil {
		return err
	}
	frameCount := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			name := entry.Name()
			if strings.HasSuffix(strings.ToLower(name), ".jpg") || strings.HasSuffix(strings.ToLower(name), ".jpeg") {
				frameCount++
			}
		}
	}
	if frameCount == 0 {
		return fmt.Errorf("no frames found in %s", framesDir)
	}

	// Sort new frame paths to ensure chronological order across packages
	sort.Strings(newFramePaths)

	if _, err := os.Stat(outputPath); err == nil && len(newFramePaths) > 0 {
		sendProgress("encode", 0, 1, fmt.Sprintf("Encoding %d new frames at %d fps...", len(newFramePaths), fps))
		tmpOutput := outputPath + ".tmp.mp4"
		if err := encodeFrames(newFramePaths, tmpOutput, fps, func(current, total int) {
			sendProgress("encode", 0, 1, fmt.Sprintf("Encoding %d new frames at %d fps... (preparing %d/%d)", len(newFramePaths), fps, current, total))
		}); err != nil {
			os.Remove(tmpOutput)
			return fmt.Errorf("failed to encode new frames for %s: %w", group, err)
		}
		sendProgress("encode", 0, 1, fmt.Sprintf("Appending to existing %s...", outputPath))
		concatOutput := outputPath + ".concat.mp4"
		if err := concatVideos(outputPath, tmpOutput, concatOutput); err != nil {
			os.Remove(tmpOutput)
			os.Remove(concatOutput)
			return fmt.Errorf("failed to concatenate video for %s: %w", group, err)
		}
		os.Remove(tmpOutput)
		if err := os.Rename(concatOutput, outputPath); err != nil {
			os.Remove(concatOutput)
			return fmt.Errorf("failed to replace output video: %w", err)
		}
		sendProgress("encode", 1, 1, fmt.Sprintf("Appended to %s", outputPath))
	} else {
		sendProgress("encode", 0, 1, fmt.Sprintf("Encoding %d frames at %d fps...", frameCount, fps))
		if err := encodeVideo(framesDir, outputPath, fps, func(current, total int) {
			sendProgress("encode", 0, 1, fmt.Sprintf("Encoding %d frames at %d fps... (preparing %d/%d)", frameCount, fps, current, total))
		}); err != nil {
			return fmt.Errorf("failed to encode video for %s: %w", group, err)
		}
		sendProgress("encode", 1, 1, fmt.Sprintf("Encoded %s", outputPath))
	}
	log.Printf("video saved to %s", outputPath)
	return nil
}

func groupPackages(packages []string) map[string][]string {
	groups := make(map[string][]string)
	for _, pkg := range packages {
		group := extractGroup(pkg)
		groups[group] = append(groups[group], pkg)
	}
	for group := range groups {
		sort.Strings(groups[group])
	}
	return groups
}

func extractGroup(pkgPath string) string {
	basename := path.Base(pkgPath)
	if !strings.HasPrefix(basename, "timelapse_") || !strings.HasSuffix(basename, ".tar.gz") {
		return "timelapse"
	}
	stripped := strings.TrimPrefix(basename, "timelapse_")
	stripped = strings.TrimSuffix(stripped, ".tar.gz")
	parts := strings.Split(stripped, "_")
	if len(parts) < 2 {
		return stripped
	}
	return strings.Join(parts[:len(parts)-1], "_")
}

func fetchPackageList(server string) ([]string, error) {
	url := server + "/api/v1/timelapse"
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var result struct {
		Packages []string `json:"packages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Packages, nil
}

func downloadFile(server, pkgPath, workDir string, onProgress func(downloaded, total int64)) (string, error) {
	filename := path.Base(pkgPath)
	archivePath := filepath.Join(workDir, filename)

	url := server + pkgPath
	log.Printf("downloading %s", url)

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download status: %s", resp.Status)
	}

	total := resp.ContentLength

	f, err := os.Create(archivePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var downloaded int64
	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			_, werr := f.Write(buf[:n])
			if werr != nil {
				return "", werr
			}
			downloaded += int64(n)
			if onProgress != nil {
				onProgress(downloaded, total)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
	}

	return archivePath, nil
}

func downloadAndExtract(server, pkgPath, workDir, framesDir string) ([]string, error) {
	archivePath, err := downloadFile(server, pkgPath, workDir, nil)
	if err != nil {
		return nil, err
	}

	log.Printf("extracting %s", path.Base(pkgPath))
	extracted, err := extractTarGz(archivePath, framesDir)
	if err != nil {
		os.Remove(archivePath)
		return nil, err
	}

	os.Remove(archivePath)
	return extracted, nil
}

func extractTarGz(archivePath, destDir string) ([]string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	var extracted []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		if hdr.Typeflag != tar.TypeReg {
			continue
		}

		outPath := filepath.Join(destDir, filepath.Base(hdr.Name))
		outFile, err := os.Create(outPath)
		if err != nil {
			return nil, err
		}
		_, err = io.Copy(outFile, tr)
		outFile.Close()
		if err != nil {
			return nil, err
		}
		extracted = append(extracted, outPath)
	}
	return extracted, nil
}

func encodeVideo(framesDir, output string, fps int, onProgress func(current, total int)) error {
	entries, err := os.ReadDir(framesDir)
	if err != nil {
		return err
	}
	var frames []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(strings.ToLower(name), ".jpg") || strings.HasSuffix(strings.ToLower(name), ".jpeg") {
			frames = append(frames, filepath.Join(framesDir, name))
		}
	}
	if len(frames) == 0 {
		return fmt.Errorf("no frames found in %s", framesDir)
	}
	sort.Strings(frames)
	return encodeFrames(frames, output, fps, onProgress)
}

func encodeFrames(frames []string, output string, fps int, onProgress func(current, total int)) error {
	encoder, err := findH264Encoder()
	if err != nil {
		return err
	}

	tmpDir, err := os.MkdirTemp("", "timelapse_frames_*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	for i, src := range frames {
		dst := filepath.Join(tmpDir, fmt.Sprintf("frame_%06d.jpg", i))
		if err := os.Link(src, dst); err != nil {
			if err := copyFile(src, dst); err != nil {
				return fmt.Errorf("failed to copy frame %s: %w", src, err)
			}
		}
		if onProgress != nil {
			onProgress(i+1, len(frames))
		}
	}

	log.Printf("encoding %d frames at %d fps using %s", len(frames), fps, encoder)
	pattern := filepath.Join(tmpDir, "frame_%06d.jpg")
	cmd := exec.Command("ffmpeg",
		"-y",
		"-framerate", fmt.Sprintf("%d", fps),
		"-i", pattern,
		"-c:v", encoder,
		"-pix_fmt", "yuv420p",
		"-vsync", "0",
		"-vf", "scale=1920:-2",
		"-movflags", "+faststart",
		output,
	)
	cmd.Stdout = log.Writer()
	cmd.Stderr = log.Writer()
	return cmd.Run()
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func concatVideos(videoA, videoB, output string) error {
	listFile := output + ".concat.txt"
	f, err := os.Create(listFile)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f, "file '%s'\nfile '%s'\n", videoA, videoB)
	f.Close()
	if err != nil {
		return err
	}
	defer os.Remove(listFile)

	encoder, err := findH264Encoder()
	if err != nil {
		return err
	}

	cmd := exec.Command("ffmpeg",
		"-y",
		"-f", "concat",
		"-safe", "0",
		"-i", listFile,
		"-c", "copy",
		"-movflags", "+faststart",
		output,
	)
	cmd.Stdout = log.Writer()
	cmd.Stderr = log.Writer()
	err = cmd.Run()
	if err != nil {
		log.Printf("concat with copy failed, falling back to re-encode: %v", err)
		cmd = exec.Command("ffmpeg",
			"-y",
			"-i", videoA,
			"-i", videoB,
			"-filter_complex", "[0:v][1:v]concat=n=2:v=1:a=0[outv]",
			"-map", "[outv]",
			"-c:v", encoder,
			"-pix_fmt", "yuv420p",
			"-movflags", "+faststart",
			output,
		)
		cmd.Stdout = log.Writer()
		cmd.Stderr = log.Writer()
		return cmd.Run()
	}
	return nil
}

func findH264Encoder() (string, error) {
	preferred := []string{"libx264", "h264_nvenc", "h264_amf", "h264_qsv", "h264_vaapi", "libopenh264", "h264_v4l2m2m"}
	out, err := exec.Command("ffmpeg", "-hide_banner", "-encoders").Output()
	if err != nil {
		return "", fmt.Errorf("failed to query ffmpeg encoders: %w", err)
	}
	available := make(map[string]bool)
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			available[fields[1]] = true
		}
	}
	for _, enc := range preferred {
		if available[enc] {
			return enc, nil
		}
	}
	return "", fmt.Errorf("no H.264 encoder found in ffmpeg")
}
