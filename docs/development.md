# Development

```bash
cd api
go test ./...
go run ./cmd/server
```

## Security notes

- Rotate the API key by regenerating `data/.credentials` (stop stack, delete file, restart — new key printed once)
- API key is in `data/.credentials` — back it up securely
- Never log or commit secrets
- Prepared SQL statements throughout; request body limited to 10 MiB
