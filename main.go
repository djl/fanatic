package main

import (
	"crypto/sha1"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/jbub/podcasts"
)

const progname = "rollinss"
const version = "1.0.0"

const endpoint = "https://www.kcrw.com/music/shows/henry-rollins"
const mp3link = "https://od-media.kcrw.com/kcrw/audio/website/music/hr/KCRW-henry_rollins-kcrw_broadcast_%d-%s.mp3"

type Episode struct {
	Title    string
	Link     string
	MP3      string
	UUID     string
	PubDate  time.Time
	Duration time.Duration
}

// This doesn't really generate a UUID. I'm too lazy to grab the JSON
// and get the real UUID. sha1 here is good enough
func genUUID(s string) string {
	h := sha1.New()
	h.Write([]byte(s))
	bs := h.Sum(nil)
	return fmt.Sprintf("%x", bs)
}

// Take a string like "2h, 2min" and return a time.Duration
func getDuration(s string) time.Duration {
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, "hr", "h")
	s = strings.ReplaceAll(s, "min", "m")
	duration, err := time.ParseDuration(s)
	if err != nil {
		duration = time.Second * 120
	}
	return duration
}

// Given an episode number and time.Time, return a URL to
// the MP3 file for that episode.
func getMP3URL(epnum int, pubdate time.Time) string {
	return fmt.Sprintf(mp3link, epnum, pubdate.Format("060102"))
}

// Get the episodes from the endpoint
// Errors will likely be either HTTP errors or HTML parsing errors
// (e.g. the HTML changed and this needs to be rewritten accordingly)
func fetchEpisodes(url string) ([]Episode, error) {
	res, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		err = errors.New(fmt.Sprintf("status code error: %d %s", res.StatusCode, res.Status))
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return nil, err
	}

	var episodes []Episode

	doc.Find("div#episodes div.four-col div.single").Each(func(i int, s *goquery.Selection) {
		link, _ := s.Find("a.title-link").Attr("href")
		title := s.Find("h3").Text()
		parts := strings.Split(title, " ")
		epnumStr := parts[len(parts)-1]

		// No episode number or no pub date means we can't generate
		// the link to the MP3, so just return here
		epnum, err := strconv.Atoi(epnumStr)
		if err != nil {
			return
		}

		var pubdate time.Time
		datestr, exists := s.Find("time.pubdate").Attr("datetime")
		if exists {
			parsed, err := time.Parse("2006-01-02T15:04:05Z", datestr)
			if err != nil {
				return
			}
			// The time.pubdate HTML is always off by one day for some reason
			// so we need to subtract one day
			pubdate = parsed.AddDate(0, 0, -1)
		}

		duration := getDuration(s.Find("span.duration").Text())
		mp3 := getMP3URL(epnum, pubdate)

		episode := Episode{
			Title:    title,
			Link:     link,
			MP3:      mp3,
			UUID:     genUUID(fmt.Sprintf("%s-%s", title, mp3)),
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
