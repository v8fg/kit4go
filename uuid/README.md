# [UUID](https://en.wikipedia.org/wiki/Universally_unique_identifier)

>**Universally Unique Identifier**

Several uuid libraries are commonly used for integration.

- [go.uuid](https://github.com/satori/go.uuid)
- [ksuid](https://github.com/segmentio/ksuid)
- [xid](https://github.com/rs/xid) 

## Compare

|Name|Binary Size| String Size | Features                      | Notes                                             |
|:---|:---|:------------|:------------------------------|:--------------------------------------------------|
|[go.uuid](https://github.com/satori/go.uuid)|16 bytes| 36 chars    |configuration free, not sortable| recommended **V4**                                    |
|[ksuid](https://github.com/segmentio/ksuid) |20 bytes| 27 chars    | configuration free, sortable   | collision-free, coordination-free, dependency-free|
|[xid](https://github.com/rs/xid)            |12 bytes| 20 chars    | configuration free, sortable   | not cryptographically secure                      |

## Notes

- xid: dependent on the system time, a monotonic counter and so is not cryptographically secure.

## Features

>go.uuid

- ~~Version 1~~, based on timestamp and MAC address (RFC 4122)
- ~~Version 2~~, based on timestamp, MAC address and POSIX UID/GID (DCE 1.1)
- *Version 3*, based on MD5 hashing (RFC 4122)
- **Version 4**, based on random numbers (RFC 4122)
- *Version 5*, based on SHA-1 hashing (RFC 4122)

>ksuid

1. Naturally ordered by generation time 
2. Collision-free, coordination-free, dependency-free 
3. Highly portable representations
4. 20-bytes: a 32-bit unsigned integer UTC timestamp and a 128-bit randomly generated payload. 
   1. The timestamp uses big-endian encoding, to support lexicographic sorting. 
   2. The timestamp epoch is adjusted to May 13th, 2014, providing over 100 years of life. 
   3. The payload is generated by a cryptographically-strong pseudorandom number generator.

>xid

- Size: 12 bytes (96 bits), smaller than UUID, larger than snowflake
- Base32 hex encoded by default (20 chars when transported as printable string, still sortable)
- Non configured, you don't need set a unique machine and/or data center id
- K-ordered
- Embedded time with 1 second precision
- Unicity guaranteed for 16,777,216 (24 bits) unique ids per second and per host/process
- Lock-free (i.e.: unlike UUIDv1 and v2)

## Benchmarks

```text
goos: darwin
goarch: amd64
pkg: github.com/v8fg/kit4go/uuid
cpu: Intel(R) Core(TM) i5-8279U CPU @ 2.40GHz
BenchmarkRequestID
BenchmarkRequestID-8                     1525274               778.1 ns/op            48 B/op          2 allocs/op
BenchmarkRequestIDCanonicalFormat
BenchmarkRequestIDCanonicalFormat-8      1529751               788.2 ns/op            64 B/op          2 allocs/op
BenchmarkNewV4
BenchmarkNewV4-8                         1677464               740.2 ns/op            16 B/op          1 allocs/op
BenchmarkNewKSUID
BenchmarkNewKSUID-8                      1453513               794.9 ns/op             0 B/op          0 allocs/op
BenchmarkNewKSUIDRandomWithTime
BenchmarkNewKSUIDRandomWithTime-8        1534045               760.3 ns/op             0 B/op          0 allocs/op
BenchmarkNewXID
BenchmarkNewXID-8                       13338049                85.44 ns/op            0 B/op          0 allocs/op
BenchmarkNewXIDWithTime
BenchmarkNewXIDWithTime-8               14852886                80.44 ns/op            0 B/op          0 allocs/op
```
