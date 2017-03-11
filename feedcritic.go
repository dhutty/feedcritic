package main

import (
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {

	var mode = flag.Int("mode", 1, "Mode 1 downloads the feeds locally and mode 2 operates on locally downloaded files.")
	flag.Parse()
	var filesDownloaded = "files.json"
	// Based on the feeds in the OPML file, download each feed to 1.xml, 2.xml, etc.
	if *mode == 1 {
		bytes, _ := ioutil.ReadFile("antennapod-feeds.opml")
		var doc OPML
		xml.Unmarshal(bytes, &doc)
		var podcast PodcastFromOpml
		var count = 0
		var filemap map[string]PodcastFromOpml
		filemap = make(map[string]PodcastFromOpml)
		for _, outline := range doc.Body.Outlines {
			count++
			podcast.Title = outline.Title
			podcast.Feed = outline.XMLURL
			podcast.URL = outline.HTMLURL

			var xmlFile = strconv.Itoa(count) + ".xml"
			filemap[xmlFile] = podcast

			url := podcast.Feed

			response, e := http.Get(url)
			if e == nil {
				//log.Fatal(e)

				defer response.Body.Close()

				file, err := os.Create(xmlFile)
				if err != nil {
					//log.Fatal(err)
				}
				_, err = io.Copy(file, response.Body)
				if err != nil {
					//log.Fatal(err)
				}
				file.Close()
			}

		}
		// FIXME: Got this error so files.json wasn't created. :(
		// 2017/03/11 16:42:01 Get http://www.ladylovescode.com/category/podcast/feed/: dial tcp: lookup www.ladylovescode.com on 127.0.1.1:53: no such host
		//exit status 1

		jsonData, _ := json.MarshalIndent(filemap, "", "  ")
		ioutil.WriteFile(filesDownloaded, jsonData, 0644)
	}
	// Parse XML files downloaded previously (1.xml, 2.xml, etc.).
	if *mode == 2 {
		var podmap map[string]PodcastJson
		podmap = make(map[string]PodcastJson)
		file, e := ioutil.ReadFile(filesDownloaded)
		if e != nil {
			fmt.Printf("File error: %v\n", e)
			os.Exit(1)
		}
		var filemap map[string]PodcastFromOpml
		json.Unmarshal(file, &filemap)
		for k, v := range filemap {
			var podcast PodcastJson
			filename := k
			podcast.Filename = filename
			xml, _ := ioutil.ReadFile(filename)
			feed, feedOk, err := parseFeedContent(xml)
			if err == nil {
				if feedOk {
					podcast.Description = feed.Description
					podcast.Title = feed.Title
					podcast.URL = feed.Link

					smalldate := "9999999"
					bigdate := "0"
					for _, each := range feed.ItemList {
						// episodeTitle := strings.TrimSpace(each.Title)
						pubDate := ParsePubDate(each.PubDate)
						if pubDate == "0001-01-01" {
							pubDate = ParseDcDate(each.DcDate)
						}
						if pubDate > bigdate {
							bigdate = pubDate
						}
						if pubDate < smalldate {
							smalldate = pubDate
						}
					}
					podcast.Latest = bigdate

				}
			} else {
				//		fmt.Println("problem with " + filename + ": " + port)
			}
			podmap[v.Feed] = podcast
		}
		jsonData2, _ := json.MarshalIndent(podmap, "", "  ")
		//fmt.Println(string(jsonData2))
		ioutil.WriteFile("podcastdescriptions.json", jsonData2, 0644)
	}
}

type PodcastFromOpml struct {
	Title string
	Feed  string
	URL   string
}

type PodcastJson struct {
	Title       string
	Feed        string
	URL         string
	Description string
	Filename    string
	Latest      string
}

type OPML struct {
	Body Body `xml:"body"`
}

type Body struct {
	Outlines []Outline `xml:"outline"`
}

type Outline struct {
	Title   string `xml:"title,attr"`
	XMLURL  string `xml:"xmlUrl,attr"`
	HTMLURL string `xml:"htmlUrl,attr"`
}

type ByTitle []PodcastJson

func (a ByTitle) Len() int           { return len(a) }
func (a ByTitle) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByTitle) Less(i, j int) bool { return a[i].Title < a[j].Title }

// modified from https://github.com/siongui/userpages/blob/master/content/code/go-xml/parseFeed.go
func parseFeedContent(content []byte) (Rss2, bool, error) {
	v := Rss2{}
	err := xml.Unmarshal(content, &v)
	if err != nil {
		if err.Error() == atomErrStr {
			// try Atom 1.0
			//return parseAtom(content), err
		}
		//log.Println(err)
		return v, false, err
	}
	if v.Version == "2.0" {
		// RSS 2.0
		for i, _ := range v.ItemList {
			if v.ItemList[i].Content != "" {
				v.ItemList[i].Description = v.ItemList[i].Content
			}
		}
		return v, true, err
	}

	log.Println("not RSS 2.0")
	return v, false, err
}

type Rss2 struct {
	XMLName     xml.Name `xml:"rss"`
	Version     string   `xml:"version,attr"`
	Title       string   `xml:"channel>title"`
	Link        string   `xml:"channel>link"`
	Description string   `xml:"channel>description"`
	PubDate     string   `xml:"channel>pubDate"`
	ItemList    []Item   `xml:"channel>item"`
}

type Item struct {
	Title       string        `xml:"title"`
	Link        string        `xml:"link"`
	Description template.HTML `xml:"description"`
	Content     template.HTML `xml:"encoded"`
	PubDate     string        `xml:"pubDate"`
	DcDate      string        `xml:"date"`
}

const atomErrStr = "expected element type <rss> but have <feed>"

func ParsePubDate(datein string) string {
	//log.Println(datein)
	datein = strings.TrimSpace(datein)
	parsedTime, err := time.Parse(time.RFC1123Z, datein)
	if err != nil {
		parsedTime, err = time.Parse(time.RFC1123, datein)
		if err != nil {
			// added for http://leoville.tv/podcasts/floss.xml etc.
			parsedTime, err = time.Parse("Mon, _2 Jan 2006 15:04:05 -0700", datein)
			if err != nil {
				// Monday, 7 December 2015 9:30:00 EST
				parsedTime, err = time.Parse("Monday, _2 January 2006 15:04:05 MST", datein)
				if err != nil {
					// 22 Dec 2015 03:00:00 GMT
					parsedTime, err = time.Parse("02 Jan 2006 15:04:05 MST", datein)
				}
			}
		}
	}
	customTime := parsedTime.Format("2006-01-02")
	return customTime
}

func ParseDcDate(datein string) string {
	//log.Println(datein)
	parsedTime, _ := time.Parse(time.RFC3339, datein)
	customTime := parsedTime.Format("2006-01-02")
	return customTime
}