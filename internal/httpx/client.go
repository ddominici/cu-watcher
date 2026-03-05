package httpx

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"
)

type ScraperCfg struct {
	UserAgent              string
	TimeoutSeconds         int
	DelayBetweenRequestsMs int
}

type RawPage struct {
	SourceKey      string
	URL            string
	RetrievedAtUTC time.Time
	StatusCode     int
	ETag           string
	LastModified   string
	ContentType    string
	SHA256         string
	HTML           string
}

type Client struct {
	cfg ScraperCfg
	log *zap.Logger
	hc  *http.Client
}

func New(cfg ScraperCfg, log *zap.Logger) *Client {
	return &Client{
		cfg: cfg,
		log: log,
		hc:  &http.Client{Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second},
	}
}

func (c *Client) Get(ctx context.Context, sourceKey, url string) (*RawPage, error) {
	if c.cfg.DelayBetweenRequestsMs > 0 {
		time.Sleep(time.Duration(c.cfg.DelayBetweenRequestsMs) * time.Millisecond)
	}

	var lastErr error
	for attempt := 1; attempt <= 5; attempt++ {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		req.Header.Set("User-Agent", c.cfg.UserAgent)
		req.Header.Set("Accept-Language", "en-US,en;q=0.9,en-US;q=0.8,en;q=0.7")

		res, err := c.hc.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt*attempt) * 300 * time.Millisecond)
			continue
		}

		body, err := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt*attempt) * 300 * time.Millisecond)
			continue
		}

		// Retry on 429/5xx
		if res.StatusCode == 429 || res.StatusCode >= 500 {
			lastErr = errStatus(res.StatusCode)
			time.Sleep(time.Duration(attempt*attempt) * 500 * time.Millisecond)
			continue
		}

		sum := sha256.Sum256(body)
		page := &RawPage{
			SourceKey:      sourceKey,
			URL:            url,
			RetrievedAtUTC: time.Now().UTC(),
			StatusCode:     res.StatusCode,
			ETag:           res.Header.Get("ETag"),
			LastModified:   res.Header.Get("Last-Modified"),
			ContentType:    res.Header.Get("Content-Type"),
			SHA256:         hex.EncodeToString(sum[:]),
			HTML:           string(body),
		}

		c.log.Info("GET",
			zap.String("url", url),
			zap.Int("status", res.StatusCode),
			zap.Int("bytes", len(body)),
		)

		return page, nil
	}

	return nil, lastErr
}

type errStatus int

func (e errStatus) Error() string { return "http status " + http.StatusText(int(e)) }
