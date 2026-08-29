package hn

import "github.com/jonhadfield/orange/internal/htmltext"

// Item is a Hacker News item: story, comment, job, poll, or pollopt.
type Item struct {
	ID          int    `json:"id"`
	Type        string `json:"type"`
	By          string `json:"by"`
	Time        int64  `json:"time"`
	Text        string `json:"text"`
	Dead        bool   `json:"dead"`
	Deleted     bool   `json:"deleted"`
	Parent      int    `json:"parent"`
	Kids        []int  `json:"kids"`
	URL         string `json:"url"`
	Score       int    `json:"score"`
	Title       string `json:"title"`
	Descendants int    `json:"descendants"`
}

// sanitize strips control characters from the strings that came off the
// wire. Everything here is rendered into a terminal — titles and authors
// without going anywhere near the HTML renderer — so this is the one place
// that can guarantee none of it carries a sequence the terminal would act
// on.
func (it *Item) sanitize() {
	it.Type = htmltext.StripControl(it.Type)
	it.By = htmltext.StripControl(it.By)
	it.Text = htmltext.StripControl(it.Text)
	it.URL = htmltext.StripControl(it.URL)
	it.Title = htmltext.StripControl(it.Title)
}

// Feed identifies one of the Hacker News story feeds.
type Feed string

const (
	FeedTop  Feed = "topstories"
	FeedNew  Feed = "newstories"
	FeedBest Feed = "beststories"
	FeedAsk  Feed = "askstories"
	FeedShow Feed = "showstories"
	FeedJobs Feed = "jobstories"
)
