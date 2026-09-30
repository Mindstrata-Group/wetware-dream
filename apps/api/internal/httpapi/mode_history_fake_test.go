package httpapi

// fakeModeHistoryStore is an in-memory modeHistoryStore implementation for tests.
// It does not touch disk/network/SSH: it just stores snapshots per modeID in
// commit order, as GitModeHistoryStore does, but without a real git.
//
// The file deliberately has no integration build tag: it is used both by unit
// tests and by *_integration_test.go (they have their own integration tag, but
// see symbols from regular _test.go files of the same package).

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

type fakeModeHistoryCommit struct {
	sha         string
	snap        modeSnapshot
	authorName  string
	authorEmail string
	message     string
	createdAt   time.Time
}

type fakeModeHistoryStore struct {
	mu   sync.Mutex
	on   bool // configured()
	seq  int
	byID map[int64][]fakeModeHistoryCommit

	// CommitErr, if not nil, makes commitSnapshot always return this error
	// (to check that a git push error does not break saving to the DB).
	CommitErr error
}

func newFakeModeHistoryStore(configured bool) *fakeModeHistoryStore {
	return &fakeModeHistoryStore{on: configured, byID: map[int64][]fakeModeHistoryCommit{}}
}

func (s *fakeModeHistoryStore) configured() bool { return s.on }

func (s *fakeModeHistoryStore) commitSnapshot(_ context.Context, modeID int64, snap modeSnapshot, authorName, authorEmail, message string) (string, error) {
	if !s.on {
		return "", nil
	}
	if s.CommitErr != nil {
		return "", s.CommitErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	sum := sha1.Sum([]byte(fmt.Sprintf("%d:%d:%s", modeID, s.seq, message)))
	sha := hex.EncodeToString(sum[:])
	commit := fakeModeHistoryCommit{
		sha: sha, snap: snap, authorName: authorName, authorEmail: authorEmail,
		message: message, createdAt: time.Now(),
	}
	// Newest first, as in git log.
	s.byID[modeID] = append([]fakeModeHistoryCommit{commit}, s.byID[modeID]...)
	return sha, nil
}

func (s *fakeModeHistoryStore) history(_ context.Context, modeID int64, limit int) ([]modeHistoryEntry, error) {
	if !s.on {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	commits := s.byID[modeID]
	if limit <= 0 || limit > len(commits) {
		limit = len(commits)
	}
	out := make([]modeHistoryEntry, 0, limit)
	for _, c := range commits[:limit] {
		out = append(out, modeHistoryEntry{
			SHA: c.sha, AuthorName: c.authorName, AuthorEmail: c.authorEmail,
			CreatedAt: c.createdAt, Message: c.message,
		})
	}
	return out, nil
}

func (s *fakeModeHistoryStore) snapshotAt(_ context.Context, modeID int64, sha string) (modeSnapshot, error) {
	if !s.on {
		return modeSnapshot{}, errModeHistoryNotConfigured
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.byID[modeID] {
		if c.sha == sha {
			return c.snap, nil
		}
	}
	return modeSnapshot{}, fmt.Errorf("версия не найдена")
}
