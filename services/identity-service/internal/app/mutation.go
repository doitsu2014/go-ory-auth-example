package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/audit"
)

// mutationTimeout bounds an audited mutation. It runs detached from the
// request context so a client disconnect cannot leave a half-applied change.
const mutationTimeout = 10 * time.Second

// audited is the single audit strategy for admin mutations (FR-12):
//
//  1. open a DB transaction (detached from request cancellation),
//  2. run pre (locks, invariant checks; may fill ev.Details),
//  3. INSERT the audit row,
//  4. apply the external mutation (Kratos/Keto),
//  5. COMMIT.
//
// If the audit insert fails, nothing is changed. If the external mutation
// fails, the audit row is rolled back. The only remaining gap is a COMMIT
// failure after a successful mutation; it is logged as "unaudited_mutation"
// with ids only. This knowingly spans Ory calls inside a short transaction
// (2 s Ory timeouts, 10 s idle-in-transaction limit) to make audit and
// mutation atomic in the common failure cases.
func audited(ctx context.Context, tx TxRunner, log *slog.Logger, ev *audit.Event,
	pre func(ctx context.Context, r Repos) error, mutate func(ctx context.Context) error,
) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), mutationTimeout)
	defer cancel()
	mutated := false
	err := tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		if pre != nil {
			if err := pre(ctx, r); err != nil {
				return err
			}
		}
		if err := r.Audit.Append(ctx, *ev); err != nil {
			return fmt.Errorf("append audit: %w", err)
		}
		if err := mutate(ctx); err != nil {
			return err
		}
		mutated = true
		return nil
	})
	if err != nil && mutated && log != nil {
		log.ErrorContext(ctx, "unaudited_mutation",
			"action", string(ev.Action), "target_type", ev.TargetType, "target_id", ev.TargetID,
			"actor_id", ev.ActorID.String(), "request_id", ev.RequestID, "error", err.Error())
	}
	return err
}
