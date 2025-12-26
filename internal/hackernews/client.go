package hackernews

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/soyokaze83/invictus/internal/domain"
)

const baseURL = "https://hacker-news.firebaseio.com/v0"

type Client struct {
	http *http.Client
}

func New() *Client {
	return &Client{http: &http.Client{}}
}

func (c *Client) GetBestStories(ctx context.Context) ([]int, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/beststories.json", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var ids []int
	if err := json.NewDecoder(resp.Body).Decode(&ids); err != nil {
		return nil, err
	}
	return ids, nil
}

func (c *Client) GetStory(ctx context.Context, id int) (*domain.Story, error) {
	url := fmt.Sprintf("%s/item/%d.json", baseURL, id)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var story domain.Story
	if err := json.NewDecoder(resp.Body).Decode(&story); err != nil {
		return nil, err
	}

	// Only fetch content if story has a URL
	if story.URL != "" {
		content, err := c.FetchAndClean(ctx, story.URL)
		if err != nil {
			slog.Warn("Failed to fetch story content", "storyID", id, "url", story.URL, "error", err)
		} else {
			story.Content = content
		}
	}

	return &story, nil
}

func (c *Client) FetchAndClean(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; InvictusBot/1.0)")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	markdown, err := htmltomarkdown.ConvertString(string(body))
	if err != nil {
		return "", err
	}

	return markdown, nil
}
