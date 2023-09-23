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


## Development

The application has been developed using go 1.21.1.
You should at least have go 1.18+ installed to build the application.


To get started clone the repository and run go run on the main file to list the available commands.

```bash
go run main.go --help
```

The application uses the [cobra](https://github.com/spf13/cobra) library to manage commands and flags.
[cobra-cli](https://github.com/spf13/cobra-cli) should be used to generate new commands.

### Structure

The application follows the standard way to structure a Go project and contains the following packages:

- `main` - The main package that contains the entrypoint for the application.
- `cmd` - The command package that contains the different commands for the application using cobra.
- `server` - The server package that contains setup code for initializing a Fiber server.
- `database` - The database package that contains the database connection, models, migrations, and queries.
- `firehose` - The firehose package that contains the firehose client and the firehose subscription.

```
.
├── cmd
│   ├── gc.go
│   ├── root.go
│   ├── serve.go
│   └── subscribe.go
├── database
│   └── database.go
├── firehose
│   └── firehose.go
├── server
│   └── server.go
├── go.mod
├── go.sum
├── LICENSE
├── main.go
├── norsky
├── README.md
```

## Release

This project uses [goreleaser](https://goreleaser.com/) to build and release the application.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
