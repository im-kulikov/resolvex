package domain

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Config struct {
	Link    string        `env:"LINK"`
	List    []string      `env:"LIST"`
	Timeout time.Duration `env:"TIMEOUT" default:"30s"`

	*http.Client
}

func Fetch(cfg Config) ([]string, error) {
	return FetchContext(context.Background(), cfg)
}

func FetchContext(top context.Context, cfg Config) (
	[]string, error,
) {
	var err error
	if len(cfg.List) > 0 {
		return cfg.List, nil
	}

	ctx, cancel := context.WithTimeout(top, cfg.Timeout)
	defer cancel()

	var uri *url.URL
	if uri, err = url.Parse(cfg.Link); err != nil || cfg.Link == "" {
		return nil, fmt.Errorf("could not parse domain link(%q): %w", cfg.Link, err)
	}

	var req *http.Request
	if req, err = http.NewRequestWithContext(ctx, http.MethodGet, uri.String(), nil); err != nil {
		return nil, fmt.Errorf("could not create request(%q): %w", cfg.Link, err)
	}

	var res *http.Response
	if res, err = new(http.Client).Do(req); err != nil {
		return nil, fmt.Errorf("could not fetch response(%q): %w", cfg.Link, err)
	}

	defer func() { _ = res.Body.Close() }()
	var data []byte
	if data, err = io.ReadAll(res.Body); err != nil || res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%d: %s => %w", res.StatusCode, string(data), err)
	}

	return strings.Split(string(data), "\n"), nil
}
