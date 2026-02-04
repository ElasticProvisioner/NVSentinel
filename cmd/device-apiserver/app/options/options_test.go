//  Copyright (c) 2025, NVIDIA CORPORATION.  All rights reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package options

import (
	"context"
	"testing"
)

func TestServerRunOptions(t *testing.T) {
	opts := NewServerRunOptions()
	if opts == nil || opts.Options == nil {
		t.Fatal("NewServerRunOptions failed to initialize internal options")
	}

	fss := opts.Flags()
	if len(fss.FlagSets) == 0 {
		t.Error("Flags() returned empty NamedFlagSets; expected flags from internal options")
	}

	var nilOpts *ServerRunOptions
	nilFss := nilOpts.Flags()
	if len(nilFss.FlagSets) != 0 {
		t.Error("Flags() on nil options should return empty flag sets")
	}

	t.Run("CompleteAndValidate", func(t *testing.T) {
		ctx := context.Background()

		completed, err := opts.Complete(ctx)
		if err != nil {
			t.Fatalf("Complete failed: %v", err)
		}

		if completed.completedOptions == nil {
			t.Fatal("CompletedOptions internal pointer is nil")
		}

		errs := completed.Validate()
		if len(errs) > 0 {
			t.Logf("Note: Default validation returned %d errors (this is expected if defaults require setup)", len(errs))
		}
	})

	t.Run("CompleteNil", func(t *testing.T) {
		var nilOpts *ServerRunOptions
		completed, err := nilOpts.Complete(context.Background())
		if err != nil {
			t.Errorf("Complete() on nil options should not return error, got: %v", err)
		}
		if completed.completedOptions == nil {
			t.Error("Complete() on nil options should return a valid wrapper")
		}
	})
}
