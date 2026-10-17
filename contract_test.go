package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExportContractCoversEveryZoneBoundaryAndBothSeats(t *testing.T) {
	var out bytes.Buffer
	if code := run([]string{"export-contract"}, &out); code != 0 {
		t.Fatal(code)
	}
	var cases []struct {
		Name, Method, Path string
		Payload            object
	}
	if err := json.Unmarshal(out.Bytes(), &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != len(scenarios()) {
		t.Fatal("export lost scenarios")
	}
	names := map[string]bool{}
	for _, s := range cases {
		if names[s.Name] || s.Method != "POST" || s.Path != "/play" {
			t.Fatalf("invalid case: %+v", s)
		}
		names[s.Name] = true
	}
	for _, seat := range []string{"first", "second"} {
		for _, turn := range []int{69, 70, 74, 75, 79, 80, 84, 85, 89, 90, 94, 95, 99, 100, 104, 105} {
			if !names[fmt.Sprintf("%s/zone-rouge-tour-%d", seat, turn)] {
				t.Fatal("missing zone boundary", seat, turn)
			}
		}
		for _, name := range []string{"piege-en-armement", "piege-arme-sous-bot", "recharge-et-piege-remplace", "pose-disponible-tour-10", "piege-propre-sous-bot-avec-tirs", "pieges-des-deux-bots", "piege-expire-et-recharge-disponible"} {
			if !names[seat+"/"+name] {
				t.Fatal("missing trap case", seat, name)
			}
		}
	}
}

func TestTrapActionAvailabilityUsesNextRoundAndRecharge(t *testing.T) {
	response := map[string]json.RawMessage{"action": json.RawMessage(`"PLACE_TRAP"`), "coordinates": json.RawMessage(`{"x":3,"y":4}`)}
	for _, tc := range []struct {
		current, next int
		valid         bool
	}{{8, 10, false}, {9, 10, true}, {18, 20, false}, {19, 20, true}, {20, 30, false}, {29, 30, true}} {
		payload := []byte(fmt.Sprintf(`{"game":{"currentTurn":%d},"bot":{"data":{"nextTrapTurn":%d}}}`, tc.current, tc.next))
		if err := validateTrapAction(response, payload); (err == nil) != tc.valid {
			t.Fatalf("%+v: %v", tc, err)
		}
	}
}

func TestCheckerRejectsBotsUnableToDecodeTrapsAndRedZoneInputs(t *testing.T) {
	for _, reject := range []string{"trap", "zone"} {
		t.Run(reject, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == "GET" {
					fmt.Fprint(w, `{"name":"old-bot","version":"1"}`)
					return
				}
				var input object
				_ = json.NewDecoder(r.Body).Decode(&input)
				game := input["game"].(map[string]any)
				encoded, _ := json.Marshal(game["board"])
				if reject == "trap" && bytes.Contains(encoded, []byte(`"placedBy"`)) || reject == "zone" && game["currentTurn"].(float64) >= 70 {
					w.WriteHeader(400)
					fmt.Fprint(w, `{"error":"unsupported input"}`)
					return
				}
				fmt.Fprint(w, `{"action":"MOVE_UP"}`)
			}))
			defer server.Close()
			var output bytes.Buffer
			if run([]string{"check-bot", "-url", server.URL}, &output) != 1 || !strings.Contains(output.String(), "ECHEC") {
				t.Fatal(output.String())
			}
		})
	}
}
