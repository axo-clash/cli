package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestRelayURLsAndDuplicates(t *testing.T) {
	for _, raw := range []string{"http://192.168.1.1/bot", "http://169.254.169.254", "http://127.0.0.1@evil.test", "file:///tmp/a", "http://localhost/?a=b", "http://localhost/#token", "http://mybot.example"} {
		if _, err := normalizeLoopback(raw); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	if _, err := relayBots([]string{"a=http://localhost:9000/api/v1/bots/", "b=http://127.0.0.1:9000/api/v1/bots"}); err == nil {
		t.Error("accepted duplicate normalized URL")
	}
	if _, err := relayBots([]string{"a=http://127.0.0.1:9000", "a=http://127.0.0.1:9001"}); err == nil {
		t.Error("accepted duplicate alias")
	}
	if bots, err := relayBots([]string{"a=http://[::1]:9000", "b=http://127.0.0.1:9001"}); err != nil || len(bots) != 2 {
		t.Errorf("valid bots: %v", err)
	}
}

func TestLocalRelayBlocksRedirectsAndBoundsResponses(t *testing.T) {
	targetCalls := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetCalls++
		w.Write([]byte(`{"name":"bot","version":"1"}`))
	}))
	defer target.Close()
	bot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect/info" {
			http.Redirect(w, r, target.URL, http.StatusFound)
			return
		}
		if r.URL.Path == "/large/info" {
			w.Write([]byte(strings.Repeat("x", relayMessageLimit+1)))
			return
		}
		io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
		}
	}))
	defer bot.Close()
	client := loopbackClient()
	defer client.CloseIdleConnections()
	for _, path := range []string{"/redirect", "/large"} {
		_, _, _, code := invokeLocal(context.Background(), client, bot.URL+path, "INFO", nil, 2000)
		if code != "INVALID_RESPONSE" {
			t.Errorf("%s: %s", path, code)
		}
	}
	if targetCalls != 0 {
		t.Fatal("followed local redirect")
	}
	start := time.Now()
	_, _, _, code := invokeLocal(context.Background(), client, bot.URL, "PLAY", json.RawMessage(`{}`), 25)
	if code != "LOCAL_TIMEOUT" || time.Since(start) > time.Second {
		t.Fatalf("timeout: %s", code)
	}
}

func TestRelayExchangeRejectsRedirectAndForeignWebsocket(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]string
		json.NewDecoder(r.Body).Decode(&request)
		if request["code"] != "masked-code" || r.URL.RawQuery != "" {
			t.Error("code must be only in body")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"credential":"secret","websocketUrl":"wss://foreign.invalid/relay"}`))
	}))
	defer api.Close()
	if _, _, err := exchangeRelay(context.Background(), api.URL, "masked-code", true); err == nil {
		t.Error("accepted foreign credential destination")
	}
}

func TestRelayTransportsTwoBindingsAndCorrelatesResponses(t *testing.T) {
	bot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-Auth-Token") != "" {
			t.Error("credential reached local bot")
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/info") {
			w.Write([]byte(`{"name":"local","version":"1"}`))
			return
		}
		w.Write([]byte(`{"action":"MOVE_UP"}`))
	}))
	defer bot.Close()
	bots := []localRelayBot{{Alias: "alpha", base: bot.URL + "/a", Info: map[string]json.RawMessage{"name": json.RawMessage(`"local"`)}}, {Alias: "beta", base: bot.URL + "/b", Info: map[string]json.RawMessage{"name": json.RawMessage(`"local"`)}}}
	complete := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer credential" || r.URL.RawQuery != "" {
			t.Error("bad relay authentication")
		}
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			complete <- err
			return
		}
		defer c.CloseNow()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		var register struct {
			Type            string
			ProtocolVersion int
			Bots            []map[string]json.RawMessage
		}
		if err = wsjson.Read(ctx, c, &register); err != nil {
			complete <- err
			return
		}
		if register.Type != "register" || len(register.Bots) != 2 || register.ProtocolVersion != 1 {
			t.Error("invalid register")
		}
		for _, b := range register.Bots {
			if _, ok := b["url"]; ok {
				t.Error("local URL sent to server")
			}
		}
		wsjson.Write(ctx, c, map[string]any{"type": "registered", "protocolVersion": 1, "connectionId": "conn-1", "bots": []map[string]string{{"alias": "alpha", "bindingId": "binding-a"}, {"alias": "beta", "bindingId": "binding-b"}}})
		for i, binding := range []string{"binding-a", "binding-b"} {
			operation := "INFO"
			if i == 1 {
				operation = "PLAY"
			}
			wsjson.Write(ctx, c, relayMessage{Type: "bot.request", ProtocolVersion: 1, ConnectionID: "conn-1", RequestID: binding, BindingID: binding, Operation: operation, TimeoutMS: 2000, GameID: 1, GameRunID: "run-1", Seat: "FIRST_BOT", Payload: json.RawMessage(`{}`)})
		}
		seen := map[string]bool{}
		for i := 0; i < 2; i++ {
			var reply relayMessage
			if err = wsjson.Read(ctx, c, &reply); err != nil {
				complete <- err
				return
			}
			if reply.Type != "bot.response" || reply.Status != 200 || reply.ConnectionID != "conn-1" || reply.RequestID != reply.BindingID || seen[reply.BindingID] {
				t.Errorf("invalid reply: %+v", reply)
			}
			seen[reply.BindingID] = true
		}
		complete <- nil
		c.Close(websocket.StatusNormalClosure, "")
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := loopbackClient()
	defer client.CloseIdleConnections()
	connectRelay(ctx, strings.Replace(server.URL, "http", "ws", 1), "credential", bots, client, io.Discard)
	if err := <-complete; err != nil {
		t.Fatal(err)
	}
}

func TestRelayRejectsOldConnectionBeforeCallingBot(t *testing.T) {
	calls := make(chan struct{}, 1)
	bot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls <- struct{}{}
		w.Write([]byte(`{"action":"MOVE_UP"}`))
	}))
	defer bot.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer c.CloseNow()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		var register any
		wsjson.Read(ctx, c, &register)
		wsjson.Write(ctx, c, map[string]any{"type": "registered", "protocolVersion": 1, "connectionId": "new-connection", "bots": []map[string]string{{"alias": "alpha", "bindingId": "alpha-id"}}})
		wsjson.Write(ctx, c, relayMessage{Type: "bot.request", ProtocolVersion: 1, ConnectionID: "old-connection", RequestID: "request", BindingID: "alpha-id", Operation: "PLAY", TimeoutMS: 2000, GameID: 1, GameRunID: "run", Seat: "FIRST_BOT", Payload: json.RawMessage(`{}`)})
		c.Read(ctx)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	client := loopbackClient()
	defer client.CloseIdleConnections()
	err := connectRelay(ctx, strings.Replace(server.URL, "http", "ws", 1), "credential", []localRelayBot{{Alias: "alpha", base: bot.URL}}, client, io.Discard)
	if err == nil {
		t.Error("accepted stale connection")
	}
	select {
	case <-calls:
		t.Error("called local bot for stale connection")
	default:
	}
}
