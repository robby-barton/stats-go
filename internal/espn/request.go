package espn

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const maxBackoff = 30 * time.Second

// Client holds configuration for ESPN HTTP requests.
type Client struct {
	MaxRetries     int
	InitialBackoff time.Duration
	RequestTimeout time.Duration
	RateLimit      time.Duration // delay between batch API calls
}

// NewClient returns a Client with sensible defaults.
func NewClient() *Client {
	return &Client{
		MaxRetries:     5,
		InitialBackoff: 1 * time.Second,
		RequestTimeout: 1 * time.Second,
		RateLimit:      200 * time.Millisecond,
	}
}

type validatable interface {
	validate() error
}

type Responses interface {
	GameInfoESPN | GameScheduleESPN | TeamInfoESPN
	validatable
}

func (c *Client) makeRequest(endpoint string, data any) error {
	httpClient := &http.Client{
		Timeout: c.RequestTimeout,
	}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, endpoint, nil)

	headers := map[string]string{
		"User-Agent": "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) " +
			"Chrome/54.0.2840.90 Safari/537.36",
		"Accept": "application/json",
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	var res *http.Response
	var err error
	for attempt := range c.MaxRetries {
		res, err = httpClient.Do(req)
		if err == nil {
			if res.StatusCode >= 500 {
				res.Body.Close()
				err = fmt.Errorf("unexpected status %d from %q", res.StatusCode, endpoint)
				time.Sleep(c.backoff(attempt))
				continue
			}
			break
		}
		time.Sleep(c.backoff(attempt))
	}
	if err != nil {
		return fmt.Errorf("error from %q: %w", endpoint, err)
	}

	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d from %q", res.StatusCode, endpoint)
	}

	if err := json.NewDecoder(res.Body).Decode(data); err != nil {
		return fmt.Errorf("decoding response from %q: %w", endpoint, err)
	}

	return data.(validatable).validate()
}

func (c *Client) backoff(attempt int) time.Duration {
	d := c.InitialBackoff << attempt
	if d > maxBackoff {
		return maxBackoff
	}
	return d
}
