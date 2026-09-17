package infra

import "testing"

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    Config
		wantErr bool
	}{
		{
			name: "defaults applied",
			env:  map[string]string{"DATABASE_URL": "postgres://localhost/app"},
			want: Config{Port: "8080", DatabaseURL: "postgres://localhost/app", DBMaxConns: 10},
		},
		{
			name: "explicit values",
			env: map[string]string{
				"PORT":         "9090",
				"DATABASE_URL": "postgres://localhost/app",
				"JWT_SECRET":   "test-only-secret",
				"DB_MAX_CONNS": "4",
			},
			want: Config{Port: "9090", DatabaseURL: "postgres://localhost/app", JWTSecret: "test-only-secret", DBMaxConns: 4},
		},
		{
			name:    "missing DATABASE_URL fails",
			env:     map[string]string{},
			wantErr: true,
		},
		{
			name:    "non-numeric DB_MAX_CONNS fails",
			env:     map[string]string{"DATABASE_URL": "postgres://localhost/app", "DB_MAX_CONNS": "lots"},
			wantErr: true,
		},
		{
			name:    "non-positive DB_MAX_CONNS fails",
			env:     map[string]string{"DATABASE_URL": "postgres://localhost/app", "DB_MAX_CONNS": "0"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LoadConfig(func(key string) string { return tt.env[key] })
			if tt.wantErr {
				if err == nil {
					t.Fatal("LoadConfig returned nil error, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadConfig returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("LoadConfig = %+v, want %+v", got, tt.want)
			}
		})
	}
}
