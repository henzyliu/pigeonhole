// mautrix-discord - A Matrix-Discord puppeting bridge.
// Copyright (C) 2026 Tulir Asokan
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package discordid

import "testing"

func TestMediaIDRoundTrip(t *testing.T) {
	testCases := []struct {
		name         string
		userLoginID  string
		channelID    string
		messageID    string
		attachmentID string
	}{
		{
			name:         "single digit",
			userLoginID:  "1",
			channelID:    "2",
			messageID:    "3",
			attachmentID: "4",
		},
		{
			name:         "mixed short lengths",
			userLoginID:  "12",
			channelID:    "345",
			messageID:    "6789",
			attachmentID: "12345",
		},
		{
			name:         "discord sized",
			userLoginID:  "12345678901234567",
			channelID:    "234567890123456789",
			messageID:    "345678901234567890",
			attachmentID: "456789012345678901",
		},
		{
			name:         "nineteen digits",
			userLoginID:  "1000000000000000000",
			channelID:    "1000000000000000001",
			messageID:    "1000000000000000002",
			attachmentID: "1000000000000000003",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			want := NewMediaInfoV1(
				MakeUserLoginID(tc.userLoginID),
				tc.channelID,
				tc.messageID,
				tc.attachmentID,
			)

			encoded, err := want.Encode()
			if err != nil {
				t.Fatalf("Encode() failed: %v", err)
			}
			if len(encoded) != encodedMediaIDV1Size {
				t.Fatalf("Encode() returned %d bytes, want %d", len(encoded), encodedMediaIDV1Size)
			}

			got, err := ParseMediaID(encoded)
			if err != nil {
				t.Fatalf("ParseMediaID() failed: %v", err)
			}
			if *got != want {
				t.Fatalf("roundtrip mismatch:\n got:  %#v\n want: %#v", *got, want)
			}
		})
	}
}

func TestParseMediaIDRejectsTruncatedData(t *testing.T) {
	info := NewMediaInfoV1(
		MakeUserLoginID("123456789012345678"),
		"223456789012345678",
		"323456789012345678",
		"423456789012345678",
	)

	encoded, err := info.Encode()
	if err != nil {
		t.Fatalf("Encode() returned error: %v", err)
	}

	_, err = ParseMediaID(encoded[:len(encoded)-1])
	if err == nil {
		t.Fatal("ParseMediaID() unexpectedly succeeded for truncated data")
	}
}
