## `POST /api/v9/auth/login`

### request

...

### response

#### new login location

HTTP 400

```json
{
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
}
```

## `POST /api/v9/auth/authorize-ip`

### request

```json
{
	"token": "..."
}
```

### response

#### when link has expired

```json
{
	"message": "Invalid authentication token",
	"code": 50014
}
```

- UI prompts the user to log in again to get another link
