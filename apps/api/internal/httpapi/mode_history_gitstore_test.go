package httpapi

// mode_history_gitstore_test.go: a unit test of the real go-git store
// (GitModeHistoryStore) without Postgres and without real SSH/GitHub: "origin"
// is a local bare repository on disk (go-git handles the file:// transport for
// local paths itself), the deploy key is a one-off ed25519 key generated in the
// test (used only for go-git's SSH object; the file transport does not check
// authentication, and it is not needed for local paths).

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"net"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	xssh "golang.org/x/crypto/ssh"
)

// initBareOriginWithMainHead creates a bare repository with HEAD pointing at
// modeHistoryBranch ("main"), as modern GitHub does even for a freshly created
// empty repository (unlike go-git PlainInit by default, whose HEAD -> master).
func initBareOriginWithMainHead(t *testing.T, dir string) {
	t.Helper()
	if _, err := git.PlainInitWithOptions(dir, &git.PlainInitOptions{
		Bare:        true,
		InitOptions: git.InitOptions{DefaultBranch: plumbing.NewBranchReferenceName(modeHistoryBranch)},
	}); err != nil {
		t.Fatalf("init bare origin: %v", err)
	}
}

func testDeployKeyPEM(t *testing.T) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}
	block, err := xssh.MarshalPrivateKey(priv, "mode-history-test")
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	return string(pem.EncodeToMemory(block))
}

func newTestGitModeHistoryStore(t *testing.T) *GitModeHistoryStore {
	t.Helper()
	bareDir := filepath.Join(t.TempDir(), "origin.git")
	initBareOriginWithMainHead(t, bareDir)
	workDir := filepath.Join(t.TempDir(), "work")
	return NewGitModeHistoryStore(workDir, bareDir, testDeployKeyPEM(t))
}

// TestGitModeHistoryStore_NoRepoURLMeansOff: there is no default repository.
// An empty URL used to be replaced with our own repository, so another
// installation with a deploy key would try to push its prompt history to us.
func TestGitModeHistoryStore_NoRepoURLMeansOff(t *testing.T) {
	t.Parallel()
	store := NewGitModeHistoryStore(t.TempDir(), "  ", "any-key")
	if store.configured() {
		t.Fatalf("without a repository URL the feature must be off")
	}
	if store.repoURL != "  " {
		t.Fatalf("repository URL replaced with a default: %q", store.repoURL)
	}
}

func TestGitModeHistoryStore_CommitHistorySnapshotRoundtrip(t *testing.T) {
	ctx := context.Background()
	store := newTestGitModeHistoryStore(t)
	if !store.configured() {
		t.Fatalf("store must report configured() == true with a deploy key set")
	}

	snap1 := modeSnapshot{Name: "Режим", Prompt: "Промпт v1", WelcomeMessage: "Привет", Criteria: "крит1"}
	sha1, err := store.commitSnapshot(ctx, 42, snap1, "Admin One", "admin1@test.local", "режим 42: создание")
	if err != nil {
		t.Fatalf("commitSnapshot #1: %v", err)
	}
	if sha1 == "" {
		t.Fatalf("commitSnapshot #1: empty sha")
	}

	snap2 := modeSnapshot{Name: "Режим", Prompt: "Промпт v2", WelcomeMessage: "Привет", Criteria: "крит1"}
	sha2, err := store.commitSnapshot(ctx, 42, snap2, "Admin Two", "admin2@test.local", "режим 42: правка")
	if err != nil {
		t.Fatalf("commitSnapshot #2: %v", err)
	}
	if sha2 == "" || sha2 == sha1 {
		t.Fatalf("commitSnapshot #2: expected a new sha, got %q (prev %q)", sha2, sha1)
	}

	// An identical snapshot again: nothing to commit, the sha does not change.
	sha2Again, err := store.commitSnapshot(ctx, 42, snap2, "Admin Two", "admin2@test.local", "no-op")
	if err != nil {
		t.Fatalf("commitSnapshot (no-op): %v", err)
	}
	if sha2Again != sha2 {
		t.Fatalf("identical snapshot must not create a new commit: got %q want %q", sha2Again, sha2)
	}

	entries, err := store.history(ctx, 42, 50)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 versions, got %d (%+v)", len(entries), entries)
	}
	// Newest first.
	if entries[0].SHA != sha2 || entries[1].SHA != sha1 {
		t.Fatalf("history order: got [%s, %s], want [%s, %s]", entries[0].SHA, entries[1].SHA, sha2, sha1)
	}
	if entries[0].AuthorEmail != "admin2@test.local" || entries[1].AuthorEmail != "admin1@test.local" {
		t.Fatalf("history authors mismatch: %+v", entries)
	}

	restored1, err := store.snapshotAt(ctx, 42, sha1)
	if err != nil {
		t.Fatalf("snapshotAt sha1: %v", err)
	}
	if restored1 != snap1 {
		t.Fatalf("snapshotAt sha1: got %+v want %+v", restored1, snap1)
	}
	restored2, err := store.snapshotAt(ctx, 42, sha2)
	if err != nil {
		t.Fatalf("snapshotAt sha2: %v", err)
	}
	if restored2 != snap2 {
		t.Fatalf("snapshotAt sha2: got %+v want %+v", restored2, snap2)
	}
}

// TestGitModeHistoryStore_MultiReplicaSync: two API replicas (two different local
// working directories) write to the same remote repository: the second replica
// must see the first one's commit before its own write and not lose history
// (not diverge).
func TestGitModeHistoryStore_MultiReplicaSync(t *testing.T) {
	ctx := context.Background()
	bareDir := filepath.Join(t.TempDir(), "origin.git")
	initBareOriginWithMainHead(t, bareDir)
	deployKey := testDeployKeyPEM(t)

	replicaA := NewGitModeHistoryStore(filepath.Join(t.TempDir(), "work-a"), bareDir, deployKey)
	replicaB := NewGitModeHistoryStore(filepath.Join(t.TempDir(), "work-b"), bareDir, deployKey)

	shaA, err := replicaA.commitSnapshot(ctx, 7, modeSnapshot{Prompt: "от реплики A"}, "A", "a@test.local", "commit A")
	if err != nil {
		t.Fatalf("replica A commit: %v", err)
	}

	shaB, err := replicaB.commitSnapshot(ctx, 7, modeSnapshot{Prompt: "от реплики B"}, "B", "b@test.local", "commit B")
	if err != nil {
		t.Fatalf("replica B commit: %v", err)
	}

	// Replica B must see both commits (A is pulled before B writes).
	entries, err := replicaB.history(ctx, 7, 50)
	if err != nil {
		t.Fatalf("replica B history: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("replica B must see both commits after sync, got %d (%+v)", len(entries), entries)
	}
	if entries[0].SHA != shaB || entries[1].SHA != shaA {
		t.Fatalf("replica B history order: got [%s, %s], want [%s, %s]", entries[0].SHA, entries[1].SHA, shaB, shaA)
	}
}

// TestGitModeHistoryStore_HostKeyAlgorithmsForced: a regression test for the prod
// bug of 2026-07-04: ssh.FixedHostKey by itself does not force host-key
// algorithm negotiation (go-git copies into ClientConfig only the explicitly set
// field PublicKeys.HostKeyAlgorithms), so the server could negotiate RSA/ECDSA
// instead of ed25519, and the comparison with the hard-coded ed25519 key wrongly
// failed as "host key mismatch". We check that sshAuth() publishes
// HostKeyAlgorithms=["ssh-ed25519"] and that the callback accepts exactly the
// hard-coded github.com key for the right host.
func TestGitModeHistoryStore_HostKeyAlgorithmsForced(t *testing.T) {
	store := newTestGitModeHistoryStore(t)

	auth, err := store.sshAuth()
	if err != nil {
		t.Fatalf("sshAuth: %v", err)
	}
	if len(auth.HostKeyAlgorithms) == 0 {
		t.Fatalf("HostKeyAlgorithms must be explicitly set (иначе GO не форсирует negotiation на ed25519)")
	}
	for _, algo := range auth.HostKeyAlgorithms {
		if algo != xssh.KeyAlgoED25519 {
			t.Fatalf("HostKeyAlgorithms must only offer ed25519, got %q (полный список %v)", algo, auth.HostKeyAlgorithms)
		}
	}
	if auth.HostKeyCallback == nil {
		t.Fatalf("HostKeyCallback must be set")
	}

	// The callback must accept exactly the hard-coded github.com key...
	githubKey, _, _, _, err := xssh.ParseAuthorizedKey([]byte(githubSSHHostKey))
	if err != nil {
		t.Fatalf("parse github key: %v", err)
	}
	addr := &net.TCPAddr{IP: net.ParseIP("140.82.121.3"), Port: 22}
	if err := auth.HostKeyCallback(githubSSHHostWithPort, addr, githubKey); err != nil {
		t.Fatalf("callback must accept the exact hardcoded github.com ed25519 key: %v", err)
	}

	// ...and reject any other key (including one of another type) for the same host.
	_, otherKey, err := generateTestEd25519SSHKey(t)
	if err != nil {
		t.Fatalf("generate other key: %v", err)
	}
	if err := auth.HostKeyCallback(githubSSHHostWithPort, addr, otherKey); err == nil {
		t.Fatalf("callback must reject a key that is not the hardcoded github.com key")
	}
}

func generateTestEd25519SSHKey(t *testing.T) (ed25519.PrivateKey, xssh.PublicKey, error) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	sshPub, err := xssh.NewPublicKey(pub)
	if err != nil {
		return nil, nil, err
	}
	return priv, sshPub, nil
}
