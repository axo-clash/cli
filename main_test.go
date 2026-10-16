package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRunCompatibleBot(t *testing.T) {
	infoCalls, playCalls := 0, 0
	roles := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Accept") != "application/json" {
			t.Error("missing Accept header")
		}
		switch r.URL.Path {
		case "/api/v1/bots/info":
			if r.Method != http.MethodGet {
				t.Errorf("info method: %s", r.Method)
			}
			infoCalls++
			fmt.Fprint(w, `{"name":"Test","version":"1.0.0","extra":true}`)
		case "/api/v1/bots/play":
			if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
				t.Error("play must POST JSON")
			}
			var input struct {
				Game struct {
					Board [][]struct {
						Coordinates struct{ X, Y int }
					}
				}
				Bot struct{ BotType string }
			}
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Errorf("invalid payload: %v", err)
			}
			if len(input.Game.Board) != 8 {
				t.Fatalf("board height: %d", len(input.Game.Board))
			}
			for y, row := range input.Game.Board {
				if len(row) != 8 {
					t.Fatalf("board width: %d", len(row))
				}
				for x, cell := range row {
					if cell.Coordinates.X != x || cell.Coordinates.Y != y {
						t.Error("board must use board[y][x]")
					}
				}
			}
			roles[input.Bot.BotType] = true
			playCalls++
			fmt.Fprint(w, `{"action":"MOVE_UP"}`)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	var output bytes.Buffer
	code := run([]string{"check-bot", "-url", server.URL + "/api/v1/bots/", "-repeat", "2"}, &output)
	if code != 0 || infoCalls != 2 || playCalls != 80 || !roles["FIRST_BOT"] || !roles["SECOND_BOT"] {
		t.Fatalf("code=%d info=%d play=%d roles=%v\n%s", code, infoCalls, playCalls, roles, output.String())
	}
}

func TestCheckRejectsInvalidResponses(t *testing.T) {
	for _, tc := range []struct {
		name, method, body, contentType, want string
		status                                int
	}{
		{"wrong status", "POST", `{"action":"MOVE_UP"}`, "application/json", "HTTP 201", 201},
		{"server error", "POST", `oops`, "text/plain", "HTTP 500", 500},
		{"HTML", "POST", `<html/>`, "text/html", "Content-Type", 200},
		{"malformed", "POST", `{`, "application/json", "objet JSON", 200},
		{"trailing JSON", "POST", `{"action":"MOVE_UP"} {}`, "application/json", "objet JSON", 200},
		{"null object", "POST", `null`, "application/json", "objet JSON", 200},
		{"missing action", "POST", `{}`, "application/json", `champ "action"`, 200},
		{"null action", "POST", `{"action":null}`, "application/json", `champ "action"`, 200},
		{"numeric action", "POST", `{"action":1}`, "application/json", `champ "action"`, 200},
		{"unknown action", "POST", `{"action":"JUMP"}`, "application/json", "action inconnue", 200},
		{"info version missing", "GET", `{"name":"Test"}`, "application/json", `champ "version"`, 200},
		{"info name blank", "GET", `{"name":" ","version":"1"}`, "application/json", `champ "name"`, 200},
		{"too big", "POST", strings.Repeat(" ", maxResponseBytes+1), "application/json", "dépassant", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			err := check(server.Client(), server.URL, scenario{method: tc.method, path: "/test"})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}

func TestRunDetectsParserFailureAndContinues(t *testing.T) {
	// Simulates an old DTO requiring lastActionTimeMs, while the backend omits it.
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			fmt.Fprint(w, `{"name":"Test","version":"1"}`)
			return
		}
		var input struct {
			Bot struct {
				Data map[string]json.RawMessage
			}
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
		}
		if _, ok := input.Bot.Data["lastActionTimeMs"]; !ok {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":"missing lastActionTimeMs"}`)
			return
		}
		fmt.Fprint(w, `{"action":"UNKNOWN"}`)
	}))
	defer server.Close()
	var output bytes.Buffer
	if code := run([]string{"-url", server.URL}, &output); code != 1 || calls != 41 || !strings.Contains(output.String(), "missing lastActionTimeMs") {
		t.Fatalf("code=%d calls=%d\n%s", code, calls, output.String())
	}
}

func TestRunDoesNotFollowRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/different-endpoint", http.StatusFound)
	}))
	defer server.Close()
	var output bytes.Buffer
	if code := run([]string{"-url", server.URL}, &output); code != 1 || !strings.Contains(output.String(), "HTTP 302") {
		t.Fatalf("code=%d\n%s", code, output.String())
	}
}

func TestCheckTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	client := &http.Client{Timeout: 30 * time.Millisecond}
	if err := check(client, server.URL, scenario{method: "GET", path: "/info"}); err == nil {
		t.Fatal("expected timeout")
	}
}

func TestRunInvalidArguments(t *testing.T) {
	for _, args := range [][]string{
		{"-url", "://invalid"}, {"-url", "ftp://localhost"},
		{"-url", "http://localhost?query=1"}, {"-timeout", "0s"},
		{"-repeat", "0"}, {"unexpected-command"},
	} {
		var output bytes.Buffer
		if code := run(args, &output); code != 2 {
			t.Errorf("%v: code=%d", args, code)
		}
	}
}
