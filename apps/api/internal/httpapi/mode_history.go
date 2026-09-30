package httpapi

// mode_history.go is the git mirror of AI prompt edit history (the modes table).
//
// The DB remains the source of truth for modes. A separate git repository
// (address: MODE_HISTORY_REPO_URL) is used ONLY as a version log for
// history/diff/rollback in the admin UI; the user never sees the words
// git/commit/branch/SHA, only "version from {time}".
//
// The API container is built as distroless (no /bin/sh, git binary or ssh client),
// so all git operations go through the pure Go library go-git with an SSH
// deploy key (ssh.PublicKeys), not through git/ssh shell calls.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/plumbing/transport"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/skeema/knownhosts"
)

// modeSnapshot is the versioned content of a mode: only the meaningful
// text fields of the prompt. Technical fields (ai_model, temperature,
// hidden_at, demo_chat, orchestrator_check_interval, reminder_count)
// are configuration, not content, and versions do not store them.
type modeSnapshot struct {
	Name           string `json:"name"`
	Prompt         string `json:"prompt"`
	WelcomeMessage string `json:"welcomeMessage"`
	Criteria       string `json:"criteria"`
}

// modeHistoryEntry is one version in the history list (no diff: the
// frontend computes the diff by comparing the content of two versions).
type modeHistoryEntry struct {
	SHA         string    `json:"sha"`
	AuthorName  string    `json:"authorName"`
	AuthorEmail string    `json:"authorEmail"`
	CreatedAt   time.Time `json:"createdAt"`
	Message     string    `json:"message"`
}

// modeHistoryStore abstracts the git store of mode history. The production
// implementation (gitModeHistoryStore) is a go-git repository over an SSH deploy key;
// in tests, an in-memory fakeModeHistoryStore without real SSH access.
type modeHistoryStore interface {
	// configured reports whether git versioning is on (whether keys are set). If
	// false, all methods must silently no-op (empty history, no commit),
	// so CI and local development do not require a key.
	configured() bool
	// commitSnapshot commits and pushes the mode snapshot modes/<id>.json.
	// Returns the SHA of the new commit (or the current HEAD if the content
	// did not change and there is nothing to commit).
	commitSnapshot(ctx context.Context, modeID int64, snap modeSnapshot, authorName, authorEmail, message string) (sha string, err error)
	// history lists the versions of modes/<id>.json, newest first,
	// at most limit entries.
	history(ctx context.Context, modeID int64, limit int) ([]modeHistoryEntry, error)
	// snapshotAt returns the content of modes/<id>.json at commit sha.
	snapshotAt(ctx context.Context, modeID int64, sha string) (modeSnapshot, error)
}

// modeHistoryBranch is the only branch of the history repository.
const modeHistoryBranch = "main"

// githubSSHHostKey is github.com's public ed25519 host key (see GitHub
// "SSH key fingerprints" in the official documentation). Hardcoded instead of
// a pre-built known_hosts file: a distroless container has no
// ~/.ssh; the key is public and not a secret. We build our own
// known_hosts file from it at runtime (see githubHostKeyCallback).
//
// Important: an ssh.HostKeyCallback alone (golang.org/x/crypto/ssh/knownhosts.New)
// is NOT ENOUGH: go-git copies into ssh.ClientConfig.HostKeyAlgorithms ONLY
// the value of the PublicKeys.HostKeyAlgorithms field (see
// go-git/plumbing/transport/ssh: HostKeyCallbackHelper.SetHostKeyCallbackAndAlgorithms),
// not anything that could be "read" from the callback itself; golang.org/x/crypto/ssh
// never type-asserts a HostKeyCallback to get an algorithm list
// (confirmed by reading the v0.53.0 sources). If HostKeyAlgorithms is not
// set explicitly, the client offers the server all default key types, and
// github.com (which has ed25519/rsa/ecdsa) may negotiate something other than
// ed25519; then the comparison with the hardcoded ed25519 key wrongly
// fails as "host key mismatch" although the key itself is correct (bug found in
// prod on 2026-07-04). The right source of both fields at once (callback +
// a matching algorithm list) is github.com/skeema/knownhosts
// (a thin wrapper over x/crypto/ssh/knownhosts, which go-git itself uses
// inside NewKnownHostsDb/NewKnownHostsCallback for default known_hosts).
const githubSSHHostKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl"

// githubSSHHostWithPort is the host:port that go-git's SSH transport actually connects to
// (net.JoinHostPort(host, port), see plumbing/transport/ssh/common.go:
// getHostWithPort). Used as the lookup key in HostKeyAlgorithms(); it
// must match character for character what go-git computes when
// connecting, otherwise the algorithm list is empty and we are back to
// the old bug.
const githubSSHHostWithPort = "github.com:22"

const modeHistoryModesDir = "modes"

var errModeHistoryNotConfigured = errors.New("mode history: не настроен MODE_HISTORY_DEPLOY_KEY")

// GitModeHistoryStore is the production modeHistoryStore on top of go-git.
// The repository is lazily cloned/opened in repoPath on first
// access; before every write the fresh state is pulled from origin
// so several API replicas do not diverge in history.
type GitModeHistoryStore struct {
	repoPath     string
	repoURL      string
	deployKeyPEM string

	mu   sync.Mutex // serialises all git operations (clone/commit/push to disk, one worker)
	repo *git.Repository

	authOnce sync.Once
	auth     *gitssh.PublicKeys
	authErr  error
}

// NewGitModeHistoryStore creates the production mode history store.
// An empty deployKeyPEM or repoURL silently disables the feature
// (configured() == false). There is no default repository: every
// installation has its own.
func NewGitModeHistoryStore(repoPath, repoURL, deployKeyPEM string) *GitModeHistoryStore {
	if strings.TrimSpace(repoPath) == "" {
		repoPath = "/data/mode-history"
	}
	return &GitModeHistoryStore{repoPath: repoPath, repoURL: repoURL, deployKeyPEM: deployKeyPEM}
}

func (s *GitModeHistoryStore) configured() bool {
	return strings.TrimSpace(s.deployKeyPEM) != "" && strings.TrimSpace(s.repoURL) != ""
}

func (s *GitModeHistoryStore) sshAuth() (*gitssh.PublicKeys, error) {
	s.authOnce.Do(func() {
		auth, err := gitssh.NewPublicKeys("git", []byte(s.deployKeyPEM), "")
		if err != nil {
			s.authErr = fmt.Errorf("разбор MODE_HISTORY_DEPLOY_KEY: %w", err)
			return
		}
		db, err := s.githubHostKeyDB()
		if err != nil {
			s.authErr = fmt.Errorf("настройка проверки host key: %w", err)
			return
		}
		// Both fields are required and must come from the same db: a callback alone
		// without HostKeyAlgorithms does not force negotiation to ed25519 (see
		// the comment on githubSSHHostKey).
		auth.HostKeyCallback = db.HostKeyCallback()
		auth.HostKeyAlgorithms = db.HostKeyAlgorithms(githubSSHHostWithPort)
		s.auth = auth
	})
	return s.auth, s.authErr
}

// githubHostKeyDB writes a one-off known_hosts file with github.com's hardcoded
// ed25519 key and builds a HostKeyDB from it, the source of both the
// callback and the matching HostKeyAlgorithms list (the same
// pattern go-git itself uses for default known_hosts, see
// plumbing/transport/ssh/auth_method.go: NewKnownHostsDb).
func (s *GitModeHistoryStore) githubHostKeyDB() (*knownhosts.HostKeyDB, error) {
	dir := filepath.Dir(s.repoPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("создание каталога для known_hosts: %w", err)
	}
	knownHostsPath := filepath.Join(dir, "mode_history_known_hosts")
	line := "github.com " + githubSSHHostKey + "\n"
	if err := os.WriteFile(knownHostsPath, []byte(line), 0o600); err != nil {
		return nil, fmt.Errorf("запись known_hosts: %w", err)
	}
	return knownhosts.NewDB(knownHostsPath)
}

func modeSnapshotRelPath(modeID int64) string {
	// git objects are always addressed with "/", not the OS separator.
	return path.Join(modeHistoryModesDir, fmt.Sprintf("%d.json", modeID))
}

func modeSnapshotFullPath(repoPath string, modeID int64) string {
	return filepath.Join(repoPath, modeHistoryModesDir, fmt.Sprintf("%d.json", modeID))
}

// ensureRepo lazily clones/opens the repository and pulls the fresh
// state from origin. The caller already holds s.mu.
func (s *GitModeHistoryStore) ensureRepoLocked(ctx context.Context) (*git.Repository, error) {
	auth, err := s.sshAuth()
	if err != nil {
		return nil, err
	}

	if s.repo != nil {
		s.syncRemoteLocked(ctx, s.repo, auth)
		return s.repo, nil
	}

	if _, statErr := os.Stat(filepath.Join(s.repoPath, ".git")); statErr == nil {
		repo, openErr := git.PlainOpen(s.repoPath)
		if openErr != nil {
			return nil, fmt.Errorf("открытие репозитория истории режимов: %w", openErr)
		}
		s.syncRemoteLocked(ctx, repo, auth)
		s.repo = repo
		return repo, nil
	}

	if err := os.MkdirAll(s.repoPath, 0o755); err != nil {
		return nil, fmt.Errorf("создание каталога репозитория истории режимов: %w", err)
	}
	repo, cloneErr := git.PlainCloneContext(ctx, s.repoPath, false, &git.CloneOptions{
		URL:  s.repoURL,
		Auth: auth,
	})
	if cloneErr != nil {
		if !errors.Is(cloneErr, transport.ErrEmptyRemoteRepository) {
			return nil, fmt.Errorf("клонирование репозитория истории режимов: %w", cloneErr)
		}
		// The GitHub repository exists but is empty (no branches):
		// go-git cannot clone from it. Initialise locally
		// and add origin; the very first commit creates the branch on the remote.
		if existing, openErr := git.PlainOpen(s.repoPath); openErr == nil {
			repo = existing
		} else {
			repo, err = git.PlainInitWithOptions(s.repoPath, &git.PlainInitOptions{
				InitOptions: git.InitOptions{DefaultBranch: plumbing.NewBranchReferenceName(modeHistoryBranch)},
			})
			if err != nil {
				return nil, fmt.Errorf("инициализация репозитория истории режимов: %w", err)
			}
			if _, err := repo.CreateRemote(&config.RemoteConfig{
				Name: "origin",
				URLs: []string{s.repoURL},
				Fetch: []config.RefSpec{
					config.RefSpec("+refs/heads/*:refs/remotes/origin/*"),
				},
			}); err != nil {
				return nil, fmt.Errorf("добавление origin: %w", err)
			}
		}
	}
	s.repo = repo
	return repo, nil
}

// syncRemoteLocked pulls the fresh state of origin/main and hard
// resets the working copy to it, so several API replicas do not
// diverge in history. Network errors (GitHub unreachable) are not fatal:
// we continue with what we have locally, and report it through the returned
// error only in commitSnapshot, not on reads.
func (s *GitModeHistoryStore) syncRemoteLocked(ctx context.Context, repo *git.Repository, auth *gitssh.PublicKeys) {
	fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	err := repo.FetchContext(fetchCtx, &git.FetchOptions{
		RemoteName: "origin",
		Auth:       auth,
		RefSpecs:   []config.RefSpec{config.RefSpec(fmt.Sprintf("+refs/heads/%s:refs/remotes/origin/%s", modeHistoryBranch, modeHistoryBranch))},
	})
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return
	}
	remoteRef, err := repo.Reference(plumbing.NewRemoteReferenceName("origin", modeHistoryBranch), true)
	if err != nil {
		return
	}
	wt, err := repo.Worktree()
	if err != nil {
		return
	}
	_ = wt.Reset(&git.ResetOptions{Commit: remoteRef.Hash(), Mode: git.HardReset})
}

func (s *GitModeHistoryStore) commitSnapshot(ctx context.Context, modeID int64, snap modeSnapshot, authorName, authorEmail, message string) (string, error) {
	if !s.configured() {
		return "", nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	repo, err := s.ensureRepoLocked(ctx)
	if err != nil {
		return "", err
	}

	fullPath := modeSnapshotFullPath(s.repoPath, modeID)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return "", fmt.Errorf("создание каталога modes/: %w", err)
	}
	body, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return "", err
	}
	body = append(body, '\n')
	if err := os.WriteFile(fullPath, body, 0o644); err != nil {
		return "", fmt.Errorf("запись снапшота: %w", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		return "", err
	}
	relPath := modeSnapshotRelPath(modeID)
	if _, err := wt.Add(relPath); err != nil {
		return "", fmt.Errorf("git add: %w", err)
	}
	status, err := wt.Status()
	if err == nil && status.IsClean() {
		// The content did not change: nothing to commit, which is not an error.
		if head, headErr := repo.Head(); headErr == nil {
			return head.Hash().String(), nil
		}
		return "", nil
	}

	if strings.TrimSpace(authorEmail) == "" {
		authorEmail = "admin@example.com"
	}
	if strings.TrimSpace(authorName) == "" {
		authorName = authorEmail
	}
	commitHash, err := wt.Commit(message, &git.CommitOptions{
		Author: &object.Signature{Name: authorName, Email: authorEmail, When: time.Now()},
	})
	if err != nil {
		return "", fmt.Errorf("git commit: %w", err)
	}

	auth, err := s.sshAuth()
	if err != nil {
		return commitHash.String(), err
	}
	pushCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	pushErr := repo.PushContext(pushCtx, &git.PushOptions{
		RemoteName: "origin",
		Auth:       auth,
		RefSpecs: []config.RefSpec{
			config.RefSpec(fmt.Sprintf("refs/heads/%s:refs/heads/%s", modeHistoryBranch, modeHistoryBranch)),
		},
	})
	if pushErr != nil && !errors.Is(pushErr, git.NoErrAlreadyUpToDate) {
		return commitHash.String(), fmt.Errorf("git push: %w", pushErr)
	}
	return commitHash.String(), nil
}

func (s *GitModeHistoryStore) history(ctx context.Context, modeID int64, limit int) ([]modeHistoryEntry, error) {
	if !s.configured() {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	repo, err := s.ensureRepoLocked(ctx)
	if err != nil {
		return nil, err
	}
	head, err := repo.Head()
	if err != nil {
		// An empty repository (no commits for any mode) is a
		// valid "no history yet" state, not an error.
		return nil, nil
	}
	relPath := modeSnapshotRelPath(modeID)
	iter, err := repo.Log(&git.LogOptions{From: head.Hash(), FileName: &relPath, Order: git.LogOrderCommitterTime})
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
	}
	defer iter.Close()

	var out []modeHistoryEntry
	if limit <= 0 {
		limit = 50
	}
	iterErr := iter.ForEach(func(c *object.Commit) error {
		if len(out) >= limit {
			return storer.ErrStop
		}
		out = append(out, modeHistoryEntry{
			SHA:         c.Hash.String(),
			AuthorName:  c.Author.Name,
			AuthorEmail: c.Author.Email,
			CreatedAt:   c.Author.When,
			Message:     c.Message,
		})
		return nil
	})
	if iterErr != nil {
		return nil, fmt.Errorf("чтение git log: %w", iterErr)
	}
	return out, nil
}

func (s *GitModeHistoryStore) snapshotAt(ctx context.Context, modeID int64, sha string) (modeSnapshot, error) {
	if !s.configured() {
		return modeSnapshot{}, errModeHistoryNotConfigured
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	repo, err := s.ensureRepoLocked(ctx)
	if err != nil {
		return modeSnapshot{}, err
	}
	hash := plumbing.NewHash(sha)
	if hash.IsZero() {
		return modeSnapshot{}, fmt.Errorf("некорректный идентификатор версии")
	}
	commit, err := repo.CommitObject(hash)
	if err != nil {
		return modeSnapshot{}, fmt.Errorf("версия не найдена: %w", err)
	}
	relPath := modeSnapshotRelPath(modeID)
	file, err := commit.File(relPath)
	if err != nil {
		return modeSnapshot{}, fmt.Errorf("снапшот не найден в этой версии: %w", err)
	}
	content, err := file.Contents()
	if err != nil {
		return modeSnapshot{}, err
	}
	var snap modeSnapshot
	if err := json.Unmarshal([]byte(content), &snap); err != nil {
		return modeSnapshot{}, fmt.Errorf("повреждённый снапшот: %w", err)
	}
	return snap, nil
}
