package meshmapper

import (
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testOptions(t *testing.T, url string) Options {
	t.Helper()
	dir := t.TempDir()
	return Options{OutputDir: filepath.Join(dir, "public"), StateDir: filepath.Join(dir, "private"), Key: "fixture-only-key", Regions: fixtureRegions(), Contains: fixtureContains, MaxCells: 3, URL: url, Now: time.Date(2026, 10, 12, 2, 0, 0, 0, time.UTC)}
}

func TestSyncGzipETag304AndDurableQuota(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("X-API-Key") != "fixture-only-key" || r.URL.RawQuery != "" {
			t.Error("wrong authentication transport")
		}
		if calls == 2 {
			if r.Header.Get("If-None-Match") != `"v1"` {
				t.Error("missing ETag")
			}
			w.WriteHeader(304)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		w.Header().Set("Content-Encoding", "gzip")
		z := gzip.NewWriter(w)
		fmt.Fprint(z, feed(fixtureCell))
		z.Close()
	}))
	defer s.Close()
	o := testOptions(t, s.URL)
	state, err := Sync(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	if state.HTTPStatus != 200 || !state.NextAttempt.Equal(o.Now.Add(12*time.Hour)) {
		t.Fatalf("state=%+v", state)
	}
	if _, err = Sync(context.Background(), o); err != nil || calls != 1 {
		t.Fatalf("restart violated cooldown calls=%d err=%v", calls, err)
	}
	o.Now = o.Now.Add(12 * time.Hour)
	state, err = Sync(context.Background(), o)
	if err != nil || calls != 2 || state.HTTPStatus != 304 {
		t.Fatalf("state=%+v calls=%d err=%v", state, calls, err)
	}
	if _, err = Sync(context.Background(), o); err != nil || calls != 2 {
		t.Fatalf("304 did not consume quota interval: calls=%d", calls)
	}
	for _, path := range []string{cachePath(o, "YKF"), filepath.Join(o.OutputDir, "meshmapper-status.json"), filepath.Join(o.StateDir, "state.json")} {
		b, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		if strings.Contains(string(b), o.Key) {
			t.Fatal("secret leaked into cached output")
		}
	}
}

func TestFailedFeedPreservesLastGoodAndCooldown(t *testing.T) {
	body := feed(fixtureCell)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
	defer s.Close()
	o := testOptions(t, s.URL)
	if _, err := Sync(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(cachePath(o, "YKF"))
	body = feed(fixtureCell) + "broken"
	o.Now = o.Now.Add(12 * time.Hour)
	state, err := Sync(context.Background(), o)
	if err == nil || state.Error != "refresh_failed" {
		t.Fatalf("state=%+v err=%v", state, err)
	}
	after, _ := os.ReadFile(cachePath(o, "YKF"))
	if string(after) != string(before) {
		t.Fatal("failed global feed replaced last good region")
	}
	if !state.NextAttempt.Equal(o.Now.Add(12 * time.Hour)) {
		t.Fatal("failure lost durable cooldown")
	}
}

func TestThrottlesHonorRetryAfterAndRejectPublicState(t *testing.T) {
	for _, status := range []int{429, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Retry-After", "90000")
				w.WriteHeader(status)
			}))
			defer s.Close()
			o := testOptions(t, s.URL)
			state, err := Sync(context.Background(), o)
			if err == nil || state.NextAttempt.Before(o.Now.Add(25*time.Hour)) {
				t.Fatalf("state=%+v err=%v", state, err)
			}
		})
	}
	o := testOptions(t, "http://unused.invalid")
	o.StateDir = filepath.Join(o.OutputDir, "private")
	if _, err := Sync(context.Background(), o); err == nil {
		t.Fatal("secret state allowed beneath public output")
	}
}

func TestCredentialNeverFollowsRedirect(t *testing.T) {
	forwarded := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded = true }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer source.Close()
	o := testOptions(t, source.URL)
	if _, err := Sync(context.Background(), o); err == nil || forwarded {
		t.Fatal("credential-bearing request followed redirect")
	}
}

func TestCorruptCacheIsNotRevalidatedWithETag(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("If-None-Match") != "" {
			t.Error("corrupt cache received conditional validation")
		}
		w.Header().Set("ETag", `"v1"`)
		fmt.Fprint(w, feed(fixtureCell))
	}))
	defer s.Close()
	o := testOptions(t, s.URL)
	if _, err := Sync(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cachePath(o, "YKF"), []byte("broken"), 0644); err != nil {
		t.Fatal(err)
	}
	o.Now = o.Now.Add(12 * time.Hour)
	if _, err := Sync(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal("corrupt cache did not refresh")
	}
}
