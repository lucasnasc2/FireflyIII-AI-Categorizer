package cache

import (
	"context"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/openaccountants/firefly-iii-ai-categorize/internal/classifier"
	"github.com/openaccountants/firefly-iii-ai-categorize/internal/firefly"
)

// Cache holds a TTL-backed, in-memory copy of categorized transactions fetched
// from Firefly III. It is the single source of truth for classification history,
// so manual edits made in Firefly are picked up after the TTL expires.
type Cache struct {
	mu      sync.RWMutex
	entries []classifier.HistoricalEntry

	// merchants maps a description fingerprint to the merchant name the model
	// read out of that description. It is what makes lookups stable for a bank
	// that sends "(unknown destination)" and a different date on every line.
	// In-memory only: losing it costs one model call per merchant, not data.
	merchants map[string]string

	fetchedAt time.Time
	ttl       time.Duration
	lookback  int
	client    *firefly.Client
}

func New(client *firefly.Client, ttl time.Duration, lookbackDays int) *Cache {
	return &Cache{
		merchants: make(map[string]string),
		ttl:       ttl,
		lookback:  lookbackDays,
		client:    client,
	}
}

// Key returns the key to group and look up this transaction's history by: the
// payee when Firefly III has a real one, otherwise the merchant learned for this
// description, otherwise the description itself.
func (c *Cache) Key(destinationName, description string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.keyLocked(destinationName, description)
}

// keyLocked is Key for callers that already hold the lock.
func (c *Cache) keyLocked(destinationName, description string) string {
	if classifier.IsBlankName(destinationName) {
		if m := c.merchants[classifier.DescriptionFingerprint(description)]; m != "" {
			return m
		}
	}
	return classifier.GroupKey(destinationName, description)
}

// Merchant returns the merchant learned for a description, or "".
func (c *Cache) Merchant(description string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.merchants[classifier.DescriptionFingerprint(description)]
}

// SetMerchant records the merchant a classification inferred from a
// description, so later transactions with the same description can be looked up
// under it and skip the model call.
func (c *Cache) SetMerchant(description, merchant string) {
	if merchant == "" {
		return
	}
	fp := classifier.DescriptionFingerprint(description)
	if fp == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.merchants[fp] = merchant
}

// GetHistory returns up to limit past classifications whose group key matches
// groupKey, most-recent first. Silently returns nil on refresh errors (the
// classifier still works, just without context).
func (c *Cache) GetHistory(ctx context.Context, groupKey string, limit int) []classifier.HistoricalEntry {
	if groupKey == "" {
		return nil
	}
	if err := c.ensureFresh(ctx); err != nil {
		slog.Error("history cache refresh failed", "error", err)
		return nil
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	var matches []classifier.HistoricalEntry
	for i := len(c.entries) - 1; i >= 0 && len(matches) < limit; i-- {
		// Recompute the key rather than trusting the stored one: a merchant
		// learned since this entry was fetched changes what it groups under.
		if c.keyLocked(c.entries[i].DestinationName, c.entries[i].Description) == groupKey {
			matches = append(matches, c.entries[i])
		}
	}
	return matches
}

// Append adds a freshly classified entry without waiting for TTL expiry.
// This ensures later jobs in the same batch benefit from earlier results.
func (c *Cache) Append(entry classifier.HistoricalEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, entry)
}

// Invalidate forces a re-fetch on the next GetHistory call.
func (c *Cache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fetchedAt = time.Time{}
}

func (c *Cache) ensureFresh(ctx context.Context) error {
	// Fast path: check under read lock.
	c.mu.RLock()
	fresh := c.ttl > 0 && !c.fetchedAt.IsZero() && time.Since(c.fetchedAt) < c.ttl
	c.mu.RUnlock()
	if fresh {
		return nil
	}

	// Slow path: acquire write lock and double-check.
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.ttl > 0 && !c.fetchedAt.IsZero() && time.Since(c.fetchedAt) < c.ttl {
		return nil
	}

	slog.Info("refreshing history cache from Firefly III")
	txns, err := c.client.GetCategorizedWithdrawals(ctx, c.lookback)
	if err != nil {
		return err
	}

	entries := make([]classifier.HistoricalEntry, 0, len(txns))
	for _, txn := range txns {
		for _, s := range txn.Splits {
			if s.CategoryName == "" {
				continue
			}
			amount, _ := strconv.ParseFloat(s.Amount, 64)
			entries = append(entries, classifier.HistoricalEntry{
				TransactionID:        txn.ID,
				DestinationName:      s.DestinationName,
				Description:          s.Description,
				CategoryName:         s.CategoryName,
				GroupKey:             c.keyLocked(s.DestinationName, s.Description),
				Amount:               amount,
				DestinationAccountID: s.DestinationID,
			})
		}
	}

	c.entries = entries
	c.fetchedAt = time.Now()
	slog.Info("history cache refreshed", "entries", len(entries))
	return nil
}
