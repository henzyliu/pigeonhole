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
	"encoding/json"
	"fmt"
	"io"
	"reflect"
)

// Sensitive is a trivial mitigation that guards against accidental leakage of
// important values such as passwords. It is not a security boundary and may be
// trivially unwrapped via [Sensitive.UnwrapSensitive], reflection, etc.
type Sensitive[T any] struct {
	inner T
}

var _ json.Marshaler = (*Sensitive[any])(nil)
var _ json.Unmarshaler = (*Sensitive[any])(nil)

func NewSensitive[T any](inner T) Sensitive[T] {
	return Sensitive[T]{inner}
}

func (s Sensitive[T]) IsZero() bool {
	return reflect.ValueOf(s.inner).IsZero()
}

// UnwrapSensitive returns the sensitive data inside.
func (s Sensitive[T]) UnwrapSensitive() T {
	return s.inner
}

func (Sensitive[T]) Format(f fmt.State, verb rune) {
	_, _ = io.WriteString(f, "<redacted>")
}

func (Sensitive[T]) String() string {
	return "<redacted>"
}

func (Sensitive[T]) GoString() string {
	return "<redacted>"
}

func (s Sensitive[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.inner)
}

func (s *Sensitive[T]) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &s.inner)
}
