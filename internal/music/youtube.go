package music

import (
	"fmt"
	"os/exec"
	"strings"
)

type VideoResult struct {
	Title    string
	URL      string
	Duration string
}

func Search(query string, limit int) ([]VideoResult, error) {
	args := []string{
		"--default-search", "ytsearch",
		"--no-playlist",
		"--flat-playlist",
		"--no-warnings",
		"--print", "%(title)s|||%(id)s|||%(duration)s",
		fmt.Sprintf("ytsearch%d:%s", limit, query),
	}
	cmd := exec.Command("yt-dlp", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("yt-dlp search: %w", err)
	}

	var results []VideoResult
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.Split(line, "|||")
		if len(parts) < 3 {
			continue
		}
		results = append(results, VideoResult{
			Title:    parts[0],
			URL:      fmt.Sprintf("https://www.youtube.com/watch?v=%s", parts[1]),
			Duration: parts[2],
		})
	}
	return results, nil
}

type VideoInfo struct {
	Title    string
	URL      string
	Duration float64
	Uploader string
}

func GetVideoInfo(url string) (*VideoInfo, error) {
	args := []string{
		"--no-playlist",
		"--no-warnings",
		"--print", "%(title)s|||%(duration)s|||%(uploader)s",
		url,
	}
	cmd := exec.Command("yt-dlp", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("yt-dlp info: %w", err)
	}
	parts := strings.Split(strings.TrimSpace(string(out)), "|||")
	if len(parts) < 3 {
		return nil, fmt.Errorf("unexpected yt-dlp output: %s", out)
	}
	var dur float64
	fmt.Sscanf(parts[1], "%f", &dur)
	return &VideoInfo{
		Title:    parts[0],
		URL:      url,
		Duration: dur,
		Uploader: parts[2],
	}, nil
}

func GetAudioURL(videoURL string) (string, error) {
	args := []string{
		"-f", "bestaudio[ext=m4a]/bestaudio",
		"--get-url",
		"--no-playlist",
		"--no-warnings",
		videoURL,
	}
	cmd := exec.Command("yt-dlp", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("yt-dlp audio url: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func IsYouTubeURL(s string) bool {
	return strings.Contains(s, "youtube.com/watch") || strings.Contains(s, "youtu.be/")
}

func IsPlaylistURL(s string) bool {
	return strings.Contains(s, "youtube.com/playlist") ||
		(strings.Contains(s, "youtube.com/watch") && strings.Contains(s, "list=")) ||
		(strings.Contains(s, "youtu.be/") && strings.Contains(s, "list="))
}

func GetPlaylistVideos(url string) ([]VideoResult, error) {
	args := []string{
		"--flat-playlist",
		"--playlist-end", "10",
		"--no-warnings",
		"--print", "%(title)s|||%(id)s|||%(duration)s",
		url,
	}
	cmd := exec.Command("yt-dlp", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("yt-dlp playlist: %w", err)
	}

	var results []VideoResult
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.Split(line, "|||")
		if len(parts) < 3 {
			continue
		}
		results = append(results, VideoResult{
			Title:    parts[0],
			URL:      fmt.Sprintf("https://www.youtube.com/watch?v=%s", parts[1]),
			Duration: parts[2],
		})
	}
	return results, nil
}