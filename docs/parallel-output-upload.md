# Bounded-parallel output upload

## Motivation

After packaging, a job uploads its entire output tree to object storage via
`Processor.uploadDir` (HLS dir, then DASH dir). HLS produces **many small files
per rendition** — an `init.mp4` plus one `.m4s` segment every 6 seconds, times
every rendition in the ladder, plus per-language subtitle playlists. The old
implementation walked the tree and uploaded **one file at a time**, so each
`PutObject` round-trip was paid serially. For a multi-minute video with several
renditions this is hundreds of sequential network round-trips, and it dominated
job wall-clock once encoding finished.

## Change

`uploadDir` now:

1. Walks the directory once to enumerate `{key, localPath}` items.
2. Uploads them concurrently, bounded by `maxUploadConcurrency = 8` via a
   semaphore channel.
3. Returns the first error and `cancel()`s the shared context so in-flight
   uploads stop promptly; remaining queued uploads are skipped.

Uploads are independent (distinct object keys), so parallelism is safe. The
concurrency cap keeps the storage client's connection pool from being swamped.

## Behaviour preserved

- Same object keys (`prefix` + path relative to `dir`, forward-slashed).
- A failure still fails the job — the first upload error is propagated.

## Trade-offs

- Order of uploads is no longer deterministic. Nothing downstream depends on
  upload order (the manifest/playlist files reference segments by name).
- `maxUploadConcurrency` is a fixed constant; if storage backpressure becomes an
  issue it can be lowered (or made configurable).

## Tests

`internal/worker/processor_upload_test.go`:
- `TestUploadDir_BoundedParallel` — asserts max concurrent uploads is `> 1`
  (parallel) and `<= maxUploadConcurrency` (bounded), and all files uploaded.
- `TestUploadDir_PropagatesError` — a failing upload makes `uploadDir` return an
  error.

The whole worker suite runs green under `go test -race`, which also validates the
new concurrency against data races (the test storage fakes are mutex-guarded).
