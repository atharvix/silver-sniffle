package unit

import (
	"testing"

	"github.com/atharvix/kinjo-backend/internal/config"
)

// Inline "db" photos make profile and discovery responses multi-MB, so photo
// storage must follow a driver that can actually hold images.
func TestPhotoStorageDefaultsToUsableDriver(t *testing.T) {
	cases := []struct {
		name        string
		driver      string
		explicit    string
		supabaseKey string
		want        string
	}{
		{name: "local driver is inherited", driver: "local", want: "local"},
		{name: "supabase inherited only with credentials", driver: "supabase", want: "db"},
		{name: "supabase inherited with service role key", driver: "supabase", supabaseKey: "key", want: "supabase"},
		{name: "explicit value wins", driver: "local", explicit: "db", want: "db"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("STORAGE_DRIVER", tc.driver)
			t.Setenv("PHOTO_STORAGE", tc.explicit)
			t.Setenv("SUPABASE_SERVICE_ROLE_KEY", tc.supabaseKey)

			cfg, err := config.Load()
			if err != nil {
				t.Fatalf("config.Load() error = %v", err)
			}
			if cfg.PhotoStorage != tc.want {
				t.Errorf("PhotoStorage = %q, want %q", cfg.PhotoStorage, tc.want)
			}
		})
	}
}
