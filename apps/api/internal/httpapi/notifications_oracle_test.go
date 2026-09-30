package httpapi

// Oracle tests for notification channels: they check INVARIANTS of pure functions,
// not specific examples: partition properties, idempotence, monotonicity,
// bounds. Exact equalities at the bounds are deliberately strict: they kill
// mutants like `<` → `<=`, `+1` → `-1`, `&&` → `||`.

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

func TestOracle_NormalizeDirectChannels(t *testing.T) {
	t.Parallel()
	// Idempotence: normalize(normalize(x)) == normalize(x).
	inputs := [][]string{
		nil,
		{},
		{"inbox"},
		{"max", "telegram", "inbox"},
		{"MAX", " Telegram "},
		{"max", "max", "max"},
		{"sms", "email", "carrier-pigeon"},
		{"telegram", "junk", "inbox", "telegram"},
	}
	valid := map[string]bool{"inbox": true, "max": true, "telegram": true}
	for _, in := range inputs {
		once := normalizeDirectChannels(in)
		twice := normalizeDirectChannels(once)
		if strings.Join(once, ",") != strings.Join(twice, ",") {
			t.Fatalf("не идемпотентно: %v → %v → %v", in, once, twice)
		}
		if len(once) == 0 {
			t.Fatalf("normalize(%v) пуст — должен быть fallback [inbox]", in)
		}
		seen := map[string]bool{}
		for _, ch := range once {
			if !valid[ch] {
				t.Fatalf("normalize(%v) содержит недопустимый канал %q", in, ch)
			}
			if seen[ch] {
				t.Fatalf("normalize(%v) содержит дубликат %q", in, ch)
			}
			seen[ch] = true
		}
	}
	// Empty and garbage → exactly [inbox].
	if got := normalizeDirectChannels(nil); len(got) != 1 || got[0] != "inbox" {
		t.Fatalf("normalize(nil)=%v, want [inbox]", got)
	}
	if got := normalizeDirectChannels([]string{"sms"}); len(got) != 1 || got[0] != "inbox" {
		t.Fatalf("normalize([sms])=%v, want [inbox]", got)
	}
}

func TestOracle_ChannelTextLimit(t *testing.T) {
	t.Parallel()
	// Limit = the minimum over the selected channels; adding a channel cannot raise it.
	cases := []struct {
		channels []string
		want     int
	}{
		{[]string{"inbox"}, inboxTextLimit},
		{[]string{"telegram"}, telegramTextLimit},
		{[]string{"max"}, maxTextLimit},
		{[]string{"inbox", "telegram"}, telegramTextLimit},
		{[]string{"telegram", "max"}, maxTextLimit},
		{[]string{"inbox", "max", "telegram"}, maxTextLimit},
	}
	for _, c := range cases {
		if got := channelTextLimit(c.channels); got != c.want {
			t.Fatalf("channelTextLimit(%v)=%d, want %d", c.channels, got, c.want)
		}
	}
	// Monotonicity: ∀ subset ⊆ set → limit(subset) ≥ limit(set).
	all := []string{"inbox", "max", "telegram"}
	full := channelTextLimit(all)
	for i := range all {
		subset := append([]string{}, all[:i+1]...)
		if channelTextLimit(subset) < full {
			t.Fatalf("монотонность нарушена: limit(%v) < limit(%v)", subset, all)
		}
	}
}

func makeRecipients() []adminNotificationRecipient {
	chat := int64(1)
	tg := int64(2)
	return []adminNotificationRecipient{
		{ID: 1},
		{ID: 2, MaxLinked: true, maxChatID: &chat},
		{ID: 3, TelegramLinked: true, telegramID: &tg},
		{ID: 4, MaxLinked: true, TelegramLinked: true, maxChatID: &chat, telegramID: &tg},
	}
}

func TestOracle_FilterRecipientsByChannels(t *testing.T) {
	t.Parallel()
	recipients := makeRecipients()

	// Invariant 1: inbox among channels → the filter is the identity.
	got := filterRecipientsByChannels(recipients, []string{"inbox", "max"})
	if len(got) != len(recipients) {
		t.Fatalf("inbox не должен сужать: got %d, want %d", len(got), len(recipients))
	}

	// Invariant 2: without inbox every remaining user is reachable by at least one
	// channel, and nobody reachable is lost (completeness + correctness).
	cases := []struct {
		channels []string
		wantIDs  []int64
	}{
		{[]string{"max"}, []int64{2, 4}},
		{[]string{"telegram"}, []int64{3, 4}},
		{[]string{"max", "telegram"}, []int64{2, 3, 4}},
	}
	for _, c := range cases {
		got := filterRecipientsByChannels(recipients, c.channels)
		if len(got) != len(c.wantIDs) {
			t.Fatalf("filter(%v): %d получателей, want %d", c.channels, len(got), len(c.wantIDs))
		}
		for i, rec := range got {
			if rec.ID != c.wantIDs[i] {
				t.Fatalf("filter(%v)[%d]=%d, want %d (порядок должен сохраняться)", c.channels, i, rec.ID, c.wantIDs[i])
			}
		}
	}
}

func TestOracle_ExcludeRecipients(t *testing.T) {
	t.Parallel()
	recipients := makeRecipients()

	// Empty exclude: identity (the same slice, no copies).
	if got := excludeRecipients(recipients, nil); len(got) != len(recipients) {
		t.Fatalf("пустой exclude сузил список")
	}

	// The result does not intersect exclude; non-excluded items are kept in order.
	got := excludeRecipients(recipients, []int64{2, 5, 999})
	wantIDs := []int64{1, 3, 4}
	if len(got) != len(wantIDs) {
		t.Fatalf("exclude: %d получателей, want %d", len(got), len(wantIDs))
	}
	for i, rec := range got {
		if rec.ID != wantIDs[i] {
			t.Fatalf("exclude[%d]=%d, want %d", i, rec.ID, wantIDs[i])
		}
	}

	// Excluding everyone → empty.
	if got := excludeRecipients(recipients, []int64{1, 2, 3, 4, 5}); len(got) != 0 {
		t.Fatalf("исключение всех оставило %d", len(got))
	}
}

func TestOracle_MaxBonusMessages_Boundaries(t *testing.T) {
	t.Parallel()
	// Formula: ceil(limit * 0.1), minimum 2. Bounds around the min threshold and ceil.
	cases := map[int64]int64{
		0:   2,
		1:   2,
		10:  2, // ceil(1.0)=1 → clamp 2
		19:  2, // ceil(1.9)=2
		20:  2,
		21:  3, // ceil(2.1)=3: kills the floor/ceil mutant
		29:  3,
		30:  3,
		31:  4,
		61:  7, // the case from the "+70" incident: 10% of 61 = 7, not 7×10 grants
		100: 10,
	}
	for limit, want := range cases {
		if got := maxBonusMessages(limit); got != want {
			t.Fatalf("maxBonusMessages(%d)=%d, want %d", limit, got, want)
		}
	}
	// Monotonicity and the lower bound over a range.
	prev := int64(0)
	for limit := int64(0); limit <= 500; limit++ {
		got := maxBonusMessages(limit)
		if got < 2 {
			t.Fatalf("maxBonusMessages(%d)=%d < 2", limit, got)
		}
		if got < prev {
			t.Fatalf("немонотонно: f(%d)=%d < f(%d)=%d", limit, got, limit-1, prev)
		}
		prev = got
	}
}

func TestOracle_MarkdownToTelegramHTML_Invariants(t *testing.T) {
	t.Parallel()
	inputs := []string{
		"**Жирный** текст",
		"[ссылка](https://a.b/?x=1&y=2) и **ещё**",
		"<script>alert('xss')</script> & <b>сырой</b>",
		"без разметки вообще",
		"**незакрытый жирный",
		"[битая ссылка](без закрытия",
		"a & b < c > d",
	}
	for _, in := range inputs {
		out := markdownToTelegramHTML(in)
		// Invariant 1: CommonMark bold does not leak into the output.
		if strings.Contains(out, "**") {
			t.Fatalf("вывод содержит '**': %q → %q", in, out)
		}
		// Invariant 2: after removing OUR tags not a single raw < or > remains: all
		// user HTML is escaped.
		stripped := out
		stripped = strings.ReplaceAll(stripped, "<b>", "")
		stripped = strings.ReplaceAll(stripped, "</b>", "")
		stripped = strings.ReplaceAll(stripped, "</a>", "")
		for {
			start := strings.Index(stripped, `<a href="`)
			if start < 0 {
				break
			}
			end := strings.Index(stripped[start:], ">")
			if end < 0 {
				t.Fatalf("незакрытый <a href в выводе: %q", out)
			}
			stripped = stripped[:start] + stripped[start+end+1:]
		}
		if strings.ContainsAny(stripped, "<>") {
			t.Fatalf("неэкранированный HTML в выводе: %q → %q (остаток %q)", in, out, stripped)
		}
	}
	// Spot checks of the conversion.
	if got := markdownToTelegramHTML("**x**"); got != "<b>x</b>" {
		t.Fatalf("bold: %q", got)
	}
	if got := markdownToTelegramHTML("[t](https://u.v)"); got != `<a href="https://u.v">t</a>` {
		t.Fatalf("link: %q", got)
	}
	if got := markdownToTelegramHTML("<i>&</i>"); got != "&lt;i&gt;&amp;&lt;/i&gt;" {
		t.Fatalf("escape: %q", got)
	}
}

func TestOracle_StripInlineMarkdown(t *testing.T) {
	t.Parallel()
	inputs := []string{
		"**Жирный** и [ссылка](https://a.b)",
		"обычный текст",
		"**a** **b** [c](d) [e](f)",
	}
	for _, in := range inputs {
		out := stripInlineMarkdown(in)
		if strings.Contains(out, "**") {
			t.Fatalf("strip оставил '**': %q → %q", in, out)
		}
		if stripInlineMarkdown(out) != out {
			t.Fatalf("strip не идемпотентен: %q → %q → %q", in, out, stripInlineMarkdown(out))
		}
	}
	if got := stripInlineMarkdown("[t](u)"); got != "t (u)" {
		t.Fatalf("ссылка должна сохранять URL: %q", got)
	}
}

func TestOracle_MaskEmailLabel_NeverLeaksLocalPart(t *testing.T) {
	t.Parallel()
	emails := []string{"petrov.ivan@example.com", "abc@d.e", "verylonglocalpart@example.com"}
	for _, email := range emails {
		masked := maskEmailLabel(email)
		local := email[:strings.IndexByte(email, '@')]
		if len(local) > 2 && strings.Contains(masked, local) {
			t.Fatalf("маска светит полный локал: %q → %q", email, masked)
		}
		if !strings.HasSuffix(masked, email[strings.IndexByte(email, '@'):]) {
			t.Fatalf("маска потеряла домен: %q → %q", email, masked)
		}
		if !strings.Contains(masked, "***") {
			t.Fatalf("маска без ***: %q → %q", email, masked)
		}
	}
}

func TestOracle_HMACTokens_RoundTripAndTamper(t *testing.T) {
	t.Parallel()
	hMax := Handler{MaxBotToken: "max-secret-token"}
	hTg := Handler{TelegramBotToken: "tg-secret-token"}
	rng := rand.New(rand.NewSource(42))

	for i := 0; i < 200; i++ {
		userID := rng.Int63n(1_000_000) + 1
		// Round trip: parse(gen(id)) == id for both token types.
		if id, ok := hMax.maxParseToken(hMax.maxUserToken(userID)); !ok || id != userID {
			t.Fatalf("max round-trip: id=%d → (%d,%v)", userID, id, ok)
		}
		if id, ok := hTg.tgParseToken(hTg.tgUserToken(userID)); !ok || id != userID {
			t.Fatalf("tg round-trip: id=%d → (%d,%v)", userID, id, ok)
		}
		// Tamper: replacing the last HMAC character invalidates the token.
		token := hMax.maxUserToken(userID)
		last := token[len(token)-1]
		repl := byte('0')
		if last == '0' {
			repl = '1'
		}
		if _, ok := hMax.maxParseToken(token[:len(token)-1] + string(repl)); ok {
			t.Fatalf("испорченный max-токен принят: %s", token)
		}
		// Swapping userID with the same HMAC invalidates the token.
		forged := fmt.Sprintf("%d_%s", userID+1, strings.SplitN(token, "_", 2)[1])
		if _, ok := hMax.maxParseToken(forged); ok {
			t.Fatalf("токен с подменённым id принят: %s", forged)
		}
	}

	// Cross-secret: a max token does not pass the tg parser for the same userID and
	// vice versa (the keys differ, including when the secret values are the same:
	// the tg key has the prefix "tg:").
	same := Handler{MaxBotToken: "shared", TelegramBotToken: "shared"}
	if _, ok := same.tgParseToken(same.maxUserToken(7)); ok {
		t.Fatalf("max-токен принят tg-парсером при одинаковом секрете")
	}
	if _, ok := same.maxParseToken(same.tgUserToken(7)); ok {
		t.Fatalf("tg-токен принят max-парсером при одинаковом секрете")
	}
}
