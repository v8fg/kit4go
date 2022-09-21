package ip_test

import (
	"bytes"
	"fmt"
	"math"
	"math/big"
	"net"

	"github.com/v8fg/kit4go/ip"
)

// ipv6Max, _ := big.NewInt(0).SetString("340282366920938463463374607431768211455", 10)
var ipv6MaxBytes = bytes.Repeat([]byte{0xff}, 16)

func ExampleBytesIPToIPv4Number() {

	ipSet := [][]byte{
		nil,
		big.NewInt(0).Bytes(),
		big.NewInt(1).Bytes(),
		{0, 0, 0, 0},
		{0, 0, 0, 1},
		{10, 10, 10, 1},
		big.NewInt(math.MaxInt32).Bytes(),
		big.NewInt(math.MaxUint32).Bytes(),
		{0xff, 0xff, 0xff, 0xff},
		append([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff}, []byte{0, 0, 0, 0}...),
		append([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff}, []byte{0, 0, 0, 1}...),
		ipv6MaxBytes,
		bytes.Repeat([]byte{0xff}, 16),
		append(bytes.Repeat([]byte{0}, 12), bytes.Repeat([]byte{0xff}, 4)...), // ipv6
		bytes.Repeat([]byte{0xff}, 18),
	}

	var ipRet *big.Int
	for _, _ip := range ipSet {
		ipRet = ip.BytesIPToIPv4Number(_ip)
		fmt.Printf("[BytesIPToIPv4Number] result: %39v, ip: %v\n", ipRet, _ip)
	}

	// output:
	// [BytesIPToIPv4Number] result:                                       0, ip: []
	// [BytesIPToIPv4Number] result:                                       0, ip: []
	// [BytesIPToIPv4Number] result:                                       0, ip: [1]
	// [BytesIPToIPv4Number] result:                                       0, ip: [0 0 0 0]
	// [BytesIPToIPv4Number] result:                                       1, ip: [0 0 0 1]
	// [BytesIPToIPv4Number] result:                               168430081, ip: [10 10 10 1]
	// [BytesIPToIPv4Number] result:                              2147483647, ip: [127 255 255 255]
	// [BytesIPToIPv4Number] result:                              4294967295, ip: [255 255 255 255]
	// [BytesIPToIPv4Number] result:                              4294967295, ip: [255 255 255 255]
	// [BytesIPToIPv4Number] result:                                       0, ip: [0 0 0 0 0 0 0 0 0 0 255 255 0 0 0 0]
	// [BytesIPToIPv4Number] result:                                       1, ip: [0 0 0 0 0 0 0 0 0 0 255 255 0 0 0 1]
	// [BytesIPToIPv4Number] result:                              4294967295, ip: [255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255]
	// [BytesIPToIPv4Number] result:                              4294967295, ip: [255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255]
	// [BytesIPToIPv4Number] result:                              4294967295, ip: [0 0 0 0 0 0 0 0 0 0 0 0 255 255 255 255]
	// [BytesIPToIPv4Number] result:                                       0, ip: [255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255]

}

func ExampleBytesIPToNumber() {
	ipSet := [][]byte{
		nil,
		big.NewInt(0).Bytes(),
		big.NewInt(1).Bytes(),
		{0, 0, 0, 0},
		{0, 0, 0, 1},
		{10, 10, 10, 1},
		big.NewInt(math.MaxInt32).Bytes(),
		big.NewInt(math.MaxUint32).Bytes(),
		{0xff, 0xff, 0xff, 0xff},
		append([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff}, []byte{0, 0, 0, 0}...),
		append([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff}, []byte{0, 0, 0, 1}...),
		ipv6MaxBytes,
		bytes.Repeat([]byte{0xff}, 16),
		append(bytes.Repeat([]byte{0}, 12), bytes.Repeat([]byte{0xff}, 4)...), // ipv6
		bytes.Repeat([]byte{0xff}, 18),
	}

	var ipRet *big.Int
	for _, _ip := range ipSet {
		ipRet = ip.BytesIPToNumber(_ip)
		fmt.Printf("[BytesIPToNumber] result: %39v, ip: %v\n", ipRet, _ip)
	}

	// output:
	// [BytesIPToNumber] result:                                       0, ip: []
	// [BytesIPToNumber] result:                                       0, ip: []
	// [BytesIPToNumber] result:                                       0, ip: [1]
	// [BytesIPToNumber] result:                                       0, ip: [0 0 0 0]
	// [BytesIPToNumber] result:                                       1, ip: [0 0 0 1]
	// [BytesIPToNumber] result:                               168430081, ip: [10 10 10 1]
	// [BytesIPToNumber] result:                              2147483647, ip: [127 255 255 255]
	// [BytesIPToNumber] result:                              4294967295, ip: [255 255 255 255]
	// [BytesIPToNumber] result:                              4294967295, ip: [255 255 255 255]
	// [BytesIPToNumber] result:                         281470681743360, ip: [0 0 0 0 0 0 0 0 0 0 255 255 0 0 0 0]
	// [BytesIPToNumber] result:                         281470681743361, ip: [0 0 0 0 0 0 0 0 0 0 255 255 0 0 0 1]
	// [BytesIPToNumber] result: 340282366920938463463374607431768211455, ip: [255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255]
	// [BytesIPToNumber] result: 340282366920938463463374607431768211455, ip: [255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255]
	// [BytesIPToNumber] result:                              4294967295, ip: [0 0 0 0 0 0 0 0 0 0 0 0 255 255 255 255]
	// [BytesIPToNumber] result:                                       0, ip: [255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255]

}

func ExampleBytesIPToStr() {
	ipSet := []net.IP{
		nil,
		big.NewInt(0).Bytes(),
		big.NewInt(1).Bytes(),
		{0, 0, 0, 0},
		{0, 0, 0, 1},
		{10, 10, 10, 1},
		big.NewInt(math.MaxInt32).Bytes(),
		big.NewInt(math.MaxUint32).Bytes(),
		{0xff, 0xff, 0xff, 0xff},
		append([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff}, []byte{0, 0, 0, 0}...),
		append([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff}, []byte{0, 0, 0, 1}...),
		ipv6MaxBytes,
		bytes.Repeat([]byte{0xff}, 16),
		append(bytes.Repeat([]byte{0}, 12), bytes.Repeat([]byte{0xff}, 4)...), // ipv6
		bytes.Repeat([]byte{0xff}, 18),
	}

	flagSet := []ip.Flag{
		ip.FlagVInValid, ip.FlagV4, ip.FlagV6,
	}
	var ipRet string
	for index := range ipSet {
		for _, flag := range flagSet {
			ipRet = ip.BytesIPToStr(ipSet[index], flag)
			fmt.Printf("[BytesIPToStr] ip:%18v, result: %16v, flag:%v\n", ipSet[index], ipRet, flag)
		}

	}

	// output:
	// [BytesIPToStr] ip:             <nil>, result:                 , flag:0
	// [BytesIPToStr] ip:             <nil>, result:                 , flag:4
	// [BytesIPToStr] ip:             <nil>, result:                 , flag:6
	// [BytesIPToStr] ip:             <nil>, result:                 , flag:0
	// [BytesIPToStr] ip:             <nil>, result:                 , flag:4
	// [BytesIPToStr] ip:             <nil>, result:                 , flag:6
	// [BytesIPToStr] ip:               ?01, result:              ?01, flag:0
	// [BytesIPToStr] ip:               ?01, result:                 , flag:4
	// [BytesIPToStr] ip:               ?01, result:                 , flag:6
	// [BytesIPToStr] ip:           0.0.0.0, result:          0.0.0.0, flag:0
	// [BytesIPToStr] ip:           0.0.0.0, result:          0.0.0.0, flag:4
	// [BytesIPToStr] ip:           0.0.0.0, result:                 , flag:6
	// [BytesIPToStr] ip:           0.0.0.1, result:          0.0.0.1, flag:0
	// [BytesIPToStr] ip:           0.0.0.1, result:          0.0.0.1, flag:4
	// [BytesIPToStr] ip:           0.0.0.1, result:                 , flag:6
	// [BytesIPToStr] ip:        10.10.10.1, result:       10.10.10.1, flag:0
	// [BytesIPToStr] ip:        10.10.10.1, result:       10.10.10.1, flag:4
	// [BytesIPToStr] ip:        10.10.10.1, result:                 , flag:6
	// [BytesIPToStr] ip:   127.255.255.255, result:  127.255.255.255, flag:0
	// [BytesIPToStr] ip:   127.255.255.255, result:  127.255.255.255, flag:4
	// [BytesIPToStr] ip:   127.255.255.255, result:                 , flag:6
	// [BytesIPToStr] ip:   255.255.255.255, result:  255.255.255.255, flag:0
	// [BytesIPToStr] ip:   255.255.255.255, result:  255.255.255.255, flag:4
	// [BytesIPToStr] ip:   255.255.255.255, result:                 , flag:6
	// [BytesIPToStr] ip:   255.255.255.255, result:  255.255.255.255, flag:0
	// [BytesIPToStr] ip:   255.255.255.255, result:  255.255.255.255, flag:4
	// [BytesIPToStr] ip:   255.255.255.255, result:                 , flag:6
	// [BytesIPToStr] ip:           0.0.0.0, result:          0.0.0.0, flag:0
	// [BytesIPToStr] ip:           0.0.0.0, result:          0.0.0.0, flag:4
	// [BytesIPToStr] ip:           0.0.0.0, result:       ::ffff:0:0, flag:6
	// [BytesIPToStr] ip:           0.0.0.1, result:          0.0.0.1, flag:0
	// [BytesIPToStr] ip:           0.0.0.1, result:          0.0.0.1, flag:4
	// [BytesIPToStr] ip:           0.0.0.1, result:       ::ffff:0:1, flag:6
	// [BytesIPToStr] ip:ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff, result: ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff, flag:0
	// [BytesIPToStr] ip:ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff, result:                 , flag:4
	// [BytesIPToStr] ip:ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff, result: ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff, flag:6
	// [BytesIPToStr] ip:ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff, result: ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff, flag:0
	// [BytesIPToStr] ip:ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff, result:                 , flag:4
	// [BytesIPToStr] ip:ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff, result: ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff, flag:6
	// [BytesIPToStr] ip:       ::ffff:ffff, result:      ::ffff:ffff, flag:0
	// [BytesIPToStr] ip:       ::ffff:ffff, result:                 , flag:4
	// [BytesIPToStr] ip:       ::ffff:ffff, result:      ::ffff:ffff, flag:6
	// [BytesIPToStr] ip:?ffffffffffffffffffffffffffffffffffff, result: ?ffffffffffffffffffffffffffffffffffff, flag:0
	// [BytesIPToStr] ip:?ffffffffffffffffffffffffffffffffffff, result:                 , flag:4
	// [BytesIPToStr] ip:?ffffffffffffffffffffffffffffffffffff, result:                 , flag:6

}

func ExampleBytesIPToStrIPv4() {
	ipSet := [][]byte{
		nil,
		big.NewInt(0).Bytes(),
		big.NewInt(1).Bytes(),
		{0, 0, 0, 0},
		{0, 0, 0, 1},
		{10, 10, 10, 1},
		big.NewInt(math.MaxInt32).Bytes(),
		big.NewInt(math.MaxUint32).Bytes(),
		{0xff, 0xff, 0xff, 0xff},
		append([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff}, []byte{0, 0, 0, 0}...),
		append([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff}, []byte{0, 0, 0, 1}...),
		ipv6MaxBytes,
		bytes.Repeat([]byte{0xff}, 16),
		append(bytes.Repeat([]byte{0}, 12), bytes.Repeat([]byte{0xff}, 4)...), // ipv6
		bytes.Repeat([]byte{0xff}, 18),
	}

	var ipRet string
	for index := range ipSet {
		ipRet = ip.BytesIPToStrIPv4(ipSet[index])
		fmt.Printf("[BytesIPToStrIPv4] result: %39v, ip: %v\n", ipRet, ipSet[index])
	}

	// output:
	// [BytesIPToStrIPv4] result:                                        , ip: []
	// [BytesIPToStrIPv4] result:                                        , ip: []
	// [BytesIPToStrIPv4] result:                                        , ip: [1]
	// [BytesIPToStrIPv4] result:                                 0.0.0.0, ip: [0 0 0 0]
	// [BytesIPToStrIPv4] result:                                 0.0.0.1, ip: [0 0 0 1]
	// [BytesIPToStrIPv4] result:                              10.10.10.1, ip: [10 10 10 1]
	// [BytesIPToStrIPv4] result:                         127.255.255.255, ip: [127 255 255 255]
	// [BytesIPToStrIPv4] result:                         255.255.255.255, ip: [255 255 255 255]
	// [BytesIPToStrIPv4] result:                         255.255.255.255, ip: [255 255 255 255]
	// [BytesIPToStrIPv4] result:                                 0.0.0.0, ip: [0 0 0 0 0 0 0 0 0 0 255 255 0 0 0 0]
	// [BytesIPToStrIPv4] result:                                 0.0.0.1, ip: [0 0 0 0 0 0 0 0 0 0 255 255 0 0 0 1]
	// [BytesIPToStrIPv4] result:                                        , ip: [255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255]
	// [BytesIPToStrIPv4] result:                                        , ip: [255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255]
	// [BytesIPToStrIPv4] result:                                        , ip: [0 0 0 0 0 0 0 0 0 0 0 0 255 255 255 255]
	// [BytesIPToStrIPv4] result:                                        , ip: [255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255]

}

func ExampleBytesIPToStrIPv6() {
	ipSet := [][]byte{
		nil,
		big.NewInt(0).Bytes(),
		big.NewInt(1).Bytes(),
		{0, 0, 0, 0},
		{0, 0, 0, 1},
		{10, 10, 10, 1},
		big.NewInt(math.MaxInt32).Bytes(),
		big.NewInt(math.MaxUint32).Bytes(),
		{0xff, 0xff, 0xff, 0xff},
		append([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff}, []byte{0, 0, 0, 0}...),
		append([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff}, []byte{0, 0, 0, 1}...),
		ipv6MaxBytes,
		bytes.Repeat([]byte{0xff}, 16),
		append(bytes.Repeat([]byte{0}, 12), bytes.Repeat([]byte{0xff}, 4)...), // ipv6
		bytes.Repeat([]byte{0xff}, 18),
	}

	var ipRet string
	for index := range ipSet {
		ipRet = ip.BytesIPToStrIPv6(ipSet[index])
		fmt.Printf("[BytesIPToStrIPv6] result: %39v, ip: %v\n", ipRet, ipSet[index])
	}

	// output:
	// [BytesIPToStrIPv6] result:                                        , ip: []
	// [BytesIPToStrIPv6] result:                                        , ip: []
	// [BytesIPToStrIPv6] result:                                        , ip: [1]
	// [BytesIPToStrIPv6] result:                                        , ip: [0 0 0 0]
	// [BytesIPToStrIPv6] result:                                        , ip: [0 0 0 1]
	// [BytesIPToStrIPv6] result:                                        , ip: [10 10 10 1]
	// [BytesIPToStrIPv6] result:                                        , ip: [127 255 255 255]
	// [BytesIPToStrIPv6] result:                                        , ip: [255 255 255 255]
	// [BytesIPToStrIPv6] result:                                        , ip: [255 255 255 255]
	// [BytesIPToStrIPv6] result:                              ::ffff:0:0, ip: [0 0 0 0 0 0 0 0 0 0 255 255 0 0 0 0]
	// [BytesIPToStrIPv6] result:                              ::ffff:0:1, ip: [0 0 0 0 0 0 0 0 0 0 255 255 0 0 0 1]
	// [BytesIPToStrIPv6] result: ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff, ip: [255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255]
	// [BytesIPToStrIPv6] result: ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff, ip: [255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255]
	// [BytesIPToStrIPv6] result:                             ::ffff:ffff, ip: [0 0 0 0 0 0 0 0 0 0 0 0 255 255 255 255]
	// [BytesIPToStrIPv6] result:                                        , ip: [255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255]

}

func ExampleCIDRToIPMask() {
	ipSet := []string{
		"",
		"0.0.0.0",
		"0.0.0.0/0",
		"0.0.0.1/0",
		"0.0.0.0/32",
		"0.0.0.1/32",
		"0.0.0.1/33",
		"1024:::/16",
		"1024::/16",
		"2048::/16",
		"2048::/128",
		"2048::/132",
		"2048:8226:6a02::/48",
		"192.168.192.1/20", //  == "::ffff:192.168.192.1/116", except the mask length
		"::192.168.192.1/20",
		"::ffff:192.168.192.1/20",
		"::ffff:192.168.192.1/116",
		"::ffff:192.168.192.1/116",
	}

	var ipMask string
	for index := range ipSet {
		ipMask = ip.CIDRToIPMask(ipSet[index])
		fmt.Printf("[CIDRToIPMask] ip: %24v, result:[%60v]\n", ipSet[index], ipMask)
	}

	// output:
	// [CIDRToIPMask] ip:                         , result:[                                                            ]
	// [CIDRToIPMask] ip:                  0.0.0.0, result:[                                                            ]
	// [CIDRToIPMask] ip:                0.0.0.0/0, result:[                                            0.0.0.0/00000000]
	// [CIDRToIPMask] ip:                0.0.0.1/0, result:[                                            0.0.0.1/00000000]
	// [CIDRToIPMask] ip:               0.0.0.0/32, result:[                                            0.0.0.0/ffffffff]
	// [CIDRToIPMask] ip:               0.0.0.1/32, result:[                                            0.0.0.1/ffffffff]
	// [CIDRToIPMask] ip:               0.0.0.1/33, result:[                                                            ]
	// [CIDRToIPMask] ip:               1024:::/16, result:[                                                            ]
	// [CIDRToIPMask] ip:                1024::/16, result:[                     1024::/ffff0000000000000000000000000000]
	// [CIDRToIPMask] ip:                2048::/16, result:[                     2048::/ffff0000000000000000000000000000]
	// [CIDRToIPMask] ip:               2048::/128, result:[                     2048::/ffffffffffffffffffffffffffffffff]
	// [CIDRToIPMask] ip:               2048::/132, result:[                                                            ]
	// [CIDRToIPMask] ip:      2048:8226:6a02::/48, result:[           2048:8226:6a02::/ffffffffffff00000000000000000000]
	// [CIDRToIPMask] ip:         192.168.192.1/20, result:[                                      192.168.192.1/fffff000]
	// [CIDRToIPMask] ip:       ::192.168.192.1/20, result:[                ::c0a8:c001/fffff000000000000000000000000000]
	// [CIDRToIPMask] ip:  ::ffff:192.168.192.1/20, result:[                                      192.168.192.1/00000000]
	// [CIDRToIPMask] ip: ::ffff:192.168.192.1/116, result:[                                      192.168.192.1/fffff000]
	// [CIDRToIPMask] ip: ::ffff:192.168.192.1/116, result:[                                      192.168.192.1/fffff000]

}

func ExampleInRangeCIDRStr() {
	type args struct {
		cidr  string
		ipStr string
	}

	ipSet := []args{
		{cidr: "", ipStr: ""},
		{cidr: "", ipStr: "192.168.192.1"},
		{cidr: "0.0.0.0/32", ipStr: ""},
		{cidr: "192.168.192.1", ipStr: ""},
		{cidr: "192.168.192.0/0", ipStr: "0.0.0.0"},
		{cidr: "192.168.192.0/0", ipStr: "10.0.0.0"},
		{cidr: "192.168.1.0/24", ipStr: "192.168.192.0"},
		{cidr: "192.168.1.0/24", ipStr: "192.168.1.1"},
		{cidr: "192.168.1.1/32", ipStr: "192.168.1.1"},
		{cidr: "192.168.2.1/32", ipStr: "192.168.1.1"},
		{cidr: "2048:db8::", ipStr: "2048:db8::"},
		{cidr: "2048:db8::/24", ipStr: "2048:db8::1"},
		{cidr: "2048:db8::/24", ipStr: "2048:db8::"},
		{cidr: "2048:db8::/128", ipStr: "2048:db8::1"},
		{cidr: "2048:db8::1/128", ipStr: "2048:db8::1"},
	}

	var ret bool
	for index := range ipSet {
		ret = ip.InRangeCIDRStr(ipSet[index].cidr, ipSet[index].ipStr)
		fmt.Printf("[InRangeCIDRStr] ip:%15v in CIDR:%18v, result: %v\n",
			ipSet[index].ipStr, ipSet[index].cidr, ret)
	}
	// output:
	// [InRangeCIDRStr] ip:                in CIDR:                  , result: false
	// [InRangeCIDRStr] ip:  192.168.192.1 in CIDR:                  , result: false
	// [InRangeCIDRStr] ip:                in CIDR:        0.0.0.0/32, result: false
	// [InRangeCIDRStr] ip:                in CIDR:     192.168.192.1, result: false
	// [InRangeCIDRStr] ip:        0.0.0.0 in CIDR:   192.168.192.0/0, result: true
	// [InRangeCIDRStr] ip:       10.0.0.0 in CIDR:   192.168.192.0/0, result: true
	// [InRangeCIDRStr] ip:  192.168.192.0 in CIDR:    192.168.1.0/24, result: false
	// [InRangeCIDRStr] ip:    192.168.1.1 in CIDR:    192.168.1.0/24, result: true
	// [InRangeCIDRStr] ip:    192.168.1.1 in CIDR:    192.168.1.1/32, result: true
	// [InRangeCIDRStr] ip:    192.168.1.1 in CIDR:    192.168.2.1/32, result: false
	// [InRangeCIDRStr] ip:     2048:db8:: in CIDR:        2048:db8::, result: false
	// [InRangeCIDRStr] ip:    2048:db8::1 in CIDR:     2048:db8::/24, result: true
	// [InRangeCIDRStr] ip:     2048:db8:: in CIDR:     2048:db8::/24, result: true
	// [InRangeCIDRStr] ip:    2048:db8::1 in CIDR:    2048:db8::/128, result: false
	// [InRangeCIDRStr] ip:    2048:db8::1 in CIDR:   2048:db8::1/128, result: true

}

func ExampleInCIDRsOrIPs() {
	cidrOrIPs := []string{
		"192.168.192.0/16",
		"192.168.192.0/24",
		"192.168.192.0/16",
		"192.169.192.1",
		"::ffff:ffff/24",
		"::ffff:ffff:ffff/24",
		"::ffff:ffff:ffff/120",
		"2048:8226:6a02:3822::/48",
	}

	ipSet := []net.IP{
		net.IPv4(192, 168, 192, 0),
		net.IPv4(192, 169, 192, 0),
		net.IPv4(192, 169, 192, 1),
		append(bytes.Repeat([]byte{0}, 12), bytes.Repeat([]byte{255}, 4)...),
		append(bytes.Repeat([]byte{0}, 10), bytes.Repeat([]byte{255}, 6)...), // ipv4
	}

	var ret bool
	var pos int
	for index := range ipSet {
		pos, ret = ip.InCIDRsOrIPs(cidrOrIPs, ipSet[index])
		fmt.Printf("[InCIDRsOrIPs] ip:%15v in CIDR:%18v, result: %5v, pos:%3v\n",
			ipSet[index], cidrOrIPs, ret, pos)
	}
	// output:
	// [InCIDRsOrIPs] ip:  192.168.192.0 in CIDR:[  192.168.192.0/16   192.168.192.0/24   192.168.192.0/16      192.169.192.1     ::ffff:ffff/24 ::ffff:ffff:ffff/24 ::ffff:ffff:ffff/120 2048:8226:6a02:3822::/48], result:  true, pos:  0
	// [InCIDRsOrIPs] ip:  192.169.192.0 in CIDR:[  192.168.192.0/16   192.168.192.0/24   192.168.192.0/16      192.169.192.1     ::ffff:ffff/24 ::ffff:ffff:ffff/24 ::ffff:ffff:ffff/120 2048:8226:6a02:3822::/48], result: false, pos: -1
	// [InCIDRsOrIPs] ip:  192.169.192.1 in CIDR:[  192.168.192.0/16   192.168.192.0/24   192.168.192.0/16      192.169.192.1     ::ffff:ffff/24 ::ffff:ffff:ffff/24 ::ffff:ffff:ffff/120 2048:8226:6a02:3822::/48], result:  true, pos:  3
	// [InCIDRsOrIPs] ip:    ::ffff:ffff in CIDR:[  192.168.192.0/16   192.168.192.0/24   192.168.192.0/16      192.169.192.1     ::ffff:ffff/24 ::ffff:ffff:ffff/24 ::ffff:ffff:ffff/120 2048:8226:6a02:3822::/48], result:  true, pos:  4
	// [InCIDRsOrIPs] ip:255.255.255.255 in CIDR:[  192.168.192.0/16   192.168.192.0/24   192.168.192.0/16      192.169.192.1     ::ffff:ffff/24 ::ffff:ffff:ffff/24 ::ffff:ffff:ffff/120 2048:8226:6a02:3822::/48], result:  true, pos:  6
}

func ExampleInRange() {
	type args struct {
		start string
		end   string
		ipStr string
	}

	ipSet := []args{
		{start: "", end: "", ipStr: ""},
		{start: "", end: "192.168.192.0", ipStr: ""},
		{start: "", end: "192.168.192.0", ipStr: "10.1.0.0"},
		{start: "192.168.0.1", end: "192.168.192.0", ipStr: "192.168.2.1"},
		{start: "192.168.0.1", end: "192.168.192.0", ipStr: "2048:db8::"},
		{start: "2048:db8::", end: "2048:db8::1", ipStr: "2048:db8::"},
		{start: "255.255.255.255", end: "::ffff:ffff:ffff", ipStr: "2048:db8::"},
		{start: "255.255.255.255", end: "::ffff:ffff:ffff", ipStr: "255.255.255.255"},
	}

	var ret bool
	for index := range ipSet {
		ret = ip.InRange(ipSet[index].start, ipSet[index].end, ipSet[index].ipStr)
		fmt.Printf("[InRange] ip:%15v in range[%15v, %18v], result: %v\n",
			ipSet[index].ipStr, ipSet[index].start, ipSet[index].end, ret)
	}
	// output:
	// [InRange] ip:                in range[               ,                   ], result: false
	// [InRange] ip:                in range[               ,      192.168.192.0], result: false
	// [InRange] ip:       10.1.0.0 in range[               ,      192.168.192.0], result: true
	// [InRange] ip:    192.168.2.1 in range[    192.168.0.1,      192.168.192.0], result: true
	// [InRange] ip:     2048:db8:: in range[    192.168.0.1,      192.168.192.0], result: false
	// [InRange] ip:     2048:db8:: in range[     2048:db8::,        2048:db8::1], result: true
	// [InRange] ip:     2048:db8:: in range[255.255.255.255,   ::ffff:ffff:ffff], result: false
	// [InRange] ip:255.255.255.255 in range[255.255.255.255,   ::ffff:ffff:ffff], result: true

}

func ExampleIsV4() {
	ipSet := []string{
		"",
		"-0.0.0.0",
		"1",
		"0.0.0.0",
		"0.0.0.1",
		"10.10.10.1",
		"10.10.10.1/32",
		"255.255.255.255",
		"::ffff:255.255.255.255",
		"::ffff:ffff:ffff",
		"::ffff:ffff:0000",
		"::ffff:ffff:0001",
		"ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff",
		"ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff",
	}

	var V4OrV6 bool
	for index := range ipSet {
		V4OrV6 = ip.IsV4(ipSet[index])
		fmt.Printf("[IsV4] IsV4: %5v, ip:%v\n", V4OrV6, ipSet[index])
	}

	// output:
	// [IsV4] IsV4: false, ip:
	// [IsV4] IsV4: false, ip:-0.0.0.0
	// [IsV4] IsV4: false, ip:1
	// [IsV4] IsV4:  true, ip:0.0.0.0
	// [IsV4] IsV4:  true, ip:0.0.0.1
	// [IsV4] IsV4:  true, ip:10.10.10.1
	// [IsV4] IsV4: false, ip:10.10.10.1/32
	// [IsV4] IsV4:  true, ip:255.255.255.255
	// [IsV4] IsV4:  true, ip:::ffff:255.255.255.255
	// [IsV4] IsV4:  true, ip:::ffff:ffff:ffff
	// [IsV4] IsV4:  true, ip:::ffff:ffff:0000
	// [IsV4] IsV4:  true, ip:::ffff:ffff:0001
	// [IsV4] IsV4: false, ip:ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff
	// [IsV4] IsV4: false, ip:ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff

}

func ExampleIsV4ByBytesIP() {
	ipSet := [][]byte{
		nil,
		big.NewInt(0).Bytes(),
		big.NewInt(1).Bytes(),
		{0, 0, 0, 0},
		{0, 0, 0, 1},
		{10, 10, 10, 1},
		big.NewInt(math.MaxInt32).Bytes(),
		big.NewInt(math.MaxUint32).Bytes(),
		{0xff, 0xff, 0xff, 0xff},
		append([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff}, []byte{0, 0, 0, 0}...),
		append([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff}, []byte{0, 0, 0, 1}...),
		ipv6MaxBytes,
		bytes.Repeat([]byte{0xff}, 16),
		append(bytes.Repeat([]byte{0}, 12), bytes.Repeat([]byte{0xff}, 4)...), // ipv6
		bytes.Repeat([]byte{0xff}, 18),
	}

	var V4OrV6 bool
	for index := range ipSet {
		V4OrV6 = ip.IsV4ByBytesIP(ipSet[index])
		fmt.Printf("[IsV4ByBytesIP] IsV4: %5v, ip:%v\n", V4OrV6, ipSet[index])
	}

	// output:
	// [IsV4ByBytesIP] IsV4: false, ip:[]
	// [IsV4ByBytesIP] IsV4: false, ip:[]
	// [IsV4ByBytesIP] IsV4: false, ip:[1]
	// [IsV4ByBytesIP] IsV4:  true, ip:[0 0 0 0]
	// [IsV4ByBytesIP] IsV4:  true, ip:[0 0 0 1]
	// [IsV4ByBytesIP] IsV4:  true, ip:[10 10 10 1]
	// [IsV4ByBytesIP] IsV4:  true, ip:[127 255 255 255]
	// [IsV4ByBytesIP] IsV4:  true, ip:[255 255 255 255]
	// [IsV4ByBytesIP] IsV4:  true, ip:[255 255 255 255]
	// [IsV4ByBytesIP] IsV4:  true, ip:[0 0 0 0 0 0 0 0 0 0 255 255 0 0 0 0]
	// [IsV4ByBytesIP] IsV4:  true, ip:[0 0 0 0 0 0 0 0 0 0 255 255 0 0 0 1]
	// [IsV4ByBytesIP] IsV4: false, ip:[255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255]
	// [IsV4ByBytesIP] IsV4: false, ip:[255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255]
	// [IsV4ByBytesIP] IsV4: false, ip:[0 0 0 0 0 0 0 0 0 0 0 0 255 255 255 255]
	// [IsV4ByBytesIP] IsV4: false, ip:[255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255]

}

func ExampleIsV6() {
	ipSet := []string{
		"",
		"-0.0.0.0",
		"1",
		"0.0.0.0",
		"0.0.0.1",
		"10.10.10.1",
		"10.10.10.1/32",
		"255.255.255.255",
		"::ffff:255.255.255.255",
		"::ffff:ffff:ffff",
		"::ffff:ffff:0000",
		"::ffff:ffff:0001",
		"ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff",
		"ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff",
	}

	var V4OrV6 bool
	for index := range ipSet {
		V4OrV6 = ip.IsV6(ipSet[index])
		fmt.Printf("[IsV6] IsV6: %5v, ip:%v\n", V4OrV6, ipSet[index])
	}

	// output:
	// [IsV6] IsV6: false, ip:
	// [IsV6] IsV6: false, ip:-0.0.0.0
	// [IsV6] IsV6: false, ip:1
	// [IsV6] IsV6: false, ip:0.0.0.0
	// [IsV6] IsV6: false, ip:0.0.0.1
	// [IsV6] IsV6: false, ip:10.10.10.1
	// [IsV6] IsV6: false, ip:10.10.10.1/32
	// [IsV6] IsV6: false, ip:255.255.255.255
	// [IsV6] IsV6: false, ip:::ffff:255.255.255.255
	// [IsV6] IsV6: false, ip:::ffff:ffff:ffff
	// [IsV6] IsV6: false, ip:::ffff:ffff:0000
	// [IsV6] IsV6: false, ip:::ffff:ffff:0001
	// [IsV6] IsV6:  true, ip:ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff
	// [IsV6] IsV6: false, ip:ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff

}

func ExampleIsV6ByBytesIP() {
	ipSet := [][]byte{
		nil,
		big.NewInt(0).Bytes(),
		big.NewInt(1).Bytes(),
		{0, 0, 0, 0},
		{0, 0, 0, 1},
		{10, 10, 10, 1},
		big.NewInt(math.MaxInt32).Bytes(),
		big.NewInt(math.MaxUint32).Bytes(),
		{0xff, 0xff, 0xff, 0xff},
		append([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff}, []byte{0, 0, 0, 0}...),
		append([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff}, []byte{0, 0, 0, 1}...),
		ipv6MaxBytes,
		bytes.Repeat([]byte{0xff}, 16),
		append(bytes.Repeat([]byte{0}, 12), bytes.Repeat([]byte{0xff}, 4)...), // ipv6
		bytes.Repeat([]byte{0xff}, 18),
	}

	var V4OrV6 bool
	for index := range ipSet {
		V4OrV6 = ip.IsV6ByBytesIP(ipSet[index])
		fmt.Printf("[IsV6ByBytesIP] IsV6: %5v, ip:%v\n", V4OrV6, ipSet[index])
	}

	// output:
	// [IsV6ByBytesIP] IsV6: false, ip:[]
	// [IsV6ByBytesIP] IsV6: false, ip:[]
	// [IsV6ByBytesIP] IsV6: false, ip:[1]
	// [IsV6ByBytesIP] IsV6: false, ip:[0 0 0 0]
	// [IsV6ByBytesIP] IsV6: false, ip:[0 0 0 1]
	// [IsV6ByBytesIP] IsV6: false, ip:[10 10 10 1]
	// [IsV6ByBytesIP] IsV6: false, ip:[127 255 255 255]
	// [IsV6ByBytesIP] IsV6: false, ip:[255 255 255 255]
	// [IsV6ByBytesIP] IsV6: false, ip:[255 255 255 255]
	// [IsV6ByBytesIP] IsV6: false, ip:[0 0 0 0 0 0 0 0 0 0 255 255 0 0 0 0]
	// [IsV6ByBytesIP] IsV6: false, ip:[0 0 0 0 0 0 0 0 0 0 255 255 0 0 0 1]
	// [IsV6ByBytesIP] IsV6:  true, ip:[255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255]
	// [IsV6ByBytesIP] IsV6:  true, ip:[255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255]
	// [IsV6ByBytesIP] IsV6:  true, ip:[0 0 0 0 0 0 0 0 0 0 0 0 255 255 255 255]
	// [IsV6ByBytesIP] IsV6: false, ip:[255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255]

}

func ExampleMaskByte() {
	ipSet := []string{
		"",
		"0.0.0.0/-1",
		"0.0.0.0/33",
		"0.0.0.0/32",
		"0.0.0.1/0",
		"0.0.0.1/32",
		"2048:::/16",
		"2048::/132",
		"2048::/128",
		"192.0.2.1/24",
		"192.0.2.1/32",
		"2048::/16",
		"2048:8226:6a02::/48",
		"::ffff:192.168.192.1/116",
	}

	var ipMask []byte
	for index := range ipSet {
		ipMask = ip.MaskByte(ipSet[index])
		fmt.Printf("[MaskByte] ip: %24v, result: %v\n", ipSet[index], ipMask)
	}

	// output:
	// [MaskByte] ip:                         , result: []
	// [MaskByte] ip:               0.0.0.0/-1, result: []
	// [MaskByte] ip:               0.0.0.0/33, result: []
	// [MaskByte] ip:               0.0.0.0/32, result: [255 255 255 255]
	// [MaskByte] ip:                0.0.0.1/0, result: [0 0 0 0]
	// [MaskByte] ip:               0.0.0.1/32, result: [255 255 255 255]
	// [MaskByte] ip:               2048:::/16, result: []
	// [MaskByte] ip:               2048::/132, result: []
	// [MaskByte] ip:               2048::/128, result: [255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255]
	// [MaskByte] ip:             192.0.2.1/24, result: [255 255 255 0]
	// [MaskByte] ip:             192.0.2.1/32, result: [255 255 255 255]
	// [MaskByte] ip:                2048::/16, result: [255 255 0 0 0 0 0 0 0 0 0 0 0 0 0 0]
	// [MaskByte] ip:      2048:8226:6a02::/48, result: [255 255 255 255 255 255 0 0 0 0 0 0 0 0 0 0]
	// [MaskByte] ip: ::ffff:192.168.192.1/116, result: [255 255 255 255 255 255 255 255 255 255 255 255 255 255 240 0]

}

func ExampleMaskString() {
	ipSet := []string{
		"",
		"0.0.0.0/-1",
		"0.0.0.0/33",
		"0.0.0.0/32",
		"0.0.0.1/0",
		"0.0.0.1/32",
		"2048:::/16",
		"2048::/132",
		"2048::/128",
		"192.0.2.1/24",
		"192.0.2.1/32",
		"2048::/16",
		"2048:8226:6a02::/48",
		"::ffff:192.168.192.1/116",
	}

	var ipMask string
	for index := range ipSet {
		ipMask = ip.MaskString(ipSet[index])
		fmt.Printf("[MaskString] ip: %24v, result:%v\n", ipSet[index], ipMask)
	}

	// output:
	// [MaskString] ip:                         , result:
	// [MaskString] ip:               0.0.0.0/-1, result:
	// [MaskString] ip:               0.0.0.0/33, result:
	// [MaskString] ip:               0.0.0.0/32, result:ffffffff
	// [MaskString] ip:                0.0.0.1/0, result:00000000
	// [MaskString] ip:               0.0.0.1/32, result:ffffffff
	// [MaskString] ip:               2048:::/16, result:
	// [MaskString] ip:               2048::/132, result:
	// [MaskString] ip:               2048::/128, result:ffffffffffffffffffffffffffffffff
	// [MaskString] ip:             192.0.2.1/24, result:ffffff00
	// [MaskString] ip:             192.0.2.1/32, result:ffffffff
	// [MaskString] ip:                2048::/16, result:ffff0000000000000000000000000000
	// [MaskString] ip:      2048:8226:6a02::/48, result:ffffffffffff00000000000000000000
	// [MaskString] ip: ::ffff:192.168.192.1/116, result:fffffffffffffffffffffffffffff000

}

func ExampleMaskIPToCIDR() {
	ipSet := []string{
		"192.0.2.1",
		"192.0.2.1/fff",
		"2048:::/16",
		"2048::/ffffffffffffffffffffffffffffffff",
		"0.0.0.0/ffffffff",
		"0.0.0.1/00000000",
		"0.0.0.1/ffffffff",
		"192.0.2.1/ffffffff",
		"192.0.2.1/ffffff00",
		"192.0.2.1/ffffc000",
		"2048::/ffff0000000000000000000000000000",
		"2048:8226:6a02::/ffffffffffff00000000000000000000",
		"::ffff:192.168.192.1/116",
	}

	var ipMask string
	for index := range ipSet {
		ipMask = ip.MaskIPToCIDR(ipSet[index])
		fmt.Printf("[MaskIPToCIDR] result: %20v, ip: %v\n", ipMask, ipSet[index])
	}

	// output:
	// [MaskIPToCIDR] result:                     , ip: 192.0.2.1
	// [MaskIPToCIDR] result:                     , ip: 192.0.2.1/fff
	// [MaskIPToCIDR] result:                     , ip: 2048:::/16
	// [MaskIPToCIDR] result:           2048::/128, ip: 2048::/ffffffffffffffffffffffffffffffff
	// [MaskIPToCIDR] result:           0.0.0.0/32, ip: 0.0.0.0/ffffffff
	// [MaskIPToCIDR] result:            0.0.0.1/0, ip: 0.0.0.1/00000000
	// [MaskIPToCIDR] result:           0.0.0.1/32, ip: 0.0.0.1/ffffffff
	// [MaskIPToCIDR] result:         192.0.2.1/32, ip: 192.0.2.1/ffffffff
	// [MaskIPToCIDR] result:         192.0.2.1/24, ip: 192.0.2.1/ffffff00
	// [MaskIPToCIDR] result:         192.0.2.1/18, ip: 192.0.2.1/ffffc000
	// [MaskIPToCIDR] result:            2048::/16, ip: 2048::/ffff0000000000000000000000000000
	// [MaskIPToCIDR] result:  2048:8226:6a02::/48, ip: 2048:8226:6a02::/ffffffffffff00000000000000000000
	// [MaskIPToCIDR] result:                     , ip: ::ffff:192.168.192.1/116

}

func ExampleNumberIPv4ToStr() {
	ipSet := []uint32{
		0,
		1,
		math.MaxUint16,
		math.MaxUint16 + 1,
		math.MaxInt32,
		math.MaxInt32 + 1,
		math.MaxUint32,
	}

	var ipRet string
	for _, _ip := range ipSet {
		ipRet = ip.NumberIPv4ToStr(_ip)
		fmt.Printf("[NumberIPv4ToStr] ip: %18v, result: %48v\n", _ip, ipRet)
	}

	// output:
	// [NumberIPv4ToStr] ip:                  0, result:                                          0.0.0.0
	// [NumberIPv4ToStr] ip:                  1, result:                                          0.0.0.1
	// [NumberIPv4ToStr] ip:              65535, result:                                      0.0.255.255
	// [NumberIPv4ToStr] ip:              65536, result:                                          0.1.0.0
	// [NumberIPv4ToStr] ip:         2147483647, result:                                  127.255.255.255
	// [NumberIPv4ToStr] ip:         2147483648, result:                                        128.0.0.0
	// [NumberIPv4ToStr] ip:         4294967295, result:                                  255.255.255.255

}

func ExampleNumberToIP() {
	ipSet := []*big.Int{
		big.NewInt(0),
		big.NewInt(1),
		big.NewInt(math.MaxUint16),
		big.NewInt(math.MaxUint16 + 1),
		big.NewInt(math.MaxInt32),
		big.NewInt(math.MaxInt32 + 1),
		big.NewInt(math.MaxUint32),
		big.NewInt(0).SetBytes(bytes.Repeat([]byte{255}, 12)),
		big.NewInt(0).SetBytes(bytes.Repeat([]byte{255}, 18)),
	}

	flagSet := []ip.Flag{
		ip.FlagVInValid, ip.FlagV4, ip.FlagV6,
	}

	var ipRet net.IP
	for _, _ip := range ipSet {
		for _, flag := range flagSet {
			ipRet = ip.NumberToIP(_ip, flag)
			fmt.Printf("[NumberToIP] result: %32v, flag: %v, ip: %48v\n", ipRet, flag, _ip)
		}

	}

	// output:
	// [NumberToIP] result:                          0.0.0.0, flag: 0, ip:                                                0
	// [NumberToIP] result:                          0.0.0.0, flag: 4, ip:                                                0
	// [NumberToIP] result:                               ::, flag: 6, ip:                                                0
	// [NumberToIP] result:                          0.0.0.1, flag: 0, ip:                                                1
	// [NumberToIP] result:                          0.0.0.1, flag: 4, ip:                                                1
	// [NumberToIP] result:                              ::1, flag: 6, ip:                                                1
	// [NumberToIP] result:                      0.0.255.255, flag: 0, ip:                                            65535
	// [NumberToIP] result:                      0.0.255.255, flag: 4, ip:                                            65535
	// [NumberToIP] result:                           ::ffff, flag: 6, ip:                                            65535
	// [NumberToIP] result:                          0.1.0.0, flag: 0, ip:                                            65536
	// [NumberToIP] result:                          0.1.0.0, flag: 4, ip:                                            65536
	// [NumberToIP] result:                            ::1:0, flag: 6, ip:                                            65536
	// [NumberToIP] result:                  127.255.255.255, flag: 0, ip:                                       2147483647
	// [NumberToIP] result:                  127.255.255.255, flag: 4, ip:                                       2147483647
	// [NumberToIP] result:                      ::7fff:ffff, flag: 6, ip:                                       2147483647
	// [NumberToIP] result:                        128.0.0.0, flag: 0, ip:                                       2147483648
	// [NumberToIP] result:                        128.0.0.0, flag: 4, ip:                                       2147483648
	// [NumberToIP] result:                         ::8000:0, flag: 6, ip:                                       2147483648
	// [NumberToIP] result:                  255.255.255.255, flag: 0, ip:                                       4294967295
	// [NumberToIP] result:                  255.255.255.255, flag: 4, ip:                                       4294967295
	// [NumberToIP] result:                      ::ffff:ffff, flag: 6, ip:                                       4294967295
	// [NumberToIP] result:  ::ffff:ffff:ffff:ffff:ffff:ffff, flag: 0, ip:                    79228162514264337593543950335
	// [NumberToIP] result:                  255.255.255.255, flag: 4, ip:                    79228162514264337593543950335
	// [NumberToIP] result:  ::ffff:ffff:ffff:ffff:ffff:ffff, flag: 6, ip:                    79228162514264337593543950335
	// [NumberToIP] result:                            <nil>, flag: 0, ip:     22300745198530623141535718272648361505980415
	// [NumberToIP] result:                            <nil>, flag: 4, ip:     22300745198530623141535718272648361505980415
	// [NumberToIP] result:                            <nil>, flag: 6, ip:     22300745198530623141535718272648361505980415

}

func ExampleNumberToIPv4() {
	ipSet := []*big.Int{
		big.NewInt(0),
		big.NewInt(1),
		big.NewInt(math.MaxUint16),
		big.NewInt(math.MaxUint16 + 1),
		big.NewInt(math.MaxInt32),
		big.NewInt(math.MaxInt32 + 1),
		big.NewInt(math.MaxUint32),
		big.NewInt(0).SetBytes(bytes.Repeat([]byte{255}, 12)),
		big.NewInt(0).SetBytes(bytes.Repeat([]byte{255}, 18)),
	}

	var ipRet net.IP
	for _, _ip := range ipSet {
		ipRet = ip.NumberToIPv4(_ip)
		fmt.Printf("[NumberToIPv4] result: %32v, ip: %48v\n", ipRet, _ip)

	}

	// output:
	// [NumberToIPv4] result:                          0.0.0.0, ip:                                                0
	// [NumberToIPv4] result:                          0.0.0.1, ip:                                                1
	// [NumberToIPv4] result:                      0.0.255.255, ip:                                            65535
	// [NumberToIPv4] result:                          0.1.0.0, ip:                                            65536
	// [NumberToIPv4] result:                  127.255.255.255, ip:                                       2147483647
	// [NumberToIPv4] result:                        128.0.0.0, ip:                                       2147483648
	// [NumberToIPv4] result:                  255.255.255.255, ip:                                       4294967295
	// [NumberToIPv4] result:                  255.255.255.255, ip:                    79228162514264337593543950335
	// [NumberToIPv4] result:                            <nil>, ip:     22300745198530623141535718272648361505980415

}

func ExampleNumberToIPv6() {
	ipSet := []*big.Int{
		big.NewInt(0),
		big.NewInt(1),
		big.NewInt(math.MaxUint16),
		big.NewInt(math.MaxUint16 + 1),
		big.NewInt(math.MaxInt32),
		big.NewInt(math.MaxInt32 + 1),
		big.NewInt(math.MaxUint32),
		big.NewInt(0).SetBytes(bytes.Repeat([]byte{255}, 12)),
		big.NewInt(0).SetBytes(bytes.Repeat([]byte{255}, 18)),
	}

	var ipRet net.IP
	for _, _ip := range ipSet {
		ipRet = ip.NumberToIPv6(_ip)
		fmt.Printf("[NumberToIPv6] result: %32v, ip: %48v\n", ipRet, _ip)

	}

	// output:
	// [NumberToIPv6] result:                               ::, ip:                                                0
	// [NumberToIPv6] result:                              ::1, ip:                                                1
	// [NumberToIPv6] result:                           ::ffff, ip:                                            65535
	// [NumberToIPv6] result:                            ::1:0, ip:                                            65536
	// [NumberToIPv6] result:                      ::7fff:ffff, ip:                                       2147483647
	// [NumberToIPv6] result:                         ::8000:0, ip:                                       2147483648
	// [NumberToIPv6] result:                      ::ffff:ffff, ip:                                       4294967295
	// [NumberToIPv6] result:  ::ffff:ffff:ffff:ffff:ffff:ffff, ip:                    79228162514264337593543950335
	// [NumberToIPv6] result:                            <nil>, ip:     22300745198530623141535718272648361505980415

}

func ExampleParseCIDR() {
	ipSet := []string{
		"",
		"0.0.0.0",
		"0.0.0.0/0",
		"0.0.0.1/0",
		"0.0.0.0/32",
		"0.0.0.1/32",
		"0.0.0.1/33",
		"1024:::/16",
		"1024::/16",
		"2048::/16",
		"2048::/128",
		"2048::/132",
		"2048:8226:6a02::/48",
		"::192.168.192.1/20",
		"::ffff:192.168.192.1/20",
		"192.168.192.1/20", //  == "::ffff:192.168.192.1/116", except the mask length
		"::ffff:192.168.192.1/116",
	}

	for index := range ipSet {
		flag, ipAddr, ipNet := ip.ParseCIDR(ipSet[index])
		fmt.Printf("[ParseCIDR] ip: %24v, result: [ip_flag=%-7v, ipAddr=%24v, ipNet=%24v], valid:%v\n",
			ipSet[index], flag.String(), ipAddr, ipNet, flag != ip.FlagVInValid)

	}
	// output:
	// [ParseCIDR] ip:                         , result: [ip_flag=invalid, ipAddr=                        , ipNet=                   <nil>], valid:false
	// [ParseCIDR] ip:                  0.0.0.0, result: [ip_flag=invalid, ipAddr=                        , ipNet=                   <nil>], valid:false
	// [ParseCIDR] ip:                0.0.0.0/0, result: [ip_flag=ipv4   , ipAddr=                 0.0.0.0, ipNet=               0.0.0.0/0], valid:true
	// [ParseCIDR] ip:                0.0.0.1/0, result: [ip_flag=ipv4   , ipAddr=                 0.0.0.1, ipNet=               0.0.0.0/0], valid:true
	// [ParseCIDR] ip:               0.0.0.0/32, result: [ip_flag=ipv4   , ipAddr=                 0.0.0.0, ipNet=              0.0.0.0/32], valid:true
	// [ParseCIDR] ip:               0.0.0.1/32, result: [ip_flag=ipv4   , ipAddr=                 0.0.0.1, ipNet=              0.0.0.1/32], valid:true
	// [ParseCIDR] ip:               0.0.0.1/33, result: [ip_flag=invalid, ipAddr=                        , ipNet=                   <nil>], valid:false
	// [ParseCIDR] ip:               1024:::/16, result: [ip_flag=invalid, ipAddr=                        , ipNet=                   <nil>], valid:false
	// [ParseCIDR] ip:                1024::/16, result: [ip_flag=ipv6   , ipAddr=                  1024::, ipNet=               1024::/16], valid:true
	// [ParseCIDR] ip:                2048::/16, result: [ip_flag=ipv6   , ipAddr=                  2048::, ipNet=               2048::/16], valid:true
	// [ParseCIDR] ip:               2048::/128, result: [ip_flag=ipv6   , ipAddr=                  2048::, ipNet=              2048::/128], valid:true
	// [ParseCIDR] ip:               2048::/132, result: [ip_flag=invalid, ipAddr=                        , ipNet=                   <nil>], valid:false
	// [ParseCIDR] ip:      2048:8226:6a02::/48, result: [ip_flag=ipv6   , ipAddr=        2048:8226:6a02::, ipNet=     2048:8226:6a02::/48], valid:true
	// [ParseCIDR] ip:       ::192.168.192.1/20, result: [ip_flag=ipv6   , ipAddr=             ::c0a8:c001, ipNet=                   ::/20], valid:true
	// [ParseCIDR] ip:  ::ffff:192.168.192.1/20, result: [ip_flag=ipv4   , ipAddr=           192.168.192.1, ipNet=                   ::/20], valid:true
	// [ParseCIDR] ip:         192.168.192.1/20, result: [ip_flag=ipv4   , ipAddr=           192.168.192.1, ipNet=        192.168.192.0/20], valid:true
	// [ParseCIDR] ip: ::ffff:192.168.192.1/116, result: [ip_flag=ipv4   , ipAddr=           192.168.192.1, ipNet=        192.168.192.0/20], valid:true

}

func ExampleTextNumberToIPStr() {
	ipSet := []string{
		"",
		big.NewInt(0).Text(2),
		big.NewInt(128).Text(2),
		big.NewInt(128).Text(10),
		big.NewInt(128).Text(16),
		big.NewInt(math.MaxInt16).Text(2),
		big.NewInt(math.MaxInt16).Text(16),
		big.NewInt(math.MaxInt16).Text(10),
		big.NewInt(math.MaxInt32).Text(2),
		big.NewInt(math.MaxInt32).Text(10),
		big.NewInt(math.MaxInt32).Text(16),
		big.NewInt(math.MaxUint32).Text(2),
		big.NewInt(math.MaxUint32).Text(10),
		big.NewInt(math.MaxUint32).Text(16),
	}

	var ipRet string
	baseSet := []int{0, 2, 10, 16}

	for index := range ipSet {
		for _, b := range baseSet {
			ipRet = ip.TextNumberToIPStr(ipSet[index], b)
			if len(ipRet) == 0 {
				fmt.Printf("[TextNumberToIPStr] result: %42v, base: %2v, ip: [%v]\n", ipRet, b, ipSet[index])
				continue
			}

			if b == 2 {
				fmt.Printf("[TextNumberToIPStr] result: %42v, base: %2v, ip: [%v]\n", ipRet, b, ipSet[index])
			} else {
				fmt.Printf("[TextNumberToIPStr] result: %42v, base: %2v, ip: [%v]\n", ipRet, b, ipSet[index])
			}
		}
	}

	// output:
	// [TextNumberToIPStr] result:                                           , base:  0, ip: []
	// [TextNumberToIPStr] result:                                           , base:  2, ip: []
	// [TextNumberToIPStr] result:                                           , base: 10, ip: []
	// [TextNumberToIPStr] result:                                           , base: 16, ip: []
	// [TextNumberToIPStr] result:                                           , base:  0, ip: [0]
	// [TextNumberToIPStr] result:                                    0.0.0.0, base:  2, ip: [0]
	// [TextNumberToIPStr] result:                                    0.0.0.0, base: 10, ip: [0]
	// [TextNumberToIPStr] result:                                    0.0.0.0, base: 16, ip: [0]
	// [TextNumberToIPStr] result:                                           , base:  0, ip: [10000000]
	// [TextNumberToIPStr] result:                                  0.0.0.128, base:  2, ip: [10000000]
	// [TextNumberToIPStr] result:                              0.152.150.128, base: 10, ip: [10000000]
	// [TextNumberToIPStr] result:                                   16.0.0.0, base: 16, ip: [10000000]
	// [TextNumberToIPStr] result:                                           , base:  0, ip: [128]
	// [TextNumberToIPStr] result:                                           , base:  2, ip: [128]
	// [TextNumberToIPStr] result:                                  0.0.0.128, base: 10, ip: [128]
	// [TextNumberToIPStr] result:                                   0.0.1.40, base: 16, ip: [128]
	// [TextNumberToIPStr] result:                                           , base:  0, ip: [80]
	// [TextNumberToIPStr] result:                                           , base:  2, ip: [80]
	// [TextNumberToIPStr] result:                                   0.0.0.80, base: 10, ip: [80]
	// [TextNumberToIPStr] result:                                  0.0.0.128, base: 16, ip: [80]
	// [TextNumberToIPStr] result:                                           , base:  0, ip: [111111111111111]
	// [TextNumberToIPStr] result:                                0.0.127.255, base:  2, ip: [111111111111111]
	// [TextNumberToIPStr] result:                           ::650e:124e:f1c7, base: 10, ip: [111111111111111]
	// [TextNumberToIPStr] result:                       ::111:1111:1111:1111, base: 16, ip: [111111111111111]
	// [TextNumberToIPStr] result:                                           , base:  0, ip: [7fff]
	// [TextNumberToIPStr] result:                                           , base:  2, ip: [7fff]
	// [TextNumberToIPStr] result:                                           , base: 10, ip: [7fff]
	// [TextNumberToIPStr] result:                                0.0.127.255, base: 16, ip: [7fff]
	// [TextNumberToIPStr] result:                                           , base:  0, ip: [32767]
	// [TextNumberToIPStr] result:                                           , base:  2, ip: [32767]
	// [TextNumberToIPStr] result:                                0.0.127.255, base: 10, ip: [32767]
	// [TextNumberToIPStr] result:                                 0.3.39.103, base: 16, ip: [32767]
	// [TextNumberToIPStr] result:                                           , base:  0, ip: [1111111111111111111111111111111]
	// [TextNumberToIPStr] result:                            127.255.255.255, base:  2, ip: [1111111111111111111111111111111]
	// [TextNumberToIPStr] result:           0:e:631:91ca:f8f3:b304:471c:71c7, base: 10, ip: [1111111111111111111111111111111]
	// [TextNumberToIPStr] result:     111:1111:1111:1111:1111:1111:1111:1111, base: 16, ip: [1111111111111111111111111111111]
	// [TextNumberToIPStr] result:                                           , base:  0, ip: [2147483647]
	// [TextNumberToIPStr] result:                                           , base:  2, ip: [2147483647]
	// [TextNumberToIPStr] result:                            127.255.255.255, base: 10, ip: [2147483647]
	// [TextNumberToIPStr] result:                             ::21:4748:3647, base: 16, ip: [2147483647]
	// [TextNumberToIPStr] result:                                           , base:  0, ip: [7fffffff]
	// [TextNumberToIPStr] result:                                           , base:  2, ip: [7fffffff]
	// [TextNumberToIPStr] result:                                           , base: 10, ip: [7fffffff]
	// [TextNumberToIPStr] result:                            127.255.255.255, base: 16, ip: [7fffffff]
	// [TextNumberToIPStr] result:                                           , base:  0, ip: [11111111111111111111111111111111]
	// [TextNumberToIPStr] result:                            255.255.255.255, base:  2, ip: [11111111111111111111111111111111]
	// [TextNumberToIPStr] result:         0:8c:3def:b1ed:b984:fe2a:c71c:71c7, base: 10, ip: [11111111111111111111111111111111]
	// [TextNumberToIPStr] result:    1111:1111:1111:1111:1111:1111:1111:1111, base: 16, ip: [11111111111111111111111111111111]
	// [TextNumberToIPStr] result:                                           , base:  0, ip: [4294967295]
	// [TextNumberToIPStr] result:                                           , base:  2, ip: [4294967295]
	// [TextNumberToIPStr] result:                            255.255.255.255, base: 10, ip: [4294967295]
	// [TextNumberToIPStr] result:                             ::42:9496:7295, base: 16, ip: [4294967295]
	// [TextNumberToIPStr] result:                                           , base:  0, ip: [ffffffff]
	// [TextNumberToIPStr] result:                                           , base:  2, ip: [ffffffff]
	// [TextNumberToIPStr] result:                                           , base: 10, ip: [ffffffff]
	// [TextNumberToIPStr] result:                            255.255.255.255, base: 16, ip: [ffffffff]

}

func ExampleToIP() {
	ipSet := []string{
		"",
		"-0.1.0.0",
		"0.0.0.0",
		"10.1.0.0",
		"10.1.0.0/16",
		"10.1.255.255",
		"::ffff:10.1.255.255",
		"2048:db8::",
		"2048:db8::ffff",
	}

	var ipRet []byte
	for index := range ipSet {
		ipRet = ip.ToIP(ipSet[index])
		fmt.Printf("[ToIP] ip: %20v, result: %v\n", ipSet[index], ipRet)
	}

	// output:
	// [ToIP] ip:                     , result: []
	// [ToIP] ip:             -0.1.0.0, result: []
	// [ToIP] ip:              0.0.0.0, result: [0 0 0 0 0 0 0 0 0 0 255 255 0 0 0 0]
	// [ToIP] ip:             10.1.0.0, result: [0 0 0 0 0 0 0 0 0 0 255 255 10 1 0 0]
	// [ToIP] ip:          10.1.0.0/16, result: []
	// [ToIP] ip:         10.1.255.255, result: [0 0 0 0 0 0 0 0 0 0 255 255 10 1 255 255]
	// [ToIP] ip:  ::ffff:10.1.255.255, result: [0 0 0 0 0 0 0 0 0 0 255 255 10 1 255 255]
	// [ToIP] ip:           2048:db8::, result: [32 72 13 184 0 0 0 0 0 0 0 0 0 0 0 0]
	// [ToIP] ip:       2048:db8::ffff, result: [32 72 13 184 0 0 0 0 0 0 0 0 0 0 255 255]

}

func ExampleToIPReal() {
	ipSet := []string{
		"",
		"-0.1.0.0",
		"0.0.0.0",
		"10.1.0.0",
		"10.1.0.0/16",
		"10.1.255.255",
		"::ffff:10.1.255.255",
		"2048:db8::",
		"2048:db8::ffff",
	}

	var ipRet []byte
	for index := range ipSet {
		ipRet = ip.ToIPReal(ipSet[index])
		fmt.Printf("[ToIPReal] ip: %20v, result: %v\n", ipSet[index], ipRet)
	}

	// output:
	// [ToIPReal] ip:                     , result: []
	// [ToIPReal] ip:             -0.1.0.0, result: []
	// [ToIPReal] ip:              0.0.0.0, result: [0 0 0 0]
	// [ToIPReal] ip:             10.1.0.0, result: [10 1 0 0]
	// [ToIPReal] ip:          10.1.0.0/16, result: []
	// [ToIPReal] ip:         10.1.255.255, result: [10 1 255 255]
	// [ToIPReal] ip:  ::ffff:10.1.255.255, result: [10 1 255 255]
	// [ToIPReal] ip:           2048:db8::, result: [32 72 13 184 0 0 0 0 0 0 0 0 0 0 0 0]
	// [ToIPReal] ip:       2048:db8::ffff, result: [32 72 13 184 0 0 0 0 0 0 0 0 0 0 255 255]

}

func ExampleToNumber() {
	ipSet := []string{
		"",
		"-0.1.0.0",
		"0.0.0.0",
		"10.1.0.0",
		"10.1.0.0/16",
		"10.1.255.255",
		"::ffff:10.1.255.255",
		"2048:db8::",
		"2048:db8::ffff",
	}

	var ipRet *big.Int
	for index := range ipSet {
		ipRet = ip.ToNumber(ipSet[index])
		fmt.Printf("[ToNumber] ip: %20v, base: %v, result: %v\n", ipSet[index], 10, ipRet)
	}

	// output:
	// [ToNumber] ip:                     , base: 10, result: 0
	// [ToNumber] ip:             -0.1.0.0, base: 10, result: 0
	// [ToNumber] ip:              0.0.0.0, base: 10, result: 0
	// [ToNumber] ip:             10.1.0.0, base: 10, result: 167837696
	// [ToNumber] ip:          10.1.0.0/16, base: 10, result: 0
	// [ToNumber] ip:         10.1.255.255, base: 10, result: 167903231
	// [ToNumber] ip:  ::ffff:10.1.255.255, base: 10, result: 167903231
	// [ToNumber] ip:           2048:db8::, base: 10, result: 42909419488238565618529650191028453376
	// [ToNumber] ip:       2048:db8::ffff, base: 10, result: 42909419488238565618529650191028518911

}

func ExampleToIPv4Number() {
	ipSet := []string{
		"",
		"-0.1.0.0",
		"0.0.0.0",
		"10.1.0.0",
		"10.1.0.0/16",
		"10.1.255.255",
		"::ffff:10.1.255.255",
		"2048:db8::",
		"2048:db8::ffff",
	}

	var ipRet uint32
	for index := range ipSet {
		ipRet = ip.ToIPv4Number(ipSet[index])
		fmt.Printf("[ToIPv4Number] ip: %20v, base: %v, result: %v\n", ipSet[index], 10, ipRet)
	}

	// output:
	// [ToIPv4Number] ip:                     , base: 10, result: 0
	// [ToIPv4Number] ip:             -0.1.0.0, base: 10, result: 0
	// [ToIPv4Number] ip:              0.0.0.0, base: 10, result: 0
	// [ToIPv4Number] ip:             10.1.0.0, base: 10, result: 167837696
	// [ToIPv4Number] ip:          10.1.0.0/16, base: 10, result: 0
	// [ToIPv4Number] ip:         10.1.255.255, base: 10, result: 167903231
	// [ToIPv4Number] ip:  ::ffff:10.1.255.255, base: 10, result: 167903231
	// [ToIPv4Number] ip:           2048:db8::, base: 10, result: 0
	// [ToIPv4Number] ip:       2048:db8::ffff, base: 10, result: 0

}

func ExampleToNumberIPv4() {
	ipSet := []string{
		"",
		"-0.1.0.0",
		"0.0.0.0",
		"10.1.0.0",
		"10.1.0.0/16",
		"10.1.255.255",
		"::ffff:10.1.255.255",
		"2048:db8::",
		"2048:db8::ffff",
	}

	var ipRet uint32
	for index := range ipSet {
		ipRet = ip.ToNumberIPv4(ipSet[index])
		fmt.Printf("[ToNumberIPv4] ip: %20v, base: %v, result: %v\n", ipSet[index], 10, ipRet)
	}

	// output:
	// [ToNumberIPv4] ip:                     , base: 10, result: 0
	// [ToNumberIPv4] ip:             -0.1.0.0, base: 10, result: 0
	// [ToNumberIPv4] ip:              0.0.0.0, base: 10, result: 0
	// [ToNumberIPv4] ip:             10.1.0.0, base: 10, result: 167837696
	// [ToNumberIPv4] ip:          10.1.0.0/16, base: 10, result: 0
	// [ToNumberIPv4] ip:         10.1.255.255, base: 10, result: 167903231
	// [ToNumberIPv4] ip:  ::ffff:10.1.255.255, base: 10, result: 167903231
	// [ToNumberIPv4] ip:           2048:db8::, base: 10, result: 0
	// [ToNumberIPv4] ip:       2048:db8::ffff, base: 10, result: 65535

}

func ExampleToStrIP() {
	ipSet := []string{
		"",
		"-0.1.0.0",
		"0.0.0.0",
		"10.1.0.0",
		"10.1.0.0/16",
		"10.1.255.255",
		"::ffff:10.1.255.255",
		"2048:db8::",
		"2048:db8::ffff",
	}

	flagSet := []ip.Flag{
		ip.FlagVInValid, ip.FlagV4, ip.FlagV6,
	}
	var ipRet string
	for index := range ipSet {
		for _, flag := range flagSet {
			ipRet = ip.ToStrIP(ipSet[index], flag)
			fmt.Printf("[ToStrIP] flag:%v, ip: %20v, result: [%v]\n", flag, ipSet[index], ipRet)
		}

	}

	// output:
	// [ToStrIP] flag:0, ip:                     , result: []
	// [ToStrIP] flag:4, ip:                     , result: []
	// [ToStrIP] flag:6, ip:                     , result: []
	// [ToStrIP] flag:0, ip:             -0.1.0.0, result: []
	// [ToStrIP] flag:4, ip:             -0.1.0.0, result: []
	// [ToStrIP] flag:6, ip:             -0.1.0.0, result: []
	// [ToStrIP] flag:0, ip:              0.0.0.0, result: [0.0.0.0]
	// [ToStrIP] flag:4, ip:              0.0.0.0, result: [0.0.0.0]
	// [ToStrIP] flag:6, ip:              0.0.0.0, result: [::ffff:0:0]
	// [ToStrIP] flag:0, ip:             10.1.0.0, result: [10.1.0.0]
	// [ToStrIP] flag:4, ip:             10.1.0.0, result: [10.1.0.0]
	// [ToStrIP] flag:6, ip:             10.1.0.0, result: [::ffff:a01:0]
	// [ToStrIP] flag:0, ip:          10.1.0.0/16, result: []
	// [ToStrIP] flag:4, ip:          10.1.0.0/16, result: []
	// [ToStrIP] flag:6, ip:          10.1.0.0/16, result: []
	// [ToStrIP] flag:0, ip:         10.1.255.255, result: [10.1.255.255]
	// [ToStrIP] flag:4, ip:         10.1.255.255, result: [10.1.255.255]
	// [ToStrIP] flag:6, ip:         10.1.255.255, result: [::ffff:a01:ffff]
	// [ToStrIP] flag:0, ip:  ::ffff:10.1.255.255, result: [10.1.255.255]
	// [ToStrIP] flag:4, ip:  ::ffff:10.1.255.255, result: [10.1.255.255]
	// [ToStrIP] flag:6, ip:  ::ffff:10.1.255.255, result: [::ffff:a01:ffff]
	// [ToStrIP] flag:0, ip:           2048:db8::, result: [2048:db8::]
	// [ToStrIP] flag:4, ip:           2048:db8::, result: []
	// [ToStrIP] flag:6, ip:           2048:db8::, result: [2048:db8::]
	// [ToStrIP] flag:0, ip:       2048:db8::ffff, result: [2048:db8::ffff]
	// [ToStrIP] flag:4, ip:       2048:db8::ffff, result: []
	// [ToStrIP] flag:6, ip:       2048:db8::ffff, result: [2048:db8::ffff]

}

func ExampleToStrIPv4() {
	ipSet := []string{
		"",
		"-0.1.0.0",
		"0.0.0.0",
		"10.1.0.0",
		"10.1.0.0/16",
		"10.1.255.255",
		"::ffff:10.1.255.255",
		"2048:db8::",
		"2048:db8::ffff",
	}

	var ipRet string
	for index := range ipSet {
		ipRet = ip.ToStrIPv4(ipSet[index])
		fmt.Printf("[ToStrIPv4] ip: %20v, result:%v\n", ipSet[index], ipRet)
	}

	// output:
	// [ToStrIPv4] ip:                     , result:
	// [ToStrIPv4] ip:             -0.1.0.0, result:
	// [ToStrIPv4] ip:              0.0.0.0, result:0.0.0.0
	// [ToStrIPv4] ip:             10.1.0.0, result:10.1.0.0
	// [ToStrIPv4] ip:          10.1.0.0/16, result:
	// [ToStrIPv4] ip:         10.1.255.255, result:10.1.255.255
	// [ToStrIPv4] ip:  ::ffff:10.1.255.255, result:10.1.255.255
	// [ToStrIPv4] ip:           2048:db8::, result:
	// [ToStrIPv4] ip:       2048:db8::ffff, result:

}

func ExampleToStrIPv6() {
	ipSet := []string{
		"",
		"-0.1.0.0",
		"0.0.0.0",
		"10.1.0.0",
		"10.1.0.0/16",
		"10.1.255.255",
		"::ffff:10.1.255.255",
		"2048:db8::",
		"2048:db8::ffff",
	}

	var ipRet string
	for index := range ipSet {
		ipRet = ip.ToStrIPv6(ipSet[index])
		fmt.Printf("[ToStrIPv6] ip: %20v, result:[%v]\n", ipSet[index], ipRet)
	}

	// output:
	// [ToStrIPv6] ip:                     , result:[]
	// [ToStrIPv6] ip:             -0.1.0.0, result:[]
	// [ToStrIPv6] ip:              0.0.0.0, result:[::ffff:0:0]
	// [ToStrIPv6] ip:             10.1.0.0, result:[::ffff:a01:0]
	// [ToStrIPv6] ip:          10.1.0.0/16, result:[]
	// [ToStrIPv6] ip:         10.1.255.255, result:[::ffff:a01:ffff]
	// [ToStrIPv6] ip:  ::ffff:10.1.255.255, result:[::ffff:a01:ffff]
	// [ToStrIPv6] ip:           2048:db8::, result:[2048:db8::]
	// [ToStrIPv6] ip:       2048:db8::ffff, result:[2048:db8::ffff]

}

func ExampleToTextIPv4Number() {
	ipSet := []string{
		"",
		"-0.1.0.0",
		"0.0.0.0",
		"10.1.0.0",
		"10.1.0.0/16",
		"10.1.255.255",
		"::ffff:10.1.255.255",
		"2048:db8::",
		"2048:db8::ffff",
	}

	var ipRet string
	baseSet := []int{2, 10, 16, 64}
	for index := range ipSet {
		for _, b := range baseSet {
			ipRet = ip.ToTextIPv4Number(ipSet[index], b)
			if len(ipRet) == 0 {
				fmt.Printf("[ToTextIPv4Number] ip: %20v, base: %2v, result: %32v, min_len:%3v\n", ipSet[index], b, ipRet, len(ipRet))
				continue
			}

			if b == 2 {
				fmt.Printf("[ToTextIPv4Number] ip: %20v, base: %2v, result: %032v, min_len:%3v\n", ipSet[index], b, ipRet, len(ipRet))
			} else {
				fmt.Printf("[ToTextIPv4Number] ip: %20v, base: %2v, result: %32v, min_len:%3v\n", ipSet[index], b, ipRet, len(ipRet))
			}
		}
	}

	// output:
	// [ToTextIPv4Number] ip:                     , base:  2, result:                                 , min_len:  0
	// [ToTextIPv4Number] ip:                     , base: 10, result:                                 , min_len:  0
	// [ToTextIPv4Number] ip:                     , base: 16, result:                                 , min_len:  0
	// [ToTextIPv4Number] ip:                     , base: 64, result:                                 , min_len:  0
	// [ToTextIPv4Number] ip:             -0.1.0.0, base:  2, result:                                 , min_len:  0
	// [ToTextIPv4Number] ip:             -0.1.0.0, base: 10, result:                                 , min_len:  0
	// [ToTextIPv4Number] ip:             -0.1.0.0, base: 16, result:                                 , min_len:  0
	// [ToTextIPv4Number] ip:             -0.1.0.0, base: 64, result:                                 , min_len:  0
	// [ToTextIPv4Number] ip:              0.0.0.0, base:  2, result: 00000000000000000000000000000000, min_len:  1
	// [ToTextIPv4Number] ip:              0.0.0.0, base: 10, result:                                0, min_len:  1
	// [ToTextIPv4Number] ip:              0.0.0.0, base: 16, result:                                0, min_len:  1
	// [ToTextIPv4Number] ip:              0.0.0.0, base: 64, result:                                 , min_len:  0
	// [ToTextIPv4Number] ip:             10.1.0.0, base:  2, result: 00001010000000010000000000000000, min_len: 28
	// [ToTextIPv4Number] ip:             10.1.0.0, base: 10, result:                        167837696, min_len:  9
	// [ToTextIPv4Number] ip:             10.1.0.0, base: 16, result:                          a010000, min_len:  7
	// [ToTextIPv4Number] ip:             10.1.0.0, base: 64, result:                                 , min_len:  0
	// [ToTextIPv4Number] ip:          10.1.0.0/16, base:  2, result:                                 , min_len:  0
	// [ToTextIPv4Number] ip:          10.1.0.0/16, base: 10, result:                                 , min_len:  0
	// [ToTextIPv4Number] ip:          10.1.0.0/16, base: 16, result:                                 , min_len:  0
	// [ToTextIPv4Number] ip:          10.1.0.0/16, base: 64, result:                                 , min_len:  0
	// [ToTextIPv4Number] ip:         10.1.255.255, base:  2, result: 00001010000000011111111111111111, min_len: 28
	// [ToTextIPv4Number] ip:         10.1.255.255, base: 10, result:                        167903231, min_len:  9
	// [ToTextIPv4Number] ip:         10.1.255.255, base: 16, result:                          a01ffff, min_len:  7
	// [ToTextIPv4Number] ip:         10.1.255.255, base: 64, result:                                 , min_len:  0
	// [ToTextIPv4Number] ip:  ::ffff:10.1.255.255, base:  2, result: 00001010000000011111111111111111, min_len: 28
	// [ToTextIPv4Number] ip:  ::ffff:10.1.255.255, base: 10, result:                        167903231, min_len:  9
	// [ToTextIPv4Number] ip:  ::ffff:10.1.255.255, base: 16, result:                          a01ffff, min_len:  7
	// [ToTextIPv4Number] ip:  ::ffff:10.1.255.255, base: 64, result:                                 , min_len:  0
	// [ToTextIPv4Number] ip:           2048:db8::, base:  2, result: 00000000000000000000000000000000, min_len:  1
	// [ToTextIPv4Number] ip:           2048:db8::, base: 10, result:                                0, min_len:  1
	// [ToTextIPv4Number] ip:           2048:db8::, base: 16, result:                                0, min_len:  1
	// [ToTextIPv4Number] ip:           2048:db8::, base: 64, result:                                 , min_len:  0
	// [ToTextIPv4Number] ip:       2048:db8::ffff, base:  2, result: 00000000000000001111111111111111, min_len: 16
	// [ToTextIPv4Number] ip:       2048:db8::ffff, base: 10, result:                            65535, min_len:  5
	// [ToTextIPv4Number] ip:       2048:db8::ffff, base: 16, result:                             ffff, min_len:  4
	// [ToTextIPv4Number] ip:       2048:db8::ffff, base: 64, result:                                 , min_len:  0

}

func ExampleToTextIPv4NumberDecimal() {
	ipSet := []string{
		"",
		"-0.1.0.0",
		"0.0.0.0",
		"10.1.0.0",
		"10.1.0.0/16",
		"10.1.255.255",
		"::ffff:10.1.255.255",
		"2048:db8::",
		"2048:db8::ffff",
	}

	var ipRet string
	for index := range ipSet {
		ipRet = ip.ToTextIPv4NumberDecimal(ipSet[index])
		fmt.Printf("[ToTextIPv4NumberDecimal] ip: %20v, result: %12v, min_len:%3v\n", ipSet[index], ipRet, len(ipRet))
	}

	// output:
	// [ToTextIPv4NumberDecimal] ip:                     , result:             , min_len:  0
	// [ToTextIPv4NumberDecimal] ip:             -0.1.0.0, result:             , min_len:  0
	// [ToTextIPv4NumberDecimal] ip:              0.0.0.0, result:            0, min_len:  1
	// [ToTextIPv4NumberDecimal] ip:             10.1.0.0, result:    167837696, min_len:  9
	// [ToTextIPv4NumberDecimal] ip:          10.1.0.0/16, result:             , min_len:  0
	// [ToTextIPv4NumberDecimal] ip:         10.1.255.255, result:    167903231, min_len:  9
	// [ToTextIPv4NumberDecimal] ip:  ::ffff:10.1.255.255, result:    167903231, min_len:  9
	// [ToTextIPv4NumberDecimal] ip:           2048:db8::, result:            0, min_len:  1
	// [ToTextIPv4NumberDecimal] ip:       2048:db8::ffff, result:        65535, min_len:  5

}

func ExampleToTextNumber() {
	ipSet := []string{
		"",
		"-0.1.0.0",
		"0.0.0.0",
		"10.1.0.0",
		"10.1.0.0/16",
		"10.1.255.255",
		"::ffff:10.1.255.255",
		"2048:db8::",
		"2048:db8::ffff",
	}

	var ipRet string
	baseSet := []int{2, 10, 16, 64}
	for index := range ipSet {
		for _, b := range baseSet {
			ipRet = ip.ToTextNumber(ipSet[index], b)
			if len(ipRet) == 0 {
				fmt.Printf("[ToTextNumber] ip: %20v, base: %2v, result: %32v, min_len:%3v\n", ipSet[index], b, ipRet, len(ipRet))
				continue
			}

			if b == 2 {
				fmt.Printf("[ToTextNumber] ip: %20v, base: %2v, result: %032v, min_len:%3v\n", ipSet[index], b, ipRet, len(ipRet))
			} else {
				fmt.Printf("[ToTextNumber] ip: %20v, base: %2v, result: %32v, min_len:%3v\n", ipSet[index], b, ipRet, len(ipRet))
			}
		}
	}

	// output:
	// [ToTextNumber] ip:                     , base:  2, result:                                 , min_len:  0
	// [ToTextNumber] ip:                     , base: 10, result:                                 , min_len:  0
	// [ToTextNumber] ip:                     , base: 16, result:                                 , min_len:  0
	// [ToTextNumber] ip:                     , base: 64, result:                                 , min_len:  0
	// [ToTextNumber] ip:             -0.1.0.0, base:  2, result:                                 , min_len:  0
	// [ToTextNumber] ip:             -0.1.0.0, base: 10, result:                                 , min_len:  0
	// [ToTextNumber] ip:             -0.1.0.0, base: 16, result:                                 , min_len:  0
	// [ToTextNumber] ip:             -0.1.0.0, base: 64, result:                                 , min_len:  0
	// [ToTextNumber] ip:              0.0.0.0, base:  2, result: 00000000000000000000000000000000, min_len:  1
	// [ToTextNumber] ip:              0.0.0.0, base: 10, result:                                0, min_len:  1
	// [ToTextNumber] ip:              0.0.0.0, base: 16, result:                                0, min_len:  1
	// [ToTextNumber] ip:              0.0.0.0, base: 64, result:                                 , min_len:  0
	// [ToTextNumber] ip:             10.1.0.0, base:  2, result: 00001010000000010000000000000000, min_len: 28
	// [ToTextNumber] ip:             10.1.0.0, base: 10, result:                        167837696, min_len:  9
	// [ToTextNumber] ip:             10.1.0.0, base: 16, result:                          a010000, min_len:  7
	// [ToTextNumber] ip:             10.1.0.0, base: 64, result:                                 , min_len:  0
	// [ToTextNumber] ip:          10.1.0.0/16, base:  2, result:                                 , min_len:  0
	// [ToTextNumber] ip:          10.1.0.0/16, base: 10, result:                                 , min_len:  0
	// [ToTextNumber] ip:          10.1.0.0/16, base: 16, result:                                 , min_len:  0
	// [ToTextNumber] ip:          10.1.0.0/16, base: 64, result:                                 , min_len:  0
	// [ToTextNumber] ip:         10.1.255.255, base:  2, result: 00001010000000011111111111111111, min_len: 28
	// [ToTextNumber] ip:         10.1.255.255, base: 10, result:                        167903231, min_len:  9
	// [ToTextNumber] ip:         10.1.255.255, base: 16, result:                          a01ffff, min_len:  7
	// [ToTextNumber] ip:         10.1.255.255, base: 64, result:                                 , min_len:  0
	// [ToTextNumber] ip:  ::ffff:10.1.255.255, base:  2, result: 00001010000000011111111111111111, min_len: 28
	// [ToTextNumber] ip:  ::ffff:10.1.255.255, base: 10, result:                        167903231, min_len:  9
	// [ToTextNumber] ip:  ::ffff:10.1.255.255, base: 16, result:                          a01ffff, min_len:  7
	// [ToTextNumber] ip:  ::ffff:10.1.255.255, base: 64, result:                                 , min_len:  0
	// [ToTextNumber] ip:           2048:db8::, base:  2, result: 100000010010000000110110111000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000, min_len:126
	// [ToTextNumber] ip:           2048:db8::, base: 10, result: 42909419488238565618529650191028453376, min_len: 38
	// [ToTextNumber] ip:           2048:db8::, base: 16, result: 20480db8000000000000000000000000, min_len: 32
	// [ToTextNumber] ip:           2048:db8::, base: 64, result:                                 , min_len:  0
	// [ToTextNumber] ip:       2048:db8::ffff, base:  2, result: 100000010010000000110110111000000000000000000000000000000000000000000000000000000000000000000000000000000000001111111111111111, min_len:126
	// [ToTextNumber] ip:       2048:db8::ffff, base: 10, result: 42909419488238565618529650191028518911, min_len: 38
	// [ToTextNumber] ip:       2048:db8::ffff, base: 16, result: 20480db800000000000000000000ffff, min_len: 32
	// [ToTextNumber] ip:       2048:db8::ffff, base: 64, result:                                 , min_len:  0

}

func ExampleToTextNumberDecimal() {
	ipSet := []string{
		"",
		"-0.1.0.0",
		"0.0.0.0",
		"10.1.0.0",
		"10.1.0.0/16",
		"10.1.255.255",
		"::ffff:10.1.255.255",
		"2048:db8::",
		"2048:db8::ffff",
	}

	var ipRet string
	for index := range ipSet {
		ipRet = ip.ToTextNumberDecimal(ipSet[index])
		fmt.Printf("[ToTextNumberDecimal] ip: %20v, result: %48v, min_len:%3v\n", ipSet[index], ipRet, len(ipRet))
	}

	// output:
	// [ToTextNumberDecimal] ip:                     , result:                                                 , min_len:  0
	// [ToTextNumberDecimal] ip:             -0.1.0.0, result:                                                 , min_len:  0
	// [ToTextNumberDecimal] ip:              0.0.0.0, result:                                                0, min_len:  1
	// [ToTextNumberDecimal] ip:             10.1.0.0, result:                                        167837696, min_len:  9
	// [ToTextNumberDecimal] ip:          10.1.0.0/16, result:                                                 , min_len:  0
	// [ToTextNumberDecimal] ip:         10.1.255.255, result:                                        167903231, min_len:  9
	// [ToTextNumberDecimal] ip:  ::ffff:10.1.255.255, result:                                        167903231, min_len:  9
	// [ToTextNumberDecimal] ip:           2048:db8::, result:           42909419488238565618529650191028453376, min_len: 38
	// [ToTextNumberDecimal] ip:       2048:db8::ffff, result:           42909419488238565618529650191028518911, min_len: 38

}

func ExampleVersionFlag() {
	ipSet := []string{
		"",
		"-0.1.0.0",
		"0.0.0.0",
		"10.1.0.0",
		"10.1.0.0/16",
		"10.1.255.255",
		"::ffff:10.1.255.255",
		"2048:db8::",
		"2048:db8::ffff",
	}

	var ipVFlag ip.Flag
	for index := range ipSet {
		ipVFlag = ip.VersionFlag(ipSet[index])
		fmt.Printf("[VersionFlag] ip: %20v, ipVFlag: %7v\n", ipSet[index], ipVFlag.String())
	}

	// output:
	// [VersionFlag] ip:                     , ipVFlag: invalid
	// [VersionFlag] ip:             -0.1.0.0, ipVFlag: invalid
	// [VersionFlag] ip:              0.0.0.0, ipVFlag:    ipv4
	// [VersionFlag] ip:             10.1.0.0, ipVFlag:    ipv4
	// [VersionFlag] ip:          10.1.0.0/16, ipVFlag: invalid
	// [VersionFlag] ip:         10.1.255.255, ipVFlag:    ipv4
	// [VersionFlag] ip:  ::ffff:10.1.255.255, ipVFlag:    ipv4
	// [VersionFlag] ip:           2048:db8::, ipVFlag:    ipv6
	// [VersionFlag] ip:       2048:db8::ffff, ipVFlag:    ipv6

}

func ExampleVersionFlagByBytes() {
	ipSet := [][]byte{
		nil,
		big.NewInt(0).Bytes(),
		big.NewInt(1).Bytes(),
		{0, 0, 0, 0},
		{0, 0, 0, 1},
		{10, 10, 10, 1},
		big.NewInt(math.MaxInt32).Bytes(),
		big.NewInt(math.MaxUint32).Bytes(),
		{0xff, 0xff, 0xff, 0xff},
		append([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff}, []byte{0, 0, 0, 0}...),
		append([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff}, []byte{0, 0, 0, 1}...),
		ipv6MaxBytes,
		bytes.Repeat([]byte{0xff}, 16),
		append(bytes.Repeat([]byte{0}, 12), bytes.Repeat([]byte{0xff}, 4)...), // ipv6
		bytes.Repeat([]byte{0xff}, 18),
	}

	var ipVFlag ip.Flag
	for index := range ipSet {
		ipVFlag = ip.VersionFlagByBytes(ipSet[index])
		fmt.Printf("[VersionFlagByBytes] ipVFlag: %7v， ip:%v\n", ipVFlag.String(), ipSet[index])
	}

	// output:
	// [VersionFlagByBytes] ipVFlag: invalid， ip:[]
	// [VersionFlagByBytes] ipVFlag: invalid， ip:[]
	// [VersionFlagByBytes] ipVFlag: invalid， ip:[1]
	// [VersionFlagByBytes] ipVFlag:    ipv4， ip:[0 0 0 0]
	// [VersionFlagByBytes] ipVFlag:    ipv4， ip:[0 0 0 1]
	// [VersionFlagByBytes] ipVFlag:    ipv4， ip:[10 10 10 1]
	// [VersionFlagByBytes] ipVFlag:    ipv4， ip:[127 255 255 255]
	// [VersionFlagByBytes] ipVFlag:    ipv4， ip:[255 255 255 255]
	// [VersionFlagByBytes] ipVFlag:    ipv4， ip:[255 255 255 255]
	// [VersionFlagByBytes] ipVFlag:    ipv4， ip:[0 0 0 0 0 0 0 0 0 0 255 255 0 0 0 0]
	// [VersionFlagByBytes] ipVFlag:    ipv4， ip:[0 0 0 0 0 0 0 0 0 0 255 255 0 0 0 1]
	// [VersionFlagByBytes] ipVFlag:    ipv6， ip:[255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255]
	// [VersionFlagByBytes] ipVFlag:    ipv6， ip:[255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255]
	// [VersionFlagByBytes] ipVFlag:    ipv6， ip:[0 0 0 0 0 0 0 0 0 0 0 0 255 255 255 255]
	// [VersionFlagByBytes] ipVFlag: invalid， ip:[255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255]

}

func ExampleVersionFlagByContains() {
	ipSet := []string{
		"",
		"-0.1.0.0",
		"0.0.0.0",
		"10.1.0.0",
		"10.1.0.0/16",
		"10.1.255.255",
		"::ffff:10.1.255.255",
		"2048:db8::",
		"2048:db8::ffff",
	}

	var ipVFlag ip.Flag
	for index := range ipSet {
		ipVFlag = ip.VersionFlagByContains(ipSet[index])
		fmt.Printf("[VersionFlagByContains] ip: %20v, ipVFlag: %7v\n", ipSet[index], ipVFlag.String())
	}

	// output:
	// [VersionFlagByContains] ip:                     , ipVFlag: invalid
	// [VersionFlagByContains] ip:             -0.1.0.0, ipVFlag: invalid
	// [VersionFlagByContains] ip:              0.0.0.0, ipVFlag:    ipv4
	// [VersionFlagByContains] ip:             10.1.0.0, ipVFlag:    ipv4
	// [VersionFlagByContains] ip:          10.1.0.0/16, ipVFlag: invalid
	// [VersionFlagByContains] ip:         10.1.255.255, ipVFlag:    ipv4
	// [VersionFlagByContains] ip:  ::ffff:10.1.255.255, ipVFlag:    ipv4
	// [VersionFlagByContains] ip:           2048:db8::, ipVFlag:    ipv6
	// [VersionFlagByContains] ip:       2048:db8::ffff, ipVFlag:    ipv6

}
