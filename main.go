package main

import (
	"bytes"
	"errors"
	"fmt"
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

const progname = "fanatic"

const endpoint = "https://www.kcrw.com/music/shows/henry-rollins"

const html = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>fanatic!</title>
    <style type="text/css">
     body{font:0.8em sans-serif;margin:40px;}
     h1{font-size:1.2em;}
     h1 span{color:#ddd;}
     h1:hover span {color:black;}
     a:link,a:visited{border-bottom:1px solid #ccc;color:inherit;text-decoration:none;}
     a:hover,a:active{background:#ff0;}
     ul{margin:2em 0;padding:0;}
     ul li{line-height:1.2rem;list-style-type:none;}
     footer{bottom:40px;color:#ccc;position:absolute;}
    </style>
</head>
<body>
    <h1>fanatic!</h1>
    <p>providing an <a href="/rss.xml">RSS feed</a> for Henry Rollins' <a href="https://www.kcrw.com/music/shows/henry-rollins">KCRW show</a> (because they don't)</p>
    <footer>n.b. none of the shows are hosted here. be cool ~<a href="https://djl.io/">author</a></footer>
</body>
</html>
`

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

func generateXML() (string, error) {
	episodes, err := fetchEpisodes(endpoint)
	if err != nil {
		return "", err
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
		return "", nil
	}
	var b bytes.Buffer
	feed.Write(&b)
	return b.String(), nil

}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	xml, err := generateXML()
	go func() {
		ticker := time.NewTicker(time.Hour)
		for {
			<-ticker.C
			xml, err = generateXML()
			log.Println("Fetching XML...")
			if err != nil {
				log.Println(fmt.Sprintf("Error fetching XML: %s", err))
			}
		}
	}()

	http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if req.URL.Path != "/" {
			w.WriteHeader(404)
			w.Write([]byte("Not Found!"))
			return
		}

		w.Write([]byte(html))
		return
	})

	http.HandleFunc("/rss.xml", func(w http.ResponseWriter, req *http.Request) {
		if err != nil {
			w.Write([]byte(fmt.Sprintf("error!\n%s", err)))
			return
		}
		w.Header().Set("Content-Type", "text/xml")
		w.Write([]byte(xml))
	})

	log.Println("listening on", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
