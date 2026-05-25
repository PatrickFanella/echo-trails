package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

var errConflict = errors.New("conflict")
var errNotFound = errors.New("not found")
var errForbidden = errors.New("forbidden")

type idempotencyRecord struct {
	AuthorID string
	Hash     string
	EchoID   string
}

type Store struct {
	mu          sync.Mutex
	seq         int
	echoes      map[string]*Echo
	idempotency map[string]idempotencyRecord
}

func NewStore() *Store {
	return &Store{echoes: map[string]*Echo{}, idempotency: map[string]idempotencyRecord{}}
}

func (s *Store) CreateDraft(authorID, key string, req createEchoRequest) (*Echo, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h := hashRequest(req)
	if key != "" {
		if rec, ok := s.idempotency[key]; ok {
			if rec.AuthorID != authorID || rec.Hash != h {
				return nil, false, errConflict
			}
			return s.echoes[rec.EchoID], true, nil
		}
	}
	s.seq++
	id := fmt.Sprintf("echo_%d", s.seq)
	now := time.Now().UTC()
	e := &Echo{ID: id, AuthorID: authorID, Title: req.Title, Body: req.Body, Status: EchoStatusDraft, CreatedAt: now, UpdatedAt: now, Lat: req.Lat, Lon: req.Lon}
	s.echoes[id] = e
	if key != "" {
		s.idempotency[key] = idempotencyRecord{AuthorID: authorID, Hash: h, EchoID: id}
	}
	return e, false, nil
}

func (s *Store) Publish(id, authorID string) (*Echo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.echoes[id]
	if !ok {
		return nil, errNotFound
	}
	if e.AuthorID != authorID {
		return nil, errForbidden
	}
	e.Status = EchoStatusPublished
	e.UpdatedAt = time.Now().UTC()
	return e, nil
}

func (s *Store) Get(id, requester string) (*Echo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.echoes[id]
	if !ok {
		return nil, errNotFound
	}
	if e.Status == EchoStatusDraft && e.AuthorID != requester {
		return nil, errForbidden
	}
	return e, nil
}

func hashRequest(req createEchoRequest) string {
	h := sha256.New()
	fmt.Fprintf(h, "%q|%q|%f|%f", req.Title, req.Body, req.Lat, req.Lon)
	return hex.EncodeToString(h.Sum(nil))
}
