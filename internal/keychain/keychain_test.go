package keychain

import "testing"

func TestKeychainSetGetRemove(t *testing.T) {
	service := "hr-cli-test"
	account := "test-account"
	value := `{"accessToken":"access","refreshToken":"refresh"}`

	if err := Set(service, account, value); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	got, err := Get(service, account)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != value {
		t.Fatalf("Get() = %q, want %q", got, value)
	}
	if err := Remove(service, account); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	got, err = Get(service, account)
	if err != nil {
		t.Fatalf("Get() after remove error = %v", err)
	}
	if got != "" {
		t.Fatalf("Get() after remove = %q, want empty", got)
	}
}
