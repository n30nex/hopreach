package meshmapper

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const Endpoint = "https://meshmapper.net/coverage.php"
const wireLimit = int64(256 << 20)
const decodedLimit = int64(4 << 30)

type Options struct {
	OutputDir, StateDir, Key string
	Regions                  []Region
	Contains                 func(float64, float64) bool
	Interval                 time.Duration
	MaxCells                 int
	Client                   *http.Client
	// URL and Now make quota/HTTP tests deterministic; production leaves URL empty.
	URL            string
	Now            time.Time
	previousHashes map[string]string
}
type State struct {
	CacheHashes    map[string]string `json:"cache_hashes,omitempty"`
	MissingRegions []string          `json:"missing_regions,omitempty"`
	ETag           string            `json:"etag,omitempty"`
	Catalog        string            `json:"catalog,omitempty"`
	LastAttempt    time.Time         `json:"last_attempt"`
	LastSuccess    time.Time         `json:"last_success"`
	NextAttempt    time.Time         `json:"next_attempt"`
	HTTPStatus     int               `json:"http_status"`
	Error          string            `json:"error,omitempty"`
}

// Sync is called under the pipeline's existing cross-process lock. Quota is
// reserved on disk BEFORE HTTP so a timeout or killed process cannot turn
// restarts into a request loop. A 304 consumes the same interval as a 200.
func Sync(ctx context.Context, o Options) (state State, err error) {
	if err = validateOptions(&o); err != nil {
		return state, err
	}
	state, err = readState(o.StateDir)
	if err != nil {
		return state, err
	}
	if o.Now.Before(state.NextAttempt) {
		return state, nil
	}
	o.previousHashes = state.CacheHashes
	state.LastAttempt = o.Now
	state.HTTPStatus = 0
	state.NextAttempt = o.Now.Add(o.Interval)
	state.Error = "refresh_incomplete"
	if err = writeState(o, state); err != nil {
		return state, err
	}
	defer func() {
		if err != nil && state.Error == "refresh_incomplete" {
			state.Error = "refresh_failed"
		}
		if e := writeState(o, state); err == nil {
			err = e
		}
	}()
	url := o.URL
	if url == "" {
		url = Endpoint
	}
	request, e := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if e != nil {
		return state, fmt.Errorf("invalid MeshMapper endpoint")
	}
	request.Header.Set("X-API-Key", o.Key)
	request.Header.Set("Accept-Encoding", "gzip")
	if state.Catalog == catalogHash(o.Regions) && cacheComplete(o, state.CacheHashes) {
		request.Header.Set("If-None-Match", state.ETag)
	}
	client := o.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return fmt.Errorf("MeshMapper redirect refused") }}
	}
	response, e := client.Do(request)
	if e != nil {
		return state, fmt.Errorf("MeshMapper request failed: %w", e)
	}
	defer response.Body.Close()
	state.HTTPStatus = response.StatusCode
	if response.StatusCode == http.StatusNotModified {
		if request.Header.Get("If-None-Match") == "" {
			return state, fmt.Errorf("unexpected MeshMapper 304 without a complete cache")
		}
		state.LastSuccess = o.Now
		state.Error = ""
		return state, nil
	}
	if response.StatusCode != http.StatusOK {
		state.Error = "upstream_http_" + strconv.Itoa(response.StatusCode)
		state.NextAttempt = retryAt(o.Now, state.NextAttempt, response)
		return state, fmt.Errorf("MeshMapper returned HTTP %d; cached coverage retained", response.StatusCode)
	}
	var missing []string
	if missing, e = importResponse(o, response); e != nil {
		return state, e
	}
	state.MissingRegions = missing
	state.CacheHashes, e = cacheHashes(o)
	if e != nil {
		return state, e
	}
	state.ETag = response.Header.Get("ETag")
	state.Catalog = catalogHash(o.Regions)
	state.LastSuccess = o.Now
	state.Error = ""
	return state, nil
}

func validateOptions(o *Options) error {
	if o.Now.IsZero() {
		o.Now = time.Now().UTC()
	}
	if o.Interval < 12*time.Hour {
		o.Interval = 12 * time.Hour
	}
	if o.MaxCells == 0 {
		o.MaxCells = 100000
	}
	if o.MaxCells < 1 || o.MaxCells > 100000 || len(o.Regions) == 0 || len(o.Regions) > 64 || o.Contains == nil {
		return fmt.Errorf("invalid MeshMapper region or cell limits")
	}
	o.Key = strings.TrimSpace(o.Key)
	if len(o.Key) == 0 || len(o.Key) > 4096 {
		return fmt.Errorf("MeshMapper API key is absent or invalid")
	}
	for _, r := range o.Key {
		if r < 33 || r > 126 {
			return fmt.Errorf("MeshMapper API key contains invalid characters")
		}
	}
	for _, r := range o.Regions {
		if len(r.IATA) != 3 || strings.Trim(r.IATA, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") != "" || r.RadiusKm <= 0 || r.RadiusKm > 250 {
			return fmt.Errorf("invalid MeshMapper Canadian IATA region")
		}
	}
	if o.OutputDir == "" || o.StateDir == "" {
		return fmt.Errorf("MeshMapper output and private state directories are required")
	}
	output, e := filepath.Abs(o.OutputDir)
	if e != nil {
		return e
	}
	state, e := filepath.Abs(o.StateDir)
	if e != nil {
		return e
	}
	rel, e := filepath.Rel(output, state)
	if e != nil {
		return e
	}
	if rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..") {
		return fmt.Errorf("MeshMapper private state must not be inside public output")
	}
	return os.MkdirAll(o.StateDir, 0700)
}

func retryAt(now, next time.Time, r *http.Response) time.Time {
	if seconds, err := strconv.Atoi(r.Header.Get("Retry-After")); err == nil && seconds > 0 {
		if t := now.Add(time.Duration(seconds) * time.Second); t.After(next) {
			next = t
		}
	} else if t, err := http.ParseTime(r.Header.Get("Retry-After")); err == nil && t.After(next) {
		next = t
	}
	if r.StatusCode == http.StatusTooManyRequests {
		utc := now.UTC()
		reset := time.Date(utc.Year(), utc.Month(), utc.Day()+1, 0, 1, 0, 0, time.UTC)
		if reset.After(next) {
			next = reset
		}
	}
	return next
}

func importResponse(o Options, response *http.Response) ([]string, error) {
	if response.ContentLength > wireLimit {
		return nil, fmt.Errorf("MeshMapper compressed response exceeds 256 MiB budget")
	}
	if err := cleanupPending(o); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp(o.StateDir, "pending-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir) // generated private child directory, never operator input
	var reader io.Reader = &byteBudget{r: response.Body, remaining: wireLimit, label: "compressed"}
	if strings.EqualFold(response.Header.Get("Content-Encoding"), "gzip") {
		z, e := gzip.NewReader(reader)
		if e != nil {
			return nil, fmt.Errorf("invalid MeshMapper gzip response")
		}
		defer z.Close()
		reader = z
	}
	meta, regions, err := parseFeed(&byteBudget{r: reader, remaining: decodedLimit, label: "decoded"}, dir, o.Regions, o.Contains, o.MaxCells)
	if err != nil {
		return nil, fmt.Errorf("MeshMapper feed rejected; last good cache retained: %w", err)
	}
	var missing []string
	for _, r := range o.Regions {
		if _, ok := regions[r.IATA]; !ok {
			missing = append(missing, r.IATA)
		}
	}
	return missing, publishCaches(o, meta, regions, dir)
}
