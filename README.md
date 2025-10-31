# Matrix Gemini Bot

A @Gemini tag bot for Matrix (E2EE not supported yet) .

## Features

- Responds to @mentions (`@botname` or `@gemini`)
- End-to-end encryption support
- Conversation context (5 messages)
- Commands: tag bot @botname default usese Gemini 2.0 flash, `/about`, `/pro` (Gemini 2.5 Pro)
- Markdown responses

## Setup

```bash
go mod download
CGO_ENABLED=1 go build
./geminiMatrix
```

Production build (stripped):
```bash
CGO_ENABLED=1 go build -ldflags="-s -w" -trimpath
```

Create `.env`:
```env
GEMINI_API_KEY=your_key
SERVER_UURRLL=https://matrix.example.com
USERNAME=bot_username
PASSS=bot_password
DEBUG=false
```

## Requirements

- Go 1.21+
- libolm
- SQLite3

Created by my Cat 😺

