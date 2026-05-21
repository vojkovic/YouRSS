package main

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v2"
)

type Config struct {
	Channels map[string]string `yaml:"channels"`
}

type Feed struct {
	Entries []Entry `xml:"entry"`
}

type Entry struct {
	Title      string     `xml:"title"`
	Link       Link       `xml:"link"`
	Published  string     `xml:"published"`
	PubDate    time.Time  `xml:"-"`
	Author     string     `xml:"author>name"`
	Thumbnail  string     `xml:"-"`
	Views      string     `xml:"-"`
	MediaGroup MediaGroup `xml:"group"`
}

type Link struct {
	Href string `xml:"href,attr"`
}

type MediaGroup struct {
	Thumbnail      Thumbnail      `xml:"thumbnail"`
	MediaCommunity MediaCommunity `xml:"community"`
}

type Thumbnail struct {
	URL string `xml:"url,attr"`
}

type MediaCommunity struct {
	Statistics Statistics `xml:"statistics"`
}

type Statistics struct {
	Views string `xml:"views,attr"`
}

type FeedStatus struct {
	TotalChannels  int
	SuccessCount   int
	FailedChannels []string
	LastUpdate     time.Time
	Updating       bool
}

type PageData struct {
	Entries []Entry
	Status  FeedStatus
}

var (
	allEntries    = []Entry{}
	feedsMutex    sync.RWMutex
	feedStatus    FeedStatus
	statusMutex   sync.RWMutex
	channelCaches = make(map[string][]Entry)
	channelMutex  sync.RWMutex
	tmpl          *template.Template
	urlCache      = make(map[string]string)
	cacheMutex    sync.RWMutex
	httpClient    = &http.Client{Timeout: 15 * time.Second}
)

func main() {
	cfg, err := loadConfig("config.yaml")
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	tmpl = template.Must(template.New("feed.html").Funcs(template.FuncMap{
		"timeAgo":   timeAgo,
		"viewCount": viewCount,
		"join":      strings.Join,
	}).ParseFiles("templates/feed.html"))

	refreshFeeds(cfg)

	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			refreshFeeds(cfg)
		}
	}()

	http.HandleFunc("/", feedHandler)
	http.HandleFunc("/proxy/", proxyHandler)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	log.Println("Server started at http://localhost:8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func loadConfig(path string) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer file.Close()

	var cfg Config
	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func fetchFeed(url string) ([]Entry, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "YouRSS/1.0")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var feed Feed
	if err := xml.NewDecoder(bytes.NewReader(body)).Decode(&feed); err != nil {
		return nil, err
	}

	for i := range feed.Entries {
		feed.Entries[i].PubDate, _ = time.Parse(time.RFC3339, feed.Entries[i].Published)
		originalURL := feed.Entries[i].MediaGroup.Thumbnail.URL
		feed.Entries[i].Thumbnail = cacheURL(originalURL)
		feed.Entries[i].Views = feed.Entries[i].MediaGroup.MediaCommunity.Statistics.Views
	}

	return feed.Entries, nil
}

func fetchFeedWithRetry(url string, attempts int) ([]Entry, error) {
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		entries, err := fetchFeed(url)
		if err == nil {
			return entries, nil
		}
		lastErr = err
		log.Printf("Fetch attempt %d/%d failed for %s: %v", attempt+1, attempts, url, err)
		if attempt < attempts-1 {
			time.Sleep(time.Duration(attempt+1) * time.Second)
		}
	}
	return nil, lastErr
}

func refreshFeeds(cfg Config) {
	setUpdating(true)
	defer setUpdating(false)

	total := len(cfg.Channels)
	successCount := 0
	failedChannels := make([]string, 0)

	for name, channelID := range cfg.Channels {
		feedURL := "https://www.youtube.com/feeds/videos.xml?channel_id=" + channelID
		log.Printf("Fetching channel %s...", name)

		entries, err := fetchFeedWithRetry(feedURL, 3)
		if err != nil {
			log.Printf("Error fetching channel %s: %v", name, err)
			failedChannels = append(failedChannels, name)

			channelMutex.RLock()
			_, hasCache := channelCaches[channelID]
			channelMutex.RUnlock()
			if hasCache {
				log.Printf("Keeping cached entries for channel %s", name)
			}
			continue
		}

		log.Printf("Fetched %d entries for channel %s", len(entries), name)
		successCount++

		channelMutex.Lock()
		channelCaches[channelID] = entries
		channelMutex.Unlock()
	}

	mergeEntries()

	statusMutex.Lock()
	feedStatus = FeedStatus{
		TotalChannels:  total,
		SuccessCount:   successCount,
		FailedChannels: failedChannels,
		LastUpdate:     time.Now(),
	}
	statusMutex.Unlock()

	log.Printf("Feed update complete: %d/%d channels, %d videos", successCount, total, len(getEntries()))
}

func mergeEntries() {
	channelMutex.RLock()
	var merged []Entry
	for _, entries := range channelCaches {
		merged = append(merged, entries...)
	}
	channelMutex.RUnlock()

	twoYearsAgo := time.Now().AddDate(-2, 0, 0)
	filtered := make([]Entry, 0, len(merged))
	for _, entry := range merged {
		if entry.PubDate.After(twoYearsAgo) {
			filtered = append(filtered, entry)
		}
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].PubDate.After(filtered[j].PubDate)
	})

	feedsMutex.Lock()
	allEntries = filtered
	feedsMutex.Unlock()
}

func setUpdating(updating bool) {
	statusMutex.Lock()
	feedStatus.Updating = updating
	statusMutex.Unlock()
}

func getEntries() []Entry {
	feedsMutex.RLock()
	defer feedsMutex.RUnlock()
	return allEntries
}

func getStatus() FeedStatus {
	statusMutex.RLock()
	defer statusMutex.RUnlock()
	return feedStatus
}

func hashURL(url string) string {
	hasher := md5.New()
	hasher.Write([]byte(url))
	return hex.EncodeToString(hasher.Sum(nil))
}

func cacheURL(originalURL string) string {
	hashedURL := hashURL(originalURL)
	ext := path.Ext(originalURL)
	cacheMutex.Lock()
	urlCache[hashedURL] = originalURL
	cacheMutex.Unlock()
	return "/proxy/" + hashedURL + ext
}

func getOriginalURL(hashedURL string) (string, bool) {
	cacheMutex.RLock()
	originalURL, exists := urlCache[strings.TrimSuffix(hashedURL, path.Ext(hashedURL))]
	cacheMutex.RUnlock()
	return originalURL, exists
}

func proxyHandler(w http.ResponseWriter, r *http.Request) {
	hashedFilename := path.Base(r.URL.Path)

	originalURL, exists := getOriginalURL(hashedFilename)
	if !exists {
		http.Error(w, "Image not found", http.StatusNotFound)
		return
	}

	resp, err := httpClient.Get(originalURL)
	if err != nil {
		http.Error(w, "Failed to fetch image", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.Header().Set("Cache-Control", "public, max-age=31536000")

	if _, err := io.Copy(w, resp.Body); err != nil {
		log.Printf("Error streaming image: %v", err)
	}
}

func feedHandler(w http.ResponseWriter, r *http.Request) {
	data := PageData{
		Entries: getEntries(),
		Status:  getStatus(),
	}

	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("Template execution error: %v", err)
		http.Error(w, "Error rendering template", http.StatusInternalServerError)
		return
	}
}

func timeAgo(t time.Time) string {
	if t.IsZero() {
		return "never"
	}

	diff := time.Since(t)

	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		if diff.Minutes() == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", int(diff.Minutes()))
	case diff < 24*time.Hour:
		if diff.Hours() == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", int(diff.Hours()))
	case diff < 30*24*time.Hour:
		days := int(diff.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	case diff < 365*24*time.Hour:
		months := int(diff.Hours() / (24 * 30))
		if months == 1 {
			return "1 month ago"
		}
		return fmt.Sprintf("%d months ago", months)
	default:
		return t.Format("Jan 2, 2006")
	}
}

func viewCount(views string) string {
	viewsInt := 0
	fmt.Sscanf(views, "%d", &viewsInt)
	switch {
	case viewsInt < 1000:
		return fmt.Sprintf("%d views", viewsInt)
	case viewsInt < 10000:
		return fmt.Sprintf("%.1fK views", float64(viewsInt)/1000)
	case viewsInt < 1000000:
		return fmt.Sprintf("%.0fK views", float64(viewsInt)/1000)
	case viewsInt < 10000000:
		return fmt.Sprintf("%.1fM views", float64(viewsInt)/1000000)
	default:
		return fmt.Sprintf("%.0fM views", float64(viewsInt)/1000000)
	}
}
