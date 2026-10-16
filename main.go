package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const maxResponseBytes = 1 << 20

func main() {
	os.Exit(run(os.Args[1:], os.Stdout))
}

// run is kept separate so future CLI commands can reuse the checker.
func run(args []string, out io.Writer) int {
	if len(args) > 0 && args[0] == "relay" {
		return runRelay(args[1:], out)
	}
	flags := flag.NewFlagSet("axo-clash", flag.ContinueOnError)
	flags.SetOutput(out)
	base := flags.String("url", "http://localhost:9000/api/v1/bots", "URL de base du bot, incluant /api/v1/bots")
	timeout := flags.Duration("timeout", 2*time.Second, "Délai maximal par requête (ex. 2s)")
	repeat := flags.Int("repeat", 1, "Nombre de passages de chaque scénario")
	flags.Usage = func() {
		fmt.Fprintln(out, "Usage : axo-clash check-bot [options]")
		flags.PrintDefaults()
	}
	if len(args) > 0 && args[0] == "check-bot" {
		args = args[1:]
	}
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	u, err := url.Parse(*base)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.RawQuery != "" || u.Fragment != "" || u.User != nil || flags.NArg() != 0 || *timeout <= 0 || *repeat <= 0 {
		fmt.Fprintln(out, "Paramètres invalides : URL HTTP(S) sans identifiants/query/fragment, timeout et repeat strictement positifs requis.")
		return 2
	}
	client := &http.Client{
		Timeout: *timeout,
		// A redirect must not hide a missing endpoint or a non-200 response.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	defer client.CloseIdleConnections()
	cases := append([]scenario{{name: "info", method: http.MethodGet, path: "/info"}}, scenarios()...)
	failed, total := 0, 0
	for pass := 1; pass <= *repeat; pass++ {
		for _, s := range cases {
			total++
			start := time.Now()
			err := check(client, strings.TrimRight(u.String(), "/"), s)
			if err != nil {
				failed++
				fmt.Fprintf(out, "ECHEC [%d/%d] %s : %v\n", pass, *repeat, s.name, err)
			} else {
				fmt.Fprintf(out, "OK    [%d/%d] %s (%s)\n", pass, *repeat, s.name, time.Since(start).Round(time.Millisecond))
			}
		}
	}
	fmt.Fprintf(out, "\n%d/%d vérifications réussies, %d échec(s).\n", total-failed, total, failed)
	if failed > 0 {
		return 1
	}
	return 0
}

func check(client *http.Client, base string, s scenario) error {
	var body []byte
	if s.payload != nil {
		var err error
		body, err = json.Marshal(s.payload)
		if err != nil {
			return fmt.Errorf("encodage de la requête : %w", err)
		}
	}
	req, err := http.NewRequest(s.method, base+s.path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if s.payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s : %w", s.method, s.path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("lecture de la réponse : %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		if len(data) > 300 {
			data = data[:300]
		}
		return fmt.Errorf("HTTP %d, attendu 200 ; réponse : %q", resp.StatusCode, data)
	}
	if len(data) > maxResponseBytes {
		return fmt.Errorf("réponse dépassant %d octets", maxResponseBytes)
	}
	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || (mediaType != "application/json" && !(strings.HasPrefix(mediaType, "application/") && strings.HasSuffix(mediaType, "+json"))) {
		return fmt.Errorf("Content-Type JSON attendu, reçu %q", resp.Header.Get("Content-Type"))
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(data, &result); err != nil || result == nil {
		return fmt.Errorf("réponse attendue : un objet JSON valide")
	}
	fields := []string{"action"}
	if s.method == http.MethodGet {
		fields = []string{"name", "version"}
	}
	for _, field := range fields {
		var value string
		if err := json.Unmarshal(result[field], &value); err != nil || strings.TrimSpace(value) == "" {
			return fmt.Errorf("champ %q absent, vide ou non textuel", field)
		}
		if field == "action" && !knownAction(value) {
			return fmt.Errorf("action inconnue %q ; valeurs acceptées : %s", value, strings.Join(actions, ", "))
		}
	}
	return nil
}

func knownAction(value string) bool {
	for _, action := range actions {
		if action == value {
			return true
		}
	}
	return false
}
