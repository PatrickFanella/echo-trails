package apperror

import (
	"encoding/json"
	"testing"
)

func TestErrorJSONShape(t *testing.T) {
	err := New(400, "invalid_cursor", "Invalid cursor", "The cursor could not be decoded.")
	err.Meta = map[string]any{"parameter": "cursor"}

	body, jsonErr := err.JSON()
	if jsonErr != nil {
		t.Fatalf("JSON: %v", jsonErr)
	}

	var decoded map[string]map[string]any
	if jsonErr := json.Unmarshal(body, &decoded); jsonErr != nil {
		t.Fatalf("Unmarshal: %v", jsonErr)
	}
	errorBody := decoded["error"]
	if errorBody["code"] != "invalid_cursor" || errorBody["title"] != "Invalid cursor" || errorBody["detail"] != "The cursor could not be decoded." {
		t.Fatalf("unexpected error shape: %s", body)
	}
	meta, ok := errorBody["meta"].(map[string]any)
	if !ok || meta["parameter"] != "cursor" {
		t.Fatalf("unexpected meta shape: %s", body)
	}
}
