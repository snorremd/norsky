# Norsky

Norsky is a lightweight ATProto feed server that serves Norwegian posts from the Bluesky instance.
It is written in Go and stores posts in a PostgreSQL database.
A dashboard is available at the root of the server `/`.
The dashboard shows interesting statistics about the feed and Norwegian language posts.
It is written in TypeScript using Solid.js and Tailwind CSS.


> [!IMPORTANT]
> Version 3.0.0 introduces a new more powerful feed configuration format and system.
> You can now flexibly add filters and layered scoring to your feeds.
> Read more about this in the [feed configuration](#feed-configuration) section.

> [!IMPORTANT]
> Version 2.0.0 onwards uses PostgreSQL instead of SQLite. This is a breaking change and requires a new database configuration.
> See [./docker](./docker) for an examples on how to configure Norsky to use PostgreSQL with Docker Compose.


## Installation

The feed server is a standalone go binary that you can run on your machine.
It is also available as a Docker image.

### From GitHub release

You can find the latest release on the [releases page](/releases).

```bash
# On Linux you would typically do
curl -O https://github.com/snorremd/norsky/releases/download/v2.0.1/norsky_Linux_x86_64.tar.gz
tar -xvf norsky_Linux_x86_64.tar.gz
chmod +x norsky
# If you want to install it globally you can move it to /usr/local/bin
sudo mv norsky /usr/local/bin
```

### Docker

```bash
docker pull ghrc.io/snorreio/norsky:latest
```

See [./docker](./docker) for examples on how to run Norsky with [Docker Compose](https://docs.docker.com/compose/).

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
    --env NORSKY_DB_HOST="localhost" \
    --env NORSKY_DB_PORT="5432" \
    --env NORSKY_DB_USER="postgres" \
    --env NORSKY_DB_PASSWORD="postgres" \
    --env NORSKY_DB_NAME="postgres" \
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

Since version 3.0.0, Norsky supports a flexible feed configuration system using `feeds.toml`. Each feed is defined in a `[[feeds]]` section with the following structure:

### Basic Feed Settings

- `id` - The unique identifier of the feed
- `display_name` - The display name shown in Bluesky
- `description` - The feed description
- `avatar_path` - Path to the feed's avatar image
- `filters` - Filters determine which posts appear in the feed
- `scoring` - Scoring determines how posts are ranked
- `keywords` - Predefined keyword lists that can be referenced by keyword filters and scoring

### Filters

Filters determine which posts appear in the feed. Multiple filters can be combined in the `filters` array:

```toml
[[feeds]]
id = "norwegian-tech"
display_name = "Norwegian Tech"
description = "Tech posts in Norwegian"
avatar_path = "./assets/avatar.png"

filters = [
    # Language filter - show only Norwegian posts
    { type = "language", languages = ["nb", "nn", "no"] },
    
    # Keyword filter using predefined keyword lists
    { type = "keyword", include = ["tech-terms"], exclude = ["spam"] },
    
    # Exclude replies to keep only top-level posts
    { type = "exclude_replies" }
]
```

Available filter types:
- `language` - Filter by language(s)
- `keyword` - Filter by keyword lists (include and/or exclude)
- `exclude_replies` - Remove reply posts from feed

Filters are translated to SQL WHERE clauses and combined using AND.
This allow you to set up any combination of available filter types without having to write code.
New filter types can be added later by extending the types of filters and adding additional data to the database.

### Scoring
Scoring determines how posts are ranked. Multiple scoring layers can be combined in the `scoring` array, where each layer's score multiplies with the previous layers:

```toml
scoring = [
    # Posts naturally decay over time
    { type = "time_decay", weight = 1.0 },
    
    # Boost posts matching tech keywords
    { type = "keyword", weight = 1.0, keywords = "tech-terms" },
    
    # Adjust specific author visibility
    { type = "author", weight = 1.0, authors = [
        { did = "did:plc:expert", weight = 2.0 },  # Double this author's score
        { did = "did:plc:spammer", weight = 0.5 }  # Halve this author's score
    ]}
]
```

Available scoring types:
- `time_decay` - Score decreases as posts age using inverse square root
- `keyword` - Score based on keyword relevance (normalized 0-1)
- `author` - Adjust scores for specific authors

Scoring is translated to a SQL SELECT statement that is then used in the ORDER BY clause of the SQL query.
New scoring types can be added later by extending the types of scoring and adding additional data to the database.

### Keyword Lists
Define reusable keyword lists that can be referenced by filters and scoring:

```toml
[keywords]
tech-terms = [
    "teknologi*",
    "tech*", 
    "programming",
    "open source"
]
spam = [
    "spam*",
    "scam*",
    "buy*"
]
```

The `*` suffix enables prefix matching in the full-text search. Keywords are combined with OR operators, so posts matching any keyword in the list will be included.

### Complete Example

```toml
[keywords]
tech-terms = ["teknologi*", "tech*", "programming"]
spam = ["spam*", "scam*"]

[[feeds]]
id = "norwegian-tech"
display_name = "Norwegian Tech"
description = "Tech discussions in Norwegian"
avatar_path = "./assets/avatar.png"

filters = [
    { type = "language", languages = ["nb", "nn", "no"] },
    { type = "keyword", include = ["tech-terms"], exclude = ["spam"] },
    { type = "exclude_replies" }
]

scoring = [
    { type = "time_decay", weight = 1.0 },
    { type = "keyword", weight = 1.0, keywords = "tech-terms" },
    { type = "author", weight = 1.0, authors = [
        { did = "did:plc:expert", weight = 2.0 }
    ]}
]
```

### Feed Pagination

Norsky uses cursor-based pagination to reliably return feed posts in chunks. Here's how it works:

1. **Cursor Format**: 
   - The cursor is simply a post ID
   - Each response includes a `cursor` field pointing to the last post in that page
   - First request can use cursor="" or omit it entirely

2. **Stable Ordering**:
   - Posts are ordered by score and then by post ID
   - Even as scores change (due to time decay), the post IDs ensure stable pagination
   - The combination `ORDER BY final_score DESC, posts.id DESC` ensures deterministic ordering

3. **No Duplicates**:
   - Each page request uses `WHERE posts.id < cursor`
   - This ensures you only see posts older than your current position
   - You'll never see the same post twice, even if scores have changed

Example API usage:
```bash
# First page (most recent posts)
GET /xrpc/app.bsky.feed.getFeed?feed=at://did:plc:xyz/app.bsky.feed.generator/tech

# Next page using cursor from previous response
GET /xrpc/app.bsky.feed.getFeed?feed=at://did:plc:xyz/app.bsky.feed.generator/tech&cursor=123456
```

Response format:
```json
{
  "feed": [
    { "post": "at://..." },
    { "post": "at://..." }
  ],
  "cursor": "123456"  // Use this for the next page
}
```

Response format end of feed:
```json
{
  "feed": [
    { "post": "at://..." },
    { "post": "at://..." }
  ]
}
```

The cursor system works reliably even with complex scoring because it's anchored to the immutable post IDs rather than the changing scores.

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