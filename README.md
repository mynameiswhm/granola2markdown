# granola2markdown

CLI to convert meeting notes from [Granola](https://www.granola.ai/) to Markdown files that can be used with local tools like [Obsidian](https://obsidian.md/).

## Installation

Make sure that [Homebrew](https://brew.sh/) is installed. Then make sure that Go is installed:
```shell
brew update
brew install go
```

```shell
go install github.com/mynameiswhm/granola2markdown@latest
```

Make sure that your `$PATH` contains path to `GOBIN`:
```shell
# ~/.zshrc
export PATH="$(go env GOPATH)/bin:$PATH"
```

## Usage

One-time conversion:
```shell
granola2markdown directory-to-save-your-meeting-notes/
```

Using [Watchman](https://facebook.github.io/watchman/) to update meeting notes in the background:

Install Watchman:
```shell
brew update
brew install watchman
```

Configure a trigger from the CLI:
```shell
granola2markdown watchman install $HOME/notes/meetings
```

Where `$HOME/notes/meetings` is a directory where you want to place resulting Markdown files.

Remove the trigger:
```shell
granola2markdown watchman uninstall $HOME/notes/meetings
```

Optional: if your cache file is not in the default Granola location, pass `--cache-path`:
```shell
granola2markdown watchman install --cache-path /path/to/cache-v6.json $HOME/notes/meetings
granola2markdown watchman uninstall --cache-path /path/to/cache-v6.json $HOME/notes/meetings
```

By default, the CLI scans the Granola config directory, picks the newest available `cache-v*.json`, and falls back to the current known filename (`cache-v6.json`) when none exists yet. This lets the exporter pick up future cache version bumps such as `cache-v7.json` without a code change, as long as the payload shape remains compatible.

The supported export flow is cache/editor-backed only. No local config or key setup is required.

Manual fallback (equivalent Watchman commands):
```shell
watchman watch-project $HOME/Library/Application\ Support/Granola/
watchman -j <<< '["trigger", "'$HOME/Library/Application\ Support/Granola/'", {"name":"cache", "expression": ["match", "cache-v*.json", "wholename"], "command": ["granola2markdown", "'$HOME/notes/meetings'"], "append_files": false}]'
```

```
watchman trigger-del $HOME/Library/Application\ Support/Granola cache
```
