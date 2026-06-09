# BirdSpy

BirdSpy is now a Go application that serves the existing frontend and queries BIRD route servers directly through their Unix control sockets.

The runtime deployment is intentionally small:

- `birdspy`: one self-contained binary with the compiled frontend embedded
- `config.json`: one configuration file

No external cache service is required. Runtime caches are in-process Go memory caches with a configurable TTL, LRU entry limit, and periodic expiry cleanup.

## Build

Requirements for building from source:

- Go 1.26 or newer
- Node.js and npm

Build the frontend and binary:

```sh
make package
```

The release files are written to `dist/`:

```text
dist/birdspy
dist/config.json
```

## Run

```sh
./dist/birdspy -config ./dist/config.json
```

`config.json` controls the listen address, BIRD servers, cache behavior, flags, known communities, and filtering rules. The cache settings are:

```json
"cache_ttl_seconds": 300,
"cache_max_entries": 1024,
"cache_cleanup_interval_seconds": 60
```

For live BIRD sockets, set:

```json
"reader": {
  "mode": "socket",
  "tiny_samples": false,
  "command_timeout_seconds": 180
}
```

The sample mode is useful for local smoke tests because it uses the embedded sample BIRD output files.

## BIRD Configuration

The parser expects stable timestamp formatting. Recommended BIRD settings:

```text
timeformat base         iso long;
timeformat log          iso long;
timeformat protocol     iso long;
timeformat route        iso long;
```

## License

This application is open-sourced software licensed under the MIT license. See [LICENSE.md](LICENSE.md).
