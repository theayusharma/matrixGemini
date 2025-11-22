package modules

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type AniListMangaResponse struct {
	Data struct {
		Media struct {
			Title struct {
				Romaji  string `json:"romaji"`
				English string `json:"english"`
			} `json:"title"`
			Description  string `json:"description"`
			AverageScore int    `json:"averageScore"`
			Chapters     int    `json:"chapters"`
			Volumes      int    `json:"volumes"`
			Status       string `json:"status"`
			SiteUrl      string `json:"siteUrl"`
		} `json:"Media"`
	} `json:"data"`
}

func GetMangaInfo(search string) (string, error) {
	query := `
	query ($search: String) {
		Media (search: $search, type: MANGA) {
			title {
				romaji
				english
			}
			description
			averageScore
			chapters
			volumes
			status
			siteUrl
		}
	}
	`

	reqBody := AniListRequest{
		Query: query,
		Variables: map[string]interface{}{
			"search": search,
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	resp, err := http.Post("https://graphql.anilist.co", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result AniListMangaResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	media := result.Data.Media
	if media.SiteUrl == "" {
		return "Manga not found.", nil
	}

	title := media.Title.English
	if title == "" {
		title = media.Title.Romaji
	}

	desc := strings.ReplaceAll(media.Description, "<br>", "\n")
	desc = strings.ReplaceAll(desc, "<i>", "_")
	desc = strings.ReplaceAll(desc, "</i>", "_")

	if len(desc) > 400 {
		desc = desc[:397] + "..."
	}

	output := fmt.Sprintf("📖 **%s**\nScore: %d/100 | Vol: %d | Ch: %d | Status: %s\n\n%s\n\n🔗 %s",
		title,
		media.AverageScore,
		media.Volumes,
		media.Chapters,
		media.Status,
		desc,
		media.SiteUrl,
	)

	return output, nil
}
