package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"mindstrata-stage1/api/internal/kernel/httpjson"
)

func writeJSON(w http.ResponseWriter, status int, payload any) {
	httpjson.Write(w, status, payload)
}

// dbAcquireTimeout (K-1) is the maximum time to wait for a free pool connection
// when opening a transaction. Without this limit Begin(ctx) hangs until
// ctx is cancelled (in practice until WriteTimeout=90s on the server), and when the
// pool is exhausted under load requests hang in batches instead of failing fast. It does not affect an
// already open transaction (after a successful Begin):
// acquireCtx is used only to get the connection and run BEGIN.
const dbAcquireTimeout = 5 * time.Second

type txBeginner interface {
	Begin(context.Context) (pgx.Tx, error)
}

// beginTxTimeout opens a transaction with a bounded wait for a
// free connection (see dbAcquireTimeout). With an exhausted pool it returns
// context.DeadlineExceeded quickly without waiting for the parent ctx to be cancelled.
func beginTxTimeout(ctx context.Context, db txBeginner, timeout time.Duration) (pgx.Tx, error) {
	acquireCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return db.Begin(acquireCtx)
}

// decodeJSONStrict reads the request body (limited to maxBytes via
// http.MaxBytesReader) and decodes JSON into v with strict field checking
// (S-3): unknown fields in the request body are an error rather than silently
// ignored. This catches frontend/backend contract drift (for example, a
// renamed field) at request time rather than somewhere deeper in the code.
func decodeJSONStrict(w http.ResponseWriter, r *http.Request, maxBytes int64, v any) error {
	return httpjson.DecodeStrict(w, r, maxBytes, v)
}

// decodeJSONStrictBody decodes an already prepared r.Body (the size
// limit is applied beforehand by the caller via http.MaxBytesReader)
// with strict field checking (S-3, see decodeJSONStrict).
func decodeJSONStrictBody(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
