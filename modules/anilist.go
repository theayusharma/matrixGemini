package modules

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type AniListRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

type AniListResponse struct {
	Data struct {
		Media struct {
			Title struct {
				Romaji  string `json:"romaji"`
				English string `json:"english"`
			} `json:"title"`
			Description  string `json:"description"`
			AverageScore int    `json:"averageScore"`
			Episodes     int    `json:"episodes"`
			Status       string `json:"status"`
			SiteUrl      string `json:"siteUrl"`
		} `json:"Media"`
	} `json:"data"`
}

func GetAnimeInfo(search string) (string, error) {
	query := `
	query ($search: String) {
		Media (search: $search, type: ANIME) {
			title {
				romaji
				english
			}
			description
			averageScore
			episodes
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

	var result AniListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	media := result.Data.Media
	if media.SiteUrl == "" {
		return "Anime not found.", nil
	}

	title := media.Title.English
	if title == "" {
		title = media.Title.Romaji
	}

	// HTML description cleanup (simple)
	desc := strings.ReplaceAll(media.Description, "<br>", "\n")
	desc = strings.ReplaceAll(desc, "<i>", "_")
	desc = strings.ReplaceAll(desc, "</i>", "_")
	if len(desc) > 400 {
		desc = desc[:397] + "..."
	}

	output := fmt.Sprintf("🎬 **%s**\nScore: %d/100 | Eps: %d | Status: %s\n\n%s\n\n🔗 %s",
		title, media.AverageScore, media.Episodes, media.Status, desc, media.SiteUrl)

	return output, nil
}
