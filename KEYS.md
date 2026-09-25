# joe-md keys

<!-- Generated from the action table in internal/ui/keys.go; don't edit by hand.
     Regenerate with: UPDATE_KEYS_MD=1 go test ./internal/ui -run TestKeysMD -->

Counts work like vim (`5j`, `3]]`, `42G`, `3 ctrl+w >`). Inside joe-md, `f1` or
`g?` shows this list. Every key can be changed in the settings file
(`joe-md -dump-config` prints one to start from): the Name column is the
action's name there, under `[keys.<context>]`. The [README](README.md)
explains each feature.

## Scrolling

| Key                                  | Action                   | Name                    |
|--------------------------------------|--------------------------|-------------------------|
| `j` `down` `ctrl+e` `ctrl+n` `enter` | one line down            | `normal.scroll_down`    |
| `k` `up` `ctrl+y` `ctrl+p`           | one line up              | `normal.scroll_up`      |
| `ctrl+d`                             | half a page down         | `normal.half_page_down` |
| `ctrl+u`                             | half a page up           | `normal.half_page_up`   |
| `ctrl+f` `pgdown` `space` `f`        | a page down              | `normal.page_down`      |
| `ctrl+b` `pgup` `b`                  | a page up                | `normal.page_up`        |
| `gg` `home`                          | top, or source line n    | `normal.top`            |
| `G` `end`                            | bottom, or source line n | `normal.bottom`         |
| `]]` `}`                             | next heading             | `normal.next_heading`   |
| `[[` `{`                             | previous heading         | `normal.prev_heading`   |

`{n}gg` and `{n}G` go to source line *n*.

## File

| Key      | Action                        | Name               |
|----------|-------------------------------|--------------------|
| `e`      | edit in micro at the top line | `normal.edit`      |
| `r`      | reload from disk              | `normal.reload`    |
| `ctrl+g` | show the file's path          | `normal.show_path` |
| `o`      | open the file menu            | `normal.open_menu` |

## Search

| Key      | Action                               | Name                       |
|----------|--------------------------------------|----------------------------|
| `/`      | search this file                     | `normal.search_file`       |
| `?`      | search the directory or repo         | `normal.search_files`      |
| `n`      | next match                           | `normal.next_match`        |
| `N`      | previous match                       | `normal.prev_match`        |
| `gr`     | open / close the results list        | `normal.toggle_results`    |
| `]q`     | next result, in the focused pane     | `normal.next_result`       |
| `[q`     | previous result, in the focused pane | `normal.prev_result`       |
| `ctrl+s` | show / hide the search bar           | `normal.toggle_search_bar` |

## Search bar

| Key             | Action                                 | Name                    |
|-----------------|----------------------------------------|-------------------------|
| `enter`         | search (empty repeats the last search) | `search.submit`         |
| `esc`           | cancel                                 | `search.cancel`         |
| `ctrl+c`        | clear the input (again to cancel)      | `search.clear`          |
| `tab`           | next scope: file → dir → repo          | `search.scope_next`     |
| `shift+tab`     | previous scope                         | `search.scope_prev`     |
| `ctrl+r`        | regex / literal text                   | `search.toggle_literal` |
| `up` `ctrl+p`   | previous search in history             | `search.history_prev`   |
| `down` `ctrl+n` | next search in history                 | `search.history_next`   |

Other keys edit the text, readline style: `ctrl+w`, `ctrl+u` and `ctrl+k` delete a word, to the start and to the end; `ctrl+a` and `ctrl+e` go to the start and end; `alt+b` and `alt+f` move by words.

## Results list

| Key                 | Action                    | Name                  |
|---------------------|---------------------------|-----------------------|
| `j` `down` `ctrl+n` | next result               | `results.down`        |
| `k` `up` `ctrl+p`   | previous result           | `results.up`          |
| `ctrl+d`            | half a page down          | `results.half_down`   |
| `ctrl+u`            | half a page up            | `results.half_up`     |
| `gg` `home`         | first result, or result n | `results.first`       |
| `G` `end`           | last result, or result n  | `results.last`        |
| `enter` `l`         | open in a new tab         | `results.open`        |
| `O`                 | open in the focused pane  | `results.open_here`   |
| `v`                 | open in a new pane beside | `results.open_vsplit` |
| `s`                 | open in a new pane below  | `results.open_hsplit` |
| `q` `esc`           | close the list            | `results.close`       |

`{n}gg` and `{n}G` go to result *n*.

## Tabs

| Key                                                                     | Action                           | Name                    |
|-------------------------------------------------------------------------|----------------------------------|-------------------------|
| `gt`                                                                    | next tab, or tab n               | `normal.next_tab`       |
| `gT`                                                                    | previous tab                     | `normal.prev_tab`       |
| `alt+1` `alt+2` `alt+3` `alt+4` `alt+5` `alt+6` `alt+7` `alt+8` `alt+9` | tab 1 … 9                        | `normal.tab_n`          |
| `x`                                                                     | close the tab (not the last one) | `normal.close_tab`      |
| `alt+t`                                                                 | show / hide the tab bar          | `normal.toggle_tab_bar` |

`{n}gt` goes to tab *n*. `tab_n` goes to the tab numbered by the last digit of its key.

## Panes

| Key                                       | Action                                            | Name                      |
|-------------------------------------------|---------------------------------------------------|---------------------------|
| `ctrl+w`                                  | prefix for the pane keys below                    | `normal.window`           |
| `ctrl+w v` `ctrl+w ctrl+v`                | split side by side                                | `window.split_vertical`   |
| `ctrl+w s` `ctrl+w S` `ctrl+w ctrl+s`     | split stacked                                     | `window.split_horizontal` |
| `ctrl+w h` `ctrl+w ctrl+h` `ctrl+w left`  | focus the pane left (past the edge: the sidebar)  | `window.focus_left`       |
| `ctrl+w j` `ctrl+w ctrl+j` `ctrl+w down`  | focus the pane below (past the edge: the results) | `window.focus_down`       |
| `ctrl+w k` `ctrl+w ctrl+k` `ctrl+w up`    | focus the pane above                              | `window.focus_up`         |
| `ctrl+w l` `ctrl+w ctrl+l` `ctrl+w right` | focus the pane right                              | `window.focus_right`      |
| `ctrl+w w` `ctrl+w ctrl+w`                | next pane, then sidebar and results               | `window.next_pane`        |
| `ctrl+w W`                                | previous pane                                     | `window.prev_pane`        |
| `ctrl+w c`                                | close the pane (the tab if it's the last)         | `window.close`            |
| `ctrl+w q` `ctrl+w ctrl+q`                | close the pane, like q                            | `window.quit`             |
| `ctrl+w o` `ctrl+w ctrl+o`                | close every other pane                            | `window.only`             |
| `ctrl+w =`                                | make all panes the same size                      | `window.equalize`         |
| `ctrl+w >`                                | n columns wider                                   | `window.wider`            |
| `ctrl+w <`                                | n columns narrower                                | `window.narrower`         |
| `ctrl+w +`                                | n rows taller                                     | `window.taller`           |
| `ctrl+w -`                                | n rows shorter                                    | `window.shorter`          |
| `ctrl+w b`                                | scrollbind on / off for the pane                  | `window.scrollbind`       |

These follow the window prefix. A count before the prefix or after it resizes by that much: `5 ctrl+w >` or `ctrl+w 5 >`.

## Focus and layout

| Key         | Action                               | Name                |
|-------------|--------------------------------------|---------------------|
| `tab`       | cycle focus: panes, sidebar, results | `normal.focus_next` |
| `shift+tab` | cycle focus backwards                | `normal.focus_prev` |
| `ctrl+t`    | show / hide the table of contents    | `normal.toggle_toc` |

## Table of contents

| Key                   | Action                              | Name            |
|-----------------------|-------------------------------------|-----------------|
| `j` `down` `ctrl+n`   | next heading (the document follows) | `toc.down`      |
| `k` `up` `ctrl+p`     | previous heading                    | `toc.up`        |
| `ctrl+d`              | half a page down                    | `toc.half_down` |
| `ctrl+u`              | half a page up                      | `toc.half_up`   |
| `gg` `home`           | first heading, or heading n         | `toc.first`     |
| `G` `end`             | last heading, or heading n          | `toc.last`      |
| `enter` `l` `esc` `h` | back to the document                | `toc.back`      |

`{n}gg` and `{n}G` go to heading *n*.

## Quitting

| Key          | Action                                  | Name              |
|--------------|-----------------------------------------|-------------------|
| `q`          | close the pane, then the tab, then quit | `normal.quit`     |
| `Q` `ctrl+c` | quit                                    | `normal.quit_all` |

## Help

| Key       | Action        | Name          |
|-----------|---------------|---------------|
| `f1` `g?` | show the keys | `normal.help` |

## File menu

| Key                        | Action                                             | Name               |
|----------------------------|----------------------------------------------------|--------------------|
| `j` `down` `ctrl+n`        | next entry                                         | `menu.down`        |
| `k` `up` `ctrl+p`          | previous entry                                     | `menu.up`          |
| `ctrl+d` `pgdown`          | half a page down                                   | `menu.half_down`   |
| `ctrl+u` `pgup`            | half a page up                                     | `menu.half_up`     |
| `gg` `home`                | first entry                                        | `menu.first`       |
| `G` `end`                  | last entry                                         | `menu.last`        |
| `l` `right` `enter`        | enter the directory, or open the file in a new tab | `menu.open`        |
| `O`                        | open the file in the focused pane                  | `menu.open_here`   |
| `v`                        | open the file in a new pane beside                 | `menu.open_vsplit` |
| `s`                        | open the file in a new pane below                  | `menu.open_hsplit` |
| `h` `left` `-` `backspace` | parent directory                                   | `menu.parent`      |
| `~` `gh`                   | home directory                                     | `menu.home`        |
| `gr`                       | root of the git repository                         | `menu.repo_root`   |
| `.`                        | show / hide hidden, ignored and non-markdown files | `menu.toggle_all`  |
| `r`                        | re-read the directory                              | `menu.refresh`     |
| `/`                        | filter names                                       | `menu.filter`      |
| `esc` `q` `o` `ctrl+c`     | close the menu (esc clears a filter first)         | `menu.close`       |
| `Q`                        | quit                                               | `menu.quit`        |

## Menu filter

| Key             | Action                                | Name            |
|-----------------|---------------------------------------|-----------------|
| `enter`         | stop filtering and open the selection | `filter.accept` |
| `esc`           | clear the filter and stop             | `filter.cancel` |
| `ctrl+c`        | clear the filter (again to stop)      | `filter.clear`  |
| `down` `ctrl+n` | next entry                            | `filter.down`   |
| `up` `ctrl+p`   | previous entry                        | `filter.up`     |

Other keys edit the filter, which matches as you type.

## Help overlay

| Key                                  | Action         | Name             |
|--------------------------------------|----------------|------------------|
| `j` `down` `ctrl+n` `ctrl+e` `enter` | scroll down    | `help.down`      |
| `k` `up` `ctrl+p` `ctrl+y`           | scroll up      | `help.up`        |
| `ctrl+d` `ctrl+f` `pgdown` `space`   | a page down    | `help.page_down` |
| `ctrl+u` `ctrl+b` `pgup` `b`         | a page up      | `help.page_up`   |
| `gg` `home`                          | top            | `help.top`       |
| `G` `end`                            | bottom         | `help.bottom`    |
| `esc` `q` `f1` `g?`                  | close the help | `help.close`     |

## Mouse

Click a tab to switch, middle-click to close it, and scroll over the tab bar
to step through tabs. Click a pane to focus it, drag a border to resize it,
and scroll to move the pane under the pointer. Click a heading in the sidebar
to jump to it. In the file menu, click an entry to open it and click outside
to close it; the same goes for the help.
