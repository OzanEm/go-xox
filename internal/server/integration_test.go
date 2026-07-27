package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/ozanem/go-xox/internal/auth"
	"github.com/ozanem/go-xox/pkg/protocol"
)

type noopRecorder struct{}

func (noopRecorder) RecordDecisive(context.Context, int64, int64) error { return nil }
func (noopRecorder) RecordDraw(context.Context, int64, int64) error     { return nil }

type captureRecorder struct {
	mu       sync.Mutex
	decisive [][2]int64
	draws    [][2]int64
}

func (c *captureRecorder) RecordDecisive(_ context.Context, winnerID, loserID int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.decisive = append(c.decisive, [2]int64{winnerID, loserID})
	return nil
}

func (c *captureRecorder) RecordDraw(_ context.Context, aID, bID int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.draws = append(c.draws, [2]int64{aID, bID})
	return nil
}

func newTestServer(t *testing.T) (*httptest.Server, *auth.TokenIssuer) {
	return newTestServerWith(t, noopRecorder{})
}

func newTestServerWith(t *testing.T, rec StatsRecorder) (*httptest.Server, *auth.TokenIssuer) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	issuer := auth.NewTokenIssuer([]byte("integration-test-secret"), time.Hour)
	hub := NewHub(NewGameManager(rec, logger), logger)
	srv := httptest.NewServer(NewWSHandler(hub, issuer, logger))
	t.Cleanup(srv.Close)
	return srv, issuer
}

func mintToken(t *testing.T, issuer *auth.TokenIssuer, userID int64, name string) string {
	t.Helper()
	tok, _, err := issuer.Issue(userID, name)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return tok
}

func dial(t *testing.T, serverURL, token string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	u, _ := url.Parse(serverURL)
	u.Scheme = "ws"
	header := http.Header{}
	if token != "" {
		header.Set("Authorization", "Bearer "+token)
	}
	return websocket.DefaultDialer.Dial(u.String(), header)
}

func mustDial(t *testing.T, serverURL, token string) *websocket.Conn {
	t.Helper()
	c, _, err := dial(t, serverURL, token)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func readEnv(t *testing.T, c *websocket.Conn) protocol.Envelope {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := c.ReadMessage()
	if err != nil {
		t.Fatalf("read message: %v", err)
	}
	var env protocol.Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	return env
}

func expect(t *testing.T, c *websocket.Conn, want protocol.MessageType) protocol.Envelope {
	t.Helper()
	env := readEnv(t, c)
	if env.Type != want {
		t.Fatalf("got message type %q, want %q", env.Type, want)
	}
	return env
}

func sendMsg(t *testing.T, c *websocket.Conn, msgType protocol.MessageType, payload any) {
	t.Helper()
	env, err := protocol.NewMessage(msgType, payload)
	if err != nil {
		t.Fatalf("build message: %v", err)
	}
	if err := c.WriteJSON(env); err != nil {
		t.Fatalf("write message: %v", err)
	}
}

func joinQueue(t *testing.T, c *websocket.Conn, token string) {
	t.Helper()
	sendMsg(t, c, protocol.TypeJoinQueue, protocol.JoinQueuePayload{Token: token})
}

func move(t *testing.T, c *websocket.Conn, token string, cell int) {
	t.Helper()
	sendMsg(t, c, protocol.TypeMove, protocol.MovePayload{Cell: cell, Token: token})
}

func TestUnauthenticatedRejected(t *testing.T) {
	srv, _ := newTestServer(t)

	_, resp, err := dial(t, srv.URL, "")
	if err == nil {
		t.Fatal("dial without a token succeeded, want rejection")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %v, want 401", resp)
	}
}

func TestFullGame_XWins(t *testing.T) {
	srv, issuer := newTestServer(t)

	token1 := mintToken(t, issuer, 1, "alice")
	token2 := mintToken(t, issuer, 2, "bob")
	c1 := mustDial(t, srv.URL, token1)
	c2 := mustDial(t, srv.URL, token2)

	joinQueue(t, c1, token1)
	joinQueue(t, c2, token2)

	mf1 := decodeMatchFound(t, expect(t, c1, protocol.TypeMatchFound))
	mf2 := decodeMatchFound(t, expect(t, c2, protocol.TypeMatchFound))
	if !((mf1.Symbol == protocol.X && mf2.Symbol == protocol.O) ||
		(mf1.Symbol == protocol.O && mf2.Symbol == protocol.X)) {
		t.Fatalf("symbols = %q,%q, want one X and one O", mf1.Symbol, mf2.Symbol)
	}

	xConn, oConn := c1, c2
	xToken, oToken := token1, token2
	if mf1.Symbol == protocol.O {
		xConn, oConn = c2, c1
		xToken, oToken = token2, token1
	}

	move(t, xConn, xToken, 0)
	expect(t, xConn, protocol.TypeState)
	expect(t, oConn, protocol.TypeState)

	move(t, oConn, oToken, 3)
	expect(t, xConn, protocol.TypeState)
	expect(t, oConn, protocol.TypeState)

	move(t, xConn, xToken, 1)
	expect(t, xConn, protocol.TypeState)
	expect(t, oConn, protocol.TypeState)

	move(t, oConn, oToken, 4)
	expect(t, xConn, protocol.TypeState)
	expect(t, oConn, protocol.TypeState)

	move(t, xConn, xToken, 2)

	xOver := decodeGameOver(t, expect(t, xConn, protocol.TypeGameOver))
	oOver := decodeGameOver(t, expect(t, oConn, protocol.TypeGameOver))

	if xOver.Result != protocol.ResultWin {
		t.Errorf("X result = %q, want win", xOver.Result)
	}
	if oOver.Result != protocol.ResultLoss {
		t.Errorf("O result = %q, want loss", oOver.Result)
	}
	if xOver.Winner != protocol.X {
		t.Errorf("winner = %q, want X", xOver.Winner)
	}
}

func TestRecordsWinnerAndLoser(t *testing.T) {
	rec := &captureRecorder{}
	srv, issuer := newTestServerWith(t, rec)

	token1 := mintToken(t, issuer, 1, "alice")
	token2 := mintToken(t, issuer, 2, "bob")
	c1 := mustDial(t, srv.URL, token1)
	c2 := mustDial(t, srv.URL, token2)

	joinQueue(t, c1, token1)
	joinQueue(t, c2, token2)
	mf1 := decodeMatchFound(t, expect(t, c1, protocol.TypeMatchFound))
	decodeMatchFound(t, expect(t, c2, protocol.TypeMatchFound))

	xConn, oConn := c1, c2
	xToken, oToken := token1, token2
	var xID, oID int64 = 1, 2
	if mf1.Symbol == protocol.O {
		xConn, oConn = c2, c1
		xToken, oToken = token2, token1
		xID, oID = 2, 1
	}

	for _, m := range []struct {
		c     *websocket.Conn
		token string
		cell  int
	}{{xConn, xToken, 0}, {oConn, oToken, 3}, {xConn, xToken, 1}, {oConn, oToken, 4}} {
		move(t, m.c, m.token, m.cell)
		expect(t, c1, protocol.TypeState)
		expect(t, c2, protocol.TypeState)
	}
	move(t, xConn, xToken, 2)
	expect(t, c1, protocol.TypeGameOver)
	expect(t, c2, protocol.TypeGameOver)

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.decisive) != 1 {
		t.Fatalf("RecordDecisive called %d times, want 1", len(rec.decisive))
	}
	if got := rec.decisive[0]; got != [2]int64{xID, oID} {
		t.Errorf("recorded {winner,loser} = %v, want {%d,%d}", got, xID, oID)
	}
	if len(rec.draws) != 0 {
		t.Errorf("RecordDraw called %d times on a decisive game, want 0", len(rec.draws))
	}
}

func TestForfeitOnDisconnect(t *testing.T) {
	srv, issuer := newTestServer(t)

	token1 := mintToken(t, issuer, 1, "alice")
	token2 := mintToken(t, issuer, 2, "bob")
	c1 := mustDial(t, srv.URL, token1)
	c2 := mustDial(t, srv.URL, token2)

	joinQueue(t, c1, token1)
	joinQueue(t, c2, token2)
	expect(t, c1, protocol.TypeMatchFound)
	expect(t, c2, protocol.TypeMatchFound)

	// c2 rage-quits.
	c2.Close()

	over := decodeGameOver(t, expect(t, c1, protocol.TypeGameOver))
	if over.Result != protocol.ResultWin {
		t.Errorf("survivor result = %q, want win by forfeit", over.Result)
	}
}

func TestRejectsDoubleQueue(t *testing.T) {
	srv, issuer := newTestServer(t)
	token := mintToken(t, issuer, 1, "alice")
	c := mustDial(t, srv.URL, token)

	joinQueue(t, c, token)
	joinQueue(t, c, token)

	env := expect(t, c, protocol.TypeError)
	p, err := protocol.DecodePayload[protocol.ErrorPayload](env)
	if err != nil {
		t.Fatalf("decode error payload: %v", err)
	}
	if p.Code != "already_queued" {
		t.Errorf("error code = %q, want already_queued", p.Code)
	}
}

func TestRejectsQueueJoinWhileInGame(t *testing.T) {
	srv, issuer := newTestServer(t)

	tokenA := mintToken(t, issuer, 1, "alice")
	tokenB := mintToken(t, issuer, 2, "bob")
	tokenC := mintToken(t, issuer, 3, "carol")
	cA := mustDial(t, srv.URL, tokenA)
	cB := mustDial(t, srv.URL, tokenB)
	cC := mustDial(t, srv.URL, tokenC)

	joinQueue(t, cA, tokenA)
	joinQueue(t, cB, tokenB)
	expect(t, cA, protocol.TypeMatchFound)
	expect(t, cB, protocol.TypeMatchFound)

	// alice queues again mid-game. Without the guard she is parked, carol's
	// join pairs the two of them, and alice ends up in two games at once.
	joinQueue(t, cA, tokenA)

	env := expect(t, cA, protocol.TypeError)
	p, err := protocol.DecodePayload[protocol.ErrorPayload](env)
	if err != nil {
		t.Fatalf("decode error payload: %v", err)
	}
	if p.Code != "already_in_game" {
		t.Errorf("error code = %q, want already_in_game", p.Code)
	}

	// carol must still be waiting, not matched with a player who never left.
	joinQueue(t, cC, tokenC)
	_ = cC.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	if _, _, err := cC.ReadMessage(); err == nil {
		t.Error("carol was matched with a player who is already in a game")
	}
}

func TestEmptyMoveRejected(t *testing.T) {
	srv, issuer := newTestServer(t)
	c := mustDial(t, srv.URL, mintToken(t, issuer, 1, "alice"))

	sendMsg(t, c, protocol.TypeMove, nil)
	env := expect(t, c, protocol.TypeError)
	p, err := protocol.DecodePayload[protocol.ErrorPayload](env)
	if err != nil {
		t.Fatalf("decode error payload: %v", err)
	}
	if p.Code != "bad_message" {
		t.Errorf("error code = %q, want bad_message", p.Code)
	}
}

func TestJoinQueueRejectsInvalidToken(t *testing.T) {
	srv, issuer := newTestServer(t)

	c := mustDial(t, srv.URL, mintToken(t, issuer, 1, "alice"))

	sendMsg(t, c, protocol.TypeJoinQueue, protocol.JoinQueuePayload{Token: "not-a-jwt"})

	env := expect(t, c, protocol.TypeError)
	p, err := protocol.DecodePayload[protocol.ErrorPayload](env)
	if err != nil {
		t.Fatalf("decode error payload: %v", err)
	}
	if p.Code != "invalid_token" {
		t.Errorf("error code = %q, want invalid_token", p.Code)
	}

	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := c.ReadMessage(); err == nil {
		t.Fatal("connection still open after invalid_token, want closed")
	}
}

func TestMoveRejectsMismatchedUserToken(t *testing.T) {
	srv, issuer := newTestServer(t)

	token1 := mintToken(t, issuer, 1, "alice")
	token2 := mintToken(t, issuer, 2, "bob")
	c1 := mustDial(t, srv.URL, token1)
	c2 := mustDial(t, srv.URL, token2)

	joinQueue(t, c1, token1)
	joinQueue(t, c2, token2)
	expect(t, c1, protocol.TypeMatchFound)
	expect(t, c2, protocol.TypeMatchFound)

	move(t, c1, token2, 0)

	env := expect(t, c1, protocol.TypeError)
	p, err := protocol.DecodePayload[protocol.ErrorPayload](env)
	if err != nil {
		t.Fatalf("decode error payload: %v", err)
	}
	if p.Code != "invalid_token" {
		t.Errorf("error code = %q, want invalid_token", p.Code)
	}
}

func decodeMatchFound(t *testing.T, env protocol.Envelope) protocol.MatchFoundPayload {
	t.Helper()
	p, err := protocol.DecodePayload[protocol.MatchFoundPayload](env)
	if err != nil {
		t.Fatalf("decode match_found: %v", err)
	}
	return p
}

func decodeGameOver(t *testing.T, env protocol.Envelope) protocol.GameOverPayload {
	t.Helper()
	p, err := protocol.DecodePayload[protocol.GameOverPayload](env)
	if err != nil {
		t.Fatalf("decode game_over: %v", err)
	}
	return p
}
