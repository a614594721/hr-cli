package auth

import (
	"testing"
	"time"
)

func TestTokenStatus(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name  string
		token *StoredToken
		want  string
	}{
		{name: "missing", token: nil, want: "missing"},
		{
			name: "valid",
			token: &StoredToken{
				AccessTokenExpiresAt:  now.Add(10 * time.Minute).Format(time.RFC3339),
				RefreshTokenExpiresAt: now.Add(24 * time.Hour).Format(time.RFC3339),
			},
			want: "valid",
		},
		{
			name: "needs refresh",
			token: &StoredToken{
				AccessTokenExpiresAt:  now.Add(2 * time.Minute).Format(time.RFC3339),
				RefreshTokenExpiresAt: now.Add(24 * time.Hour).Format(time.RFC3339),
			},
			want: "needs_refresh",
		},
		{
			name: "expired",
			token: &StoredToken{
				AccessTokenExpiresAt:  now.Add(-time.Hour).Format(time.RFC3339),
				RefreshTokenExpiresAt: now.Add(-time.Minute).Format(time.RFC3339),
			},
			want: "expired",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tokenStatus(tt.token); got != tt.want {
				t.Fatalf("tokenStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}
