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

package validation

import (
	"testing"
)

func TestIsTCPAddress(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		wantErr bool
	}{
		{"Missing port", "127.0.0.1", true},
		{"Valid localhost", "localhost:8080", false},
		{"Invalid IP format (strict)", "not-an-ip:80", true},
		{"Valid IPv4", "1.1.1.1:53", false},
		{"Valid IPv6", "[::1]:443", false},
		{"Non-numeric port", "1.1.1.1:port", true},
		{"Port out of range", "127.0.0.1:65536", true},
		{"Port zero", "127.0.0.1:0", true},
		{"Empty host (valid bind all)", ":80", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := IsTCPAddress(tt.addr)
			if (len(errs) > 0) != tt.wantErr {
				t.Errorf("IsTCPAddress(%s) errors = %v, wantErr %v", tt.addr, errs, tt.wantErr)
			}
		})
	}
}

func TestIsUnixSocketURI(t *testing.T) {
	tests := []struct {
		name    string
		uri     string
		wantErr bool
	}{
		{"Valid absolute path", "unix:///var/run/test.sock", false},
		{"Missing prefix", "/var/run/test.sock", true},
		{"Relative path", "unix://var/run/test.sock", true},
		{"Trailing slash", "unix:///var/run/test/", true},
		{"Empty path", "unix://", true},
		{"Wrong scheme", "http:///var/run/test.sock", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := IsUnixSocketURI(tt.uri)
			if (len(errs) > 0) != tt.wantErr {
				t.Errorf("IsUnixSocketURI(%s) errors = %v, wantErr %v", tt.uri, errs, tt.wantErr)
			}
		})
	}
}
