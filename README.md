# go-morss

`go-morss` is a lightweight server for enriching RSS/Atom/JSON feeds by adding full content to each
feed item. It fetches the content from item links, parses it for readability, and repackages the
feed with the full content included. This makes it easier to consume feeds in a reader or automate
content processing.

## Features

- Fetches full content for feed items using readability parsing (inspired by [Readability.js by
  Mozilla](https://github.com/mozilla/readability)).
- Supports RSS, Atom, and JSON feed formats.
- Filters items by a configurable time window and limits the number of items per feed to reduce the
  load.
- Easy-to-use HTTP API for on-the-fly feed enrichment.

## Installation

1. Clone the repository:

   ```sh
   git clone https://github.com/traut/go-morss.git
   ```

2. Build the binary:

   ```sh
   cd go-morss
   go build
   ```

## Usage

### Starting the Server

Run the server with default options:

```sh
./go-morss
```

Available flags:
- `--ip`: The IP address to listen on (default: `0.0.0.0`).
- `--port`: The port to listen on (default: `8080`).
- `--items-cap`: Maximum number of items to process per feed (default: `10`).

Example:

```sh
./go-morss --port 9090 --items-cap 20
```

### API Usage

#### Endpoint Format

To use the API, append the feed URL (without schema) to the root endpoint:

```text
https://<server-address>/<feed-url-without-schema>
```

Example:

```text
http://localhost:8080/news.ycombinator.com/rss
```

- Assumes `https://` as the default schema for feed URLs.
- The enriched feed retains the original format (RSS, Atom, or JSON).

#### Query Parameters

- `from_time`: Only include items published/updated after this time (ISO8601 format).
- `items_cap`: Override the server's item cap for the request.

Example:

```text
http://localhost:8080/news.ycombinator.com/rss?from_time=2023-01-01T00:00:00Z&items_cap=5
```

## Development

To run the project locally during development:
```sh
go run morss.go
```

### Dependencies

- [`charmbracelet/log`](https://github.com/charmbracelet/log) for beautiful logs.
- [`mmcdole/gofeed`](https://github.com/mmcdole/gofeed) for parsing feeds.
- [`go-shiori/go-readability`](https://github.com/go-shiori/go-readability) for extracting readable content.
- [`gorilla/feeds`](https://github.com/gorilla/feeds) for feed generation.

## Inspired By

- [morss.it](https://morss.it/) and [`pictuga/morss`](https://github.com/pictuga/morss)
- [`Kombustor/rss-fulltext-proxy`](https://github.com/Kombustor/rss-fulltext-proxy)
- [Full-Text RSS Feeds](http://ftr.fivefilters.org/) by [FiveFilters.org](https://www.fivefilters.org/) 

## Contributing

Contributions are welcome! Feel free to open issues or submit pull requests to improve the project.

## License

This project is licensed under the [MIT License](LICENSE).

---

Enjoy! 🚀
