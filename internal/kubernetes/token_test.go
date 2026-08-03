package kubernetes

import (
	"testing"
	"time"

	"bootimus/internal/models"
)

func approvedClient(t *testing.T, expires time.Time) *models.Client {
	t.Helper()
	token, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	return &models.Client{
		MACAddress:      "aa:bb:cc:dd:ee:ff",
		EnrollmentState: models.EnrollmentStateApproved,
		EnrollmentToken: token,
		TokenExpiresAt:  &expires,
	}
}

func TestValidateAcceptsApprovedAndInstalling(t *testing.T) {
	now := time.Now()
	client := approvedClient(t, now.Add(time.Hour))

	for _, state := range []string{models.EnrollmentStateApproved, models.EnrollmentStateInstalling} {
		client.EnrollmentState = state
		if err := ValidateToken(client, client.EnrollmentToken, now); err != nil {
			t.Errorf("state %s: expected valid, got %v", state, err)
		}
	}
}

func TestValidateRejectsWrongStates(t *testing.T) {
	now := time.Now()
	client := approvedClient(t, now.Add(time.Hour))

	for _, state := range []string{
		models.EnrollmentStateUnmanaged,
		models.EnrollmentStatePending,
		models.EnrollmentStateInstalled,
		models.EnrollmentStateRejected,
	} {
		client.EnrollmentState = state
		if err := ValidateToken(client, client.EnrollmentToken, now); err != ErrNotApproved {
			t.Errorf("state %q: expected ErrNotApproved, got %v", state, err)
		}
	}
}

func TestValidateRejectsTamperedToken(t *testing.T) {
	now := time.Now()
	client := approvedClient(t, now.Add(time.Hour))

	if err := ValidateToken(client, client.EnrollmentToken+"x", now); err != ErrTokenInvalid {
		t.Errorf("expected ErrTokenInvalid for tampered token, got %v", err)
	}
	if err := ValidateToken(client, "", now); err != ErrTokenInvalid {
		t.Errorf("expected ErrTokenInvalid for empty token, got %v", err)
	}
	other := approvedClient(t, now.Add(time.Hour))
	if err := ValidateToken(client, other.EnrollmentToken, now); err != ErrTokenInvalid {
		t.Errorf("expected ErrTokenInvalid for another node's token, got %v", err)
	}
}

func TestValidateRejectsExpiredToken(t *testing.T) {
	now := time.Now()
	client := approvedClient(t, now.Add(-time.Minute))

	if err := ValidateToken(client, client.EnrollmentToken, now); err != ErrTokenExpired {
		t.Errorf("expected ErrTokenExpired, got %v", err)
	}

	client.TokenExpiresAt = nil
	if err := ValidateToken(client, client.EnrollmentToken, now); err != ErrTokenExpired {
		t.Errorf("expected ErrTokenExpired for missing expiry, got %v", err)
	}
}

func TestValidateNilClient(t *testing.T) {
	if err := ValidateToken(nil, "token", time.Now()); err != ErrNotApproved {
		t.Errorf("expected ErrNotApproved for nil client, got %v", err)
	}
}

func TestValidateUUID(t *testing.T) {
	if err := ValidateUUID("", "anything"); err != nil {
		t.Errorf("expected empty expected uuid to skip check, got %v", err)
	}
	if err := ValidateUUID("ABC-123", "abc-123"); err != nil {
		t.Errorf("expected case-insensitive match, got %v", err)
	}
	if err := ValidateUUID("abc-123", "abc-999"); err != ErrUUIDMismatch {
		t.Errorf("expected ErrUUIDMismatch, got %v", err)
	}
}
