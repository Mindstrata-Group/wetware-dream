package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// telegramAPIBase returns the Telegram Bot API base. api.telegram.org
// is blocked from Russian servers, so in prod
// TELEGRAM_API_BASE_URL is set to a proxy URL (Deno Deploy) that
// forwards requests to api.telegram.org.
func (h Handler) telegramAPIBase() string {
	base := strings.TrimRight(strings.TrimSpace(h.TelegramAPIBaseURL), "/")
	if base == "" {
		return "https://api.telegram.org"
	}
	return base
}

// mdTGBoldRE/mdTGLinkRE convert the CommonMark markup of notifications into
// Telegram HTML. Telegram has no CommonMark: legacy Markdown uses
// single *, MarkdownV2 requires escaping punctuation; both are fragile.
// parse_mode=HTML digests arbitrary text as long as &<> are escaped.
var (
	mdTGBoldRE = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	mdTGLinkRE = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
)

func markdownToTelegramHTML(text string) string {
	// First hide the links (a URL must not be escaped), then escape everything
	// else, then restore the markup.
	type link struct{ label, url string }
	var links []link
	text = mdTGLinkRE.ReplaceAllStringFunc(text, func(m string) string {
		parts := mdTGLinkRE.FindStringSubmatch(m)
		links = append(links, link{label: parts[1], url: parts[2]})
		return fmt.Sprintf("\x00LINK%d\x00", len(links)-1)
	})
	text = html.EscapeString(text)
	text = mdTGBoldRE.ReplaceAllString(text, "<b>$1</b>")
	// An orphaned (unclosed) '**' is removed: the function's contract is that the output has no
	// CommonMark markers; the reader does not need the asterisks.
	text = strings.ReplaceAll(text, "**", "")
	for i, l := range links {
		anchor := fmt.Sprintf(`<a href="%s">%s</a>`, html.EscapeString(l.url), html.EscapeString(l.label))
		text = strings.Replace(text, fmt.Sprintf("\x00LINK%d\x00", i), anchor, 1)
	}
	return text
}

// sendTelegramMessage sends text to a chat/user via the Bot API.
// chatID is a string, so both numeric ids and @channelname are supported. Markup
// goes out as HTML (see markdownToTelegramHTML); if Telegram rejects the
// markup, we retry as plain text so the notification gets through.
func (h Handler) sendTelegramMessage(ctx context.Context, chatID, text string) error {
	if strings.TrimSpace(h.TelegramBotToken) == "" {
		return fmt.Errorf("telegram bot token is not configured")
	}
	if strings.TrimSpace(chatID) == "" {
		return fmt.Errorf("telegram chat id is empty")
	}
	if err := h.postTelegramMessage(ctx, map[string]any{
		"chat_id": chatID, "text": markdownToTelegramHTML(text), "parse_mode": "HTML",
	}); err == nil {
		return nil
	}
	return h.postTelegramMessage(ctx, map[string]any{"chat_id": chatID, "text": stripInlineMarkdown(text)})
}

func (h Handler) postTelegramMessage(ctx context.Context, body map[string]any) error {
	payload, _ := json.Marshal(body)
	url := fmt.Sprintf("%s/bot%s/sendMessage", h.telegramAPIBase(), h.TelegramBotToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return redactSecretInError(err, h.TelegramBotToken)
	}
	req.Header.Set("Content-Type", "application/json")
	client := h.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return redactSecretInError(err, h.TelegramBotToken)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("telegram sendMessage: HTTP %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// redactSecretInError replaces every occurrence of secret in err's text with
// "<redacted>". The Telegram Bot API puts the bot token in the URL path, and
// both *url.Error (network failures) and URL parse errors quote the full URL,
// so without this the token ends up in logs and in stored failure reasons.
// A *url.Error keeps its type (only URL is rewritten) so Timeout()/errors.As
// still work; any other error becomes a plain one with the redacted text.
func redactSecretInError(err error, secret string) error {
	if err == nil || secret == "" || !strings.Contains(err.Error(), secret) {
		return err
	}
	const mask = "<redacted>"
	if ue, ok := err.(*url.Error); ok {
		return &url.Error{Op: ue.Op, URL: strings.ReplaceAll(ue.URL, secret, mask), Err: redactSecretInError(ue.Err, secret)}
	}
	return errors.New(strings.ReplaceAll(err.Error(), secret, mask))
}
