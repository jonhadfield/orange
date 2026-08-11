# orange

A terminal reader for [Hacker News](https://news.ycombinator.com), built with
[Bubble Tea](https://github.com/charmbracelet/bubbletea). Browse the front
page, read threaded comments, and open stories in your browser without
leaving the terminal.

![orange showing the Hacker News front page](docs/screenshot.png)

## Features

orange knows the history and the momentum of a story, not just its snapshot:

- **Previously on HN** — opening a story shows earlier submissions of the
  same link; press `1`–`5` to jump straight into a past discussion

  ![a comment thread with past discussions and a clickable URL](docs/story.png)
- **Pulse** (`p`) — an auto-refreshing live view of the front page with rank
  movement (`↑3`), score velocity (`+18`), and new-arrival markers

  ![the pulse view tracking rank and score movement](docs/pulse.png)
- **Watched threads** (`w` / `W`) — watch a discussion and orange counts the
  comments posted since you last read it and marks each new one in the tree
- **Who is hiring?** (`H`) — finds the latest monthly hiring thread and lets
  you filter job posts by keyword (`/ remote golang`)

And the fundamentals:

- All six HN feeds: Top, New, Best, Ask, Show, and Jobs
- Threaded, collapsible comment trees, streamed in as they load so large
  discussions stay responsive
- Comment HTML rendered as readable text: paragraphs, emphasis, links, and
  indented code blocks
- Vim-style navigation alongside the arrow keys
- Domains and links are clickable ([OSC 8] hyperlinks) in terminals that
  support them — iTerm2, Kitty, WezTerm, Ghostty, and others; IDE-embedded
  terminals (e.g. JetBrains) may ignore them, where `o`/`c` still work
- Adapts to light and dark terminal backgrounds
- Fetches concurrently and caches items in memory, so switching feeds and
  revisiting stories is instant
- No configuration and no login — it talks only to the official
  [Hacker News API](https://github.com/HackerNews/API) and the
  [HN Algolia API](https://hn.algolia.com/api); the watch list lives in a
  local JSON file

## Install

With [Homebrew](https://brew.sh):

```sh
brew install jonhadfield/tap/orange
```

Always use the fully-qualified name: an unrelated Homebrew package
(Orange Data Mining) shares the `orange` token, so a bare
`brew install orange` or `brew upgrade orange` installs that instead.
Upgrade with `brew upgrade jonhadfield/tap/orange`.

Or with Go 1.26 or later:

```sh
go install github.com/jonhadfield/orange/cmd/orange@latest
```

Or from a checkout:

```sh
go build -o orange ./cmd/orange
```

## Usage

```sh
orange
```

### Keys

| Key                 | Action                                        |
| ------------------- | --------------------------------------------- |
| `j` / `k`, `↓` / `↑`| Move selection / scroll                       |
| `g` / `G`           | Jump to top / bottom                          |
| `ctrl+d` / `ctrl+u` | Scroll half a page; the selection follows      |
| Mouse wheel / trackpad | Scroll; the selection follows              |
| `tab` / `shift+tab` | Next / previous feed                          |
| `1`–`6`             | Jump straight to a feed                       |
| `enter` / `l`       | Open story; in a thread, fold/unfold comment  |
| `1`–`5` (in thread) | Open a past discussion of the same link       |
| `o`                 | Open the story link in your browser           |
| `c`                 | Open the HN discussion page in your browser   |
| `w`                 | Watch / unwatch the discussion                |
| `W`                 | Watched stories, with new-comment counts      |
| `p`                 | Pulse: live front page with velocity          |
| `H`                 | Who is hiring? browser                        |
| `/` (in hiring)     | Filter job posts by keywords                  |
| `r`                 | Refresh the current view                      |
| `esc` / `h` / `b`   | Back                                          |
| `?`                 | Toggle help                                   |
| `q`                 | Quit                                          |

The feed hotkeys (`1`–`6`) and the `p`/`H`/`W` views are shown in the top
bar, so nothing is hidden behind the help screen.

## Development

```sh
go test ./...
```

Layout: `internal/hn` is the API client (official Firebase API plus the
Algolia search API), `internal/htmltext` converts HN comment HTML to
terminal text, `internal/store` persists the watch list, and `internal/ui`
holds the Bubble Tea models.

## License

[MIT](LICENSE)

[OSC 8]: https://gist.github.com/egmontkob/eb114294efbcd5adb1944c9f3cb5feda
