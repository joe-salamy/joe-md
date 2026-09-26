# joe-md

A terminal markdown viewer with vim keys, a table of contents, editing in
[micro](https://micro-editor.github.io) on the line you are reading, ripgrep
search, tabs, split panes and a pop-up file menu.

Rendering uses [glamour](https://github.com/charmbracelet/glamour), the
library behind glow, so documents look the same as they do in glow.

## Install

```sh
go install github.com/joe-salamy/joe-md@latest
```

This needs Go 1.27 or newer and puts `joe-md` in `$GOBIN` (`~/go/bin` by
default), which should be on your `PATH`. Run the same command again to update;
`joe-md -version` shows what you have.

joe-md also runs two programs, which it expects on your `PATH`:

- [ripgrep](https://github.com/BurntSushi/ripgrep) (`rg`) for search
- [micro](https://micro-editor.github.io) for editing (`e`)

To build from a clone instead: `go build` in the repository puts `joe-md` in
the current directory.

## Usage

```sh
joe-md [flags] [FILE.md | DIR]...
```

Each file opens in its own tab. Given a directory, or nothing at all, joe-md
starts with the file menu open there (the current directory by default).

| Flag         | Default | Meaning                                              |
|--------------|---------|------------------------------------------------------|
| `-style`     | `auto`  | glamour style name or JSON path; honours `$GLAMOUR_STYLE` |
| `-width`     | `120`   | maximum word-wrap width                              |
| `-no-toc`    | off     | start with the table of contents hidden             |
| `-no-search-bar` | off | start with the search bar hidden (it still appears while typing) |
| `-no-tabs`   | off     | start with the tab bar hidden                        |
| `-config`    | see below | read this settings file instead (it must exist)    |
| `-dump-config` |       | print a settings file with every setting at its default |
| `-version`   |         | print the version                                    |

Flags override the settings file, which overrides the defaults.

## Settings

joe-md reads `$XDG_CONFIG_HOME/joe-md/config.toml` (`~/.config/joe-md/config.toml`
by default) if it exists. Start from the full template, where every setting is
commented out at its default:

```sh
mkdir -p ~/.config/joe-md
joe-md -dump-config > ~/.config/joe-md/config.toml
```

```toml
style = "tokyo-night"   # glamour style, as -style
width = 100             # maximum word-wrap width, as -width

[startup]               # what's shown at start-up
toc = true
search_bar = false
tabs = true

[search]
scope = "dir"           # where ? starts: file, dir or repo
literal = false         # fixed strings (rg -F) instead of regexes
case = "smart"          # ignore (rg -i), smart (rg -S) or sensitive

[theme]                 # ANSI 0-255 or "#rrggbb"; see -dump-config for all
accent = "#7aa2f7"
match = "186"

[keys.normal]           # replace an action's keys; [] unbinds it
scroll_down = ["j", "down", "ctrl+e"]
open_menu = ["o", "ctrl+o"]

[keys.window]           # the key after ctrl+w (normal.window)
split_vertical = ["v", "|"]
```

Unknown settings, bad values and key conflicts stop joe-md with a message
naming the line, so typos don't go unnoticed.

Every key belongs to a named action in a context: `normal` (the document,
and keys that work everywhere), `toc` and `results` (when focused; other keys
fall through to `normal`), `window` (after `ctrl+w`), `search` (the search
bar), `menu`, `filter` (the menu's name filter) and `help`. [KEYS.md](KEYS.md)
lists each action's name and keys. Keys are spelled as joe-md sees them
(`j`, `G`, `ctrl+d`, `alt+t`, `shift+tab`, `enter`, `space`, `f1`); a
sequence is keys separated by spaces (`"g g"`, `"] q"`). Giving a key to one
action takes it from any other action in the same context. A key can't be
both an action and the start of a sequence (`g` alone while `g g` exists),
and digits are always counts.

## Keys

Counts work like vim: `5j`, `3]]`, `42G`. [KEYS.md](KEYS.md) has every key on
one page, and `f1` (or `g?`) shows them inside joe-md, as currently bound:
`j` `k` scroll, `esc` kills the search, `q` closes.

### Document

| Key                          | Action                              |
|------------------------------|-------------------------------------|
| `j` `k` `↓` `↑`              | scroll one line                     |
| `ctrl+d` `ctrl+u`            | half page down / up                 |
| `ctrl+f` `ctrl+b` `space` `b` | page down / up                     |
| `gg` `G`                     | top / bottom                        |
| `{n}G` `{n}gg`               | go to source line *n*               |
| `]]` `[[` (or `}` `{`)       | next / previous heading             |
| `e`                          | edit in micro at the top line       |
| `r`                          | reload the file from disk           |
| `/`                          | search this file                    |
| `?`                          | search across files (last used scope, repo at first) |
| `n` `N`                      | next / previous match               |
| `]q` `[q`                    | open next / previous search result  |
| `gr`                         | open / close the results list       |
| `esc`                        | kill the search: no more highlights or results |
| `ctrl+s`                     | show / hide the search bar          |
| `tab` `shift+tab`            | cycle focus: sidebar, each pane, results |
| `ctrl+g`                     | show the file's full path           |
| `o`                          | open the file menu                  |
| `q`                          | close the pane, else the tab; quits on the last one (the only way) |
| `Q` `ctrl+c`                 | quit                                |

### Tabs

| Key                          | Action                              |
|------------------------------|-------------------------------------|
| `gt` `gT`                    | next / previous tab (wraps)         |
| `{n}gt` `alt+1`…`alt+9`      | go to tab *n*                       |
| `{n}gT`                      | *n* tabs back                       |
| `x`                          | close the tab and all its panes (not the last tab) |
| `alt+t`                      | show / hide the tab bar             |

Opening a file that is already open (from the menu or a search result)
switches to the tab and pane showing it; only splits open a file twice. New
tabs go right after the current one, and a tab is labelled with its focused
pane's file, plus `+n` for its other panes.
Click a tab to switch to it, middle-click to close it, or scroll the wheel over
the bar to step through them. With the bar hidden, the status line shows
`[n/total]`.

### Panes

Each tab can be split into panes, like vim's windows. A new split shows the
same file at the same place, so you can read two parts of one file; press `o`
in it to open something else, or use `v` / `s` in the file menu or results
list to open a file straight into a split. With more than one pane, each gets
its own status row, highlighted on the focused one.

| Key                          | Action                              |
|------------------------------|-------------------------------------|
| `ctrl+w v` `ctrl+w s`        | split side by side / stacked        |
| `ctrl+w h` `j` `k` `l`       | focus the pane left / below / above / right |
| `ctrl+w w` `ctrl+w W`        | next / previous pane (then results and sidebar) |
| `ctrl+w o`                   | close every other pane              |
| `ctrl+w =`                   | make all panes the same size        |
| `{n}ctrl+w >` `<`            | *n* columns wider / narrower (or `ctrl+w {n} >`) |
| `{n}ctrl+w +` `-`            | *n* rows taller / shorter           |
| `ctrl+w b`                   | scrollbind on / off for the pane    |

`ctrl+w h` from the leftmost pane moves to the sidebar, and `ctrl+w j` from
the bottom pane to the results list. Panes with scrollbind on scroll together
by the same number of source lines, like vim, which helps when comparing two
files; panes showing the same file at the same width move by exactly the same
rendered lines.

Click a pane to focus it; the wheel scrolls the pane under the mouse. Drag a
`│` separator, or the status row between stacked panes, to resize.

### Table of contents

The sidebar shows the focused pane's headings; each pane keeps its own place
in it.

| Key                          | Action                              |
|------------------------------|-------------------------------------|
| `ctrl+t`                     | show / hide the sidebar             |
| `tab`, `ctrl+w h` / `ctrl+w l` | move focus between sidebar and document |
| `j` `k`                      | select heading (the document follows) |
| `gg` `G` `{n}G`              | first / last / *n*-th heading       |
| `enter` `esc` `l` `h`        | back to the document                |

### File menu

`o` pops up a file browser at the current file's directory, with the cursor
on that file. It lists one directory at a time, like lf: directories first,
then markdown files. Hidden files, files ignored by git and other files are
left out until you press `.`, which shows them dimmed. A `•` marks files
that are already open.

| Key                          | Action                              |
|------------------------------|-------------------------------------|
| `j` `k` `ctrl+d` `ctrl+u`    | move                                |
| `gg` `G`                     | first / last entry                  |
| `l` `enter` `→`              | enter the directory, or open the file in a new tab |
| `O`                          | open the file in the focused pane instead |
| `v` `s`                      | open the file in a new pane beside / below |
| `h` `-` `backspace` `←`      | parent directory                    |
| `~` `gh`                     | home directory                      |
| `gr`                         | root of the git repository          |
| `.`                          | show / hide hidden, ignored and non-markdown files |
| `/`                          | filter names (`enter` opens the selection, `esc` clears) |
| `r`                          | re-read the directory               |
| `esc` `q` `o`                | close the menu                      |

The filter matches as you type. That is safe here, unlike searching file
contents, because it only compares the names in one directory. Space-separated
words must all appear in the name, ignoring case. While filtering, `↑` `↓`
move the selection and the search bar's line-editing keys work.

Click an entry to open it, click outside the menu to close it, and scroll
with the wheel.

The mouse wheel scrolls whichever side it is over, and clicking a heading
jumps to it. Hold `shift` while dragging to select text in most terminals.

## Search

Searching uses [ripgrep](https://github.com/BurntSushi/ripgrep) (`rg -i`: a
case-insensitive regex, unless the settings file says otherwise). `ctrl+r` in
the search bar switches between regex and literal text (`rg -F`), shown by the
`regex` / `literal` chip; the choice sticks for later searches. It runs only
when you press `enter`, never while you type, and in the background, so big
files and repos never make typing lag. Placing thousands of matches in the
rendered text happens in the background too.

| Key in the search bar        | Action                              |
|------------------------------|-------------------------------------|
| `enter`                      | search (empty repeats the last search) |
| `tab` `shift+tab`            | cycle scope: file → dir → repo      |
| `ctrl+r`                     | regex / literal text                |
| `ctrl+c`                     | clear the input (again to cancel)   |
| `esc`                        | cancel and kill the search          |
| `ctrl+w` `ctrl+u` `ctrl+k`   | delete word / to start / to end     |
| `ctrl+a` `ctrl+e`            | start / end of line                 |
| `ctrl+h` `ctrl+d`            | delete backwards / forwards         |
| `alt+b` `alt+f`              | word left / right                   |
| `↑` `↓`                      | previous / next search in history   |

Scopes: **file** is the open file; **dir** is its directory, recursively;
**repo** is the enclosing git repository (the nearest `.git`), or the directory
outside one. Directory scopes search markdown files only and respect
`.gitignore`.

A **file** search works like vim's `/`: it jumps to the first match below the
view and `n` / `N` step through the rest. A **dir** or **repo** search opens a
results list (`j` `k` to move, `enter` to open in a tab, `O` to open in the
focused pane, `v` / `s` to open in a split, `q` or `esc` to close, killing the
search either way). `]q` / `[q` step through results in the focused pane, like
vim's quickfix, instead of opening a tab each. The opened file's matches are
highlighted and `n` / `N` step through them. A file search belongs to the pane
it was typed in.

Separate terms with spaces to require them all: `alpha needle` matches only
lines containing both, in any order, and highlights each one, the same AND the
file menu's filter uses. `alpha|needle` still matches lines with either, and a
phrase with a space is one regex term: `alpha\s+needle`.

Matches are found in the markdown source but you read the rendered text, so
joe-md looks for the matched text in the rendered lines of the block ripgrep
matched and highlights it there, with the current match in a stronger colour.
When the text is not visible in the rendering (an HTML attribute, say), the
line gets a mark in the left margin instead. After `r` or an edit in micro
the search re-runs so the highlights follow the new text.

## How line tracking works

Each top-level markdown block (paragraph, list, code block, table, …) is
rendered on its own, and joe-md records which source lines produced which
rendered lines. That map drives the table of contents, `{n}G`, keeping your
place across resizes and reloads, and the micro round trip.

## Editing in micro

`e` suspends joe-md and runs `micro +LINE FILE`, where LINE is the source line
at the top of the view (the `Ln` in the status bar). When micro exits, joe-md
reloads the file and puts micro's final cursor line back at the top of the
view. If the cursor never moved, the view is left exactly as it was.

The cursor line comes back through a small plugin, `joemd`, which joe-md
installs into micro's config directory (`$MICRO_CONFIG_HOME`, else
`$XDG_CONFIG_HOME/micro`, else `~/.config/micro`) under `plug/joemd/`. It does
nothing unless micro was started by joe-md.

## Roadmap

1. ~~Viewer, table of contents, vim scrolling~~
2. ~~Toggle to micro and back on the same line~~
3. ~~Search bar with ripgrep (file / directory / repo scopes, runs on enter)~~
4. ~~Tabs and the pop-up file menu~~
5. ~~Panes~~
6. ~~Settings file, key rebinding, help overlay, literal search~~
7. Maybe later: micro inside a pane (needs a PTY and a terminal emulator)
