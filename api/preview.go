package api

// preview fetches a representative image URL for a given web page URL.
// Strategy (in order):
//  1. <meta property="og:image">
//  2. <meta name="twitter:image">
//  3. <link rel="apple-touch-icon">
//  4. <link rel="icon" type="image/png">
//  5. First <img src> in the body
//  6. /apple-touch-icon.png on the origin

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	previewCache   = map[string]previewEntry{}
	previewCacheMu sync.Mutex

	httpClient = &http.Client{Timeout: 5 * time.Second}

	reOgImage      = regexp.MustCompile(`(?i)<meta[^>]+property=["']og:image["'][^>]+content=["']([^"']+)["']`)
	reOgImage2     = regexp.MustCompile(`(?i)<meta[^>]+content=["']([^"']+)["'][^>]+property=["']og:image["']`)
	reTwImage      = regexp.MustCompile(`(?i)<meta[^>]+name=["']twitter:image["'][^>]+content=["']([^"']+)["']`)
	reTwImage2     = regexp.MustCompile(`(?i)<meta[^>]+content=["']([^"']+)["'][^>]+name=["']twitter:image["']`)
	reAppleTouch   = regexp.MustCompile(`(?i)<link[^>]+rel=["']apple-touch-icon["'][^>]+href=["']([^"']+)["']`)
	reAppleTouch2  = regexp.MustCompile(`(?i)<link[^>]+href=["']([^"']+)["'][^>]+rel=["']apple-touch-icon["']`)
	reIconPng      = regexp.MustCompile(`(?i)<link[^>]+type=["']image/png["'][^>]+href=["']([^"']+)["']`)
	reIconPng2     = regexp.MustCompile(`(?i)<link[^>]+href=["']([^"']+)["'][^>]+type=["']image/png["']`)
	reFirstImg     = regexp.MustCompile(`(?i)<img[^>]+src=["']([^"']+)["']`)
)

type previewEntry struct {
	imageURL string
	fetchedAt time.Time
}

// PreviewImage returns a proxied preview image for the given page URL.
// Results are cached for 10 minutes.
func PreviewImage(pageURL string) string {
	previewCacheMu.Lock()
	if e, ok := previewCache[pageURL]; ok && time.Since(e.fetchedAt) < 10*time.Minute {
		previewCacheMu.Unlock()
		return e.imageURL
	}
	previewCacheMu.Unlock()

	imageURL := fetchPreviewImage(pageURL)

	previewCacheMu.Lock()
	previewCache[pageURL] = previewEntry{imageURL: imageURL, fetchedAt: time.Now()}
	previewCacheMu.Unlock()

	return imageURL
}

func fetchPreviewImage(pageURL string) string {
	base, err := url.Parse(pageURL)
	if err != nil {
		return ""
	}

	resp, err := httpClient.Get(pageURL)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	buf := make([]byte, 64*1024) // read up to 64KB — enough for <head>
	n, _ := resp.Body.Read(buf)
	html := string(buf[:n])

	resolve := func(raw string) string {
		u, err := url.Parse(raw)
		if err != nil {
			return ""
		}
		return base.ResolveReference(u).String()
	}

	firstMatch := func(re, re2 *regexp.Regexp) string {
		if m := re.FindStringSubmatch(html); len(m) > 1 {
			return resolve(m[1])
		}
		if m := re2.FindStringSubmatch(html); len(m) > 1 {
			return resolve(m[1])
		}
		return ""
	}

	// 1. og:image
	if u := firstMatch(reOgImage, reOgImage2); u != "" {
		return u
	}
	// 2. twitter:image
	if u := firstMatch(reTwImage, reTwImage2); u != "" {
		return u
	}
	// 3. apple-touch-icon
	if u := firstMatch(reAppleTouch, reAppleTouch2); u != "" {
		return u
	}
	// 4. icon png
	if u := firstMatch(reIconPng, reIconPng2); u != "" {
		return u
	}
	// 5. first <img> — skip tiny icons/data URIs
	if m := reFirstImg.FindStringSubmatch(html); len(m) > 1 {
		if !strings.HasPrefix(m[1], "data:") {
			if u := resolve(m[1]); u != "" {
				return u
			}
		}
	}
	// 6. try /apple-touch-icon.png on the origin
	fallback := base.Scheme + "://" + base.Host + "/apple-touch-icon.png"
	if r, err := httpClient.Head(fallback); err == nil && r.StatusCode == 200 {
		return fallback
	}

	return ""
}
