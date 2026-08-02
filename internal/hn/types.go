package hn

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
