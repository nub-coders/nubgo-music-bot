package media

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// GenerateThumbnail runs the Python render script to produce a glassmorphic Now Playing cover.
func GenerateThumbnail(ctx context.Context, track Track, cacheDir string) (string, error) {
	if cacheDir == "" {
		cacheDir = "cache"
	}
	_ = os.MkdirAll(cacheDir, 0o755)

	durStr := "-"
	if track.Duration > 0 {
		m := int(track.Duration.Minutes())
		s := int(track.Duration.Seconds()) % 60
		durStr = fmt.Sprintf("%02d:%02d", m, s)
	}

	author := track.Author
	if author == "" {
		author = "YouTube"
	}

	thumbURL := track.ThumbnailURL
	if thumbURL == "" && track.Kind == SourceYouTube && track.ID != "" {
		thumbURL = fmt.Sprintf("https://img.youtube.com/vi/%s/hqdefault.jpg", track.ID)
	}

	renderScript := "assets/thumbnails/render.py"
	if _, err := os.Stat(renderScript); err != nil {
		// Try absolute path if run from subdirectory
		cwd, _ := os.Getwd()
		renderScript = filepath.Join(cwd, "assets", "thumbnails", "render.py")
	}

	cmdCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "python3", renderScript,
		"--title", track.Title,
		"--duration", durStr,
		"--channel", author,
		"--views", "1M",
		"--thumbnail", thumbURL,
		"--videoid", track.ID,
		"--cache-dir", cacheDir,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("render thumbnail: %w (stderr: %s)", err, stderr.String())
	}

	outPath := strings.TrimSpace(stdout.String())
	if outPath == "" || !strings.HasSuffix(outPath, ".png") {
		return "", fmt.Errorf("unexpected render output: %s", outPath)
	}

	return outPath, nil
}
