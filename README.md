# go-xox — Multiplayer Tic Tac Toe

A multiplayer Tic Tac Toe game server and CLI client in Go. Players register and
log in over REST, then play in real time over an authenticated WebSocket:
join the queue, get matched, alternate moves until a win or a draw. Results
feed a persistent leaderboard.

## Requirements

- Go 1.26+
- Docker & Docker Compose
- [`air`](https://github.com/air-verse/air) (optional, for live reload during development)

## Running it

```bash
cp .env.example .env      # JWT_SECRET has no default; the server won't boot without it
make up                   # builds and starts server + postgres
```

Then play a game — open **two terminals**:

```bash
# terminal 1
go run ./cmd/client -user alice -pass password123

# terminal 2
go run ./cmd/client -user bob -pass password123
```

The client registers the account if it doesn't exist, logs in, connects, and
joins the matchmaking queue. As soon as both are queued they are matched; the
board renders in the terminal and each player types a cell number (0–8) on
their turn. The result — win, loss, or draw — is shown to both players, and
the leaderboard updates.

```bash
# view the leaderboard (any logged-in user's token works)
TOKEN=$(curl -s -X POST localhost:8080/login -H 'Content-Type: application/json' \
  -d '{"username":"alice","password":"password123"}' | jq -r .access_token)
curl -s localhost:8080/leaderboard -H "Authorization: Bearer $TOKEN"
```

### Local development

```bash
make db-up    # postgres only
make dev      # air, rebuilding on save   (or: make run)
make help     # every other target: test, test-race, cover, db-reset, ...
```

## HTTP API

All requests and responses are JSON. Every error uses one shape, so clients
have exactly one thing to parse on failure:

```json
{
  "error": {
    "code": "invalid_credentials",
    "message": "username or password is incorrect"
  }
}
```

| Method | Path           | Auth   | Description                                  |
| ------ | -------------- | ------ | -------------------------------------------- |
| `GET`  | `/healthz`     | —      | Liveness probe                               |
| `POST` | `/register`    | —      | Create an account → `201 {username}`         |
| `POST` | `/login`       | —      | Credentials → access + refresh tokens        |
| `POST` | `/refresh`     | —      | Rotate a refresh token → fresh pair          |
| `POST` | `/logout`      | Bearer | Revoke a refresh token → `204`               |
| `GET`  | `/leaderboard` | Bearer | Standings → `[{username,wins,losses,draws}]` |
| `GET`  | `/ws`          | Bearer | Upgrade to the game WebSocket                |

- `/register` validates username (3–50 chars) and password (8–72 — bcrypt's
  input limit) and returns `409 username_taken` on conflict. Usernames are
  case-insensitive — uniqueness is enforced by a unique index on
  `lower(username)`, and login matches the same way, so `Alice` can log in
  as `alice`.
- `/login` returns `{access_token, access_expires_at, refresh_token,
refresh_expires_at}`. Unknown user and wrong password are deliberately the
  same `401`.
- An empty leaderboard is `[]`, never `null`.

## Game protocol (WebSocket)

Defined in [`pkg/protocol`](pkg/protocol) — deliberately outside `internal/`,
since it is the one package a third-party client legitimately needs.

The client opens `GET /ws` with `Authorization: Bearer <access_token>`. An
invalid or missing token is rejected with a plain HTTP `401` _before_ the
upgrade. After the upgrade, every message in both directions is one JSON
envelope:

```json
{ "type": "<message type>", "payload": { ... } }
```

**Client → server** — every operation carries the access token, and the server
re-verifies it (signature, expiry, and that it belongs to this connection's
authenticated user) on **each** call, not just once at connection time. A
token that fails re-verification gets an `invalid_token` error and the
connection is closed.

| Type         | Payload                       | Meaning                               |
| ------------ | ----------------------------- | ------------------------------------- |
| `join_queue` | `{"token": "..."}`            | Enter matchmaking                     |
| `move`       | `{"cell": 0, "token": "..."}` | Claim a cell, `0..8` in reading order |

**Server → client**

| Type          | Payload                          | Meaning                                  |
| ------------- | -------------------------------- | ---------------------------------------- |
| `match_found` | `{game_id, symbol, board, turn}` | Game started; you are `symbol`           |
| `state`       | `{board, turn}`                  | Board after a legal move                 |
| `game_over`   | `{result, winner, board}`        | `result` is _yours_: `win`/`loss`/`draw` |
| `error`       | `{code, message}`                | A rejected request                       |

The board is an array of nine strings — `"X"`, `"O"`, or `""` — indexed 0–8 in
reading order (0 top-left, 8 bottom-right), so raw frames stay readable when
debugging with `websocat`. X always moves first; the longer-waiting player is X.

Error codes a client can branch on: `not_your_turn`, `cell_occupied`,
`out_of_bounds`, `game_over`, `no_active_game`, `already_queued`,
`bad_message`, `unknown_type`, `invalid_token`.

Rules are enforced **server-side only** — the CLI's own checks are UX, not
security, since a hostile client is just a websocket session sending raw JSON.
Disconnecting mid-game forfeits: the opponent is notified and wins. A second
connection by the same user replaces the first.

## Authentication design

### Why JWT

The access token is a JWT signed with HMAC-SHA256, carrying the user ID in
`sub`, plus `username`, `iat` and `exp`.

The reason is that verification needs no I/O. Gameplay runs over a WebSocket,
and the upgrade handler has to authenticate before the connection is
established; a self-contained token means that check is a signature
verification rather than a database round trip on the connection path. The
same property is what makes **per-operation re-verification** affordable:
every `join_queue` and `move` carries the token and is re-checked (the common
WebSocket pattern is to authenticate the connection once and trust it
afterwards; re-checking per operation is stricter, costs only an in-memory
HMAC check, and means a token expiring mid-session is caught on the next
operation instead of never).

### Refresh tokens

The tradeoff of a self-contained JWT is that it cannot be revoked, so the
access token is deliberately short-lived (30 min, `JWT_TTL`) and paired with
an opaque refresh token that _is_ server-side state: 32 random bytes, stored
**hashed** (SHA-256) so a database leak leaks nothing exchangeable, revocable
via `POST /logout`, and **rotated on every use** — a refresh token that is
replayed after being exchanged is already revoked. The CLI client refreshes
its access token in the background shortly before expiry, so the
per-operation token it sends over the WebSocket stays valid for the whole
session without the player ever re-logging in.

### Password storage

Passwords are hashed with bcrypt at cost 12. bcrypt is deliberately slow and
salts each hash itself, which is what a general-purpose hash like SHA-256 does
not give you. argon2id would be the more modern choice; bcrypt was preferred
here for maturity and ubiquity.

## Architecture

```
cmd/server/            entrypoint: config, wiring, graceful shutdown
cmd/client/            CLI client: register/login → WebSocket → play
pkg/protocol/          the wire contract (shared with any client)
internal/game/         pure rules: board, moves, win/draw — no I/O
internal/matchmaking/  FIFO queue pairing two waiting players
internal/server/       WebSocket transport, hub, game manager
internal/auth/         credentials, JWT issue/verify, auth middleware
internal/user/         user records, stats, leaderboard (Postgres)
internal/refreshtoken/ refresh token storage (hashed at rest)
internal/httpapi/      HTTP handlers, routing, error shape
migrations/            schema, applied on first database start
```

Dependencies point inward, and interfaces are declared by their consumers
(`httpapi.AuthService`, `httpapi.LeaderBoard`, `server.StatsRecorder`), so
each layer states only what it uses and unit tests run with in-memory fakes —
no database, no sockets.

**Concurrency model**

- **One writer per WebSocket.** `gorilla/websocket` panics on concurrent
  writers, so each connection has a buffered outbound channel drained by a
  single `writePump` goroutine; everything else only sends to the channel. The
  hub's mutex serialises sends against channel close, so "send on closed
  channel" cannot happen.
- **Two-level locking in the game manager.** A manager lock guards only the
  `map[gameID]*session` bookkeeping (create, look up, delete); each session
  has its own lock for applying moves. Games never contend with each other on
  the move path — which is what "supports multiple simultaneous games" means
  in practice.
- **Matchmaking under one lock hold.** Popping two players and creating their
  game is a single critical section, so the same player cannot be paired
  twice and a disconnect cannot interleave mid-pairing.
- **A game ends exactly once.** A winning move and the loser's disconnect can
  land at the same instant, and both the move path and the forfeit path end
  the game and record a result. Each therefore check-and-sets a `done` flag
  inside the session lock; whichever loses that race becomes a no-op, so a
  result is never double-counted. Pinned by a regression test that races the
  two paths a thousand times under the race detector.
- **Stats recording happens after every lock is released on the move path** —
  when a game ends by a move, persistence I/O runs outside every critical
  section, and a failed stat write is logged rather than allowed to break
  gameplay. (Known simplification: the forfeit-on-disconnect path records
  while the hub lock is held; correct, but a slow write there would briefly
  stall other connections.)

The suite runs clean under `go test -race ./...`, including an integration
test that plays full games over real WebSocket connections.

**Other choices:**

- **Stdlib `net/http`, no router** — Go 1.22+ `ServeMux` method patterns cover it.
- **pgx/v5** over `lib/pq` (maintenance mode).
- **Insert-and-catch, not check-then-insert** for registration: the unique
  index is the authority, so concurrent duplicate registrations cannot race.
- **Config fails fast** — no default `JWT_SECRET`.
- **`scratch` runtime image** — the multi-stage Dockerfile ships a statically
  linked binary and nothing else: no shell, no libc, minimal attack surface.
- **Graceful shutdown** on SIGINT/SIGTERM with a bounded drain timeout.

## Testing

```bash
make test-race
```

- **Game** — table-driven: all eight win lines, draws, and every rejection
  (occupied, out-of-turn, out-of-bounds, post-game), plus check-order pins.
- **Auth** — bcrypt round trips, JWT expiry via an injected clock, tampering,
  foreign keys, `alg:none`; refresh rotation and revocation; middleware 401s.
- **Matchmaking** — a conformance suite plus race-detector tests for
  concurrent joins and join/leave interleavings.
- **HTTP** — every status/shape branch, with fakes; leaderboard DTO never
  leaks entity fields; empty board is `[]`.
- **Integration** — real WebSocket clients against an `httptest` server: a
  full game to a win with per-player results, forfeit on disconnect, empty
  move payloads rejected, unauthenticated upgrades rejected, and stats
  recorded with the right winner/loser.
