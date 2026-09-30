# joe-md keys

<!-- Generated from the action table in internal/ui/actions.go; don't edit by hand.
     Regenerate with: UPDATE_KEYS_MD=1 go test ./internal/ui -run TestKeysMD -->

Counts work like vim (`5j`, `3]]`, `42G`, `3 ctrl+w >`). Inside joe-md, `f1` or
`g?` shows this list. Every key can be changed in the settings file
(`joe-md -dump-config` prints one to start from): the Name column is the
action's name there, under `[keys.<context>]`. The [README](README.md)
explains each feature.

## Scrolling

| Key                                                | Action                         | Name                                     |
|----------------------------------------------------|--------------------------------|------------------------------------------|
| `j/k` `down/up`                                    | one line down / up             | `normal.scroll_down` / `scroll_up`       |
| `ctrl+d/u`                                         | half a page down / up          | `normal.half_page_down` / `half_page_up` |
| `ctrl+f/b` `pgdown/pgup` `space/shift+space` `f/b` | a page down / up               | `normal.page_down` / `page_up`           |
| `gg/G` `home/end`                                  | top / bottom, or source line n | `normal.top` / `bottom`                  |
| `]]/[[` `}/{`                                      | next / previous heading        | `normal.next_heading` / `prev_heading`   |

`{n}gg` and `{n}G` go to source line *n*.

## File

| Key      | Action                           | Name                             |
|----------|----------------------------------|----------------------------------|
| `e`      | edit in micro at the top line    | `normal.edit`                    |
| `r`      | reload from disk                 | `normal.reload`                  |
| `ctrl+g` | show the file's full path        | `normal.show_path`               |
| `yn/yp`  | copy the file's name / full path | `normal.copy_name` / `copy_path` |
| `o`      | open the file menu               | `normal.open_menu`               |

## Search

| Key      | Action                                         | Name                                 |
|----------|------------------------------------------------|--------------------------------------|
| `/`      | search this file                               | `normal.search_file`                 |
| `?`      | search the directory or repo                   | `normal.search_files`                |
| `n/N`    | next / previous match                          | `normal.next_match` / `prev_match`   |
| `gr`     | open / close the results list                  | `normal.toggle_results`              |
| `]q/[q`  | next / previous result, in the focused pane    | `normal.next_result` / `prev_result` |
| `esc`    | kill the search: no more highlights or results | `normal.clear_search`                |
| `ctrl+s` | show / hide the search bar                     | `normal.toggle_search_bar`           |

## Search bar

| Key             | Action                                   | Name                                   |
|-----------------|------------------------------------------|----------------------------------------|
| `enter`         | search (empty repeats the last search)   | `search.submit`                        |
| `esc`           | cancel and kill the search               | `search.cancel`                        |
| `ctrl+c`        | clear the input (again to cancel)        | `search.clear`                         |
| `tab/shift+tab` | next / previous scope: file → dir → repo | `search.scope_next` / `scope_prev`     |
| `ctrl+r`        | regex / literal text                     | `search.toggle_literal`                |
| `up/down`       | previous / next search in history        | `search.history_prev` / `history_next` |
| `ctrl+w`        | delete the word before the cursor        | (fixed)                                |
| `ctrl+u/k`      | delete to the start / end                | (fixed)                                |
| `ctrl+a/e`      | go to the start / end                    | (fixed)                                |
| `alt+b/f`       | a word left / right                      | (fixed)                                |

## Results list

| Key               | Action                                 | Name                                  |
|-------------------|----------------------------------------|---------------------------------------|
| `j/k` `down/up`   | next / previous result                 | `results.down` / `up`                 |
| `ctrl+d/u`        | half a page down / up                  | `results.half_down` / `half_up`       |
| `gg/G` `home/end` | first / last result, or result n       | `results.first` / `last`              |
| `enter` `l`       | open in a new tab                      | `results.open`                        |
| `O`               | open in the focused pane               | `results.open_here`                   |
| `v/s`             | open in a new pane beside / below      | `results.open_vsplit` / `open_hsplit` |
| `q` `esc`         | close the list, killing the search too | `results.close`                       |

`{n}gg` and `{n}G` go to result *n*.

## Tabs

| Key       | Action                                                         | Name                                      |
|-----------|----------------------------------------------------------------|-------------------------------------------|
| `gt/gT`   | next / previous tab, or tab n                                  | `normal.next_tab` / `prev_tab`            |
| `alt+1…9` | tab 1 … 9                                                      | `normal.tab_n`                            |
| `<</>>`   | move the tab n places left / right                             | `normal.move_tab_left` / `move_tab_right` |
| `x`       | close the tab (not the last one)                               | `normal.close_tab`                        |
| `X`       | reopen the last closed tab or pane where it was, or the last n | `normal.reopen`                           |
| `alt+t`   | show / hide the tab bar                                        | `normal.toggle_tab_bar`                   |

`{n}gt` goes to tab *n*; `3>>` moves the tab three places, stopping at the ends. `tab_n` goes to the tab numbered by the last digit of its key.

## Panes

| Key              | Action                                                                         | Name                                                            |
|------------------|--------------------------------------------------------------------------------|-----------------------------------------------------------------|
| `ctrl+w`         | prefix for the pane keys below                                                 | `normal.window`                                                 |
| `ctrl+w v/s`     | split side by side / stacked                                                   | `window.split_vertical` / `split_horizontal`                    |
| `ctrl+w h/j/k/l` | focus the pane left / below / above / right (past the edge: sidebar / results) | `window.focus_left` / `focus_down` / `focus_up` / `focus_right` |
| `ctrl+w w/W`     | next / previous pane, then results and sidebar                                 | `window.next_pane` / `prev_pane`                                |
| `ctrl+w o`       | close every other pane                                                         | `window.only`                                                   |
| `ctrl+w =`       | make all panes the same size                                                   | `window.equalize`                                               |
| `ctrl+w >/<`     | n columns wider / narrower                                                     | `window.wider` / `narrower`                                     |
| `ctrl+w +/-`     | n rows taller / shorter                                                        | `window.taller` / `shorter`                                     |
| `ctrl+w b`       | scrollbind on / off for the pane                                               | `window.scrollbind`                                             |

These follow the window prefix. A count before the prefix or after it resizes by that much: `5 ctrl+w >` or `ctrl+w 5 >`.

## Focus and layout

| Key             | Action                                           | Name                               |
|-----------------|--------------------------------------------------|------------------------------------|
| `tab/shift+tab` | cycle focus: sidebar, panes, results / backwards | `normal.focus_next` / `focus_prev` |
| `ctrl+t`        | show / hide the table of contents                | `normal.toggle_toc`                |

## Table of contents

| Key                   | Action                                         | Name                        |
|-----------------------|------------------------------------------------|-----------------------------|
| `j/k` `down/up`       | next / previous heading (the document follows) | `toc.down` / `up`           |
| `ctrl+d/u`            | half a page down / up                          | `toc.half_down` / `half_up` |
| `gg/G` `home/end`     | first / last heading, or heading n             | `toc.first` / `last`        |
| `enter` `l` `esc` `h` | back to the document                           | `toc.back`                  |

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

| Key                        | Action                                             | Name                               |
|----------------------------|----------------------------------------------------|------------------------------------|
| `j/k` `down/up`            | next / previous entry                              | `menu.down` / `up`                 |
| `ctrl+d/u` `pgdown/pgup`   | half a page down / up                              | `menu.half_down` / `half_up`       |
| `gg/G` `home/end`          | first / last entry                                 | `menu.first` / `last`              |
| `l` `right` `enter`        | enter the directory, or open the file in a new tab | `menu.open`                        |
| `O`                        | open the file in the focused pane                  | `menu.open_here`                   |
| `v/s`                      | open the file in a new pane beside / below         | `menu.open_vsplit` / `open_hsplit` |
| `h` `left` `-` `backspace` | parent directory                                   | `menu.parent`                      |
| `~` `gh`                   | home directory                                     | `menu.home`                        |
| `gr`                       | root of the git repository                         | `menu.repo_root`                   |
| `.`                        | show / hide hidden, ignored and non-markdown files | `menu.toggle_all`                  |
| `r`                        | re-read the directory                              | `menu.refresh`                     |
| `/`                        | filter names                                       | `menu.filter`                      |
| `esc` `q` `o` `ctrl+c`     | close the menu (esc clears a filter first)         | `menu.close`                       |
| `Q`                        | quit                                               | `menu.quit`                        |

## Menu filter

| Key       | Action                                | Name                 |
|-----------|---------------------------------------|----------------------|
| `enter`   | stop filtering and open the selection | `filter.accept`      |
| `esc`     | clear the filter and stop             | `filter.cancel`      |
| `ctrl+c`  | clear the filter (again to stop)      | `filter.clear`       |
| `down/up` | next / previous entry                 | `filter.down` / `up` |

Other keys edit the filter, which matches as you type; the search bar's line-editing keys work here too.

## Help overlay

| Key                                           | Action           | Name                         |
|-----------------------------------------------|------------------|------------------------------|
| `j/k` `down/up`                               | scroll down / up | `help.down` / `up`           |
| `ctrl+d/u` `ctrl+f/b` `pgdown/pgup` `space/b` | a page down / up | `help.page_down` / `page_up` |
| `gg/G` `home/end`                             | top / bottom     | `help.top` / `bottom`        |
| `esc` `q` `f1` `g?`                           | close the help   | `help.close`                 |

## Mouse

Click a tab to switch, middle-click to close it, and scroll over the tab bar
to step through tabs. Click a pane to focus it, drag a border to resize it,
and scroll to move the pane under the pointer. Click a heading in the sidebar
to jump to it. In the file menu, click an entry to open it and click outside
to close it; the same goes for the help.
