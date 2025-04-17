package main

import (
    "encoding/xml"
    "fmt"
    "gopkg.in/yaml.v2"
    "html/template"
    "log"
    "net/http"
    "os"
    "sort"
    "sync"
    "io"
    "bytes"
    "time"
    "strings"
    "path"
    "crypto/md5"
    "encoding/hex"
)

type Config struct {
    Channels map[string]string `yaml:"channels"`
}

type Feed struct {
    Entries []Entry `xml:"entry"`
}

type Entry struct {
    Title         string    `xml:"title"`
    Link          Link      `xml:"link"`
    Published     string    `xml:"published"`
    Updated       string    `xml:"updated"`
    PubDate       time.Time `xml:"-"`
    UpdateDate    time.Time `xml:"-"`
    Author        string    `xml:"author>name"`
    Thumbnail     string    `xml:"-"`
    Views         string    `xml:"-"`
    VideoID       string    `xml:"videoId"`
    MediaGroup    MediaGroup `xml:"group"`
}

type Link struct {
    Href string `xml:"href,attr"`
}

type MediaGroup struct {
    Thumbnail      Thumbnail      `xml:"thumbnail"`
    MediaCommunity MediaCommunity `xml:"community"`
    Description    string         `xml:"description"`
}

type Thumbnail struct {
    URL string `xml:"url,attr"`
}

type MediaCommunity struct {
    Statistics Statistics `xml:"statistics"`
    Rating     Rating    `xml:"starRating"`
}

type Statistics struct {
    Views string `xml:"views,attr"`
}

type Rating struct {
    Count   string `xml:"count,attr"`
    Average string `xml:"average,attr"`
}

var (
    allEntries = []Entry{}
    feedsMutex sync.RWMutex
    tmpl       *template.Template
    urlCache = make(map[string]string)
    cacheMutex sync.RWMutex
)

func main() {
    cfg, err := loadConfig("config.yaml")
    if err != nil {
        log.Fatalf("Error loading config: %v", err)
    }

    tmpl = template.Must(template.New("feed.html").Funcs(template.FuncMap{
        "timeAgo": timeAgo,
        "viewCount": viewCount,
        "truncate": truncateText,
        "likeCount": likeCount,
    }).ParseFiles("templates/feed.html"))

    go updateFeeds(cfg, 5*time.Minute)

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
    log.Printf("Fetching feed from: %s", url)
    resp, err := http.Get(url)
    if err != nil {
        log.Printf("Error fetching feed: %v", err)
        return nil, err
    }
    defer resp.Body.Close()

    body, err := io.ReadAll(resp.Body)
    if err != nil {
        log.Printf("Error reading response body: %v", err)
        return nil, err
    }

    resp.Body = io.NopCloser(bytes.NewReader(body))

    var feed Feed
    if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
        log.Printf("Error decoding XML: %v", err)
        return nil, err
    }

    log.Printf("Found %d entries in feed", len(feed.Entries))

    for i := range feed.Entries {
        feed.Entries[i].PubDate, _ = time.Parse(time.RFC3339, feed.Entries[i].Published)
        if feed.Entries[i].Updated != "" {
            feed.Entries[i].UpdateDate, _ = time.Parse(time.RFC3339, feed.Entries[i].Updated)
        }
        originalURL := feed.Entries[i].MediaGroup.Thumbnail.URL
        feed.Entries[i].Thumbnail = cacheURL(originalURL)
        feed.Entries[i].Views = feed.Entries[i].MediaGroup.MediaCommunity.Statistics.Views
    }

    return feed.Entries, nil
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
    // Extract the hashed filename from the URL
    hashedFilename := path.Base(r.URL.Path)
    
    // Get the original URL from our cache
    originalURL, exists := getOriginalURL(hashedFilename)
    if !exists {
        http.Error(w, "Image not found", http.StatusNotFound)
        return
    }

    // Fetch the image from the original URL
    resp, err := http.Get(originalURL)
    if err != nil {
        http.Error(w, "Failed to fetch image", http.StatusInternalServerError)
        return
    }
    defer resp.Body.Close()

    // Copy the content type header
    w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
    w.Header().Set("Cache-Control", "public, max-age=31536000") // Cache for 1 year

    // Stream the image data to the response
    if _, err := io.Copy(w, resp.Body); err != nil {
        log.Printf("Error streaming image: %v", err)
    }
}

func updateFeeds(cfg Config, interval time.Duration) {
    for {
        log.Printf("Starting feed update...")
        var newEntries []Entry
        for name, url := range cfg.Channels {
            log.Printf("Fetching channel %s...", name)
            entries, err := fetchFeed("https://www.youtube.com/feeds/videos.xml?channel_id=" + url)
            if err != nil {
                log.Printf("Error fetching channel %s: %v", name, err)
                continue
            }
            newEntries = append(newEntries, entries...)
        }

        log.Printf("Total entries fetched: %d", len(newEntries))

        // Filter out entries older than 2 years
        twoYearsAgo := time.Now().AddDate(-2, 0, 0)
        filteredEntries := make([]Entry, 0)
        for _, entry := range newEntries {
            if entry.PubDate.After(twoYearsAgo) {
                filteredEntries = append(filteredEntries, entry)
            }
        }
        log.Printf("Entries after filtering (< 2 years old): %d", len(filteredEntries))

        sort.Slice(filteredEntries, func(i, j int) bool {
            return filteredEntries[i].PubDate.After(filteredEntries[j].PubDate)
        })

        feedsMutex.Lock()
        allEntries = filteredEntries
        feedsMutex.Unlock()

        log.Printf("Feed update complete. Next update in %v", interval)
        time.Sleep(interval)
    }
}

func feedHandler(w http.ResponseWriter, r *http.Request) {
    feedsMutex.RLock()
    defer feedsMutex.RUnlock()

    if _, err := r.Cookie("visited"); err != nil {
        http.SetCookie(w, &http.Cookie{
            Name:   "visited",
            Value:  "true",
            Path:   "/",
            MaxAge: 3600 * 24 * 365,
        })
    }

    if err := tmpl.Execute(w, allEntries); err != nil {
        log.Printf("Template execution error: %v", err)
        http.Error(w, "Error rendering template", http.StatusInternalServerError)
        return
    }
}

func timeAgo(t time.Time) string {
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

func truncateText(text string, length int) string {
    if len(text) <= length {
        return text
    }
    return text[:length] + "..."
}

func likeCount(likes string) string {
    likesInt := 0
    fmt.Sscanf(likes, "%d", &likesInt)
    switch {
    case likesInt < 1000:
        return fmt.Sprintf("%d", likesInt)
    case likesInt < 10000:
        return fmt.Sprintf("%.1fK", float64(likesInt)/1000)
    case likesInt < 1000000:
        return fmt.Sprintf("%.0fK", float64(likesInt)/1000)
    case likesInt < 10000000:
        return fmt.Sprintf("%.1fM", float64(likesInt)/1000000)
    default:
        return fmt.Sprintf("%.0fM", float64(likesInt)/1000000)
    }
}
