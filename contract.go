package main

import (
	"encoding/json"
	"io"
)

// Exported snapshots are embedded by publication validators; no runtime dependency on this CLI.
func exportContract(out io.Writer) error {
	type entry struct {
		Name    string `json:"name"`
		Method  string `json:"method"`
		Path    string `json:"path"`
		Payload object `json:"payload"`
	}
	result := []entry{}
	for _, s := range scenarios() {
		result = append(result, entry{s.name, s.method, s.path, s.payload})
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	return encoder.Encode(result)
}
