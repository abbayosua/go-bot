# Go-Bot desktop

Thin Electron shell around the same web UI the binary serves. Build the
binary first (`go build -o bin/go-bot ./cmd/go-bot`), then:

```
cd desktop && npm install && npm start
```

The shell starts `bin/go-bot` next to it when present, otherwise it connects
to a backend already listening on `GOBOT_PORT` (default 7417). No feature
lives in the shell: it is a window, a menu-less frame, and external-link
handling.
