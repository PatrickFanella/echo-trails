package apperror

import (
	"encoding/json"
	"net/http"
)

type Error struct {
	Status int
	Code   string
	Title  string
	Detail string
	Meta   map[string]any
}

type response struct {
	Error responseError `json:"error"`
}

type responseError struct {
	Code   string         `json:"code"`
	Title  string         `json:"title"`
	Detail string         `json:"detail,omitempty"`
	Meta   map[string]any `json:"meta,omitempty"`
}

func New(status int, code, title, detail string) Error {
	return Error{Status: status, Code: code, Title: title, Detail: detail}
}

func (e Error) JSON() ([]byte, error) {
	status := e.Status
	if status == 0 {
		status = http.StatusInternalServerError
	}
	title := e.Title
	if title == "" {
		title = http.StatusText(status)
	}

	return json.Marshal(response{Error: responseError{Code: e.Code, Title: title, Detail: e.Detail, Meta: e.Meta}})
}
