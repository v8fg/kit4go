package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/v8fg/kit4go/config"
)

func TestEnv_Normalization(t *testing.T) {
	t.Setenv("APP_REDIS_ADDR", "localhost:6379")
	t.Setenv("APP_BIDDER_TIMEOUT", "1500ms")
	t.Setenv("APP_FEATURE_FAST_PATH", "true")

	store := config.New(config.Env("app"))
	require.Equal(t, "localhost:6379", store.String("redis.addr", ""))
	require.Equal(t, "1500ms", store.String("bidder.timeout", ""))
	require.Equal(t, "localhost:6379", store.String("REDIS.ADDR", "")) // case-insensitive key
}

func TestEnv_NoPrefix(t *testing.T) {
	t.Setenv("MY_KEY", "v")
	store := config.New(config.Env(""))
	require.Equal(t, "v", store.String("my.key", ""))
	require.Equal(t, "v", store.String("MY-KEY", ""))
}

func TestMapSource(t *testing.T) {
	store := config.New(config.MapSource{"k": "1"})
	require.Equal(t, "1", store.String("k", ""))
	require.False(t, store.Has("missing"))
}

func TestFileSource(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.json")
	require.NoError(t, os.WriteFile(path, []byte(`{
		"redis.addr": "h:6379",
		"bidder.timeout": "2s",
		"flag": "on"
	}`), 0o600))

	src, err := config.FromFile(path)
	require.NoError(t, err)
	store := config.New(src)
	require.Equal(t, "h:6379", store.String("redis.addr", ""))
	require.True(t, store.Bool("flag", false))
}

func TestFileSource_BadPath(t *testing.T) {
	_, err := config.FromFile(filepath.Join(t.TempDir(), "nope.json"))
	require.Error(t, err)
}

func TestFileSource_BadJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	require.NoError(t, os.WriteFile(path, []byte(`{not json`), 0o600))
	_, err := config.FromFile(path)
	require.Error(t, err)
}

// First source wins — env should override file.
func TestPriorityOverlay(t *testing.T) {
	t.Setenv("APP_K", "from-env")
	file := config.MapSource{"k": "from-file", "only-file": "f"}
	store := config.New(config.Env("app"), file) // env first = highest
	require.Equal(t, "from-env", store.String("k", ""))
	require.Equal(t, "f", store.String("only-file", ""))
}

func TestTypedGetters(t *testing.T) {
	store := config.New(config.MapSource{
		"i":   "42",
		"i64": "9000000000",
		"b":   "yes",
		"b2":  "0",
		"f":   "3.14",
		"d":   "1m30s",
		"neg": "not-a-number",
	})
	require.Equal(t, 42, store.Int("i", -1))
	require.Equal(t, int64(9000000000), store.Int64("i64", -1))
	require.Equal(t, true, store.Bool("b", false))
	require.Equal(t, false, store.Bool("b2", true))
	require.InDelta(t, 3.14, store.Float64("f", 0), 1e-9)
	require.Equal(t, 90*time.Second, store.Duration("d", 0))
}

func TestDefaultsOnMissingAndParseError(t *testing.T) {
	store := config.New(config.MapSource{"bad": "x"})
	require.Equal(t, -1, store.Int("bad", -1))     // parse error -> default
	require.Equal(t, -1, store.Int("missing", -1)) // missing -> default
	require.Equal(t, 9.9, store.Float64("bad", 9.9))
	require.Equal(t, 5*time.Second, store.Duration("bad", 5*time.Second))
	require.False(t, store.Bool("bad", false))
}

func TestBoolAcceptsVariants(t *testing.T) {
	cases := map[string]bool{
		"1": true, "t": true, "TRUE": true, "Yes": true, "on": true, "Y": true,
		"0": false, "f": false, "False": false, "no": false, "off": false, "N": false,
	}
	for raw, want := range cases {
		store := config.New(config.MapSource{"b": raw})
		require.Equal(t, want, store.Bool("b", !want), "raw=%q", raw)
	}
	// Missing -> default preserved.
	empty := config.New(config.MapSource{})
	require.Equal(t, true, empty.Bool("missing", true))
}

func TestSlices(t *testing.T) {
	store := config.New(config.MapSource{
		"ss": "a, b ,c,,d",
		"is": "1,2,3,4",
		"ib": "1,notanint,3",
	})
	require.Equal(t, []string{"a", "b", "c", "d"}, store.StringSlice("ss", ",", nil))
	require.Equal(t, []int{1, 2, 3, 4}, store.IntSlice("is", ",", nil))
	require.Nil(t, store.IntSlice("ib", ",", nil))                                    // any parse failure -> default
	require.Equal(t, []string{"d"}, store.StringSlice("missing", ",", []string{"d"})) // missing -> default
}

func TestUnmarshal(t *testing.T) {
	store := config.New(config.MapSource{
		"obj": `{"host":"h","port":6379}`,
	})
	var dst struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	}
	require.NoError(t, store.Unmarshal("obj", &dst))
	require.Equal(t, "h", dst.Host)
	require.Equal(t, 6379, dst.Port)

	// Missing key -> ErrMissing.
	err := store.Unmarshal("nope", &dst)
	require.ErrorIs(t, err, config.ErrMissing)

	// Present but bad JSON -> the json error.
	store2 := config.New(config.MapSource{"bad": "{"})
	err = store2.Unmarshal("bad", &dst)
	require.Error(t, err)
	require.False(t, errors.Is(err, config.ErrMissing))
}

func TestEmptyStore(t *testing.T) {
	store := config.New()
	require.False(t, store.Has("x"))
	require.Equal(t, "d", store.String("x", "d"))
	require.Equal(t, 7, store.Int("x", 7))
}
