package money

import "sync"

// currency registry — ISO 4217. Mutations guarded by a RWMutex so custom
// currencies can be registered at init time from any goroutine.
var (
	currenciesMu sync.RWMutex
	currencies   = map[string]Currency{}
)

// Lookup reports the Currency registered under code (case-insensitive). Codes
// are normalized to upper case.
func Lookup(code string) (Currency, bool) {
	currenciesMu.RLock()
	defer currenciesMu.RUnlock()
	c, ok := currencies[upperASCII(code)]
	return c, ok
}

// MustCurrency is Lookup that panics when code is unknown. Use for codes that
// are compile-time-known to exist.
func MustCurrency(code string) Currency {
	c, ok := Lookup(code)
	if !ok {
		panic("money: unknown currency: " + code)
	}
	return c
}

// RegisterCurrency adds or replaces a currency in the registry. Intended for
// init-time registration of non-ISO or token currencies.
func RegisterCurrency(c Currency) {
	if c.Code == "" {
		return
	}
	currenciesMu.Lock()
	currencies[upperASCII(c.Code)] = c
	currenciesMu.Unlock()
}

// Currencies returns a snapshot of all registered currency codes.
func Currencies() []string {
	currenciesMu.RLock()
	defer currenciesMu.RUnlock()
	out := make([]string, 0, len(currencies))
	for code := range currencies {
		out = append(out, code)
	}
	return out
}

func upperASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			b[i] = c - 32
		}
	}
	return string(b)
}

func init() {
	for _, c := range iso4217 {
		currencies[upperASCII(c.Code)] = c
	}
}

// iso4217 is a curated set of commonly traded world currencies (code, numeric,
// decimals). Not exhaustive; RegisterCurrency adds the rest.
var iso4217 = []Currency{
	{"USD", "840", 2}, {"EUR", "978", 2}, {"GBP", "826", 2}, {"JPY", "392", 0},
	{"CHF", "756", 2}, {"CAD", "124", 2}, {"AUD", "036", 2}, {"NZD", "554", 2},
	{"CNY", "156", 2}, {"HKD", "344", 2}, {"SGD", "702", 2}, {"INR", "356", 2},
	{"KRW", "410", 0}, {"BRL", "986", 2}, {"MXN", "484", 2}, {"RUB", "643", 2},
	{"ZAR", "710", 2}, {"TRY", "949", 2}, {"SEK", "752", 2}, {"NOK", "578", 2},
	{"DKK", "208", 2}, {"PLN", "985", 2}, {"THB", "764", 2}, {"IDR", "360", 2},
	{"MYR", "458", 2}, {"PHP", "608", 2}, {"VND", "704", 0}, {"ILS", "376", 2},
	{"AED", "784", 2}, {"SAR", "682", 2}, {"KWD", "414", 3}, {"BHD", "048", 3},
	{"OMR", "512", 3}, {"JOD", "400", 3}, {"CLP", "152", 0}, {"COP", "170", 2},
	{"PEN", "604", 2}, {"ARS", "032", 2}, {"TWD", "901", 2}, {"CZK", "203", 2},
	{"HUF", "348", 2}, {"RON", "946", 2}, {"NGN", "566", 2}, {"PKR", "586", 2},
	{"BDT", "050", 2}, {"EGP", "818", 2}, {"QAR", "634", 2}, {"KZT", "398", 2},
}
