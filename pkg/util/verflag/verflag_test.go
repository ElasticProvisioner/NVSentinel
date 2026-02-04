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

package verflag

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionValue_Set(t *testing.T) {
	tests := []struct {
		input    string
		expected versionValue
	}{
		{"raw", VersionRaw},
		{"true", VersionTrue},
		{"1", VersionTrue},
		{"false", VersionFalse},
		{"0", VersionFalse},
		{"random-string", VersionTrue}, // Fallback logic in Set()
	}

	for _, tt := range tests {
		v := new(versionValue)
		if err := v.Set(tt.input); err != nil {
			t.Errorf("Set(%s) unexpected error: %v", tt.input, err)
		}
		if *v != tt.expected {
			t.Errorf("Set(%s) = %v; want %v", tt.input, *v, tt.expected)
		}
	}
}

func TestPrintAndExitIfRequested(t *testing.T) {
	buf := &bytes.Buffer{}
	output = buf

	var exitedWith *int
	exit = func(code int) {
		exitedWith = &code
	}

	oldFlag := versionFlag
	defer func() {
		versionFlag = oldFlag
		output = &bytes.Buffer{} // Reset to avoid leak
		exit = func(int) {}      // Reset to avoid side effects
	}()

	tests := []struct {
		name          string
		state         versionValue
		wantExit      bool
		wantSubstring string
	}{
		{
			name:     "VersionFalse does nothing",
			state:    VersionFalse,
			wantExit: false,
		},
		{
			name:          "VersionTrue prints table",
			state:         VersionTrue,
			wantExit:      true,
			wantSubstring: "NVIDIA Device API",
		},
		{
			name:          "VersionRaw prints Go struct style",
			state:         VersionRaw,
			wantExit:      true,
			wantSubstring: "version.Info{",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			exitedWith = nil

			v := tt.state
			versionFlag = &v

			PrintAndExitIfRequested()

			if tt.wantExit {
				if exitedWith == nil || *exitedWith != 0 {
					t.Errorf("Expected exit(0), got %v", exitedWith)
				}
			} else if exitedWith != nil {
				t.Errorf("Expected no exit, but exited with %d", *exitedWith)
			}

			if tt.wantSubstring != "" && !strings.Contains(buf.String(), tt.wantSubstring) {
				t.Errorf("Output missing expected content: %q\nGot: %q", tt.wantSubstring, buf.String())
			}
		})
	}
}
