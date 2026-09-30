package httpapi

import (
	"context"
	"log"
	"strconv"
	"time"
)

// processDueNotificationQueue sends deferred messages that are due
// (the broadcast's "second channel"). The binding is re-read at send time:
// if the recipient has meanwhile unbound the messenger, mark it failed and do not send.
func (h Handler) processDueNotificationQueue(ctx context.Context) (int, error) {
	rows, err := h.DB.Query(ctx, `
		SELECT q.id, q.user_id, q.channel, q.text, u.max_chat_id, u.telegram_id
		FROM notification_channel_queue q
		JOIN users u ON u.id = q.user_id AND u.deleted_at IS NULL
		WHERE q.sent_at IS NULL AND q.failed IS NULL AND q.send_after <= now()
		ORDER BY q.send_after
		LIMIT 100`)
	if err != nil {
		return 0, err
	}
	type queueItem struct {
		id, userID int64
		channel    string
		text       string
		maxChatID  *int64
		telegramID *int64
	}
	items := []queueItem{}
	for rows.Next() {
		var it queueItem
		if err := rows.Scan(&it.id, &it.userID, &it.channel, &it.text, &it.maxChatID, &it.telegramID); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	sent := 0
	for _, it := range items {
		var sendErr error
		switch it.channel {
		case "max":
			if it.maxChatID == nil {
				h.markQueueItemFailed(ctx, it.id, "unlinked")
				continue
			}
			sendErr = h.sendMaxMessage(ctx, *it.maxChatID, it.text)
		case "telegram":
			if it.telegramID == nil {
				h.markQueueItemFailed(ctx, it.id, "unlinked")
				continue
			}
			sendErr = h.sendTelegramMessage(ctx, strconv.FormatInt(*it.telegramID, 10), it.text)
		default:
			h.markQueueItemFailed(ctx, it.id, "unknown channel")
			continue
		}
		if sendErr != nil {
			if isMessengerBlockedError(sendErr) {
				h.unlinkBlockedMessenger(it.userID, it.channel)
			}
			h.markQueueItemFailed(ctx, it.id, sendErr.Error())
			continue
		}
		_, _ = h.DB.Exec(ctx, `UPDATE notification_channel_queue SET sent_at = now() WHERE id = $1`, it.id)
		sent++
	}
	return sent, nil
}

func (h Handler) markQueueItemFailed(ctx context.Context, id int64, reason string) {
	if len(reason) > 500 {
		reason = reason[:500]
	}
	_, _ = h.DB.Exec(ctx, `UPDATE notification_channel_queue SET failed = $2 WHERE id = $1`, id, reason)
}

// StartNotificationQueueWorker is the background ticker that delivers deferred messages.
// Started once from main; stops on ctx.
func (h Handler) StartNotificationQueueWorker(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		tickCtx, cancel := context.WithTimeout(ctx, 50*time.Second)
		if sent, err := h.processDueNotificationQueue(tickCtx); err != nil {
			log.Printf("notification queue: %v", err)
		} else if sent > 0 {
			log.Printf("notification queue: отправлено отложенных: %d", sent)
		}
		cancel()
	}
}
