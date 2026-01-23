// Copyright (c) 2025, NVIDIA CORPORATION.  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package metricsutil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCalculateDurationSeconds(t *testing.T) {
	tests := []struct {
		name        string
		timeAgo     time.Duration
		description string
	}{
		{
			name:        "5 seconds ago",
			timeAgo:     5 * time.Second,
			description: "Should calculate duration correctly for 5 seconds ago",
		},
		{
			name:        "1 minute ago",
			timeAgo:     1 * time.Minute,
			description: "Should calculate duration correctly for 1 minute ago",
		},
		{
			name:        "1 hour ago",
			timeAgo:     1 * time.Hour,
			description: "Should calculate duration correctly for 1 hour ago",
		},
		{
			name:        "5 minutes ago",
			timeAgo:     5 * time.Minute,
			description: "Should calculate duration correctly for 5 minutes ago",
		},
		{
			name:        "3 hours ago",
			timeAgo:     3 * time.Hour,
			description: "Should calculate duration correctly for 3 hours ago",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timestamp := time.Now().Add(-tt.timeAgo)
			duration := CalculateDurationSeconds(timestamp)

			// Verify accuracy: should match time.Since calculation
			expectedDuration := time.Since(timestamp).Seconds()
			assert.InDelta(t, expectedDuration, duration, 0.1, tt.description)

			t.Logf("Time ago: %v, Calculated duration: %.2f seconds", tt.timeAgo, duration)
		})
	}

	t.Run("Zero time returns 0", func(t *testing.T) {
		duration := CalculateDurationSeconds(time.Time{})
		assert.Equal(t, float64(0), duration, "Should return 0 for zero time")
	})

	t.Run("Recent timestamp", func(t *testing.T) {
		timestamp := time.Now().Add(-100 * time.Millisecond)
		duration := CalculateDurationSeconds(timestamp)
		assert.Greater(t, duration, float64(0), "Should be greater than 0 for recent timestamp.")
		assert.Less(t, duration, float64(1), "Should be less than 1 second for 100ms ago")
	})
}
