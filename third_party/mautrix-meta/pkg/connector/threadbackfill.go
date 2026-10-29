package connector

import (
	"context"
	"time"

	"github.com/rs/zerolog"
)

// Pigeonhole: this file isn't in upstream mautrix-meta.

// Sync groups that hold the thread list. Group 1 is the regular inbox, group
// 95 holds end-to-end encrypted chats. The initial sync only fetches the first
// page of each, so both need to be paged through.
const (
	syncGroupMain int64 = 1
	syncGroupE2EE int64 = 95
)

func (m *MetaClient) StartThreadBackfill(ctx context.Context) error {
	if m.Main.Config.ThreadBackfill.BatchCount == 0 {
		return nil
	}

	log := m.UserLogin.Log.With().Str("action", "thread_backfill").Logger()
	ctx = log.WithContext(ctx)

	if !m.LoginMeta.BackfillCompleted {
		log.Info().Msg("Starting thread backfill")
		if err := m.runThreadBackfill(ctx, syncGroupMain); err != nil {
			return err
		}
		m.LoginMeta.BackfillCompleted = true
		m.saveThreadBackfillState(ctx)
	} else {
		log.Debug().Msg("Thread backfill already completed, skipping")
	}

	if m.LoginMeta.Platform.IsMessenger() && !m.LoginMeta.E2EEThreadBackfillCompleted {
		log.Info().Msg("Starting encrypted thread backfill")
		if err := m.runThreadBackfill(ctx, syncGroupE2EE); err != nil {
			return err
		}
		m.LoginMeta.E2EEThreadBackfillCompleted = true
		m.saveThreadBackfillState(ctx)
	}
	return nil
}

func (m *MetaClient) runThreadBackfill(ctx context.Context, syncGroup int64) error {
	log := zerolog.Ctx(ctx).With().Int64("sync_group", syncGroup).Logger()
	delay := m.Main.Config.ThreadBackfill.BatchDelay
	batchLimit := m.Main.Config.ThreadBackfill.BatchCount
	batchCount := 0
	var prevMinThreadKey int64

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		keyStore, tbl, err := m.Client.FetchMoreThreads(ctx, syncGroup)
		if err != nil {
			log.Err(err).Msg("Failed to fetch more threads")
			return err
		} else if tbl == nil {
			log.Info().Int("batches_processed", batchCount).Msg("Thread backfill complete - no more threads")
			return nil
		}

		batchCount++

		// Process received threads (handled via normal event flow)
		m.parseAndQueueTable(ctx, tbl, false)

		// Check if more threads available - note HasMoreBefore may never become false, so we watch
		// for empty tables as well to identify when we've fully paginated.
		if keyStore == nil || !keyStore.HasMoreBefore {
			log.Info().
				Int("batches_processed", batchCount).
				Any("keystore", keyStore).
				Msg("Thread backfill complete - fully paginated (has no more)")
			return nil
		} else if keyStore.MinThreadKey == prevMinThreadKey {
			log.Info().
				Int("batches_processed", batchCount).
				Any("keystore", keyStore).
				Msg("Thread backfill complete - fully paginated (thread key did not change)")
			return nil
		} else if batchLimit > 0 && batchCount >= batchLimit {
			log.Info().Int("batched_processed", batchCount).
				Msg("Thread backfill complete - hit batch count limit")
			return nil
		}

		prevMinThreadKey = keyStore.MinThreadKey

		log.Debug().
			Int("batch", batchCount).
			Int64("min_thread_key", keyStore.MinThreadKey).
			Int64("min_activity_ts", keyStore.MinLastActivityTimestampMs).
			Msg("Processed thread backfill batch")

		// Rate limiting delay
		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

func (m *MetaClient) saveThreadBackfillState(ctx context.Context) {
	if err := m.UserLogin.Save(ctx); err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to save thread backfill state")
	}
}
