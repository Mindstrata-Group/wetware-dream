//go:build integration

package httpapi

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

// testPubKey makes a syntactically valid ed25519 key: the body must be real
// base64, otherwise the fingerprint cannot be computed, and the server checks it.
func testPubKey(seed string) string {
	blob := []byte(fmt.Sprintf("%-51s", "ssh-ed25519-test-"+seed))
	return "ssh-ed25519 " + base64.StdEncoding.EncodeToString(blob) + " user@host"
}

// TestMachines_RegisterIsIdempotent: re-running the installer on the same
// machine must return the same port. Otherwise dead records would pile up in the
// registry after reinstalls, and the machine's autostart would keep the old port.
func TestMachines_RegisterIsIdempotent(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	setMCPKey(t, env, "mch-1")

	key := testPubKey("AAAA")
	body := map[string]any{"action": "machine.register", "name": "office-pc", "publicKey": key, "os": "windows"}

	code, first := mcpCall(t, ts, "mch-1", body)
	if code != http.StatusOK {
		t.Fatalf("регистрация: %d %+v", code, first)
	}
	port := first["port"]
	if port == nil || port.(float64) < 22001 || port.(float64) > 22099 {
		t.Fatalf("порт вне диапазона: %+v", first)
	}
	if first["alreadyKnown"] != false {
		t.Fatalf("первая регистрация помечена как повторная: %+v", first)
	}

	code, second := mcpCall(t, ts, "mch-1", body)
	if code != http.StatusOK {
		t.Fatalf("повторная регистрация: %d %+v", code, second)
	}
	if second["port"] != port {
		t.Fatalf("порт сменился: было %v, стало %v", port, second["port"])
	}
	if second["alreadyKnown"] != true {
		t.Fatalf("повторная регистрация не распознана: %+v", second)
	}

	var rows int
	if err := env.Pool.QueryRow(context.Background(), `select count(*) from tunnel_machines`).Scan(&rows); err != nil {
		t.Fatalf("чтение реестра: %v", err)
	}
	if rows != 1 {
		t.Fatalf("в реестре %d строк, want 1 — повторная установка наплодила записей", rows)
	}
}

// TestMachines_PortsAreUniqueAndReused: different machines get different ports,
// and a revoked machine's port goes back into circulation; otherwise reinstalls
// would exhaust the range.
func TestMachines_PortsAreUniqueAndReused(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	setMCPKey(t, env, "mch-2")

	ports := map[float64]bool{}
	for i := 0; i < 3; i++ {
		code, resp := mcpCall(t, ts, "mch-2", map[string]any{
			"action": "machine.register", "name": fmt.Sprintf("host-%d", i), "publicKey": testPubKey(fmt.Sprintf("BB%02d", i)),
		})
		if code != http.StatusOK {
			t.Fatalf("регистрация %d: %d %+v", i, code, resp)
		}
		p := resp["port"].(float64)
		if ports[p] {
			t.Fatalf("порт %v выдан дважды", p)
		}
		ports[p] = true
	}

	// Revoke the middle machine: its port must go to the next one.
	code, resp := mcpCall(t, ts, "mch-2", map[string]any{"action": "machine.forget", "name": "host-1"})
	if code != http.StatusOK {
		t.Fatalf("отзыв: %d %+v", code, resp)
	}
	code, resp = mcpCall(t, ts, "mch-2", map[string]any{
		"action": "machine.register", "name": "host-new", "publicKey": testPubKey("CCCC"),
	})
	if code != http.StatusOK {
		t.Fatalf("регистрация после отзыва: %d %+v", code, resp)
	}
	if got := resp["port"].(float64); got != 22002 {
		t.Fatalf("освободившийся порт не переиспользован: выдан %v, ждали 22002", got)
	}
}

// TestMachines_RejectsBadInput: the name goes into authorized_keys and the unit
// name, and the key straight into the file that decides who gets in. Garbage
// must not get there at all.
func TestMachines_RejectsBadInput(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	setMCPKey(t, env, "mch-3")

	cases := []struct {
		name string
		body map[string]any
	}{
		{"имя с пробелом и кавычкой", map[string]any{"action": "machine.register", "name": `bad" name`, "publicKey": testPubKey("DD")}},
		{"имя пустое", map[string]any{"action": "machine.register", "name": "", "publicKey": testPubKey("DD")}},
		{"ключ не ssh", map[string]any{"action": "machine.register", "name": "ok-host", "publicKey": "просто текст"}},
		{"ключ rsa не принимаем", map[string]any{"action": "machine.register", "name": "ok-host", "publicKey": "ssh-rsa AAAAB3Nza user@host"}},
		{"перевод строки в ключе", map[string]any{"action": "machine.register", "name": "ok-host", "publicKey": "ssh-ed25519 AAAA\nssh-ed25519 BBBB"}},
	}
	for _, c := range cases {
		if code, resp := mcpCall(t, ts, "mch-3", c.body); code != http.StatusBadRequest {
			t.Fatalf("%s: код %d, want 400 (%+v)", c.name, code, resp)
		}
	}
	var rows int
	if err := env.Pool.QueryRow(context.Background(), `select count(*) from tunnel_machines`).Scan(&rows); err != nil {
		t.Fatalf("чтение: %v", err)
	}
	if rows != 0 {
		t.Fatalf("мусор попал в реестр: %d строк", rows)
	}
}

// TestMachines_SyncBuildsRestrictedKeys: the main security check.
// The authorized_keys line must pin the key to its port and take away the
// shell; otherwise a machine could grab someone else's port or get a shell.
func TestMachines_SyncBuildsRestrictedKeys(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	setMCPKey(t, env, "mch-4")

	code, reg := mcpCall(t, ts, "mch-4", map[string]any{
		"action": "machine.register", "name": "nas", "publicKey": testPubKey("EEEE"), "os": "linux",
	})
	if code != http.StatusOK {
		t.Fatalf("регистрация: %d %+v", code, reg)
	}
	port := int(reg["port"].(float64))

	code, resp := mcpCall(t, ts, "mch-4", map[string]any{"action": "machine.sync", "ports": []int{port}})
	if code != http.StatusOK {
		t.Fatalf("синхронизация: %d %+v", code, resp)
	}
	keys, _ := resp["authorizedKeys"].(string)
	for _, want := range []string{
		"restrict",
		fmt.Sprintf(`permitlisten="localhost:%d"`, port),
		`command="/usr/local/bin/tunnel-shell"`,
		"ssh-ed25519 ",
		"tunnel-nas",
	} {
		if !strings.Contains(keys, want) {
			t.Fatalf("в строке ключа нет %q:\n%s", want, keys)
		}
	}

	// The port came up alive: the machine must count as online.
	code, list := mcpCall(t, ts, "mch-4", map[string]any{"action": "machine.list"})
	if code != http.StatusOK {
		t.Fatalf("список: %d %+v", code, list)
	}
	machines, _ := list["machines"].([]any)
	if len(machines) != 1 {
		t.Fatalf("машин в списке %d, want 1", len(machines))
	}
	m, _ := machines[0].(map[string]any)
	if m["online"] != true {
		t.Fatalf("машина с живым портом не отмечена как на связи: %+v", m)
	}

	// After revocation the key must disappear from the synchroniser's output.
	if code, _ := mcpCall(t, ts, "mch-4", map[string]any{"action": "machine.forget", "port": port}); code != http.StatusOK {
		t.Fatalf("отзыв: %d", code)
	}
	_, resp = mcpCall(t, ts, "mch-4", map[string]any{"action": "machine.sync"})
	if keys, _ := resp["authorizedKeys"].(string); strings.TrimSpace(keys) != "" {
		t.Fatalf("ключ отозванной машины остался в выдаче:\n%s", keys)
	}
}

// TestMachines_ForgetNeedsTarget: revoking without a target must not wipe the registry.
func TestMachines_ForgetNeedsTarget(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	setMCPKey(t, env, "mch-5")

	if code, resp := mcpCall(t, ts, "mch-5", map[string]any{
		"action": "machine.register", "name": "laptop", "publicKey": testPubKey("FFFF"),
	}); code != http.StatusOK {
		t.Fatalf("регистрация: %d %+v", code, resp)
	}
	if code, _ := mcpCall(t, ts, "mch-5", map[string]any{"action": "machine.forget"}); code != http.StatusBadRequest {
		t.Fatalf("отзыв без цели: %d, want 400", code)
	}
	if code, _ := mcpCall(t, ts, "mch-5", map[string]any{"action": "machine.forget", "name": "нет-такой"}); code != http.StatusNotFound {
		t.Fatalf("отзыв несуществующей: %d, want 404", code)
	}
	var live int
	if err := env.Pool.QueryRow(context.Background(),
		`select count(*) from tunnel_machines where revoked_at is null`).Scan(&live); err != nil {
		t.Fatalf("чтение: %v", err)
	}
	if live != 1 {
		t.Fatalf("живых машин %d, want 1 — неудачный отзыв задел реестр", live)
	}
}

// TestMachines_AdminKeyAndSSHConfig: the installer must receive the admin's
// public key (otherwise logging into the machine stays password-based, i.e.
// manual), and the ssh config must contain the jump through the server;
// otherwise the machine cannot be reached with one command.
func TestMachines_AdminKeyAndSSHConfig(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	setMCPKey(t, env, "mch-6")

	const adminKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIHPQ7EFsWs3eMMfi16Nd3T2KHR0ml4HEaOtEkTeaJbAL admin"
	if _, err := env.Pool.Exec(context.Background(),
		`insert into system_settings (key, value) values ('tunnel_admin_pubkey', $1)
		 on conflict (key) do update set value = excluded.value`, adminKey); err != nil {
		t.Fatalf("ключ администратора: %v", err)
	}

	code, resp := mcpCall(t, ts, "mch-6", map[string]any{
		"action": "machine.register", "name": "office-pc", "publicKey": testPubKey("GGGG"), "os": "windows",
	})
	if code != http.StatusOK {
		t.Fatalf("регистрация: %d %+v", code, resp)
	}
	if resp["adminPublicKey"] != adminKey {
		t.Fatalf("установщику не отдали ключ администратора: %+v", resp["adminPublicKey"])
	}

	// Without a hub there is no config: a clear refusal instead of silently
	// substituting somebody else's address.
	if code, resp := mcpCall(t, ts, "mch-6", map[string]any{"action": "machine.sshconfig"}); code != http.StatusBadRequest {
		t.Fatalf("no hub: want 400, got %d %+v", code, resp)
	}
	if _, err := env.Pool.Exec(context.Background(),
		`insert into system_settings (key, value) values ('tunnel_jump_host', 'hubuser@hub.example'), ('tunnel_jump_key', '~/.ssh/id_hub')
		 on conflict (key) do update set value = excluded.value`); err != nil {
		t.Fatalf("hub settings: %v", err)
	}

	code, cfg := mcpCall(t, ts, "mch-6", map[string]any{"action": "machine.sshconfig"})
	if code != http.StatusOK {
		t.Fatalf("ssh-конфиг: %d %+v", code, cfg)
	}
	text, _ := cfg["config"].(string)
	port := int(resp["port"].(float64))
	for _, want := range []string{
		"Host mindstrata-hub",        // without a separate block for the server the jump goes
		"IdentityFile ~/.ssh/id_hub", // with the machines' key and gets refused
		"HostName hub.example",
		"User hubuser",
		"Host office-pc",
		fmt.Sprintf("Port %d", port),
		"ProxyJump mindstrata-hub",
		"IdentityFile ~/.ssh/mindstrata_admin",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("в конфиге нет %q:\n%s", want, text)
		}
	}
	if cfg["machines"] != float64(1) {
		t.Fatalf("в конфиге %v машин, want 1", cfg["machines"])
	}

	// A revoked machine must not stay in the config: otherwise "ssh office-pc" would
	// lead to a port that already belongs to another machine.
	if code, _ := mcpCall(t, ts, "mch-6", map[string]any{"action": "machine.forget", "name": "office-pc"}); code != http.StatusOK {
		t.Fatalf("отзыв не прошёл")
	}
	_, cfg = mcpCall(t, ts, "mch-6", map[string]any{"action": "machine.sshconfig"})
	if text, _ := cfg["config"].(string); strings.Contains(text, "Host office-pc") {
		t.Fatalf("отозванная машина осталась в ssh-конфиге:\n%s", text)
	}
}
