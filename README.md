# Norsky

Norsky is a lightweight ATProto feed server that serves Norwegian posts from the Bluesky instance.
It is written in Go and stores posts in a SQLite database to keep things simple.

## Installation

The feed server is a standalone go binary that you can run on your machine.
It is also available as a Docker image.


### From GitHub release

```bash
curl -L 
```

### Go install

```bash
go install github.com/snorreio/norsky
```

### Docker

```bash
docker run -d --name norsky -p 8080:8080 -v /path/to/db:/db ghcr.io/snorreio/norsky:latest
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
    --name norsky \
    -p 3000:3000 \
    -v /path/to/db:/db \
    ghrc.io/snorreio/norsky:latest
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


## Contributing

Pull requests are welcome.
For major changes, please open an issue first to discuss what you would like to change.
If you find a bug or have a feature request, please open an issue.
I am also open to suggestions for improvements to the codebase as I'm not a golang expert.

If you want to provide feedback in private, you can send me an email at [contact@snorre.io](mailto:contact@snorre.io).


## Release

This project uses [goreleaser](https://goreleaser.com/) to build and release the application.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
