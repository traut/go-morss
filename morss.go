package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"

	"github.com/gorilla/feeds"
	"github.com/mmcdole/gofeed"

	readability "github.com/go-shiori/go-readability"
)

const (
	defaultIP          = "0.0.0.0"
	defaultPort        = 8080
	defaultItemsCap    = 10
	defaultFromDaysAgo = time.Duration(30*24) * time.Hour
	defaultHTTPTimeout = time.Duration(30) * time.Second
	badgePage          = `
<div style="font-family:sans-serif">

<p><a href="https://github.com/traut/go-morss"><code>go-morss</code></a> adds full content to the items in RSS / Atom / JSON feeds.</p>
<p>
	To use <code>go-morss</code> API, add a feed URL without a schema to the root endpoint:
	<pre>&lt;go-morss-domain&gt;/&lt;feed-url-without-schema&gt;</pre>
	For example, <a href="/news.ycombinator.com/rss"><code>&lt;this-domain&gt;/news.ycombinator.com/rss</code></a>.
	You can use this new URL in a feed reader or download the feed with a HTTP GET request.
</p>
<p>
Note:
	<ul>
	<li>the schema <code>https://</code> is assumed for a feed URL</li>
	<li>the new feed is returned in the format of the original feed (RSS / Atom / JSON)
	<li>there is no cache support at the moment</li>
	</ul>
</p>
</div>
`
	fromTimeParamName = "from_time"
	itemsCapParamName = "items_cap"
)

// https://techblog.willshouse.com/2012/01/03/most-common-user-agents/
var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:133.0) Gecko/20100101 Firefox/133.0",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64; rv:133.0) Gecko/20100101 Firefox/133.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:133.0) Gecko/20100101 Firefox/133.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1.1 Safari/605.1.15",
	"Mozilla/5.0 (X11; Linux x86_64; rv:128.0) Gecko/20100101 Firefox/128.0",
	"Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:133.0) Gecko/20100101 Firefox/133.0",
}

func getLogHandler() *log.Logger {
	handler := log.New(os.Stderr)
	handler.SetReportTimestamp(true)
	handler.SetLevel(log.DebugLevel)
	return handler
}

// Produce a new slice by applying function fn to items of the slice s.
func FnMap[I, O any](s []I, fn func(I) O) []O {
	if s == nil {
		return nil
	}
	out := make([]O, len(s))
	for i, v := range s {
		out[i] = fn(v)
	}
	return out
}

func isValidUrl(value string) bool {
	u, err := url.Parse(value)
	return err == nil && u.Scheme != "" && u.Host != ""
}

func getRandUserAgent() string {
	return userAgents[rand.Intn(len(userAgents))]
}

func fetchFeed(ctx context.Context, url string) (*gofeed.Feed, error) {
	parser := gofeed.NewParser()
	parser.UserAgent = getRandUserAgent()
	// parser.AuthConfig = &gofeed.Auth{
	// 	Username: basicAuth.GetAttrVal("username").AsString(),
	// 	Password: basicAuth.GetAttrVal("password").AsString(),
	// }
	feed, err := parser.ParseURLWithContext(url, ctx)
	return feed, err
}

func fetchFeedItems(ctx context.Context, feed *gofeed.Feed, from time.Time, itemsCap int) *gofeed.Feed {
	log := slog.New(getLogHandler())
	log = log.With("feed_url", feed.Link, "from_time", from, "items_cap", itemsCap)
	log.InfoContext(ctx, "Fetching items for the feed")

	wg := sync.WaitGroup{}
	count := 0
	for i := range feed.Items {

		if count >= itemsCap {
			log.InfoContext(ctx, "Items cap reached", "feed_items_count", len(feed.Items))
			break
		}

		item := feed.Items[i]

		if item.Content != "" {
			log.DebugContext(ctx, "The item has content, skipping", "item_title", item.Title)
			continue
		}

		var itemTime *time.Time
		if item.UpdatedParsed != nil {
			itemTime = item.UpdatedParsed
		} else if item.PublishedParsed != nil {
			itemTime = item.PublishedParsed
		}
		if itemTime == nil {
			log.DebugContext(ctx, "The item has no time set in it, skipping", "item", item.Title)
			continue
		} else if itemTime.Before(from) {
			log.DebugContext(ctx, "The item's time is outside the time window, skipping", "item_time", itemTime, "item", item.Title)
			continue
		}

		wg.Add(1)
		count += 1
		go func(item *gofeed.Item) {
			defer wg.Done()

			_log := log.With("item", item.Title, "item_link", item.Link)
			_log.DebugContext(ctx, "Fetching content for the item")

			article, err := readability.FromURL(item.Link, defaultHTTPTimeout)
			if err != nil {
				_log.ErrorContext(ctx, "Failed to parse a page for the item", "err", err)
				return
			}
			item.Content = article.Content
		}(item)
	}
	wg.Wait()
	log.InfoContext(ctx, "All items has been fetched", "items_fetched_count", count)
	return feed
}

func getHandlerFunc(itemsCap int) func(w http.ResponseWriter, req *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		processRequest(w, req, itemsCap)
	}
}

func parseTimeParam(value string) (time.Time, error) {
	return time.Parse("2006-01-02T15:04:05Z", value)
}

func processRequest(w http.ResponseWriter, req *http.Request, itemsCapByServer int) {
	ctx := req.Context()
	log := slog.New(getLogHandler())

	feedURL := strings.TrimPrefix(req.URL.Path, "/")

	switch feedURL {
	case "":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, badgePage)
		return
	case "favicon.ico":
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	// Assume HTTPS schema
	feedURL = "https://" + feedURL
	if !isValidUrl(feedURL) {
		log.WarnContext(
			ctx,
			"No valid URL provided in the request",
			"remote_address",
			req.RemoteAddr,
			"path",
			req.URL.Path,
			"feed_url",
			feedURL,
		)
		http.Error(w, "No valid RSS URL found in the path", http.StatusBadRequest)
		return
	}

	query := req.URL.Query()

	// Get the lower limit for the time frame

	fromTimeDefault := time.Now().Add(-defaultFromDaysAgo)

	log.InfoContext(ctx, "DEFAULT", "def", fromTimeDefault, "dur")

	var fromTime time.Time
	fromTime = fromTimeDefault
	if len(query[fromTimeParamName]) > 0 {
		var err error
		fromTime, err = parseTimeParam(query[fromTimeParamName][0])
		if err != nil {
			errorMsg := fmt.Sprintf("Can't parse `%s` query param value", fromTimeParamName)
			log.ErrorContext(ctx, errorMsg, "err", err)
			http.Error(w, errorMsg, http.StatusBadRequest)
			return
		}
	}

	var itemsCap int
	itemsCap = itemsCapByServer
	if len(query[itemsCapParamName]) > 0 {
		var err error
		itemsCap, err = strconv.Atoi(query[itemsCapParamName][0])
		if err != nil {
			errorMsg := fmt.Sprintf("Can't parse `%s` query param value", fromTimeParamName)
			log.ErrorContext(ctx, errorMsg, "err", err)
			http.Error(w, errorMsg, http.StatusBadRequest)
		}
	}

	log = log.With("feed_url", feedURL, "from_time", fromTime)
	log.InfoContext(ctx, "Request with RSS URL received")

	feed, err := fetchFeed(ctx, feedURL)
	if err != nil {
		log.ErrorContext(ctx, "Can't fetch the feed", "err", err)
		return
	}
	log = log.With("feed_title", feed.Title)
	log.InfoContext(ctx, "Feed downloaded")

	feed = fetchFeedItems(ctx, feed, fromTime, itemsCap)
	log.InfoContext(ctx, "Feed items downloaded")

	feedNew := &feeds.Feed{
		Title:       fmt.Sprintf("%s (repacked by go-morss)", feed.Title),
		Link:        &feeds.Link{Href: feed.Link},
		Description: feed.Description,
	}
	if feed.UpdatedParsed != nil {
		feedNew.Updated = *feed.UpdatedParsed
	}
	if feed.PublishedParsed != nil {
		feedNew.Created = *feed.PublishedParsed
	}
	if len(feed.Authors) > 0 {
		feedNew.Author = &feeds.Author{Name: feed.Authors[0].Name, Email: feed.Authors[0].Email}
	}

	feedNew.Items = FnMap(feed.Items, func(item *gofeed.Item) *feeds.Item {
		itemNew := &feeds.Item{
			Id:          item.GUID,
			Title:       item.Title,
			Link:        &feeds.Link{Href: item.Link},
			Description: item.Description,
			Content:     item.Content,
		}
		if item.UpdatedParsed != nil {
			itemNew.Updated = *item.UpdatedParsed
		}
		if item.PublishedParsed != nil {
			itemNew.Created = *item.PublishedParsed
		}
		if len(item.Authors) > 0 {
			itemNew.Author = &feeds.Author{Name: item.Authors[0].Name, Email: item.Authors[0].Email}
		}
		return itemNew
	})
	log.InfoContext(ctx, "New feed prepared for serialization")

	switch feed.FeedType {
	case "rss":
		rssBody, err := feedNew.ToRss()
		if err != nil {
			log.ErrorContext(ctx, "Can't serialize new feed to RSS", "err", err)
			http.Error(w, "Server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprintln(w, rssBody)
	case "atom":
		atomBody, err := feedNew.ToAtom()
		if err != nil {
			log.ErrorContext(ctx, "Can't serialize new feed to Atom", "err", err)
			http.Error(w, "Server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprintln(w, atomBody)
	case "json":
		jsonBody, err := feedNew.ToJSON()
		if err != nil {
			log.ErrorContext(ctx, "Can't serialize new feed to JSON", "err", err)
			http.Error(w, "Server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		err = json.NewEncoder(w).Encode(jsonBody)
		if err != nil {
			log.ErrorContext(ctx, "Can't encode JSON feed", "err", err)
			http.Error(w, "Server error", http.StatusInternalServerError)
			return
		}
	}
}

func main() {
	ctx := context.Background()
	log := slog.New(getLogHandler())

	ipFlag := flag.String("ip", defaultIP, "IP to listen on")
	portFlag := flag.Int("port", defaultPort, "port to listen on")
	itemsCapFlag := flag.Int("items-cap", defaultItemsCap, "max last items per feed to process")

	flag.Parse()

	http.HandleFunc("GET /", getHandlerFunc(*itemsCapFlag))

	address := fmt.Sprintf("%s:%d", *ipFlag, *portFlag)
	log.InfoContext(ctx, "Starting the server", "address", address)
	err := http.ListenAndServe(address, nil)
	if err != nil {
		log.ErrorContext(ctx, "Can't start the server", "err", err)
		return
	}
}
