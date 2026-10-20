// Package settings reads the server's configuration from the environment.
package settings

import (
	"cmp"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/language"

	"github.com/pantafive/embisto/thumbnails"
)

// ErrInvalid names a setting that cannot be used.
var ErrInvalid = errors.New("invalid setting")

const (
	defaultAddr        = ":8080"
	defaultMetricsAddr = ":9090"
	// defaultCacheMemory holds some ten thousand cards of an ordinary size.
	defaultCacheMemory = 128 << 20
	defaultLanguage    = "en"
	// cacheTTL is how long a preview is served from the cache when its origin says nothing: pages change their cards
	// rarely.
	cacheTTL = time.Hour
	// defaultMinTTL keeps even a page that forbids caching for a while, so that identical requests make one preview.
	defaultMinTTL = 5 * time.Minute
	defaultMaxTTL = 24 * time.Hour
	// failureTTL is how long a failed preview is: long enough to absorb a burst of retries, short enough that a page
	// back up is soon previewed again.
	failureTTL = time.Minute
)

// Settings say where the server listens, whom it serves and how much it keeps.
type Settings struct {
	Addr        string
	MetricsAddr string
	// PublicURL is where clients reach the API, for the OpenAPI document; empty names no server.
	PublicURL string
	// DefaultAcceptLanguage is the language pages are asked in, unless a request names its own.
	DefaultAcceptLanguage string
	// ImageKey signs the URLs of thumbnails; empty turns thumbnails off.
	ImageKey string
	// YouTubeNoCookie embeds YouTube's player from youtube-nocookie.com.
	YouTubeNoCookie bool
	// Debug serves the debug page on the internal listener.
	Debug bool
	// TemplatesDir holds template files, each an output format; empty adds none.
	TemplatesDir string
	// ImagePresets are the sizes the URLs of thumbnails name.
	ImagePresets []thumbnails.Preset
	// CacheURL names a cache shared by every instance; empty keeps previews in each instance's memory.
	CacheURL string
	// CacheMemory is how many bytes of previews the in-memory cache holds; zero turns it off, but not a shared cache,
	// whose stand-in while it does not answer holds at least a MiB.
	CacheMemory int64
	CacheTTL    time.Duration
	// CacheMinTTL and CacheMaxTTL bound the TTL an origin asks for.
	CacheMinTTL time.Duration
	CacheMaxTTL time.Duration
	FailureTTL  time.Duration
	LogLevel    slog.Level
}

// Load reads EMBISTO_* variables; an unset variable takes its default.
func Load(environ func() []string) (Settings, error) {
	env := map[string]string{}
	for _, pair := range environ() {
		if key, value, ok := strings.Cut(pair, "="); ok {
			env[key] = value
		}
	}
	loaded := Settings{
		Addr:         cmp.Or(env["EMBISTO_ADDR"], defaultAddr),
		MetricsAddr:  cmp.Or(env["EMBISTO_METRICS_ADDR"], defaultMetricsAddr),
		TemplatesDir: env["EMBISTO_TEMPLATES_DIR"],
		// Sites answer in the language of wherever the server runs, unless asked otherwise.
		DefaultAcceptLanguage: cmp.Or(env["EMBISTO_DEFAULT_ACCEPT_LANGUAGE"], defaultLanguage),
		CacheTTL:              cacheTTL,
		FailureTTL:            failureTTL,
	}
	var err error
	if loaded.PublicURL, err = publicURL(env["EMBISTO_PUBLIC_URL"]); err != nil {
		return Settings{}, err
	}
	if err := loaded.readCache(env); err != nil {
		return Settings{}, err
	}
	if _, _, err := language.ParseAcceptLanguage(loaded.DefaultAcceptLanguage); err != nil {
		return Settings{}, fmt.Errorf("%w: EMBISTO_DEFAULT_ACCEPT_LANGUAGE=%q is not an Accept-Language value", ErrInvalid, loaded.DefaultAcceptLanguage)
	}
	if err := loaded.readFlags(env); err != nil {
		return Settings{}, err
	}
	if loaded.ImageKey, loaded.ImagePresets, err = images(env); err != nil {
		return Settings{}, err
	}
	if raw := env["EMBISTO_LOG_LEVEL"]; raw != "" {
		if err := loaded.LogLevel.UnmarshalText([]byte(raw)); err != nil {
			return Settings{}, fmt.Errorf("%w: EMBISTO_LOG_LEVEL=%q is not debug, info, warn or error", ErrInvalid, raw)
		}
	}
	return loaded, nil
}

// images are the key and presets of thumbnails; a key too short to resist guessing is refused.
func images(env map[string]string) (string, []thumbnails.Preset, error) {
	key := env["EMBISTO_IMAGE_KEY"]
	if key != "" && len(key) < thumbnails.MinKeyLength {
		return "", nil, fmt.Errorf("%w: EMBISTO_IMAGE_KEY needs at least %d bytes; make one with `openssl rand -hex 32`", ErrInvalid, thumbnails.MinKeyLength)
	}
	if env["EMBISTO_IMAGE_PRESETS"] == "" {
		return key, thumbnails.DefaultPresets, nil
	}
	presets, err := thumbnails.ParsePresets(env["EMBISTO_IMAGE_PRESETS"])
	if err != nil {
		return "", nil, fmt.Errorf("%w: EMBISTO_IMAGE_PRESETS: %w", ErrInvalid, err)
	}
	return key, presets, nil
}

// readCache reads where previews are kept and how many.
func (s *Settings) readCache(env map[string]string) error {
	var err error
	if s.CacheURL, err = cacheURL(env["EMBISTO_CACHE_URL"]); err != nil {
		return err
	}
	if s.CacheMemory, err = bytesOf(env, "EMBISTO_CACHE_MEMORY", defaultCacheMemory); err != nil {
		return err
	}
	if s.CacheMinTTL, err = duration(env, "EMBISTO_CACHE_MIN_TTL", defaultMinTTL); err != nil {
		return err
	}
	if s.CacheMaxTTL, err = duration(env, "EMBISTO_CACHE_MAX_TTL", defaultMaxTTL); err != nil {
		return err
	}
	if s.CacheMinTTL > s.CacheMaxTTL {
		return fmt.Errorf("%w: EMBISTO_CACHE_MIN_TTL=%s is over EMBISTO_CACHE_MAX_TTL=%s", ErrInvalid, s.CacheMinTTL, s.CacheMaxTTL)
	}
	return nil
}

func duration(env map[string]string, name string, fallback time.Duration) (time.Duration, error) {
	raw := env[name]
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("%w: %s=%q is not a duration such as 5m or 24h", ErrInvalid, name, raw)
	}
	return value, nil
}

// readFlags reads the switches.
func (s *Settings) readFlags(env map[string]string) error {
	var err error
	if s.YouTubeNoCookie, err = flag(env, "EMBISTO_YOUTUBE_NOCOOKIE"); err != nil {
		return err
	}
	s.Debug, err = flag(env, "EMBISTO_DEBUG")
	return err
}

func flag(env map[string]string, name string) (bool, error) {
	raw := env[name]
	if raw == "" {
		return false, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%w: %s=%q is not true or false", ErrInvalid, name, raw)
	}
	return value, nil
}

// publicURL is the address clients reach the server at, without its trailing slash: the paths of the API and of
// thumbnails are appended to it, so it is an absolute http or https URL with no query or fragment.
func publicURL(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("%w: EMBISTO_PUBLIC_URL=%q is not an http or https address such as https://embisto.example.com", ErrInvalid, raw)
	}
	return strings.TrimRight(raw, "/"), nil
}

// cacheURL accepts the schemes of the shared caches embisto can talk to; the URL itself, which may hold a password, is
// never repeated.
func cacheURL(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "redis" && parsed.Scheme != "rediss" {
		return "", fmt.Errorf("%w: EMBISTO_CACHE_URL must be a redis:// or rediss:// URL", ErrInvalid)
	}
	return raw, nil
}

// byteUnits are the suffixes a size may carry, longest first so that MiB is not read as B.
var byteUnits = []struct {
	suffix string
	bytes  int64
}{
	{"KiB", 1 << 10}, {"MiB", 1 << 20}, {"GiB", 1 << 30}, {"KB", 1000}, {"MB", 1000 * 1000}, {"GB", 1000 * 1000 * 1000}, {"B", 1},
}

// bytesOf reads a size such as 128MiB, 500MB or 1048576.
func bytesOf(env map[string]string, name string, fallback int64) (int64, error) {
	raw := strings.TrimSpace(env[name])
	if raw == "" {
		return fallback, nil
	}
	number, unit := raw, int64(1)
	for _, known := range byteUnits {
		if before, ok := strings.CutSuffix(raw, known.suffix); ok {
			number, unit = strings.TrimSpace(before), known.bytes
			break
		}
	}
	value, err := strconv.ParseInt(number, 10, 64)
	if err != nil || value < 0 || value > (1<<62)/unit {
		return 0, fmt.Errorf("%w: %s=%q is not a size such as 128MiB", ErrInvalid, name, raw)
	}
	return value * unit, nil
}
