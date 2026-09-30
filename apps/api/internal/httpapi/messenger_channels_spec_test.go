package httpapi

import "testing"

// Acceptance-criteria stubs from docs/specs/messenger-channels.md — the part
// that is checked without a database. The feature is not implemented: every
// test skips with the SPEC prefix so CI keeps showing accepted but open
// criteria. Implementation starts by removing t.Skip and writing code until
// the test passes.
//
// Database/HTTP criteria live in messenger_channels_spec_integration_test.go.

// specMessengerChannelsSkip keeps the skip message uniform so grep and the
// CI "Spec backlog" step can find open criteria.
func specMessengerChannelsSkip(t *testing.T, ac string) {
	t.Helper()
	t.Skip("SPEC messenger-channels " + ac + ": not implemented")
}

// AC-6: relay and proxy addresses are validated on save.
//
// Table cases for the channel address validator:
//   - relay_url/api_base_url not https → error;
//   - proxy_url scheme outside http/https/socks5/socks5h → error;
//   - 127.0.0.1, ::1, localhost, 169.254.169.254, 0.0.0.0, 224.0.0.1 → error;
//   - 10.0.0.5 → error unless the host is listed in MESSENGER_PRIVATE_HOSTS_ALLOW;
//   - https://relay.example/secret-path and socks5h://user:pass@proxy.example:1080 → ok.
func TestSpecMessengerChannels_AC06_AddressValidationOnSave(t *testing.T) {
	t.Parallel()
	specMessengerChannelsSkip(t, "AC-6")
}

// AC-7: the check is repeated at dial time. A name that resolved to a public
// address on save but now resolves to 10.0.0.1 or 169.254.169.254 is refused
// by the channel transport's dial control; the request never reaches it
// (fake resolver, no network).
func TestSpecMessengerChannels_AC07_DialTimeAddressCheck(t *testing.T) {
	t.Parallel()
	specMessengerChannelsSkip(t, "AC-7")
}

// AC-14: neither send errors nor logs contain secrets. Sending through an
// unreachable address with token "123456:SECRET-token" and a proxy_url with
// password "p4ss" returns an error; err.Error() and the captured log contain
// neither "SECRET-token", nor "p4ss", nor the "/bot123456:" path. Today this
// is violated: *url.Error carries the full URL with the token and
// lead_notifications.go logs it.
func TestSpecMessengerChannels_AC14_SecretsNeverInErrorsOrLogs(t *testing.T) {
	t.Parallel()
	specMessengerChannelsSkip(t, "AC-14")
}
