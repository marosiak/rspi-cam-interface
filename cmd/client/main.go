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
)

func main() {
	serverURL := flag.String("server", "http://raspberrypi.local/", "Server base URL (e.g. http://192.168.1.100:8080)")
	output := flag.String("output", "timelapse.mp4", "Output video file path")
	workDir := flag.String("work-dir", "./timelapse_work", "Working directory for downloads and frames")
	fps := flag.Int("fps", 30, "Frames per second for output video")
	keep := flag.Bool("keep", false, "Keep working directory after encoding")
	flag.Parse()

	server := strings.TrimSuffix(*serverURL, "/")

	if err := os.MkdirAll(*workDir, 0755); err != nil {
		log.Fatalf("failed to create work dir: %v", err)
	}

	packages, err := fetchPackageList(server)
	if err != nil {
		log.Fatalf("failed to fetch package list: %v", err)
	}
	if len(packages) == 0 {
		log.Println("no packages found")
		return
	}

	log.Printf("found %d package(s)", len(packages))

	framesDir := filepath.Join(*workDir, "frames")
	if err := os.MkdirAll(framesDir, 0755); err != nil {
		log.Fatalf("failed to create frames dir: %v", err)
	}

	for _, pkg := range packages {
		if err := downloadAndExtract(server, pkg, *workDir, framesDir); err != nil {
			log.Printf("failed to process %s: %v", pkg, err)
		}
	}

	if err := encodeVideo(framesDir, *output, *fps); err != nil {
		log.Fatalf("failed to encode video: %v", err)
	}

	log.Printf("video saved to %s", *output)

	if !*keep {
		if err := os.RemoveAll(*workDir); err != nil {
			log.Printf("failed to clean up work dir: %v", err)
		}
	}
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

func downloadAndExtract(server, pkgPath, workDir, framesDir string) error {
	filename := path.Base(pkgPath)
	archivePath := filepath.Join(workDir, filename)

	url := server + pkgPath
	log.Printf("downloading %s", url)

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download status: %s", resp.Status)
	}

	f, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		return err
	}

	log.Printf("extracting %s", filename)
	if err := extractTarGz(archivePath, framesDir); err != nil {
		return err
	}

	os.Remove(archivePath)
	return nil
}

func extractTarGz(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if hdr.Typeflag != tar.TypeReg {
			continue
		}

		outPath := filepath.Join(destDir, filepath.Base(hdr.Name))
		outFile, err := os.Create(outPath)
		if err != nil {
			return err
		}
		_, err = io.Copy(outFile, tr)
		outFile.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func encodeVideo(framesDir, output string, fps int) error {
	encoder, err := findH264Encoder()
	if err != nil {
		return err
	}

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
			frames = append(frames, name)
		}
	}
	if len(frames) == 0 {
		return fmt.Errorf("no frames found in %s", framesDir)
	}
	sort.Strings(frames)

	for i, name := range frames {
		padded := fmt.Sprintf("frame_%06d.jpg", i)
		oldPath := filepath.Join(framesDir, name)
		newPath := filepath.Join(framesDir, padded)
		if err := os.Rename(oldPath, newPath); err != nil {
			return err
		}
	}

	log.Printf("encoding %d frames at %d fps using %s", len(frames), fps, encoder)
	pattern := filepath.Join(framesDir, "frame_%06d.jpg")
	cmd := exec.Command("ffmpeg",
		"-y",
		"-framerate", fmt.Sprintf("%d", fps),
		"-i", pattern,
		"-c:v", encoder,
		"-pix_fmt", "yuv420p",
		"-vf", "scale=1920:-2",
		"-movflags", "+faststart",
		output,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
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
