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

package discordauth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// EncodeBasicContextProperties creates a value for [HeaderContextProperties]
// when only the "location" key is needed. This is unlikely to suffice for most
// location types.
func EncodeBasicContextProperties(location ContextLocation) (string, error) {
	encoded, err := json.Marshal(map[string]string{"location": string(location)})
	if err != nil {
		return "", fmt.Errorf("failed to marshal basic context properties: %w", err)
	}

	return base64.StdEncoding.EncodeToString(encoded), nil
}

type ContextLocation string

// This is not a comprehensive listing.
const (
	ContextLocationLogin                               ContextLocation = "Login"
	ContextLocationRegister                            ContextLocation = "Register"
	ContextLocationInvite                              ContextLocation = "Accept Invite Page"
	ContextLocationVerify                              ContextLocation = "Verify Email"
	ContextLocationDisableEmailNotifications           ContextLocation = "Disable Email Notifications"
	ContextLocationDisableServerHighlightNotifications ContextLocation = "Disable Server Highlight Notifications"
	ContextLocationAuthorizeIp                         ContextLocation = "Authorize Ip"
	ContextLocationRejectIp                            ContextLocation = "Reject Ip"
	ContextLocationRejectMfa                           ContextLocation = "Reject MFA"
	ContextLocationReport                              ContextLocation = "Report Illegal Content"
	ContextLocationReportSecondLook                    ContextLocation = "Report Second Look"
	ContextLocationAuthorizePayment                    ContextLocation = "Authorize Payment"
	ContextLocationReset                               ContextLocation = "Reset"
	ContextLocationAccountRevert                       ContextLocation = "Account Revert"
	ContextLocationHandoff                             ContextLocation = "Handoff"
	ContextLocationUnknown                             ContextLocation = "Unknown"
	ContextLocationLanding                             ContextLocation = "Landing"
)
