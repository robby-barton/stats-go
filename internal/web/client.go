package web

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

const (
	timeout        = 1 * time.Second
	webDomain      = "https://cfb.robbybarton.com"
	revalidateBase = "/api/revalidate"
)

type Client struct {
	RevalidateSecret string
}

func (c *Client) RevalidateWeek(ctx context.Context) error {
	err := c.revalidateRequest(ctx, fmt.Sprintf("%s%s/week", webDomain, revalidateBase))
	if err != nil {
		return fmt.Errorf("error trying to invalidate week: %w", err)
	}

	return nil
}

func (c *Client) RevalidateAll(ctx context.Context) error {
	err := c.revalidateRequest(ctx, fmt.Sprintf("%s%s/all", webDomain, revalidateBase))
	if err != nil {
		return fmt.Errorf("error trying to invalidate all: %w", err)
	}

	return nil
}

func (c *Client) revalidateRequest(ctx context.Context, endpoint string) error {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		fmt.Sprintf("%s?secret=%s", endpoint, c.RevalidateSecret),
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
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("non-OK status: %d", res.StatusCode)
	}

	return nil
}
