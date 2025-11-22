package modules

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

type WikiResponse struct {
	Type         string `json:"type"`
	Title        string `json:"title"`
	DisplayTitle string `json:"displaytitle"`
	Extract      string `json:"extract"`
	ContentURLs  struct {
		Desktop struct {
			Page string `json:"page"`
		} `json:"desktop"`
	} `json:"content_urls"`
}

var htmlTagRegex = regexp.MustCompile(`<[^>]*>`)

func GetWikiSummary(query string) (string, error) {
	formattedQuery := strings.ReplaceAll(query, " ", "_")
	safeQuery := url.PathEscape(formattedQuery)

	apiURL := fmt.Sprintf("https://en.wikipedia.org/api/rest_v1/page/summary/%s", safeQuery)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("User-Agent", "RakkaBot/1.0 (matrix-bot)")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return "❌ Article not found.", nil
	}
	if resp.StatusCode != 200 {
		return fmt.Sprintf("❌ Wikipedia API Error: %d", resp.StatusCode), nil
	}

	var result WikiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to parse Wikipedia response: %w", err)
	}

	if result.Type == "disambiguation" {
		return fmt.Sprintf("⚠️ **%s** is ambiguous. Please be more specific.", result.DisplayTitle), nil
	}

	cleanTitle := htmlTagRegex.ReplaceAllString(result.DisplayTitle, "")

	if cleanTitle == "" {
		cleanTitle = strings.ReplaceAll(result.Title, "_", " ")
	}

	output := fmt.Sprintf("📖 **%s**\n\n%s\n\n🔗 %s",
		cleanTitle,
		result.Extract,
		result.ContentURLs.Desktop.Page,
	)
	return output, nil
}
