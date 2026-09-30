// Package pseudonym masks personal data before a text leaves for an external
// LLM and puts it back into the answer.
//
//	masked, ref, err := svc.Transform(ctx, userID, "ru", text)
//	// ... send masked to the model, stream the answer back ...
//	r := svc.NewStreamRestorer(ctx, userID, ref)
//	for chunk := range answer { w.Write(r.Write(chunk)) }
//	w.Write(r.Flush())
//
// Detection and Russian declension live in the Python service
// apps/anonymizer (Natasha + pymorphy3 have no Go equivalent of the same
// quality). This package owns everything stateful:
//
//   - the per-user vault in Postgres (table pii_vault): alias numbers and the
//     encrypted original values;
//   - the keys: every user gets an HMAC key derived from PII_MASTER_KEY, so the
//     same value has unrelated identities for two users; originals are sealed
//     with AES-256-GCM bound to the user and kind;
//   - the user's own dictionary (profile values + vault originals) sent to the
//     detector with each request, which makes recall on the user's own data
//     complete.
//
// Fail closed: when the service is unreachable, the key is missing, or the
// detector reports a residual, Transform returns an error and the caller
// must not send the text to an external model.
//
// Status: not yet wired into the chat path; see docs/specs/data-protection.md
// (pre-send transform / post-receive restore).
package pseudonym
