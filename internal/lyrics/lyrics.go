package lyrics

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type LyricsResult struct {
	ID          int64   `json:"id"`
	TrackName   string  `json:"trackName"`
	ArtistName  string  `json:"artistName"`
	AlbumName   string  `json:"albumName"`
	Duration    float64 `json:"duration"`
	PlainLyrics string  `json:"plainLyrics"`
	SyncedLyrics string `json:"syncedLyrics"`
}

var (
	stripNoiseRegex = regexp.MustCompile(`(?i)\((?:official\s*(?:video|audio|music\s*video|lyric\s*video|visualizer|clip)?|video\s*oficial|áudio\s*oficial|ao\s*vivo|live|clip|lyrics?|hd|4k|remastered?|hq)\)|\[(?:official\s*(?:video|audio|music\s*video|lyric\s*video|visualizer|clip)?|video\s*oficial|áudio\s*oficial|ao\s*vivo|live|clip|lyrics?|hd|4k|remastered?|hq)\]|(?i)\b(?:ft\.?|feat\.?)\s+[^\-\(\]]+`)
	cleanSpaceRegex = regexp.MustCompile(`\s+`)
)

func CleanSongTitle(title string) string {
	cleaned := stripNoiseRegex.ReplaceAllString(title, " ")
	cleaned = cleanSpaceRegex.ReplaceAllString(cleaned, " ")
	return strings.TrimSpace(cleaned)
}

func FetchLyrics(title, author string) (*LyricsResult, error) {
	client := &http.Client{Timeout: 8 * time.Second}

	cleanTitle := CleanSongTitle(title)
	queries := []string{
		fmt.Sprintf("%s %s", cleanTitle, author),
		cleanTitle,
		title,
	}

	for _, q := range queries {
		q = strings.TrimSpace(q)
		if q == "" {
			continue
		}

		apiURL := fmt.Sprintf("https://lrclib.net/api/search?q=%s", url.QueryEscape(q))
		req, err := http.NewRequest("GET", apiURL, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", "MusicianBot/2.0 (Discord Audio Bot)")

		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			continue
		}

		var results []LyricsResult
		if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
			continue
		}

		for _, item := range results {
			if strings.TrimSpace(item.PlainLyrics) != "" {
				return &item, nil
			}
		}
	}

	return nil, fmt.Errorf("nenhuma letra encontrada")
}
