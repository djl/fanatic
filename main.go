package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/jbub/podcasts"
	"github.com/tidwall/gjson"
)

const progname = "rollinss"
const version = "1.0.1"

const endpoint = "https://www.kcrw.com/music/shows/henry-rollins"

type Episode struct {
	Title    string
	Link     string
	MP3      string
	UUID     string
	PubDate  time.Time
	Duration time.Duration
}

// Fetch given URL
func get(url string) (string, error) {
	res, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		err = errors.New(fmt.Sprintf("status code error: %d %s", res.StatusCode, res.Status))
		return "", err
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

// Get the episodes from the endpoint
// Errors will likely be either HTTP errors or HTML parsing errors
// (e.g. the HTML changed and this needs to be rewritten accordingly)
func fetchEpisodes(url string) ([]Episode, error) {
	res, err := get(url)
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(res))
	if err != nil {
		return nil, err
	}

	var episodes []Episode

	doc.Find("div.four-col.hub-row.no-border button.audio").Each(func(i int, s *goquery.Selection) {
		jurl, exists := s.Attr("data-player-json")
		if !exists {
			return
		}

		res, err := get(jurl)
		if err != nil {
			return
		}

		json := string(res)
		id := gjson.Get(json, "uuid").String()
		link := gjson.Get(json, "url").String()
		title := gjson.Get(json, "title").String()
		mp3 := gjson.Get(json, "media.0.url").String()

		durstr := gjson.Get(json, "duration").Int()
		duration, err := time.ParseDuration(fmt.Sprintf("%ds", durstr))
		if err != nil {
			return
		}

		var pubdate time.Time
		datestr := gjson.Get(json, "date").String()
		parsed, err := time.Parse("2006-01-02T15:04:05Z", datestr)
		if err != nil {
			return
		}
		pubdate = parsed.AddDate(0, 0, -1)

		episode := Episode{
			Title:    title,
			Link:     link,
			MP3:      mp3,
			UUID:     id,
			PubDate:  pubdate,
			Duration: duration,
		}

		episodes = append(episodes, episode)
	})

	if len(episodes) < 1 {
		return nil, errors.New("No episodes found.")
	}

	return episodes, nil
}

func main() {
	var fileName = flag.String("f", "", "file to write to (default: stdout)")
	var showVersion = flag.Bool("v", false, "show version information and exit")

	flag.Parse()

	if *showVersion {
		fmt.Printf("%s %s\n", progname, version)
		os.Exit(0)
	}

	episodes, err := fetchEpisodes(endpoint)
	if err != nil {
		log.Fatal(err)
	}

	podcast := podcasts.Podcast{
		Title:       "Henry Rollins - KCRW",
		Description: "Henry Rollins hosts a mix of all kinds, from all over and all time.",
		Language:    "EN",
		Copyright:   "KCRW",
		Link:        endpoint,
	}

	for _, episode := range episodes {
		podcast.AddItem(&podcasts.Item{
			Title:    episode.Title,
			GUID:     episode.UUID,
			Duration: podcasts.NewDuration(episode.Duration),
			Enclosure: &podcasts.Enclosure{
				URL:  episode.MP3,
				Type: "MP3",
			},
			PubDate: podcasts.NewPubDate(episode.PubDate),
		})
	}

	feed, err := podcast.Feed()
	if err != nil {
		log.Fatal(err)
	}

	var file io.Writer = os.Stdout

	if *fileName != "" {
		file, err = os.Create(*fileName)
		if err != nil {
			log.Fatal(err)
		}
	}

	feed.Write(file)
}
