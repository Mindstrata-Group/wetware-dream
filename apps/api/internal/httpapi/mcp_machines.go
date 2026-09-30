package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Registry of machines with a reverse SSH tunnel (see migration 20260921_100000).
//
// How it works end to end:
//  1. the installer on the machine generates an ssh key and calls machine.register,
//     sending only the public part;
//  2. the server gives the machine a free port from 22001-22099 and stores the key;
//  3. on the host, sync-tunnel-keys.sh runs once a minute: it rewrites the
//     tunnel user's authorized_keys from this table and also records
//     which ports are actually listening; that is last_seen;
//  4. the machine keeps `ssh -N -R <port>:localhost:22 tunnel@server` running;
//  5. to log into it: from the server, `ssh -p <port> <user>@localhost`.
//
// The port listens on the server's localhost (GatewayPorts is off), so it is not
// exposed: the machine can only be reached by someone who already has access to the server.

// tunnelPortFrom/To is the port range. It is kept narrow on purpose: a hundred machines is
// the ceiling of a home fleet, and a narrow range is easy to check in the sshd config and
// in the firewall if that is ever needed.
const (
	tunnelPortFrom = 22001
	tunnelPortTo   = 22099
)

// machineNamePattern: the name goes into authorized_keys as a comment and into the name of
// the autostart unit. Anything other than a letter, digit, hyphen or dot is rejected.
var machineNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{1,40}$`)

// sshKeyPattern: modern types only. rsa is deliberately not accepted: long
// keys in authorized_keys and weak variants on old clients.
var sshKeyPattern = regexp.MustCompile(`^(ssh-ed25519|ecdsa-sha2-nistp256|ecdsa-sha2-nistp384) [A-Za-z0-9+/=]{20,} ?.*$`)

// sshFingerprint computes the fingerprint the same way as ssh-keygen -lf: SHA256 of the key
// body in base64 without padding. It identifies a reinstall.
func sshFingerprint(publicKey string) (string, error) {
	parts := strings.Fields(strings.TrimSpace(publicKey))
	if len(parts) < 2 {
		return "", errors.New("ключ должен быть вида «тип тело [комментарий]»")
	}
	blob, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", errors.New("тело ключа не разбирается как base64")
	}
	sum := sha256.Sum256(blob)
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:]), nil
}

// mcpMachineRegister gives a machine a port and stores its key.
//
// Re-running the installer on the same machine returns the same port: otherwise
// reinstalls would pile up dead records in the registry, and live ports
// would drift apart from what is written in the machine's autostart.
func (h Handler) mcpMachineRegister(ctx context.Context, req mcpRequest) (map[string]any, int, error) {
	name := strings.TrimSpace(req.Name)
	if !machineNamePattern.MatchString(name) {
		return nil, http.StatusBadRequest, fmt.Errorf("имя машины: латиница, цифры, дефис и точка, 2–41 знак")
	}
	key := strings.Join(strings.Fields(strings.TrimSpace(req.PublicKey)), " ")
	if !sshKeyPattern.MatchString(key) {
		return nil, http.StatusBadRequest, fmt.Errorf("нужен публичный ssh-ключ ed25519 или ecdsa")
	}
	fp, err := sshFingerprint(key)
	if err != nil {
		return nil, http.StatusBadRequest, err
	}

	var port int
	var existing bool
	err = h.DB.QueryRow(ctx, `select port from tunnel_machines where fingerprint = $1 and revoked_at is null`, fp).Scan(&port)
	switch {
	case err == nil:
		existing = true
		// The name and OS may have changed: update them, keep the port and key.
		if _, err := h.DB.Exec(ctx, `
			update tunnel_machines set name = $2, os = $3, note = $4, public_key = $5
			where fingerprint = $1 and revoked_at is null`,
			fp, name, strings.TrimSpace(req.OS), strings.TrimSpace(req.Comment), key); err != nil {
			return nil, http.StatusInternalServerError, err
		}
	case errors.Is(err, pgx.ErrNoRows):
		// Argument types are set explicitly: without ::int postgres cannot
		// pick the generate_series overload and answers "function is not
		// unique", and only when the query runs without a prepared plan.
		// The first free port in the range. generate_series instead of max+1:
		// after a machine is revoked its port must become available again, otherwise
		// reinstalls would exhaust the range.
		err = h.DB.QueryRow(ctx, `
			select p from generate_series($1::int, $2::int) as p
			where not exists (select 1 from tunnel_machines m where m.port = p and m.revoked_at is null)
			order by p limit 1`, tunnelPortFrom, tunnelPortTo).Scan(&port)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, http.StatusConflict, fmt.Errorf("свободных портов не осталось (%d–%d)", tunnelPortFrom, tunnelPortTo)
		}
		if err != nil {
			return nil, http.StatusInternalServerError, err
		}
		if _, err := h.DB.Exec(ctx, `
			insert into tunnel_machines (name, port, public_key, fingerprint, os, note)
			values ($1,$2,$3,$4,$5,$6)`,
			name, port, key, fp, strings.TrimSpace(req.OS), strings.TrimSpace(req.Comment)); err != nil {
			return nil, http.StatusInternalServerError, err
		}
	default:
		return nil, http.StatusInternalServerError, err
	}

	// The administrator's public key is returned together with the port: the installer immediately
	// puts it into the machine's authorized_keys, and login stops depending on a password.
	// Without this step the tunnel exists, but one can only log in manually knowing the password.
	adminKey, _ := h.systemSetting(ctx, tunnelAdminKeySetting)

	return map[string]any{
		"name": name, "port": port, "fingerprint": fp, "alreadyKnown": existing,
		"adminPublicKey": strings.TrimSpace(adminKey),
		// The command is returned ready to use: the installer does not need to know the format, and
		// a human does not need to keep it in their head.
		"tunnelCommand": fmt.Sprintf("ssh -N -T -o ServerAliveInterval=30 -o ServerAliveCountMax=3 -o ExitOnForwardFailure=yes -R %d:localhost:22 tunnel@%%SERVER%%", port),
		"connectHint":   fmt.Sprintf("с сервера: ssh -p %d <пользователь>@localhost", port),
	}, http.StatusOK, nil
}

func (h Handler) mcpMachineList(ctx context.Context, req mcpRequest) (map[string]any, int, error) {
	rows, err := h.DB.Query(ctx, `
		select id, name, port, fingerprint, os, note,
		       created_at::date::text,
		       coalesce(to_char(last_seen_at at time zone 'UTC', 'YYYY-MM-DD HH24:MI'), ''),
		       (last_seen_at is not null and last_seen_at > now() - interval '5 minutes') as online,
		       revoked_at is not null as revoked
		from tunnel_machines
		where ($1 or revoked_at is null)
		order by port`, req.All)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var port int
		var name, fp, os, note, created, lastSeen string
		var online, revoked bool
		if err := rows.Scan(&id, &name, &port, &fp, &os, &note, &created, &lastSeen, &online, &revoked); err != nil {
			return nil, http.StatusInternalServerError, err
		}
		out = append(out, map[string]any{
			"id": id, "name": name, "port": port, "fingerprint": fp, "os": os, "note": note,
			"registered": created, "lastSeen": lastSeen, "online": online, "revoked": revoked,
			"connect": fmt.Sprintf("ssh -p %d <пользователь>@localhost", port),
		})
	}
	return map[string]any{"machines": out}, http.StatusOK, rows.Err()
}

// mcpMachineForget revokes a machine: the key leaves authorized_keys at the
// next sync, the port is freed. The record remains, so one can see that
// the machine existed and when it was removed.
func (h Handler) mcpMachineForget(ctx context.Context, req mcpRequest) (map[string]any, int, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" && req.Port == 0 {
		return nil, http.StatusBadRequest, fmt.Errorf("нужно имя машины или порт")
	}
	tag, err := h.DB.Exec(ctx, `
		update tunnel_machines set revoked_at = now()
		where revoked_at is null and (($1 <> '' and name = $1) or ($2 > 0 and port = $2))`, name, req.Port)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	if tag.RowsAffected() == 0 {
		return nil, http.StatusNotFound, fmt.Errorf("живая машина не найдена")
	}
	return map[string]any{"forgotten": tag.RowsAffected()}, http.StatusOK, nil
}

// mcpMachineSync is a service command for the synchroniser on the host.
//
// It fetches the key list (what to put into authorized_keys) and sends
// the list of ports that are actually listening right now; that is how the registry gets
// an honest "last seen alive" without trusting the machine itself.
func (h Handler) mcpMachineSync(ctx context.Context, req mcpRequest) (map[string]any, int, error) {
	if len(req.Ports) > 0 {
		if _, err := h.DB.Exec(ctx, `
			update tunnel_machines set last_seen_at = now()
			where revoked_at is null and port = any($1::int[])`, req.Ports); err != nil {
			return nil, http.StatusInternalServerError, err
		}
	}
	rows, err := h.DB.Query(ctx, `
		select name, port, public_key from tunnel_machines
		where revoked_at is null order by port`)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	defer rows.Close()
	// authorized_keys lines are built on the server, not in bash on the host:
	// restrictions matter more than convenience, and they must be defined in one place.
	// permitlisten pins the key to its own port so a machine cannot take someone
	// else's; command+restrict remove the shell, agent, X11 and -L.
	lines := []string{}
	machines := []map[string]any{}
	for rows.Next() {
		var name, key string
		var port int
		if err := rows.Scan(&name, &port, &key); err != nil {
			return nil, http.StatusInternalServerError, err
		}
		lines = append(lines, fmt.Sprintf(
			`restrict,port-forwarding,permitlisten="localhost:%d",command="/usr/local/bin/tunnel-shell" %s tunnel-%s`,
			port, key, name))
		machines = append(machines, map[string]any{"name": name, "port": port})
	}
	if err := rows.Err(); err != nil {
		return nil, http.StatusInternalServerError, err
	}
	return map[string]any{"authorizedKeys": strings.Join(lines, "\n"), "machines": machines}, http.StatusOK, nil
}

// tunnelAdminKeySetting is the public key the installer puts on every
// machine. It lives in system_settings, not in code: changing the key should be
// a one-line change, without a rollout.
const tunnelAdminKeySetting = "tunnel_admin_pubkey"

// tunnelJumpHostSetting and tunnelJumpKeySetting hold the jump hub
// ("user@host") and the key used to log in to it. They live in
// system_settings rather than in code: the hub address belongs to a
// particular installation and has no place in the public code.
const (
	tunnelJumpHostSetting = "tunnel_jump_host"
	tunnelJumpKeySetting  = "tunnel_jump_key"
)

// mcpMachineSSHConfig builds a ready-made ~/.ssh/config snippet.
//
// The point: getting to a machine takes two hops (first the server, then the port on its
// localhost). Keeping that in one's head and typing it by hand every time is exactly the
// manual work this was all meant to eliminate. With this config
// `ssh office-pc` is enough, and any MCP is installed over the same channel: `ssh office-pc
// "claude mcp add ..."`.
func (h Handler) mcpMachineSSHConfig(ctx context.Context, req mcpRequest) (map[string]any, int, error) {
	jumpHost := strings.TrimSpace(req.Query)
	if jumpHost == "" {
		jumpHost, _ = h.systemSetting(ctx, tunnelJumpHostSetting)
		jumpHost = strings.TrimSpace(jumpHost)
	}
	if jumpHost == "" {
		return nil, http.StatusBadRequest, fmt.Errorf("сервер-хаб не задан: передайте query=user@host или заполните system_settings.%s", tunnelJumpHostSetting)
	}
	rows, err := h.DB.Query(ctx, `
		select name, port, os from tunnel_machines
		where revoked_at is null order by port`)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	defer rows.Close()

	// The key for the jump is separate from the machines' key: the server is entered with the account
	// you already use. Without an explicit block for the hub, ssh tried to
	// log into the server with the machines' key and got "Permission denied".
	jumpKey := strings.TrimSpace(req.Comment)
	if jumpKey == "" {
		jumpKey, _ = h.systemSetting(ctx, tunnelJumpKeySetting)
		jumpKey = strings.TrimSpace(jumpKey)
	}
	if jumpKey == "" {
		jumpKey = "~/.ssh/id_ed25519"
	}
	jumpUser, jumpAddr := "", jumpHost
	if at := strings.Index(jumpHost, "@"); at > 0 {
		jumpUser, jumpAddr = jumpHost[:at], jumpHost[at+1:]
	}

	var b strings.Builder
	b.WriteString("# Машины контура Mindstrata. Блок создан командой machine.sshconfig.\n")
	b.WriteString("# Вставьте в ~/.ssh/config — после этого «ssh <имя>» заходит на машину в один шаг.\n\n")
	fmt.Fprintf(&b, "Host mindstrata-hub\n")
	fmt.Fprintf(&b, "    HostName %s\n", jumpAddr)
	if jumpUser != "" {
		fmt.Fprintf(&b, "    User %s\n", jumpUser)
	}
	fmt.Fprintf(&b, "    IdentityFile %s\n", jumpKey)
	fmt.Fprintf(&b, "    IdentitiesOnly yes\n\n")
	count := 0
	for rows.Next() {
		var name, os string
		var port int
		if err := rows.Scan(&name, &port, &os); err != nil {
			return nil, http.StatusInternalServerError, err
		}
		user := "user"
		if strings.HasPrefix(strings.ToLower(os), "windows") {
			user = "User"
		}
		fmt.Fprintf(&b, "Host %s\n", name)
		fmt.Fprintf(&b, "    HostName localhost\n")
		fmt.Fprintf(&b, "    Port %d\n", port)
		fmt.Fprintf(&b, "    User %s\n", user)
		fmt.Fprintf(&b, "    ProxyJump mindstrata-hub\n")
		fmt.Fprintf(&b, "    IdentityFile ~/.ssh/mindstrata_admin\n")
		fmt.Fprintf(&b, "    IdentitiesOnly yes\n")
		fmt.Fprintf(&b, "    StrictHostKeyChecking accept-new\n\n")
		count++
	}
	if err := rows.Err(); err != nil {
		return nil, http.StatusInternalServerError, err
	}
	return map[string]any{"config": b.String(), "machines": count}, http.StatusOK, nil
}
