package discordauth

import (
	"encoding/json"
	"testing"
)

func TestFormFieldErrors_AccountLoginVerificationEmail(t *testing.T) {
	body := []byte(`{
		"message": "Invalid Form Body",
		"code": 50035,
		"errors": {
			"login": {
				"_errors": [
					{
						"code": "ACCOUNT_LOGIN_VERIFICATION_EMAIL",
						"message": "New login location detected, please check your e-mail."
					}
				]
			}
		}
	}`)

	var apiErr APIError
	if err := json.Unmarshal(body, &apiErr); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if apiErr.Code != InvalidFormBody {
		t.Fatalf("expected code %d, got %d", InvalidFormBody, apiErr.Code)
	}

	errs, err := apiErr.FormFieldErrors("login")
	if err != nil {
		t.Fatalf("FormFieldErrors returned error: %v", err)
	}
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if FormErrorCode(errs[0].Code) != AccountLoginVerificationEmail {
		t.Fatalf("expected code %s, got %s", AccountLoginVerificationEmail, errs[0].Code)
	}
}
