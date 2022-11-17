package espn

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"
)

const (
	timeout = 1 * time.Second
)

var (
	headers = map[string]string{
		"User-Agent": "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/54.0.2840.90 Safari/537.36",
		"Accept":     "application/json",
	}
)

type Responses interface {
	GameInfoESPN | GameScheduleESPN
}

func makeRequest[R Responses](endpoint string, data *R) error {
	client := &http.Client{
		Timeout: timeout,
	}
	req, _ := http.NewRequest("GET", endpoint, nil)

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	res, err := client.Do(req)
	count := 1
	for count < 5 && os.IsTimeout(err) {
		time.Sleep(1 * time.Second)
		res, err = client.Do(req)
		count += 1
	}
	if err != nil {
		return errors.New(fmt.Sprintf("Error from \"%s\": %v", endpoint, err))
	}

	defer res.Body.Close()

	return json.NewDecoder(res.Body).Decode(&data)
}
