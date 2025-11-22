package modules

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type UrbanResponse struct {
	List []UrbanDef `json:"list"`
}

type UrbanDef struct {
	Definition string `json:"definition"`
	Permalink  string `json:"permalink"`
	ThumbsUp   int    `json:"thumbs_up"`
	Word       string `json:"word"`
	Example    string `json:"example"`
}

func GetUrbanDef(term string) (string, error) {
	safeTerm := url.QueryEscape(term)
	url := fmt.Sprintf("http://api.urbandictionary.com/v0/define?term=%s", safeTerm)

	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result UrbanResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if len(result.List) == 0 {
		return "No definition found for that term.", nil
	}

	def := result.List[0]

	// Clean up brackets [word] -> word
	cleanDef := strings.ReplaceAll(def.Definition, "[", "")
	cleanDef = strings.ReplaceAll(cleanDef, "]", "")
	cleanExample := strings.ReplaceAll(def.Example, "[", "")
	cleanExample = strings.ReplaceAll(cleanExample, "]", "")

	// truncate
	if len(cleanDef) > 500 {
		cleanDef = cleanDef[:497] + "..."
	}

	output := fmt.Sprintf("📚 **Urban Dictionary: %s**\n\n%s\n\n*Example: %s*", def.Word, cleanDef, cleanExample)
	return output, nil
}
