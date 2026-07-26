// Command client is the Tic Tac Toe CLI: it registers/logs in over REST, opens
// an authenticated WebSocket, joins the queue, and plays a game from the
// terminal. Run two of them against a running server to play a match.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/gorilla/websocket"

	"github.com/ozanem/go-xox/pkg/protocol"
)

func main() {
	server := flag.String("server", "http://localhost:8080", "server base URL")
	username := flag.String("user", "", "username")
	password := flag.String("pass", "", "password")
	flag.Parse()

	if *username == "" || *password == "" {
		fmt.Fprintln(os.Stderr, "both -user and -pass are required")
		os.Exit(2)
	}

	if err := run(*server, *username, *password); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(server, username, password string) error {
	if err := register(server, username, password); err != nil {
		return err
	}
	pair, err := login(server, username, password)
	if err != nil {
		return err
	}
	sess := newSession(pair)

	// Refresh the access token in the background for the life of the session,
	// so the per-message token never expires mid-game. Stops when run returns.
	stop := make(chan struct{})
	defer close(stop)
	go sess.keepFresh(server, stop)

	conn, err := dialWS(server, sess.token())
	if err != nil {
		return err
	}
	defer conn.Close()

	fmt.Printf("connected as %s — joining the queue...\n", username)
	if err := send(conn, protocol.TypeJoinQueue, protocol.JoinQueuePayload{Token: sess.token()}); err != nil {
		return err
	}

	return play(conn, sess)
}

// Main game loop
func play(conn *websocket.Conn, sess *session) error {
	incoming := make(chan protocol.Envelope)
	go func() {
		defer close(incoming)
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var env protocol.Envelope
			if err := json.Unmarshal(data, &env); err != nil {
				continue
			}
			incoming <- env
		}
	}()

	lines := make(chan string)
	go func() {
		defer close(lines)
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			lines <- sc.Text()
		}
	}()

	var mySymbol protocol.Mark
	myTurn := false

	for {
		select {
		case env, ok := <-incoming:
			if !ok {
				fmt.Println("disconnected from server")
				return nil
			}
			done, sym, turn := handleServerMessage(env, mySymbol)
			if sym != "" {
				mySymbol = sym
			}
			myTurn = turn
			if done {
				return nil
			}
			if myTurn {
				fmt.Print("your move — enter a cell 0-8: ")
			}

		case line, ok := <-lines:
			if !ok {
				return nil
			}
			if !myTurn {
				fmt.Println("(not your turn — wait for the board to update)")
				continue
			}
			cell, err := strconv.Atoi(strings.TrimSpace(line))
			if err != nil {
				fmt.Print("that's not a number 0-8, try again: ")
				continue
			}
			if err := send(conn, protocol.TypeMove, protocol.MovePayload{Cell: cell, Token: sess.token()}); err != nil {
				return err
			}

			myTurn = false
		}
	}
}

func handleServerMessage(env protocol.Envelope, mySymbol protocol.Mark) (done bool, symbol protocol.Mark, myTurn bool) {
	switch env.Type {
	case protocol.TypeMatchFound:
		p, err := protocol.DecodePayload[protocol.MatchFoundPayload](env)
		if err != nil {
			return false, "", false
		}
		fmt.Printf("\nmatch found! you are %s\n%s\n", p.Symbol, renderBoard(p.Board))
		return false, p.Symbol, p.Turn == p.Symbol

	case protocol.TypeState:
		p, err := protocol.DecodePayload[protocol.StatePayload](env)
		if err != nil {
			return false, "", false
		}
		fmt.Printf("\n%s\n", renderBoard(p.Board))
		yours := p.Turn == mySymbol
		if !yours {
			fmt.Println("waiting for opponent...")
		}
		return false, "", yours

	case protocol.TypeGameOver:
		p, err := protocol.DecodePayload[protocol.GameOverPayload](env)
		if err != nil {
			return true, "", false
		}
		fmt.Printf("\n%s\n", renderBoard(p.Board))
		switch p.Result {
		case protocol.ResultWin:
			fmt.Println("you win! 🎉")
		case protocol.ResultLoss:
			fmt.Println("you lose.")
		default:
			fmt.Println("it's a draw.")
		}
		return true, "", false

	case protocol.TypeError:
		p, _ := protocol.DecodePayload[protocol.ErrorPayload](env)
		fmt.Printf("server error [%s]: %s\n", p.Code, p.Message)
		switch p.Code {
		case "cell_occupied", "out_of_bounds", "invalid_move":
			return false, "", true
		case "invalid_token":
			// The server closes the connection
			fmt.Println("your session expired — log in again to keep playing")
			return false, "", false
		default:
			return false, "", false
		}

	default:
		return false, "", false
	}
}

func renderBoard(b protocol.Board) string {
	cell := func(i int) string {
		if b[i] == protocol.Empty {
			return strconv.Itoa(i)
		}
		return string(b[i])
	}
	rows := make([]string, 3)
	for r := 0; r < 3; r++ {
		rows[r] = fmt.Sprintf(" %s | %s | %s ", cell(r*3), cell(r*3+1), cell(r*3+2))
	}
	return strings.Join(rows, "\n---+---+---\n")
}

func send(conn *websocket.Conn, t protocol.MessageType, payload any) error {
	env, err := protocol.NewMessage(t, payload)
	if err != nil {
		return err
	}
	return conn.WriteJSON(env)
}

func dialWS(server, token string) (*websocket.Conn, error) {
	u, err := url.Parse(server)
	if err != nil {
		return nil, fmt.Errorf("parse server URL: %w", err)
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	}
	u.Path = "/ws"

	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)

	conn, resp, err := websocket.DefaultDialer.Dial(u.String(), header)
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("ws dial: %w (server said %s)", err, statusBody(resp))
		}
		return nil, fmt.Errorf("ws dial: %w", err)
	}
	return conn, nil
}
