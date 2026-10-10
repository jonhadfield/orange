package ui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jonhadfield/orange/internal/hn"
)

// runThreadLoad runs the command beginLoad returned and hands back the
// thread it fetched, skipping the spinner tick batched alongside it.
func runThreadLoad(t *testing.T, cmd tea.Cmd) hiringThreadMsg {
	t.Helper()
	msgs := []tea.Msg{cmd()}
	if batch, ok := msgs[0].(tea.BatchMsg); ok {
		msgs = msgs[:0]
		for _, c := range batch {
			if c != nil {
				msgs = append(msgs, c())
			}
		}
	}
	for _, msg := range msgs {
		if th, ok := msg.(hiringThreadMsg); ok {
			return th
		}
	}
	t.Fatalf("the load produced no hiringThreadMsg: %#v", msgs)
	return hiringThreadMsg{}
}

// TestHiringReloadSeesNewPosts: a monthly thread gains hundreds of posts
// over its first day, and r is how a reader picks them up. Reloading has to
// fetch the thread itself again; the copy in the item cache still lists
// only the posts there were the first time.
func TestHiringReloadSeesNewPosts(t *testing.T) {
	var kids atomic.Value
	kids.Store("[1]")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/item/99.json" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, `{"id":99,"type":"story","title":"Ask HN: Who is hiring?","kids":%s}`, kids.Load())
	}))
	defer srv.Close()

	m := newHiringModel(hn.NewClient(srv.URL), newKeyMap())
	m.setSize(100, 20)
	m.threads = []hn.HiringThread{{ID: 99, Title: "Ask HN: Who is hiring?", Label: "hiring"}}

	m, cmd := m.beginLoad()
	if got := runThreadLoad(t, cmd).thread.Kids; len(got) != 1 {
		t.Fatalf("first load found posts %v, want [1]", got)
	}

	kids.Store("[1,2]")
	(&m).reset()
	m, cmd = m.beginLoad()
	if got := runThreadLoad(t, cmd).thread.Kids; len(got) != 2 {
		t.Errorf("reload found posts %v, want [1 2]: the thread came from the cache", got)
	}
}

// TestHiringStaleProbeDoesNotStartASecondLoad: re-entering Hiring sends a
// probe for newer threads. If the reader switches thread with tab before
// the probe replies, the reply is out of date. Acting on it starts a second
// load of the thread tab already asked for, and every post is listed twice.
func TestHiringStaleProbeDoesNotStartASecondLoad(t *testing.T) {
	m := hiringWith("Acme | Go")
	m.thread = hn.Item{ID: 99, Title: "Ask HN: Who is hiring?"}
	m.threads = []hn.HiringThread{
		{ID: 99, Title: "Ask HN: Who is hiring?", Label: "hiring"},
		{ID: 100, Title: "Ask HN: Who wants to be hired?", Label: "seeking"},
	}

	m, probe := m.start()
	if probe == nil {
		t.Fatal("re-entry sent no probe")
	}
	stale := hiringCheckMsg{gen: m.gen, threads: m.threads}

	m, cmd := m.handleKey(keyPress("tab"))
	if cmd == nil || !m.loading {
		t.Fatal("tab did not start loading the seeking thread")
	}
	gen := m.gen

	m, cmd = m.Update(stale)
	if cmd != nil || m.gen != gen {
		t.Error("the stale probe started a second load")
	}
}

// TestHiringEmptyThreadSaysSo: early on the 1st of the month the new thread
// has no replies. Once it has loaded, the view must say so rather than keep
// showing a frozen "finding the latest hiring thread…".
func TestHiringEmptyThreadSaysSo(t *testing.T) {
	m := newHiringModel(nil, newKeyMap())
	m.setSize(100, 20)
	m.threads = []hn.HiringThread{{ID: 99, Title: "Ask HN: Who is hiring?", Label: "hiring"}}
	m, _ = m.beginLoad()

	m, _ = m.Update(hiringThreadMsg{gen: m.gen, thread: hn.Item{ID: 99, Title: "Ask HN: Who is hiring?"}})
	if m.loading {
		t.Fatal("an empty thread left the view loading")
	}
	view := stripStyles(m.View())
	if strings.Contains(view, "finding the latest") {
		t.Errorf("an empty thread still says it is searching:\n%s", view)
	}
	if !strings.Contains(view, "no posts") {
		t.Errorf("the view does not say the thread has no posts:\n%s", view)
	}
}
