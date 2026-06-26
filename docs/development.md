# Development

```bash
cd api
go test ./...
go run ./cmd/server
```

## Security notes

- Rotate credentials by regenerating `data/.credentials` (stop stack, delete file, restart — new keys printed once)
- External `api_key` and internal `internal_key` live in `data/.credentials` — back them up securely
- Never log or commit secrets
- Prepared SQL statements throughout; request body limited to 10 MiB
