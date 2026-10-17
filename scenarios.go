package main

import (
	"fmt"
	"net/http"
)

// Values and fields mirror the backend DTOs and enums, not the older Rust templates.
var actions = []string{"MOVE_LEFT", "MOVE_RIGHT", "MOVE_UP", "MOVE_DOWN", "SHOOT_LEFT", "SHOOT_RIGHT", "SHOOT_UP", "SHOOT_DOWN", "PLACE_TRAP", "UNKNOWN"}
var bullets = []string{"BULLET_LEFT", "BULLET_RIGHT", "BULLET_UP", "BULLET_DOWN"}

type object = map[string]any

type scenario struct {
	name    string
	method  string
	path    string
	payload object
}

func scenarios() []scenario {
	var result []scenario
	add := func(name string, role int, mutate func(object, object)) {
		payload := newGame(role)
		if mutate != nil {
			mutate(payload["game"].(object), payload["bot"].(object))
		}
		result = append(result, scenario{name, http.MethodPost, "/play", payload})
	}
	for _, role := range []int{1, 2} {
		label := []string{"", "first", "second"}[role]
		add(label+"/initial-champs-omis", role, nil)
		add(label+"/null-explicites", role, func(game, bot object) {
			for _, key := range []string{"winner", "endReason", "failureReason"} {
				game[key] = nil
			}
			for _, b := range []object{game["firstBot"].(object), game["secondBot"].(object)} {
				b["data"].(object)["lastActionTimeMs"] = nil
			}
			for _, row := range game["board"].([][]object) {
				for _, cell := range row {
					if _, exists := cell["content"]; !exists {
						cell["content"] = nil
					}
					cell["items"] = nil
					cell["trap"] = nil
				}
			}
		})

		add(label+"/piege-en-armement", role, func(game, bot object) {
			game["currentTurn"] = 10
			bot["data"].(object)["nextTrapTurn"] = 20
			game["board"].([][]object)[2][3]["trap"] = object{"placedBy": "FIRST_BOT", "placedAtTurn": 10, "armedFromTurn": 11, "expiresAfterTurn": 20}
		})
		add(label+"/piege-arme-sous-bot", role, func(game, bot object) {
			game["currentTurn"] = 19
			bot["data"].(object)["nextTrapTurn"] = 20
			pos := bot["coordinates"].(object)
			game["board"].([][]object)[pos["y"].(int)][pos["x"].(int)]["trap"] = object{"placedBy": "SECOND_BOT", "placedAtTurn": 10, "armedFromTurn": 11, "expiresAfterTurn": 20}
		})
		add(label+"/recharge-et-piege-remplace", role, func(game, bot object) {
			game["currentTurn"] = 20
			bot["data"].(object)["nextTrapTurn"] = 30
			game["board"].([][]object)[3][3]["trap"] = object{"placedBy": "SECOND_BOT", "placedAtTurn": 20, "armedFromTurn": 21, "expiresAfterTurn": 30}
			game["actions"] = []object{{"id": 1, "botId": 1, "action": "PLACE_TRAP", "coordinates": object{"x": 3, "y": 3}, "forbidden": false, "executionTimeMs": 2, "createdDate": "2026-10-17T12:00:00Z"}}
		})
		for _, turn := range []int{69, 70, 74, 75, 79, 80, 84, 85, 89, 90, 94, 95, 99, 100, 104, 105} {
			add(fmt.Sprintf("%s/zone-rouge-tour-%d", label, turn), role, func(game, bot object) {
				game["currentTurn"] = turn
				limit := 100
				if turn >= 100 {
					limit = 120
				}
				game["rules"] = object{"startingLife": 10, "responseTimeoutMs": 2000, "maxTurns": limit, "maxForbiddenActions": 10}
				for _, seat := range []object{game["firstBot"].(object), game["secondBot"].(object)} {
					seat["data"].(object)["nextTrapTurn"] = 10
				}
			})
		}
		add(label+"/pose-disponible-tour-10", role, func(game, bot object) {
			game["currentTurn"] = 9
			for _, seat := range []object{game["firstBot"].(object), game["secondBot"].(object)} {
				seat["data"].(object)["nextTrapTurn"] = 10
			}
		})
		add(label+"/piege-propre-sous-bot-avec-tirs", role, func(game, bot object) {
			game["currentTurn"] = 11
			bot["data"].(object)["nextTrapTurn"] = 20
			pos := bot["coordinates"].(object)
			cell := game["board"].([][]object)[pos["y"].(int)][pos["x"].(int)]
			cell["trap"] = object{"placedBy": bot["botType"], "placedAtTurn": 10, "armedFromTurn": 11, "expiresAfterTurn": 20}
			cell["items"] = []object{{"type": "BULLET_RIGHT", "owner": game["firstBot"]}, {"type": "BULLET_UP", "owner": game["secondBot"]}}
		})
		add(label+"/pieges-des-deux-bots", role, func(game, bot object) {
			game["currentTurn"] = 19
			for _, key := range []string{"firstBot", "secondBot"} {
				seat := game[key].(object)
				seat["data"].(object)["nextTrapTurn"] = 20
				pos := seat["coordinates"].(object)
				game["board"].([][]object)[pos["y"].(int)][pos["x"].(int)]["trap"] = object{"placedBy": seat["botType"], "placedAtTurn": 10, "armedFromTurn": 11, "expiresAfterTurn": 20}
			}
		})
		add(label+"/piege-expire-et-recharge-disponible", role, func(game, bot object) {
			game["currentTurn"] = 20
			bot["data"].(object)["nextTrapTurn"] = 20
			game["board"].([][]object)[3][3]["trap"] = nil
		})
		add(label+"/historique-complet", role, func(game, bot object) {
			game["currentTurn"] = 10
			var history []object
			for i, action := range actions {
				entry := object{"id": i + 1, "botId": i%2 + 1, "action": action, "forbidden": action == "UNKNOWN", "executionTimeMs": 23, "createdDate": "2026-10-10T12:00:00.123456789Z"}
				if action == "PLACE_TRAP" {
					entry["coordinates"] = object{"x": 3, "y": 4}
				}
				history = append(history, entry)
			}
			game["actions"] = history
			for _, b := range []object{game["firstBot"].(object), game["secondBot"].(object)} {
				b["data"].(object)["lastActionTimeMs"] = 23
			}
			bot["faster"] = true
			bot["data"].(object)["life"] = 1
			bot["data"].(object)["forbiddenActions"] = 2
		})
		for i, bullet := range bullets {
			add(label+"/"+bullet, role, func(game, bot object) {
				board := game["board"].([][]object)
				board[3][3]["content"] = bullet
				board[3][3]["items"] = []object{{"type": bullet, "owner": game["firstBot"]}}
				// Bullet on a bot: content remains the bot, items carries the bullet.
				board[6][6]["items"] = []object{{"type": bullet, "owner": game["firstBot"]}}
				board[2][2]["items"] = []object{{"type": bullet, "owner": game["firstBot"]}, {"type": bullets[(i+1)%len(bullets)], "owner": game["secondBot"]}}
				board[2][2]["content"] = bullet
			})
		}
		add(label+"/items-vides", role, func(game, bot object) {
			for _, row := range game["board"].([][]object) {
				for _, cell := range row {
					cell["items"] = []object{}
				}
			}
		})
		add(label+"/bords-du-plateau", role, func(game, bot object) {
			board := game["board"].([][]object)
			delete(board[1][1], "content")
			delete(board[6][6], "content")
			game["firstBot"].(object)["coordinates"] = object{"x": 0, "y": 0}
			game["secondBot"].(object)["coordinates"] = object{"x": 7, "y": 7}
			board[0][0]["content"] = "FIRST_BOT"
			board[7][7]["content"] = "SECOND_BOT"
		})
	}
	// Contract variants: these states are defined by the API even when the engine
	// normally calls /play only for running games.
	for _, state := range []string{"UNKNOWN", "READY", "WAITING_FOR_OPPONENT"} {
		add("contrat/etat-"+state, 1, func(game, bot object) { bot["data"].(object)["state"] = state })
	}
	for _, status := range []string{"STARTING", "PAUSED", "FINISHED", "FAILED"} {
		add("contrat/statut-"+status, 1, func(game, bot object) { game["status"] = status })
	}
	for _, winner := range []string{"FIRST_BOT", "SECOND_BOT", "DRAW"} {
		add("contrat/gagnant-"+winner, 1, func(game, bot object) {
			game["status"], game["winner"], game["endReason"] = "FINISHED", winner, "GAME_TURN"
		})
	}
	for _, reason := range []string{"FIRST_BOT_LIFE", "SECOND_BOT_LIFE", "FIRST_BOT_FORBIDDEN_ACTION", "SECOND_BOT_FORBIDDEN_ACTION", "GAME_TURN"} {
		add("contrat/fin-"+reason, 1, func(game, bot object) {
			game["status"], game["endReason"] = "FINISHED", reason
		})
	}
	for _, reason := range []string{"BOT_CONNECTION_ERROR", "BOT_INFO_RETRIEVAL_ERROR", "GAME_EXECUTION_ERROR", "INVALID_BOARD_STATE", "BOT_COMMUNICATION_TIMEOUT", "JSON_PROCESSING_ERROR", "UNKNOWN_ERROR"} {
		add("contrat/echec-"+reason, 1, func(game, bot object) {
			game["status"], game["failureReason"] = "FAILED", reason
		})
	}
	return result
}

func newGame(role int) object {
	newBot := func(id, x, y int, botType string) object {
		return object{
			"id": id, "name": "Bot de test éà🤖", "version": "1.0.0", "url": "http://localhost:9000/api/v1/bots",
			"data":    object{"life": 10, "forbiddenActions": 0, "state": "PLAYING"},
			"botType": botType, "coordinates": object{"x": x, "y": y}, "faster": false,
		}
	}
	first, second := newBot(1, 1, 1, "FIRST_BOT"), newBot(2, 6, 6, "SECOND_BOT")
	board := make([][]object, 8)
	for y := range board {
		board[y] = make([]object, 8)
		for x := range board[y] {
			board[y][x] = object{"coordinates": object{"x": x, "y": y}}
		}
	}
	board[1][1]["content"], board[6][6]["content"] = "FIRST_BOT", "SECOND_BOT"
	bot := first
	if role == 2 {
		bot = second
	}
	return object{"game": object{
		"id": 1, "firstBot": first, "secondBot": second, "board": board,
		"currentTurn": 0, "actions": []object{}, "status": "RUNNING",
	}, "bot": bot}
}
