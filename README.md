# svtchat

Go library and tools to consume SVT Duo live chats.

## Tools

### firehose

The firehose lets you consume chats through WebSockets.

```shell
go run ./cmd/firehose/...
```

```shell
# Consume chat messages from Den Stora Älgvandringen
websocat ws://localhost:8080/api/v1/messages/67d8a71ba5107a7114cb72e0bbc5b40b
```

```jsonc
{
  "author": {
    "displayName": "Example User",
    "avatar": "...", // Avatar path
    "role": "editor",
    "title": "Moderator"
  },
  "body": "<p>Hello, world!!</p>",
  "createdAt": "2026-05-01T14:41:16.07Z",
  "highlighted": false,
  "pinned": false,
  "signature": "..." // JWT
}
```
