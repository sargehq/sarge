package db

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/sargehq/sarge/internal/db/sqlc"
)

// CacheComplexity stores a complexity estimate for a bead in the cache.
func (db *DB) CacheComplexity(ctx context.Context, beadID, descHash string, score, tokens int) error {
	err := db.queries.CacheComplexity(ctx, sqlc.CacheComplexityParams{
		BeadID:          beadID,
		DescriptionHash: descHash,
		ComplexityScore: int64(score),
		EstimatedTokens: int64(tokens),
	})
	if err != nil {
		return fmt.Errorf("failed to cache complexity for %s: %w", beadID, err)
	}
	return nil
}

// GetCachedComplexity retrieves cached complexity for a bead if it exists and the description hash matches.
func (db *DB) GetCachedComplexity(ctx context.Context, beadID, descHash string) (score, tokens int, found bool, err error) {
	row, err := db.queries.GetCachedComplexity(ctx, sqlc.GetCachedComplexityParams{
		BeadID:          beadID,
		DescriptionHash: descHash,
	})
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return 0, 0, false, nil
		}
		return 0, 0, false, fmt.Errorf("failed to get cached complexity: %w", err)
	}

	return int(row.ComplexityScore), int(row.EstimatedTokens), true, nil
}

// GetAllCachedComplexity returns all cached complexity estimates.
func (db *DB) GetAllCachedComplexity(ctx context.Context) (map[string]struct{ Score, Tokens int }, error) {
	rows, err := db.queries.GetAllCachedComplexity(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query complexity cache: %w", err)
	}

	result := make(map[string]struct{ Score, Tokens int })
	for _, row := range rows {
		result[row.BeadID] = struct{ Score, Tokens int }{
			Score:  int(row.ComplexityScore),
			Tokens: int(row.EstimatedTokens),
		}
	}
	return result, nil
}

// HashDescription creates a SHA256 hash of a description string.
func HashDescription(description string) string {
	h := sha256.Sum256([]byte(description))
	return fmt.Sprintf("%x", h)
}

// AreAllBeadsEstimated checks if all beads in the list have complexity estimates.
func (db *DB) AreAllBeadsEstimated(ctx context.Context, beadIDs []string) (bool, error) {
	if len(beadIDs) == 0 {
		return true, nil
	}

	count, err := db.queries.CountEstimatedBeads(ctx, beadIDs)
	if err != nil {
		return false, fmt.Errorf("failed to count estimated beads: %w", err)
	}

	return int(count) == len(beadIDs), nil
}
