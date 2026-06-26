package latency

import "time"

// Options configures a [Histogram]. Zero values are replaced with defaults by
// withDefaults, so the zero Options yields a histogram with a 60s window and
// [DefaultBoundaries].
//
// Field tags carry both json and mapstructure names so the struct can be
// loaded from either a JSON config or a Viper-style mapstructure source,
// matching the options structs in the breaker/limiter/httpclient packages.
type Options struct {
	// Window is the sliding-window size. Samples older than Window are dropped
	// from quantile/snapshot calculations (lazily, on the next observe/read).
	// Default 60s; values below 1s are raised to 1s.
	Window time.Duration `json:"window" mapstructure:"window"`

	// Boundaries are the bucket upper bounds (monotonic increasing, all > 0).
	// nil/empty selects [DefaultBoundaries]. A non-empty but invalid set
	// (non-monotonic, or containing a value <= 0) causes [NewHistogram] to
	// return nil — treat it as a configuration error.
	Boundaries []time.Duration `json:"boundaries" mapstructure:"boundaries"`
}

// defaultOptions returns the package defaults used to fill zero option fields.
func defaultOptions() Options {
	return Options{
		Window:     60 * time.Second,
		Boundaries: DefaultBoundaries[:],
	}
}

// withDefaults returns a copy of o with a zero Window replaced by the default
// (>=1s) and an empty Boundaries replaced by [DefaultBoundaries]. A non-empty
// Boundaries is left untouched here; [NewHistogram] validates it afterwards.
func (o Options) withDefaults() Options {
	d := defaultOptions()
	if o.Window < time.Second {
		o.Window = d.Window
	}
	if len(o.Boundaries) == 0 {
		o.Boundaries = d.Boundaries
	}
	return o
}
