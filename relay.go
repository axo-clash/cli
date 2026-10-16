package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/term"
)

const relayMessageLimit = 256 << 10
const defaultRelayAPI = "https://clash-api.axopen.cloud/api/v1"

type botFlags []string

func (b *botFlags) String() string     { return strings.Join(*b, ", ") }
func (b *botFlags) Set(v string) error { *b = append(*b, v); return nil }

type localRelayBot struct {
	Alias string                     `json:"alias"`
	Info  map[string]json.RawMessage `json:"info"`
	base  string
}

type relayMessage struct {
	Type                string          `json:"type"`
	ProtocolVersion     int             `json:"protocolVersion"`
	ConnectionID        string          `json:"connectionId,omitempty"`
	RequestID           string          `json:"requestId,omitempty"`
	BindingID           string          `json:"bindingId,omitempty"`
	Operation           string          `json:"operation,omitempty"`
	GameID              int64           `json:"gameId,omitempty"`
	GameRunID           string          `json:"gameRunId,omitempty"`
	Seat                string          `json:"seat,omitempty"`
	TimeoutMS           int64           `json:"timeoutMs,omitempty"`
	Status              int             `json:"status,omitempty"`
	Payload             json.RawMessage `json:"payload,omitempty"`
	Code                string          `json:"code,omitempty"`
	Message             string          `json:"message,omitempty"`
	LocalHTTPDurationMS int64           `json:"localHttpDurationMs,omitempty"`
	Bots                []struct {
		Alias     string `json:"alias"`
		BindingID string `json:"bindingId"`
	} `json:"bots,omitempty"`
}

var aliasPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,40}$`)

func normalizeLoopback(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("URL HTTP(S) loopback sans identifiants, query ni fragment requise")
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" {
		host = "127.0.0.1"
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return "", errors.New("seules les adresses loopback sont acceptées")
	}
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	u.Host = net.JoinHostPort(ip.String(), port)
	u.Path = strings.TrimRight(u.Path, "/")
	return u.String(), nil
}

func relayBots(values []string) ([]localRelayBot, error) {
	if len(values) == 0 || len(values) > 8 {
		return nil, errors.New("déclare entre 1 et 8 bots avec --bot alias=URL")
	}
	aliases, urls := map[string]bool{}, map[string]bool{}
	var bots []localRelayBot
	for _, v := range values {
		alias, raw, ok := strings.Cut(v, "=")
		if !ok || !aliasPattern.MatchString(alias) || aliases[alias] {
			return nil, errors.New("alias invalide ou déjà déclaré")
		}
		base, err := normalizeLoopback(raw)
		if err != nil {
			return nil, err
		}
		if urls[base] {
			return nil, errors.New("la même URL locale ne peut pas être déclarée deux fois")
		}
		aliases[alias], urls[base] = true, true
		bots = append(bots, localRelayBot{Alias: alias, base: base})
	}
	return bots, nil
}

func loopbackClient() *http.Client {
	return &http.Client{Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Transport: &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(addr)
			if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
				return nil, errors.New("destination locale interdite")
			}
			return (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, network, addr)
		},
	}}
}

func maskedPairingCode(out io.Writer) (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", errors.New("saisis le code depuis un terminal interactif")
	}
	fmt.Fprint(out, "Code d'association : ")
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(out)
	if err != nil {
		return "", err
	}
	code := strings.TrimSpace(string(b))
	for i := range b {
		b[i] = 0
	}
	if len(code) == 0 || len(code) > 128 {
		return "", errors.New("code invalide")
	}
	return code, nil
}

func runRelay(args []string, out io.Writer) int {
	f := flag.NewFlagSet("relay", flag.ContinueOnError)
	f.SetOutput(out)
	defaultAPI := strings.TrimSpace(os.Getenv("AXO_CLASH_API_URL"))
	if defaultAPI == "" {
		defaultAPI = defaultRelayAPI
	}
	api := f.String("api", defaultAPI, "API HTTPS (défaut : https://clash-api.axopen.cloud ; ou AXO_CLASH_API_URL)")
	dev := f.Bool("dev", false, "Autoriser une API HTTP loopback pour le développement")
	var declarations botFlags
	f.Var(&declarations, "bot", "Bot loopback alias=http://127.0.0.1:9000/api/v1/bots (répétable)")
	if err := f.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	u, err := normalizeRelayAPI(*api)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || f.NArg() != 0 {
		fmt.Fprintln(out, "Base API HTTPS valide requise avec --api ou AXO_CLASH_API_URL.")
		return 2
	}
	if u.Scheme != "https" {
		if !*dev || u.Scheme != "http" {
			fmt.Fprintln(out, "HTTPS obligatoire (HTTP loopback uniquement avec --dev).")
			return 2
		}
		if _, err := normalizeLoopback(u.String()); err != nil {
			fmt.Fprintln(out, err)
			return 2
		}
	}
	bots, err := relayBots(declarations)
	if err != nil {
		fmt.Fprintln(out, err)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	local := loopbackClient()
	defer local.CloseIdleConnections()
	for i := range bots {
		status, data, _, code := invokeLocal(ctx, local, bots[i].base, "INFO", nil, 2000)
		if code != "" || status != 200 {
			fmt.Fprintf(out, "Bot %s : /info ne répond pas au contrat.\n", bots[i].Alias)
			return 1
		}
		json.Unmarshal(data, &bots[i].Info)
	}
	code, err := maskedPairingCode(out)
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	credential, socketURL, err := exchangeRelay(ctx, strings.TrimRight(u.String(), "/"), code, *dev)
	code = ""
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	backoff := time.Second
	for ctx.Err() == nil {
		connectedAt := time.Now()
		err = connectRelay(ctx, socketURL, credential, bots, local, out)
		if ctx.Err() != nil {
			return 0
		}
		var terminal *relayAuthError
		if errors.As(err, &terminal) {
			fmt.Fprintln(out, "Session expirée ou révoquée. Crée une nouvelle session dans le labo.")
			return 1
		}
		fmt.Fprintln(out, "Relais déconnecté. Reconnexion ; le combat interrompu ne sera pas rejoué.")
		if time.Since(connectedAt) > time.Minute {
			backoff = time.Second
		}
		select {
		case <-ctx.Done():
			return 0
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
		}
	}
	return 0
}

// Accept the public origin as well as an explicit API prefix.
func normalizeRelayAPI(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	u.Path = strings.TrimRight(u.Path, "/")
	if u.Path == "" {
		u.Path = "/api/v1"
	}
	return u, nil
}

func exchangeRelay(ctx context.Context, base, code string, dev bool) (string, string, error) {
	data, _ := json.Marshal(map[string]string{"code": code})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/lab/relay-sessions/exchange", bytes.NewReader(data))
	if err != nil {
		return "", "", errors.New("API invalide")
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	resp, err := client.Do(req)
	if err != nil {
		return "", "", errors.New("association impossible : API inaccessible")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", "", fmt.Errorf("association refusée (HTTP %d)", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, relayMessageLimit+1))
	var result struct {
		Credential   string `json:"credential"`
		WebsocketURL string `json:"websocketUrl"`
	}
	if err != nil || len(b) > relayMessageLimit || json.Unmarshal(b, &result) != nil || result.Credential == "" {
		return "", "", errors.New("réponse d'association invalide")
	}
	if result.WebsocketURL == "" {
		result.WebsocketURL = strings.Replace(base, "http", "ws", 1) + "/lab/relay"
	}
	u, err := url.Parse(result.WebsocketURL)
	baseURL, _ := url.Parse(base)
	if err == nil && u.Host == "" && u.Scheme == "" && strings.HasPrefix(u.Path, "/") {
		u.Host = baseURL.Host
		u.Scheme = "wss"
		if dev && baseURL.Scheme == "http" {
			u.Scheme = "ws"
		}
	}
	if err != nil || u.Host != baseURL.Host || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "wss" && !(dev && baseURL.Scheme == "http" && u.Scheme == "ws")) {
		return "", "", errors.New("endpoint WSS d'association invalide")
	}
	return result.Credential, u.String(), nil
}

func invokeLocal(ctx context.Context, client *http.Client, base, operation string, payload json.RawMessage, timeout int64) (int, json.RawMessage, int64, string) {
	if timeout <= 0 || timeout > 2000 {
		timeout = 2000
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
	defer cancel()
	method, path := http.MethodGet, "/info"
	if operation == "PLAY" {
		method, path = http.MethodPost, "/play"
	} else if operation != "INFO" {
		return 0, nil, 0, "INVALID_RESPONSE"
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, bytes.NewReader(payload))
	if err != nil {
		return 0, nil, 0, "INVALID_RESPONSE"
	}
	req.Header.Set("Accept", "application/json")
	if operation == "PLAY" {
		req.Header.Set("Content-Type", "application/json")
	}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return 0, nil, time.Since(start).Milliseconds(), "LOCAL_TIMEOUT"
		}
		return 0, nil, time.Since(start).Milliseconds(), "LOCAL_UNREACHABLE"
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, relayMessageLimit+1))
	duration := time.Since(start).Milliseconds()
	var object map[string]json.RawMessage
	if err != nil || len(b) > relayMessageLimit || json.Unmarshal(b, &object) != nil || object == nil || (resp.StatusCode >= 300 && resp.StatusCode < 400) {
		return 0, nil, duration, "INVALID_RESPONSE"
	}
	if resp.StatusCode == 200 {
		fields := []string{"action"}
		if operation == "INFO" {
			fields = []string{"name", "version"}
		}
		for _, field := range fields {
			var value string
			if json.Unmarshal(object[field], &value) != nil || strings.TrimSpace(value) == "" || (field == "action" && !knownAction(value)) {
				return 0, nil, duration, "INVALID_RESPONSE"
			}
		}
	}
	return resp.StatusCode, b, duration, ""
}

type relayAuthError struct{}

func (*relayAuthError) Error() string { return "session relay unavailable" }

func connectRelay(parent context.Context, socketURL, credential string, bots []localRelayBot, client *http.Client, out io.Writer) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	dialCtx, dialCancel := context.WithTimeout(ctx, 20*time.Second)
	conn, resp, err := websocket.Dial(dialCtx, socketURL, &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Bearer " + credential}}, HTTPClient: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}})
	dialCancel()
	if err != nil {
		if resp != nil && (resp.StatusCode == 401 || resp.StatusCode == 403 || resp.StatusCode == 410) {
			return &relayAuthError{}
		}
		return errors.New("connexion WSS impossible")
	}
	defer conn.CloseNow()
	conn.SetReadLimit(relayMessageLimit)
	queue := make(chan any, 32)
	var wg sync.WaitGroup
	defer func() { cancel(); wg.Wait() }()
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-queue:
				b, e := json.Marshal(msg)
				if e != nil || len(b) > relayMessageLimit {
					cancel()
					return
				}
				writeCtx, done := context.WithTimeout(ctx, 5*time.Second)
				e = conn.Write(writeCtx, websocket.MessageText, b)
				done()
				if e != nil {
					cancel()
					return
				}
			}
		}
	}()
	send := func(msg any) {
		select {
		case queue <- msg:
		case <-ctx.Done():
		default:
			cancel()
		}
	}
	send(struct {
		Type            string          `json:"type"`
		ProtocolVersion int             `json:"protocolVersion"`
		Bots            []localRelayBot `json:"bots"`
	}{"register", 1, bots})
	bindings := map[string]localRelayBot{}
	connectionID := ""
	registered := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		timer := time.NewTimer(20 * time.Second)
		defer timer.Stop()
		select {
		case <-registered:
		case <-ctx.Done():
		case <-timer.C:
			cancel()
		}
	}()
	var mu sync.Mutex
	type runningCall struct {
		cancel    context.CancelFunc
		requestID string
	}
	active := map[string]runningCall{}
	seen := map[string]bool{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pingCtx, done := context.WithTimeout(ctx, 10*time.Second)
				e := conn.Ping(pingCtx)
				done()
				if e != nil {
					cancel()
					return
				}
			}
		}
	}()
	for {
		kind, b, e := conn.Read(ctx)
		if e != nil {
			if websocket.CloseStatus(e) == websocket.StatusPolicyViolation {
				return &relayAuthError{}
			}
			return errors.New("connexion interrompue")
		}
		var msg relayMessage
		if kind != websocket.MessageText || json.Unmarshal(b, &msg) != nil || msg.ProtocolVersion != 1 {
			return errors.New("protocole relay invalide")
		}
		switch msg.Type {
		case "hello", "connected":
			continue
		case "registered":
			if connectionID != "" || msg.ConnectionID == "" || len(msg.Bots) != len(bots) {
				return errors.New("enregistrement relay invalide")
			}
			connectionID = msg.ConnectionID
			registeredAliases := map[string]bool{}
			for _, binding := range msg.Bots {
				found := false
				for _, bot := range bots {
					if binding.Alias == bot.Alias && !registeredAliases[binding.Alias] && binding.BindingID != "" && bindings[binding.BindingID].Alias == "" {
						bindings[binding.BindingID] = bot
						registeredAliases[binding.Alias] = true
						found = true
						break
					}
				}
				if !found {
					return errors.New("liaison relay invalide")
				}
			}
			close(registered)
			fmt.Fprintf(out, "Relais connecté : %d bot(s). Lance un test depuis le labo.\n", len(bindings))
		case "ping":
			send(relayMessage{Type: "pong", ProtocolVersion: 1, ConnectionID: connectionID})
		case "pong":
			continue
		case "bot.cancel":
			if msg.ConnectionID != connectionID {
				return errors.New("génération de connexion invalide")
			}
			mu.Lock()
			call, ok := active[msg.BindingID]
			if ok && call.requestID == msg.RequestID {
				call.cancel()
			}
			mu.Unlock()
		case "bot.request":
			bot, ok := bindings[msg.BindingID]
			if connectionID == "" || msg.ConnectionID != connectionID || !ok || msg.RequestID == "" || len(msg.RequestID) > 128 || msg.TimeoutMS <= 0 || msg.TimeoutMS > 2000 || (msg.Operation != "INFO" && msg.Operation != "PLAY") {
				return errors.New("requête relay invalide")
			}
			if msg.Operation == "PLAY" && (msg.GameID <= 0 || msg.GameRunID == "" || (msg.Seat != "FIRST_BOT" && msg.Seat != "SECOND_BOT") || len(msg.Payload) == 0) {
				return errors.New("contexte de combat invalide")
			}
			mu.Lock()
			if seen[msg.RequestID] || len(seen) >= 10000 {
				mu.Unlock()
				return errors.New("requête répétée ou session trop longue")
			}
			seen[msg.RequestID] = true
			if _, busy := active[msg.BindingID]; busy || len(active) >= 4 {
				mu.Unlock()
				send(relayMessage{Type: "bot.error", ProtocolVersion: 1, ConnectionID: connectionID, RequestID: msg.RequestID, BindingID: msg.BindingID, Code: "LOCAL_UNREACHABLE", Message: "Bot occupé"})
				continue
			}
			callCtx, done := context.WithCancel(ctx)
			active[msg.BindingID] = runningCall{done, msg.RequestID}
			mu.Unlock()
			wg.Add(1)
			go func(request relayMessage, localBot localRelayBot) {
				defer wg.Done()
				defer done()
				status, payload, duration, code := invokeLocal(callCtx, client, localBot.base, request.Operation, request.Payload, request.TimeoutMS)
				mu.Lock()
				delete(active, request.BindingID)
				mu.Unlock()
				if callCtx.Err() != nil {
					return
				}
				response := relayMessage{Type: "bot.response", ProtocolVersion: 1, ConnectionID: request.ConnectionID, RequestID: request.RequestID, BindingID: request.BindingID, Status: status, Payload: payload, LocalHTTPDurationMS: duration}
				if code != "" {
					response.Type = "bot.error"
					response.Code = code
					response.Message = "Le bot local n'a pas répondu au contrat"
				}
				send(response)
			}(msg, bot)
		default:
			return errors.New("message relay inconnu")
		}
	}
}
