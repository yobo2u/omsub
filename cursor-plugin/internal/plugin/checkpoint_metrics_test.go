package plugin

import (
	"cursorplugin/internal/cursorsession"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCheckpointMissBreakdown(t *testing.T) {
	metrics := newCheckpointMetrics()
	for _, reason := range []cursorsession.Reason{"no_identity", cursorsession.ReasonMissing, cursorsession.ReasonExpired, cursorsession.ReasonPrefixMismatch, cursorsession.ReasonMessageCountMismatch} {
		metrics.recordLookupReason("a", false, reason)
	}
	metrics.recordLookupReason("a", true, cursorsession.ReasonHit)
	for _, s := range []checkpointMetricStatus{metrics.snapshot("a"), metrics.total(), mergeCheckpointMetricStatus(metrics.snapshot("a"), metrics.snapshot("b"))} {
		require.EqualValues(t, 5, s.Misses)
		require.EqualValues(t, 1, s.Hits)
		require.Equal(t, s.Misses, s.MissNoIdentity+s.MissColdStart+s.MissExpired+s.MissLineageMismatch)
		require.EqualValues(t, 1, s.MissNoIdentity)
		require.EqualValues(t, 1, s.MissColdStart)
		require.EqualValues(t, 1, s.MissExpired)
		require.EqualValues(t, 2, s.MissLineageMismatch)
	}
}
