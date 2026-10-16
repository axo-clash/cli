package main

import "net/http"

// Values and fields mirror the backend DTOs and enums, not the older Rust templates.
var actions = []string{"MOVE_LEFT", "MOVE_RIGHT", "MOVE_UP", "MOVE_DOWN", "SHOOT_LEFT", "SHOOT_RIGHT", "SHOOT_UP", "SHOOT_DOWN", "UNKNOWN"}
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
				}
			}
		})
		add(label+"/historique-complet", role, func(game, bot object) {
			game["currentTurn"] = 10
			var history []object
			for i, action := range actions {
				history = append(history, object{"id": i + 1, "botId": i%2 + 1, "action": action, "forbidden": action == "UNKNOWN", "executionTimeMs": 23, "createdDate": "2026-10-10T12:00:00.123456789Z"})
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
