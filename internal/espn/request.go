package espn

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	timeout = 1 * time.Second
)

type validatable interface {
	validate() error
}

type Responses interface {
	GameInfoESPN | GameScheduleESPN | TeamInfoESPN
	validatable
}

func makeRequest[R Responses](endpoint string, data *R) error {
	client := &http.Client{
		Timeout: timeout,
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
	count := 0
	for ok := true; ok; ok = (count < 5 && err != nil) {
		res, err = client.Do(req)
		if err == nil {
			if res.StatusCode >= 500 {
				res.Body.Close()
				err = fmt.Errorf("unexpected status %d from %q", res.StatusCode, endpoint)
				count++
				time.Sleep(1 * time.Second)
				continue
			}
			break
		}
		count++
		time.Sleep(1 * time.Second)
	}
	if err != nil {
		return fmt.Errorf("error from %q: %w", endpoint, err)
	}

	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d from %q", res.StatusCode, endpoint)
	}

	if err := json.NewDecoder(res.Body).Decode(&data); err != nil {
		return fmt.Errorf("decoding response from %q: %w", endpoint, err)
	}

	return (*data).validate()
}
