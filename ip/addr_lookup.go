package ip

import "net"

// AddrLookup is the net subsystem subset used by the local-IP / MAC helpers.
// The default implementation (netLookup) bridges net.InterfaceAddrs and
// net.Interfaces. Tests may inject a mockery-generated mock to drive
// error-path coverage without syscall patching (gomonkey used to patch
// net.InterfaceAddrs / net.Interfaces).
//
// Public package API signatures (GetIPAll, PrivateIP, MacAddress, ...) are
// unchanged; injection is through the package-level DefaultAddrLookup
// variable, which tests swap (defer restore) to inject errors.
type AddrLookup interface {
	// InterfaceAddrs mirrors net.InterfaceAddrs.
	InterfaceAddrs() ([]net.Addr, error)
	// Interfaces mirrors net.Interfaces.
	Interfaces() ([]net.Interface, error)
}

// Compile-time interface assertion: guard that netLookup stays in sync with the
// AddrLookup contract.
var _ AddrLookup = netLookup{}

// netLookup is the default AddrLookup delegating to the standard library.
type netLookup struct{}

// InterfaceAddrs implements AddrLookup.
func (netLookup) InterfaceAddrs() ([]net.Addr, error) { return net.InterfaceAddrs() }

// Interfaces implements AddrLookup.
func (netLookup) Interfaces() ([]net.Interface, error) { return net.Interfaces() }

// DefaultAddrLookup is the AddrLookup used by the package functions. It
// defaults to the standard library net.* calls; tests may temporarily replace
// it (defer restore) to inject errors or fake interface lists.
//
//go:generate mockery --name AddrLookup --inpackage --with-expecter --filename mock_AddrLookup.go
var DefaultAddrLookup AddrLookup = netLookup{}
