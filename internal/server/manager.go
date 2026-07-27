package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/ozanem/go-xox/internal/game"
	"github.com/ozanem/go-xox/pkg/protocol"
)

var errNoActiveGame = errors.New("no active game")

type outbound struct {
	userID int64
	env    protocol.Envelope
}

type StatsRecorder interface {
	RecordDecisive(ctx context.Context, winnerID, loserID int64) error
	RecordDraw(ctx context.Context, aID, bID int64) error
}

type GameManager struct {
	mu       sync.Mutex
	games    map[string]*session
	byUser   map[int64]string
	counter  atomic.Int64
	recorder StatsRecorder
	logger   *slog.Logger
}

type session struct {
	id      string
	mu      sync.Mutex
	game    *game.Game
	players [2]sessionPlayer
	// done is set (under mu) by whichever of the move / forfeit paths ends
	// the game first; the loser of that race becomes a no-op, so a result
	// is recorded exactly once even when a winning move and a disconnect
	// land at the same instant.
	done bool
}

type sessionPlayer struct {
	userID int64
	mark   game.Cell
}

func NewGameManager(recorder StatsRecorder, logger *slog.Logger) *GameManager {
	return &GameManager{
		games:    make(map[string]*session),
		byUser:   make(map[int64]string),
		recorder: recorder,
		logger:   logger,
	}
}

func (gm *GameManager) recordResult(winner game.Cell, players [2]sessionPlayer) {
	var err error
	switch {
	case winner == game.Empty:
		err = gm.recorder.RecordDraw(context.Background(), players[0].userID, players[1].userID)
	case players[0].mark == winner:
		err = gm.recorder.RecordDecisive(context.Background(), players[0].userID, players[1].userID)
	default:
		err = gm.recorder.RecordDecisive(context.Background(), players[1].userID, players[0].userID)
	}
	if err != nil {
		gm.logger.Error("record game result", "error", err,
			"players", []int64{players[0].userID, players[1].userID})
	}
}

func (gm *GameManager) Create(aUserID, bUserID int64) []outbound {
	id := fmt.Sprintf("g%d", gm.counter.Add(1))
	s := &session{
		id:   id,
		game: game.New(),
		players: [2]sessionPlayer{
			{userID: aUserID, mark: game.X},
			{userID: bUserID, mark: game.O},
		},
	}

	gm.mu.Lock()
	gm.games[id] = s
	gm.byUser[aUserID] = id
	gm.byUser[bUserID] = id
	gm.mu.Unlock()

	board := toBoard(s.game.Board())
	turn := toMark(s.game.Turn())
	return []outbound{
		{aUserID, env(protocol.TypeMatchFound, protocol.MatchFoundPayload{GameID: id, Symbol: protocol.X, Board: board, Turn: turn})},
		{bUserID, env(protocol.TypeMatchFound, protocol.MatchFoundPayload{GameID: id, Symbol: protocol.O, Board: board, Turn: turn})},
	}
}

func (gm *GameManager) Apply(userID int64, cell int) ([]outbound, error) {
	s := gm.sessionForUser(userID)
	if s == nil {
		return nil, errNoActiveGame
	}

	s.mu.Lock()
	if s.done {
		s.mu.Unlock()
		return nil, errNoActiveGame
	}
	mark, ok := s.markOf(userID)
	if !ok {
		s.mu.Unlock()
		return nil, errNoActiveGame
	}
	if err := s.game.ApplyMove(mark, cell); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	over := s.game.IsOver()
	if over {
		s.done = true
	}
	board := toBoard(s.game.Board())
	turn := toMark(s.game.Turn())
	winner := s.game.Winner()
	players := s.players
	s.mu.Unlock()

	if !over {
		state := env(protocol.TypeState, protocol.StatePayload{Board: board, Turn: turn})
		return []outbound{{players[0].userID, state}, {players[1].userID, state}}, nil
	}

	gm.remove(s.id, players[0].userID, players[1].userID)
	gm.recordResult(winner, players)
	return gameOverOutbounds(players, board, winner), nil
}

func (gm *GameManager) Forfeit(userID int64) []outbound {
	s := gm.sessionForUser(userID)
	if s == nil {
		return nil
	}

	s.mu.Lock()
	var opponent sessionPlayer
	for _, p := range s.players {
		if p.userID != userID {
			opponent = p
		}
	}
	alreadyOver := s.done
	s.done = true
	board := toBoard(s.game.Board())
	players := s.players
	s.mu.Unlock()

	gm.remove(s.id, players[0].userID, players[1].userID)
	if alreadyOver {
		return nil
	}

	if err := gm.recorder.RecordDecisive(context.Background(), opponent.userID, userID); err != nil {
		gm.logger.Error("record forfeit result", "error", err,
			"winner", opponent.userID, "loser", userID)
	}

	return []outbound{
		{opponent.userID, env(protocol.TypeGameOver, protocol.GameOverPayload{
			Result: protocol.ResultWin,
			Winner: toMark(opponent.mark),
			Board:  board,
		})},
	}
}

func (gm *GameManager) sessionForUser(userID int64) *session {
	gm.mu.Lock()
	defer gm.mu.Unlock()
	id, ok := gm.byUser[userID]
	if !ok {
		return nil
	}
	return gm.games[id]
}

func (gm *GameManager) remove(id string, a, b int64) {
	gm.mu.Lock()
	delete(gm.games, id)
	delete(gm.byUser, a)
	delete(gm.byUser, b)
	gm.mu.Unlock()
}

func (s *session) markOf(userID int64) (game.Cell, bool) {
	for _, p := range s.players {
		if p.userID == userID {
			return p.mark, true
		}
	}
	return game.Empty, false
}

func gameOverOutbounds(players [2]sessionPlayer, board protocol.Board, winner game.Cell) []outbound {
	winMark := toMark(winner)
	outs := make([]outbound, 0, len(players))
	for _, p := range players {
		result := protocol.ResultDraw
		if winner != game.Empty {
			if p.mark == winner {
				result = protocol.ResultWin
			} else {
				result = protocol.ResultLoss
			}
		}
		outs = append(outs, outbound{p.userID, env(protocol.TypeGameOver, protocol.GameOverPayload{
			Result: result,
			Winner: winMark,
			Board:  board,
		})})
	}
	return outs
}

func toMark(c game.Cell) protocol.Mark {
	switch c {
	case game.X:
		return protocol.X
	case game.O:
		return protocol.O
	default:
		return protocol.Empty
	}
}

func toBoard(b game.Board) protocol.Board {
	var pb protocol.Board
	for i, c := range b {
		pb[i] = toMark(c)
	}
	return pb
}

func env(t protocol.MessageType, payload any) protocol.Envelope {
	e, err := protocol.NewMessage(t, payload)
	if err != nil {
		panic(fmt.Sprintf("protocol.NewMessage(%s): %v", t, err))
	}
	return e
}
