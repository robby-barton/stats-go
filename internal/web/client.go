package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

const (
	timeout = 1 * time.Second
)

type Client struct {
	RevalidateSecret string
}

func (c *Client) RevalidateWeb(ctx context.Context) error {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		fmt.Sprintf("https://cfb.robbybarton.com/api/revalidate?secret=%s", c.RevalidateSecret),
		nil,
	)
	if err != nil {
		return err
	}

	client := &http.Client{
		Timeout: timeout,
	}
	var res *http.Response
	count := 0
	for ok := true; ok; ok = (count < 5 && err != nil) {
		res, err = client.Do(req) //nolint:bodyclose // allow since close is outside loop
		if err == nil {
			break
		}
		count++
		time.Sleep(1 * time.Second)
	}
	if err != nil {
		return fmt.Errorf("error trying to invalidate web: %w", err)
	} else if res.StatusCode != http.StatusOK {
		return errors.New("not 200")
	}

	return nil
}
