package settings_test

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pantafive/embisto/internal/settings"
	"github.com/pantafive/embisto/thumbnails"
)

func environ(pairs ...string) func() []string {
	return func() []string { return pairs }
}

func TestLoadReadsTheEnvironment(t *testing.T) {
	loaded, err := settings.Load(environ(
		"EMBISTO_ADDR=:9000", "EMBISTO_METRICS_ADDR=:9100", "EMBISTO_PUBLIC_URL=https://api.example.com/", "EMBISTO_DEFAULT_ACCEPT_LANGUAGE=de-DE,de;q=0.9", "EMBISTO_IMAGE_KEY=0123456789abcdef0123456789abcdef", "EMBISTO_YOUTUBE_NOCOOKIE=true", "EMBISTO_DEBUG=true", "EMBISTO_IMAGE_PRESETS=tiny=64x64-fill,wide=960x0-fit",
		"EMBISTO_CACHE_URL=redis://cache:6379/0", "EMBISTO_CACHE_MEMORY=256 MiB", "EMBISTO_CACHE_MIN_TTL=1m", "EMBISTO_CACHE_MAX_TTL=2h", "EMBISTO_LOG_LEVEL=debug",
	))

	require.NoError(t, err)
	assert.Equal(t, settings.Settings{
		Addr: ":9000", MetricsAddr: ":9100", PublicURL: "https://api.example.com", DefaultAcceptLanguage: "de-DE,de;q=0.9",
		ImageKey: "0123456789abcdef0123456789abcdef", YouTubeNoCookie: true, Debug: true,
		ImagePresets: []thumbnails.Preset{
			{Name: "tiny", Box: thumbnails.Box{Width: 64, Height: 64, Mode: thumbnails.Fill}},
			{Name: "wide", Box: thumbnails.Box{Width: 960, Mode: thumbnails.Fit}},
		},
		CacheURL: "redis://cache:6379/0", CacheMemory: 256 << 20, CacheTTL: time.Hour, CacheMinTTL: time.Minute, CacheMaxTTL: 2 * time.Hour, FailureTTL: time.Minute, LogLevel: slog.LevelDebug,
	}, loaded)
}

func TestLoadDefaultsAnEmptyEnvironment(t *testing.T) {
	loaded, err := settings.Load(environ())

	require.NoError(t, err)
	assert.Equal(t, settings.Settings{
		Addr: ":8080", MetricsAddr: ":9090", DefaultAcceptLanguage: "en", ImagePresets: thumbnails.DefaultPresets, CacheMemory: 128 << 20, CacheTTL: time.Hour, CacheMinTTL: 5 * time.Minute, CacheMaxTTL: 24 * time.Hour, FailureTTL: time.Minute,
	}, loaded)
}

func TestLoadRefusesWhatItCannotHonour(t *testing.T) {
	tests := []string{"EMBISTO_CACHE_URL=memcached://cache:11211", "EMBISTO_CACHE_MEMORY=-1", "EMBISTO_CACHE_MEMORY=lots", "EMBISTO_LOG_LEVEL=loud", "EMBISTO_DEFAULT_ACCEPT_LANGUAGE=not a language!", "EMBISTO_IMAGE_KEY=short", "EMBISTO_IMAGE_PRESETS=huge", "EMBISTO_YOUTUBE_NOCOOKIE=maybe", "EMBISTO_CACHE_MIN_TTL=soon", "EMBISTO_CACHE_MIN_TTL=48h"}
	for _, variable := range tests {
		t.Run(variable, func(t *testing.T) {
			_, err := settings.Load(environ(variable))

			require.ErrorIs(t, err, settings.ErrInvalid)
		})
	}
}
