package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// MatchResult is a unified representation of a matched post or comment.
type MatchResult struct {
	ID        string   // Reddit fullname from the feed <id> field (e.g. "t3_abc123")
	Type      string   // "post" or "comment"
	Subreddit string
	Author    string
	Title     string
	Body      string   // plain-text excerpt from the <content> field
	URL       string   // direct link to the post/comment
	Keyword   string   // matched phrase (original casing from keywordRules)
	Priority  Priority
	CreatedAt time.Time
}

// ─────────────────────────────────────────────
// RSS / Atom feed parsing
// ─────────────────────────────────────────────

// atomFeed is the minimal structure of Reddit's Atom feed.
type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	ID      string    `xml:"id"`
	Title   string    `xml:"title"`
	Updated string    `xml:"updated"`
	Link    atomLink  `xml:"link"`
	Author  atomAuthor `xml:"author"`
	Content string    `xml:"content"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
}

type atomAuthor struct {
	Name string `xml:"name"`
}

// htmlTagRE strips HTML tags when extracting plain text from <content>.
var htmlTagRE = regexp.MustCompile(`<[^>]+>`)

// plainText strips HTML tags and unescapes HTML entities.
func plainText(s string) string {
	s = htmlTagRE.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	// Collapse whitespace
	s = strings.Join(strings.Fields(s), " ")
	return s
}

// fetchFeed fetches and parses an Atom feed from url using a descriptive UA.
// On HTTP 429 it respects the Retry-After header and retries up to maxRetries
// times with exponential backoff + jitter before giving up.
func fetchFeed(ctx context.Context, url string) (*atomFeed, error) {
	const maxRetries = 4
	backoff := 10 * time.Second

	for attempt := 0; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "SmokingTrackerMonitor/1.0 (reddit rss watcher; contact via reddit)")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			wait := backoff + time.Duration(rand.N(int64(backoff/2)))
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if secs, err := strconv.Atoi(ra); err == nil {
					wait = time.Duration(secs)*time.Second + time.Duration(rand.N(int64(3*time.Second)))
				}
			}
			resp.Body.Close()
			if attempt == maxRetries {
				return nil, fmt.Errorf("HTTP 429 from %s (exhausted retries)", url)
			}
			log.Printf("429 from %s — backing off %s (attempt %d/%d)", url, wait.Round(time.Second), attempt+1, maxRetries)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
			backoff *= 2
			continue
		}

		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		var feed atomFeed
		if err := xml.Unmarshal(body, &feed); err != nil {
			return nil, fmt.Errorf("parse feed: %w", err)
		}
		return &feed, nil
	}
	return nil, fmt.Errorf("fetchFeed: unreachable")
}

// ─────────────────────────────────────────────
// SeenStore — atomic deduplication across restarts
// ─────────────────────────────────────────────

type seenFile struct {
	IDs []string `json:"ids"`
}

// SeenStore persists seen IDs across restarts using a JSON file.
type SeenStore struct {
	mu   sync.Mutex
	path string
	ids  map[string]struct{}
}

// NewSeenStore loads seen.json at path, or starts fresh if the file is absent.
func NewSeenStore(path string) (*SeenStore, error) {
	s := &SeenStore{path: path, ids: make(map[string]struct{})}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read seen store: %w", err)
	}

	var f seenFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse seen store: %w", err)
	}
	for _, id := range f.IDs {
		s.ids[id] = struct{}{}
	}
	return s, nil
}

// Has reports whether id has already been seen.
func (s *SeenStore) Has(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.ids[id]
	return ok
}

// Add marks id as seen in memory and atomically rewrites the JSON file.
func (s *SeenStore) Add(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ids[id] = struct{}{}

	list := make([]string, 0, len(s.ids))
	for k := range s.ids {
		list = append(list, k)
	}

	data, err := json.Marshal(seenFile{IDs: list})
	if err != nil {
		return fmt.Errorf("marshal seen store: %w", err)
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write seen store tmp: %w", err)
	}
	return os.Rename(tmp, s.path)
}

// firstRun returns true when the store is empty (the very first poll cycle).
func (s *SeenStore) firstRun() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.ids) == 0
}

// ─────────────────────────────────────────────
// Keyword matching
// ─────────────────────────────────────────────

// MatchKeywords checks text against the keyword rules (HIGH first).
// Returns the matched phrase (original casing), its priority, and true on match.
// Posts that match a negativeKeyword are excluded regardless of positive matches.
func MatchKeywords(text string) (keyword string, p Priority, matched bool) {
	lower := strings.ToLower(text)
	for _, neg := range negativeKeywords {
		if strings.Contains(lower, neg) {
			return "", 0, false
		}
	}
	for _, rule := range keywordRules {
		if strings.Contains(lower, rule.Phrase) {
			return rule.Phrase, rule.Priority, true
		}
	}
	return "", 0, false
}

// ─────────────────────────────────────────────
// Polling helpers
// ─────────────────────────────────────────────

// pollFeed fetches an RSS feed url, checks unseen entries against keywords,
// and returns MatchResults. feedType should be "post" or "comment".
// On the very first run all IDs are primed into the store without notifying,
// to prevent a startup flood of old content.
func pollFeed(ctx context.Context, subreddit, feedType, url string, seen *SeenStore) ([]MatchResult, error) {
	firstRun := seen.firstRun()

	feed, err := fetchFeed(ctx, url)
	if err != nil {
		return nil, err
	}

	var results []MatchResult
	for _, entry := range feed.Entries {
		id := entry.ID
		if seen.Has(id) {
			continue
		}

		if !firstRun {
			body := plainText(entry.Content)

			// Reddit comment RSS titles look like "/u/Name on Post Title" —
			// skip them so we only surface the original post, not every reply.
			if feedType == "comment" && strings.HasPrefix(entry.Title, "/u/") {
				seen.Add(id) //nolint:errcheck
				continue
			}

			text := entry.Title + " " + body

			if kw, prio, ok := MatchKeywords(text); ok {
				author := strings.TrimPrefix(entry.Author.Name, "/u/")

				createdAt := time.Now().UTC()
				if t, err := time.Parse(time.RFC3339, entry.Updated); err == nil {
					createdAt = t
				}

				results = append(results, MatchResult{
					ID:        id,
					Type:      feedType,
					Subreddit: subreddit,
					Author:    author,
					Title:     entry.Title,
					Body:      body,
					URL:       entry.Link.Href,
					Keyword:   kw,
					Priority:  prio,
					CreatedAt: createdAt,
				})
			}
		}

		if err := seen.Add(id); err != nil {
			log.Printf("warn: could not persist seen ID %s: %v", id, err)
		}
	}
	return results, nil
}

// ─────────────────────────────────────────────
// Main polling loop
// ─────────────────────────────────────────────

// RunMonitor polls all subreddits on interval, calling send() for each match.
// Errors are logged but do not stop the loop. Exits when ctx is cancelled.
// If 5 consecutive IP-level rate limits are exhausted, notify is called and
// stop cancels the context so the process exits cleanly.
func RunMonitor(
	ctx context.Context,
	stop func(),
	seen *SeenStore,
	send func(MatchResult) error,
	notify func(title, desc string) error,
	interval time.Duration,
) {
	// ipCooldown pauses all requests when Reddit throttles the IP.
	// Per-URL backoff inside fetchFeed handles transient 429s; this handles
	// the case where the whole IP is blocked and retries are exhausted.
	const ipCooldown = 4 * time.Minute
	const maxConsecutiveRL = 5
	consecutiveRL := 0

	tick := func() {
		for _, sub := range Subreddits {
			for _, feed := range []struct {
				feedType    string
				url         string
				primaryOnly bool
			}{
				{"post", fmt.Sprintf("https://www.reddit.com/r/%s/new.rss", sub), false},
				// Comment feeds only for primary cannabis subreddits — broader
				// subreddits generate too many irrelevant comments and burn quota.
				{"comment", fmt.Sprintf("https://www.reddit.com/r/%s/comments.rss", sub), true},
			} {
				if feed.primaryOnly && !PrimarySubreddit[sub] {
					continue
				}

				results, err := pollFeed(ctx, sub, feed.feedType, feed.url, seen)
				if err != nil {
					log.Printf("error polling %s feed r/%s: %v", feed.feedType, sub, err)
					if strings.Contains(err.Error(), "429") {
						consecutiveRL++
						log.Printf("IP-level rate limit detected — pausing %s before next request (%d/%d consecutive)",
							ipCooldown, consecutiveRL, maxConsecutiveRL)
						if consecutiveRL >= maxConsecutiveRL {
							log.Printf("5 consecutive rate limits — sending alert and shutting down")
							if err := notify(
								"Rate limit threshold reached",
								"Reddit has returned 5 consecutive IP-level rate limits. The monitor is shutting down. Restart manually once Reddit access recovers.",
							); err != nil {
								log.Printf("warn: could not send rate-limit alert: %v", err)
							}
							stop()
							return
						}
						select {
						case <-ctx.Done():
							return
						case <-time.After(ipCooldown):
						}
					}
					continue
				}
				consecutiveRL = 0
				for _, m := range results {
					log.Printf("[%s] %s match in r/%s — %q — %s",
						PriorityLabel(m.Priority), m.Type, m.Subreddit, m.Keyword, m.URL)
					if err := send(m); err != nil {
						log.Printf("error sending webhook for %s: %v", m.ID, err)
					}
				}

				select {
				case <-ctx.Done():
					return
				case <-time.After(2 * time.Second):
				}
			}
		}
	}

	log.Printf("Starting monitor — polling %d subreddits every %s", len(Subreddits), interval)
	tick() // first call primes seen store; no notifications sent

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tick()
		}
	}
}
