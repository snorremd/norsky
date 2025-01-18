# Norsky

Norsky is a lightweight ATProto feed server that serves Norwegian posts from the Bluesky instance.
It is written in Go and stores posts in a SQLite database to keep things simple.
A dashboard is available at the root of the server `/`.
The dashboard shows interesting statistics about the feed and Norwegian language posts.
It is written in TypeScript using Solid.js and Tailwind CSS.

> [!IMPORTANT]
> Version 1.0.0 introduces breaking changes that require a new `feeds.toml` configuration file. The feed configuration has been moved from hardcoded values to a TOML file that allows you to configure multiple feeds with different language settings. See the example configuration in `feeds.example.toml` for reference.



## Installation

The feed server is a standalone go binary that you can run on your machine.
It is also available as a Docker image.

### From GitHub release

```bash
# On Linux you would typically do
curl -O https://github.com/snorremd/norsky/releases/download/v0.1.4/norsky_Linux_x86_64.tar.gz
tar -xvf norsky_Linux_x86_64.tar.gz
chmod +x norsky
# If you want to install it globally you can move it to /usr/local/bin
sudo mv norsky /usr/local/bin
```

### Docker

```bash
docker pull ghrc.io/snorreio/norsky:latest
```

### From go source

First install [go](https://go.dev/) and [bun](https://bun.sh/).
Bun is used to build the frontend assets, go builds the application and bundles the frontend assets into the binary.
Then clone the repository and build the application.

```bash
git clone https://github.com/snorremd/norsky.git
cd norsky
bun install
bun run build
go build -o norsky main.go

# If you want to install it globally you can move it to /usr/local/bin
sudo mv norsky /usr/local/bin
```


## Usage

The feed server is a standalone application that you can run on your machine.
It is also available as a Docker image.

### Commands

```
serve      Serve the norsky feed
migrate    Run database migrations
tidy       Tidy up the database
subscribe  Log all norwegian posts to the command line
publish    Publish feeds on Bluesky
unpublish  Unpublish feeds from Bluesky
help, h    Shows a list of commands or help for one command
```

Example:

```
# Command line
norsky serve --hostname yourdomain.tld --port 8080 --database /path/to/db/feed.db

# Or docker run
docker run -d \
    --env=NORSKY_HOSTNAME="yourdomain.tld" \
    --env NORSKY_DATABASE="/db/feed.db" \
    --env NORSKY_CONFIG="/feeds.toml" \
    --name norsky \
    -p 3000:3000 \
    -v /path/to/db:/db \
    -v /path/to/feeds.toml:/feeds.toml \
    ghrc.io/snorreio/norsky:latest
```

## Norsky server configuration

The Norsky server is configured using environment variables or command line arguments.
For example, to specify that it should run language detection you can use the `--run-language-detection` flag.
It can also be configured using the `NORSKY_RUN_LANGUAGE_DETECTION` environment variable.

For a full list of configuration options, run `norsky serve --help`.

Simple example:

```
# Run language detection with a confidence threshold of 0.6
norsky serve --run-language-detection=true --confidence-threshold=0.6

# Specify as environment variable
NORSKY_RUN_LANGUAGE_DETECTION=true NORSKY_CONFIDENCE_THRESHOLD=0.6 norsky serve
```


## Feed configuration

Since version 1.0 the Norsky feed generator supports dynamically loading feeds from a `feeds.toml` file.
Each feed is defined in a `[[feeds]]` section and currently requires the following fields:

- `id` - The id of the feed.
- `display_name` - The display name of the feed.
- `description` - The description of the feed.
- `avatar_path` - The path to the avatar image for the feed.
- `languages` - The languages (iso-639-1 codes) supported by the feed.
- `keywords` - The keywords to filter posts by.
- `exclude_replies` - Whether to exclude replies from the feed.

If you want to run a german language feed you can add the following to your `feeds.toml` file:

```toml
[[feeds]]
id = "german"
display_name = "German"
description = "A feed of Bluesky posts written in German"
avatar_path = "./assets/avatar.png"
languages = ["de"]
```

### Keyword filtering

Each post is indexed with the SQLite FTS5 extension allowing for fast and efficient keyword searches.
The index uses the regular unicode61 tokenizer allowing for exact matches, prefix matches, etc.
See [SQLite FTS5 documentation](https://www.sqlite.org/fts5.html) for more information.

To specify keywords include them in the `keywords` array.
The keywords are combined using the `OR` operator allowing posts with different keywords to be included in the feed.
For example, the following keywords will match posts with the keyword "teknologi" or "tech" in the post text, allowing for prefix matches.

```toml
keywords = ["teknologi*", "tech*"]
```

## Development

The application has been developed using go 1.21.1 which is the required version to build the application as the `go.mod` file has been initialized with this version.

To get started clone the repository and run go run on the main file to list the available commands.

```bash
go run main.go --help
```

### Structure

The application follows the standard way to structure a Go project and contains the following packages:

- `main` - The main package that contains the entrypoint for the application.
- `cmd` - The command package that contains the different commands for the application using cobra.
- `server` - The server package that contains setup code for initializing a Fiber server.
- `db` - The database package that contains the database connection, models, migrations, and queries.
- `firehose` - The firehose package that contains the firehose client and the firehose subscription.
- `feeds` - The feeds package that contains the feeds and functions that generate the feed responses.
- `models` - The models package that contains the models for the application.
- `dist` - Where goreleaser puts the release artifacts if you build the application using goreleaser locally.

### Testing

Currently there are no tests for the application.
The application relies heavily on channels and external services which makes it hard to test.
If you have any ideas on how to test the application, please open an issue or a pull request.

## Contributing

Pull requests are welcome.
For major changes, please open an issue first to discuss what you would like to change.
If you find a bug or have a feature request, please open an issue.
I am also open to suggestions for improvements to the codebase as I'm not a golang expert.

If you want to provide feedback in private, you can send me an email at [contact@snorre.io](mailto:contact@snorre.io).


## Release

This project uses [goreleaser](https://goreleaser.com/) to build and release the application.
To release a new version you need to create a new tag for the release titled `vX.X.X` and push it to main.
This will trigger a GitHub action that builds the application for all platforms and creates a GitHub release with the artifacts.
The release will list the changes since the last release based on the commit messages using conventional commits.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgements

This project would not have been possible without the help of the following projects and the people behind them.

### [ATProto Feed Generator repository](https://github.com/bluesky-social/feed-generator)

The ATProto Feed Generator repository provides a reference implementation for generating feeds for Bluesky written in TypeScript.
It was of great help when developing the feed server as I could easily parse the data types and see what the endpoints should return.

### [bsky-furry-feed](https://github.com/strideynet/bsky-furry-feed)

This repository saved my day when I realized there was no ATProto API client in the official bluesky indigo golang repository.
I was looking at how the TypeScript, Python, and Ruby feed generator example projects handled feed registration and saw that they all used ATProto API clients.
I could only work on this feed in my spare time and I didn't want to spend time writing an ATProto API client in Go.
I found this repository and it was a great help as I could easily register the feed using their client implementation and focus on the feed server.
They also made me aware of a new go library for building out a command line app, [github.com/urfave/cli/v2](github.com/urfave/cli/v2).


### [blueskyfirehose](https://github.com/CharlesDardaman/blueskyfirehose)

The blueskyfirehose repository and app contained a nice example of how to use a websocket client to subscribe to the Bluesky firehose.
I used this as a reference when developing the firehose client for the feed server.