// Copyright 2018 Intel Corporation.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package generator

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"math/rand"
	"net"
	"os"
	"regexp"
	"strings"
	"unsafe"

	"github.com/intel-go/nff-go/common"
)

var mixPattern = regexp.MustCompile(`^mix[0-9]*$`)

// AddrRange describes range of addresses.
type AddrRange struct {
	Min     []byte
	Max     []byte
	Current []byte
	Incr    []byte
}

func (ar *AddrRange) String() string {
	s := "addrRange:\n"
	s += fmt.Sprintf("min: %v, max: %v, start: %v, incr: %d", ar.Min, ar.Max, ar.Current, ar.Incr)
	return s
}

// PortRange describes range of ports.
type PortRange struct {
	Min     uint16
	Max     uint16
	Current uint16
	Incr    uint16
}

func (pr *PortRange) String() string {
	s := "portRange:\n"
	s += fmt.Sprintf("min: %v, max: %v, start: %v, incr: %d", pr.Min, pr.Max, pr.Current, pr.Incr)
	return s
}

// SequenceType used in enum below.
type SequenceType uint

// Types of Sequence.
const (
	RANDOM SequenceType = iota
	INCREASING
)

// Sequence contains type and next sequence value.
type Sequence struct {
	Type SequenceType
	Next uint32
}

type DataType int

const (
	ETHERHDR = iota
	IPHDR
	TCPHDR
	UDPHDR
	ICMPHDR
	ARPHDR
	PDISTDATA
	RANDDATA
	RAWDATA
	NONE
)

// PacketConfig configures packet
type PacketConfig struct {
	DType DataType
	Data  unsafe.Pointer
}

// MixConfig contains PacketConfigs with quantity.
type MixConfig struct {
	Config   *PacketConfig
	Quantity uint32
}

func (mc *MixConfig) String() string {
	return fmt.Sprintf("config: %v; quantity: %v\n", *(mc.Config), mc.Quantity)
}

// EtherConfig configures ether header.
type EtherConfig struct {
	DAddr AddrRange
	SAddr AddrRange
	VLAN  *VlanTagConfig
	DType DataType
	Data  unsafe.Pointer
}

// VlanTagConfig configures vlan tag
type VlanTagConfig struct {
	TCI uint16
}

// IPConfig configures ip header.
type IPConfig struct {
	Version int
	SAddr   AddrRange // source address
	DAddr   AddrRange // destination address
	DType   DataType
	Data    unsafe.Pointer
}

// TCPConfig configures tcp header.
type TCPConfig struct {
	SPort PortRange
	DPort PortRange
	Seq   Sequence
	Flags common.TCPFlags
	DType DataType
	Data  unsafe.Pointer
}

// UDPConfig configures tcp header.
type UDPConfig struct {
	SPort PortRange
	DPort PortRange
	DType DataType
	Data  unsafe.Pointer
}

// ICMPConfig configures tcp header.
type ICMPConfig struct {
	Type       uint8
	Code       uint8
	Identifier uint16
	Seq        Sequence
	DType      DataType
	Data       unsafe.Pointer
}

// ARPConfig configures arp header.
type ARPConfig struct {
	Operation  uint16
	Gratuitous bool
	SHA        AddrRange
	SPA        AddrRange
	THA        AddrRange
	TPA        AddrRange
}

// PDistEntry gives data with associated probability of being chosen.
type PDistEntry struct {
	Probability float64
	DType       DataType
	Data        unsafe.Pointer
}

// RandBytes gives  a payload of random bytes, of a given size.
// Optionally it is possible to specify a deviation.
type RandBytes struct {
	Size      uint32
	Deviation uint32
}

// Raw represents raw data
type Raw struct {
	Data string
}

// ParseConfig parses json config and returns []*MixConfig.
func ParseConfig(f *os.File) (config []*MixConfig, err error) {
	r := bufio.NewReader(f)
	var in map[string]interface{}
	err = json.NewDecoder(r).Decode(&in)
	if err != nil {
		return nil, fmt.Errorf("decoding input from file returned: %v", err)
	}
	for k, v := range in {
		key := strings.ToLower(k)
		switch {
		case key == "ether":
			etherConfig := v.(map[string]interface{})
			ethHdr, err := parseEtherHdr(etherConfig)
			if err != nil {
				return nil, fmt.Errorf("parseEtherHdr returned: %v", err)
			}
			pktConfig := &PacketConfig{Data: unsafe.Pointer(&ethHdr), DType: ETHERHDR}
			return append(config, &MixConfig{Config: pktConfig, Quantity: 1}), nil
		case mixPattern.MatchString(key):
			return ParseMixConfig(in)
		default:
			return nil, fmt.Errorf("unexpected key: %s, expected mix[0-9]* or ether", k)
		}
	}

	return nil, fmt.Errorf("expected 'ether' key , but did not get")
}

// ParseMixConfig parses json config and returns []*MixConfig.
func ParseMixConfig(in map[string]interface{}) (config []*MixConfig, err error) {
	for k, v := range in {
		key := strings.ToLower(k)
		switch {
		case mixPattern.MatchString(key):
			var (
				pktConfig *PacketConfig
				q         uint32
			)
			mixUnit := v.(map[string]interface{})
			for kk, vv := range mixUnit {
				switch strings.ToLower(kk) {
				case "ether":
					etherConfig := vv.(map[string]interface{})
					ethHdr, err := parseEtherHdr(etherConfig)
					if err != nil {
						return nil, fmt.Errorf("parseEtherHdr returned: %v", err)
					}
					pktConfig = &PacketConfig{Data: unsafe.Pointer(&ethHdr), DType: ETHERHDR}
				case "quantity", "q":
					q = uint32(vv.(float64))
				default:
					return nil, fmt.Errorf("unexpected key: %s, expected ether and quantity or q", k)
				}
			}
			if q == 0 || pktConfig == nil {
				return nil, fmt.Errorf("some fields were not set")
			}
			config = append(config, &MixConfig{Config: pktConfig, Quantity: q})
		default:
			return nil, fmt.Errorf("unexpected key: %s, expected mix[0-9]*", k)
		}
	}
	return config, nil
}

func parseEtherHdr(in map[string]interface{}) (EtherConfig, error) {
	ethConfig := EtherConfig{DAddr: AddrRange{}, SAddr: AddrRange{}, VLAN: nil, DType: NONE}
	for k, v := range in {
		switch strings.ToLower(k) {
		case "saddr":
			saddr, err := parseMacAddr(v)
			if err != nil {
				return EtherConfig{}, fmt.Errorf("parseMacAddr for ether saddr returned: %v", err)
			}
			ethConfig.SAddr = saddr
		case "daddr":
			daddr, err := parseMacAddr(v)
			if err != nil {
				return EtherConfig{}, fmt.Errorf("parseMacAddr for ether daddr returned: %v", err)
			}
			ethConfig.DAddr = daddr
		case "vlan-tci":
			ethConfig.VLAN = &(VlanTagConfig{TCI: uint16(v.(float64))})
			break
		case "ip":
			ipConf, err := parseIPHdr(v.(map[string]interface{}))
			if err != nil {
				return EtherConfig{}, fmt.Errorf("parseIpHdr returned %v", err)
			}
			ethConfig.Data = unsafe.Pointer(&ipConf)
			ethConfig.DType = IPHDR
			break
		case "arp":
			arpConfig := v.(map[string]interface{})
			arpConf, err := parseARPHdr(arpConfig)
			if err != nil {
				return EtherConfig{}, fmt.Errorf("parseARPHdr returned %v", err)
			}
			ethConfig.Data = unsafe.Pointer(&arpConf)
			ethConfig.DType = ARPHDR
			break
		default:
			data, dataType, err := parseData(map[string]interface{}{k: v})
			if err != nil {
				return EtherConfig{}, fmt.Errorf("parseData for ether returned: %v", err)
			}
			ethConfig.Data = data
			ethConfig.DType = dataType
			break
		}
	}
	return ethConfig, nil
}

func parseData(in map[string]interface{}) (unsafe.Pointer, DataType, error) {
	for k, v := range in {
		switch strings.ToLower(k) {
		case "raw":
			data, err := parseRaw(v.(map[string]interface{}))
			if err != nil {
				return nil, NONE, fmt.Errorf("parseRaw returned %v", err)
			}
			return unsafe.Pointer(&data), RAWDATA, nil
		case "randbytes":
			randBytes, err := parseRandBytes(v.(map[string]interface{}))
			if err != nil {
				return nil, NONE, fmt.Errorf("parseRandBytes returned %v", err)
			}
			return unsafe.Pointer(&randBytes), RANDDATA, nil
		case "pdist":
			pdist, err := parsePdist(v.([]interface{}))
			if err != nil {
				return nil, NONE, fmt.Errorf("parsePdist returned %v", err)
			}
			return unsafe.Pointer(&pdist), PDISTDATA, nil
		default:
			fmt.Println("unknown key: ", k)
			break
		}
	}
	return nil, NONE, fmt.Errorf("failed to parse data")
}

func parseIPHdr(in map[string]interface{}) (IPConfig, error) {
	ipHdr := IPConfig{Version: 4, DAddr: AddrRange{}, SAddr: AddrRange{}}
	for k, v := range in {
		switch strings.ToLower(k) {
		case "version":
			version := (int)(v.(float64))
			if version != 4 && version != 6 {
				return IPConfig{}, fmt.Errorf("ip version should be 4 or 6, got: %d", version)
			}
			ipHdr.Version = version
		case "saddr":
			saddr, err := parseIPAddr(v)
			if err != nil {
				return IPConfig{}, fmt.Errorf("parseIPAddr for ip saddr returned: %v", err)
			}
			ipHdr.SAddr = saddr
		case "daddr":
			daddr, err := parseIPAddr(v)
			if err != nil {
				return IPConfig{}, fmt.Errorf("parseIPAddr for ip daddr returned: %v", err)
			}
			ipHdr.DAddr = daddr
		case "tcp":
			tcpConf, err := parseTCPHdr(v.(map[string]interface{}))
			if err != nil {
				return IPConfig{}, fmt.Errorf("parseTCPHdr returned %v", err)
			}
			ipHdr.Data = unsafe.Pointer(&tcpConf)
			ipHdr.DType = TCPHDR
			break
		case "udp":
			udpConf, err := parseUDPHdr(v.(map[string]interface{}))
			if err != nil {
				return IPConfig{}, fmt.Errorf("parseUDPHdr returned %v", err)
			}
			ipHdr.Data = unsafe.Pointer(&udpConf)
			ipHdr.DType = UDPHDR
			break
		case "icmp":
			icmpConf, err := parseICMPHdr(v.(map[string]interface{}))
			if err != nil {
				return IPConfig{}, fmt.Errorf("parseICMPHdr returned %v", err)
			}
			ipHdr.Data = unsafe.Pointer(&icmpConf)
			ipHdr.DType = ICMPHDR
			break
		default:
			data, dataType, err := parseData(map[string]interface{}{k: v})
			if err != nil {
				return IPConfig{}, fmt.Errorf("parseData for ip returned: %v", err)
			}
			ipHdr.Data = data
			ipHdr.DType = dataType
			break
		}
	}
	return ipHdr, nil
}

func parseARPHdr(in map[string]interface{}) (ARPConfig, error) {
	arpHdr := ARPConfig{Operation: 1, Gratuitous: false, SHA: AddrRange{}, SPA: AddrRange{}, THA: AddrRange{}, TPA: AddrRange{}}
	for k, v := range in {
		switch strings.ToLower(k) {
		case "opcode":
			opcode := v.(float64)
			if opcode < 1 || opcode > 2 {
				return ARPConfig{}, fmt.Errorf("support opcodes [1,2], got: %d", uint16(opcode))
			}
			arpHdr.Operation = uint16(opcode)
		case "gratuitous":
			arpHdr.Gratuitous = v.(bool)
		case "sha":
			sha, err := parseMacAddr(v)
			if err != nil {
				return ARPConfig{}, fmt.Errorf("parseMacAddr for sha returned: %v", err)
			}
			arpHdr.SHA = sha
		case "spa":
			spa, err := parseIPAddr(v)
			if err != nil {
				return ARPConfig{}, fmt.Errorf("parseIPAddr for spa returned: %v", err)
			}
			arpHdr.SPA = spa
		case "tha":
			tha, err := parseMacAddr(v)
			if err != nil {
				return ARPConfig{}, fmt.Errorf("parseMacAddr for tha returned: %v", err)
			}
			arpHdr.THA = tha
		case "tpa":
			tpa, err := parseIPAddr(v)
			if err != nil {
				return ARPConfig{}, fmt.Errorf("parseIPAddr for tpa returned: %v", err)
			}
			arpHdr.TPA = tpa
		default:
			return ARPConfig{}, fmt.Errorf("unrecognized key for arp configuration: %s", k)
		}
	}
	return arpHdr, nil
}

func parseTCPHdr(in map[string]interface{}) (TCPConfig, error) {
	tcpHdr := TCPConfig{SPort: PortRange{}, DPort: PortRange{}}
	for k, v := range in {
		switch strings.ToLower(k) {
		case "sport":
			sport, err := parsePort(v)
			if err != nil {
				return TCPConfig{}, fmt.Errorf("parsePort for tcp sport returned: %v", err)
			}
			tcpHdr.SPort = sport
		case "dport":
			dport, err := parsePort(v)
			if err != nil {
				return TCPConfig{}, fmt.Errorf("parsePort for tcp dport returned: %v", err)
			}
			tcpHdr.DPort = dport
		case "seq":
			seq, err := parseSeq(v.(string))
			if err != nil {
				return TCPConfig{}, fmt.Errorf("failed to parse tcp sequence: %v", err)
			}
			tcpHdr.Seq = seq
		case "flags":
			flags, err := parseTCPFlags(v.([]interface{}))
			if err != nil {
				return TCPConfig{}, fmt.Errorf("parseTCPFlags failed with %v", err)
			}
			tcpHdr.Flags = flags
		default:
			data, dataType, err := parseData(map[string]interface{}{k: v})
			if err != nil {
				return TCPConfig{}, fmt.Errorf("parseData for tcp returned: %v", err)
			}
			tcpHdr.Data = data
			tcpHdr.DType = dataType
			break
		}
	}
	return tcpHdr, nil
}

func parseUDPHdr(in map[string]interface{}) (UDPConfig, error) {
	udpHdr := UDPConfig{SPort: PortRange{}, DPort: PortRange{}}
	for k, v := range in {
		switch strings.ToLower(k) {
		case "sport":
			sport, err := parsePort(v)
			if err != nil {
				return UDPConfig{}, fmt.Errorf("parsePort for tcp sport returned: %v", err)
			}
			udpHdr.SPort = sport
		case "dport":
			dport, err := parsePort(v)
			if err != nil {
				return UDPConfig{}, fmt.Errorf("parsePort for tcp dport returned: %v", err)
			}
			udpHdr.DPort = dport
		default:
			data, dataType, err := parseData(map[string]interface{}{k: v})
			if err != nil {
				return UDPConfig{}, fmt.Errorf("parseData for udp returned: %v", err)
			}
			udpHdr.Data = data
			udpHdr.DType = dataType
			break
		}
	}
	return udpHdr, nil
}

func parseICMPHdr(in map[string]interface{}) (ICMPConfig, error) {
	icmpHdr := ICMPConfig{Data: unsafe.Pointer(&Raw{Data: ""})}
	for k, v := range in {
		switch strings.ToLower(k) {
		case "type":
			icmpHdr.Type = uint8(v.(float64))
		case "code":
			icmpHdr.Code = uint8(v.(float64))
		case "identifier", "id":
			icmpHdr.Identifier = uint16(v.(float64))
		case "seq", "seqnum":
			seq, err := parseSeq(v.(string))
			if err != nil {
				return ICMPConfig{}, fmt.Errorf("failed to parse icmp sequence: %v", err)
			}
			icmpHdr.Seq = seq
		default:
			data, dataType, err := parseData(map[string]interface{}{k: v})
			if err != nil {
				return ICMPConfig{}, fmt.Errorf("parseData for icmp returned: %v", err)
			}
			icmpHdr.Data = data
			icmpHdr.DType = dataType
			break
		}
	}
	return icmpHdr, nil
}

func parseTCPFlags(in []interface{}) (ret common.TCPFlags, err error) {
	ret = 0
	for _, flag := range in {
		switch strings.ToLower(flag.(string)) {
		case "fin":
			ret ^= common.TCPFlagFin
		case "syn":
			ret ^= common.TCPFlagSyn
		case "rst":
			ret ^= common.TCPFlagRst
		case "psh":
			ret ^= common.TCPFlagPsh
		case "ack":
			ret ^= common.TCPFlagAck
		case "urg":
			ret ^= common.TCPFlagUrg
		case "ece":
			ret ^= common.TCPFlagEce
		case "cwr":
			ret ^= common.TCPFlagCwr
		default:
			return 0, fmt.Errorf("unknown flag value: %s", flag.(string))
		}
	}
	return ret, nil
}

func parseSeq(in string) (Sequence, error) {
	switch strings.ToLower(in) {
	case "rand", "random":
		return Sequence{Type: RANDOM, Next: rand.Uint32()}, nil
	case "incr", "increasing":
		return Sequence{Type: INCREASING, Next: 0}, nil
	}
	return Sequence{}, fmt.Errorf("unknown key for tcp sequence: %s", in)
}

func parseRaw(in map[string]interface{}) (Raw, error) {
	for k, v := range in {
		switch strings.ToLower(k) {
		case "data":
			return Raw{Data: v.(string)}, nil
		default:
			return Raw{}, fmt.Errorf("unknown key in 'raw' block: %s", k)
		}
	}

	return Raw{}, fmt.Errorf("no key 'data' found inside 'raw' block")
}

func parseRandBytes(in map[string]interface{}) (RandBytes, error) {
	rBytes := RandBytes{}
	for k, v := range in {
		switch strings.ToLower(k) {
		case "size":
			if v.(float64) < 0 {
				return RandBytes{}, fmt.Errorf("invalid size value, should be positive: %f", v.(float64))
			}
			rBytes.Size = (uint32)(v.(float64))
		case "deviation":
			if v.(float64) < 0 {
				return RandBytes{}, fmt.Errorf("invalid deviation value, should be positive: %f", v.(float64))
			}
			rBytes.Deviation = (uint32)(v.(float64))
		}
	}
	if rBytes.Size < rBytes.Deviation {
		return RandBytes{}, fmt.Errorf("deviation cant be larger than size")
	}
	return rBytes, nil
}

func parsePdist(in []interface{}) ([]PDistEntry, error) {
	pdist := []PDistEntry{}
	for _, v := range in {
		pdistElem, err := parsePdistEntry(v.(map[string]interface{}))
		if err != nil {
			return []PDistEntry{}, fmt.Errorf("parsing pdistentry returned: %v", err)
		}
		pdist = append(pdist, pdistElem)
	}
	return pdist, nil
}

func parsePdistEntry(in map[string]interface{}) (PDistEntry, error) {
	pdistElem := PDistEntry{}
	for k, v := range in {
		switch strings.ToLower(k) {
		case "probability":
			pdistElem.Probability = v.(float64)
			if pdistElem.Probability > 1 || pdistElem.Probability < 0 {
				return PDistEntry{}, fmt.Errorf("invalid probability value: %f", pdistElem.Probability)
			}
		case "raw":
			data, err := parseRaw(v.(map[string]interface{}))
			if err != nil {
				return PDistEntry{}, fmt.Errorf("parsing pdist entry: parseRaw returned %v", err)
			}
			pdistElem.Data = unsafe.Pointer(&data)
			pdistElem.DType = RAWDATA
		case "randbytes":
			randBytes, err := parseRandBytes(v.(map[string]interface{}))
			if err != nil {
				return PDistEntry{}, fmt.Errorf("parsing pdist entry: parseRandBytes returned %v", err)
			}
			pdistElem.Data = unsafe.Pointer(&randBytes)
			pdistElem.DType = RANDDATA
		default:
			return PDistEntry{}, fmt.Errorf("unknown key %s", k)
		}
	}
	return pdistElem, nil
}

func parseMacAddr(value interface{}) (AddrRange, error) {
	switch v2 := value.(type) {
	case map[string]interface{}:
		for k, v := range v2 {
			switch strings.ToLower(k) {
			case "range":
				saddrRange, err := parseAddrRange(v.(map[string]interface{}), parseRawMACAddr)
				if err != nil {
					return AddrRange{}, fmt.Errorf("parseAddrRange returned: %v", err)
				}
				return saddrRange, nil

			default:
				return AddrRange{}, fmt.Errorf("unknown key in parse mac addr: %s", k)
			}
		}
	case string:
		saddr, err := net.ParseMAC(v2)
		if err != nil {
			return AddrRange{}, fmt.Errorf("parsing mac saddr returned: %v", err)
		}
		ret := AddrRange{}
		ret.Min = make([]byte, len(saddr))
		copy(ret.Min, saddr)
		ret.Max = make([]byte, len(saddr))
		copy(ret.Max, saddr)
		ret.Current = make([]byte, len(saddr))
		copy(ret.Current, saddr)
		return ret, nil
	}
	return AddrRange{}, fmt.Errorf("unknown type")
}

func parseRawIPAddr(rawAddr string) ([]uint8, error) {
	addr := net.ParseIP(rawAddr)
	if addr == nil {
		return nil, fmt.Errorf("parsing ip addr could not parse address for %s", rawAddr)
	}
	return addr, nil
}

func parseRawMACAddr(rawAddr string) ([]uint8, error) {
	addr, err := net.ParseMAC(rawAddr)
	if err != nil {
		return nil, fmt.Errorf("parsing mac addr returned: %v for %s", err, rawAddr)
	}
	return addr, nil
}

func parseIPAddr(value interface{}) (AddrRange, error) {
	switch v2 := value.(type) {
	case map[string]interface{}:
		for k, v := range v2 {
			switch strings.ToLower(k) {
			case "range":
				addrRange, err := parseAddrRange(v.(map[string]interface{}), parseRawIPAddr)
				if err != nil {
					return AddrRange{}, fmt.Errorf("parseAddrRange returned: %v", err)
				}
				return addrRange, nil
			default:
				return AddrRange{}, fmt.Errorf("unknown key in ip addr: %s", k)
			}
		}
	case string:
		saddr, err := parseRawIPAddr(v2)
		if err != nil {
			return AddrRange{}, fmt.Errorf("parsing ip addr returned: %v", err)
		}
		ret := AddrRange{}
		ret.Min = make([]byte, len(saddr))
		copy(ret.Min, saddr)
		ret.Max = make([]byte, len(saddr))
		copy(ret.Max, saddr)
		ret.Current = make([]byte, len(saddr))
		copy(ret.Current, saddr)
		return ret, nil
	}
	return AddrRange{}, fmt.Errorf("unknown type")
}

type fn func(string) ([]uint8, error)

func parseAddrRange(in map[string]interface{}, parseFunc fn) (AddrRange, error) {
	addr := AddrRange{}
	wasMin, wasMax, wasStart, wasIncr := false, false, false, false
	for k, v := range in {
		switch strings.ToLower(k) {
		case "min":
			min, err := parseFunc(v.(string))
			if err != nil {
				return AddrRange{}, fmt.Errorf("parsing min returned: %v", err)
			}
			addr.Min = make([]byte, len(min))
			copy(addr.Min, min)
			wasMin = true
		case "max":
			max, err := parseFunc(v.(string))
			if err != nil {
				return AddrRange{}, fmt.Errorf("parsing max returned: %v", err)
			}
			addr.Max = make([]byte, len(max))
			copy(addr.Max, max)
			wasMax = true
		case "start":
			start, err := parseFunc(v.(string))
			if err != nil {
				return AddrRange{}, fmt.Errorf("parsing start returned: %v", err)
			}
			addr.Current = make([]byte, len(start))
			copy(addr.Current, start)
			wasStart = true
		case "incr":
			addr.Incr = big.NewInt((int64)(v.(float64))).Bytes()
			wasIncr = true
		default:
			return AddrRange{}, fmt.Errorf("unknown key %s", k)
		}
	}
	if !wasMax || !wasMin {
		return AddrRange{}, fmt.Errorf("Min and max values should be given for range")
	}
	if bytes.Compare(addr.Max, addr.Min) < 0 {
		return AddrRange{}, fmt.Errorf("Min value should be less than Max")
	}
	if !wasStart {
		addr.Current = make([]byte, len(addr.Min))
		copy(addr.Current, addr.Min)
	}

	if bytes.Compare(addr.Current, addr.Min) < 0 || bytes.Compare(addr.Current, addr.Max) > 0 {
		return AddrRange{}, fmt.Errorf(fmt.Sprintf("Start value should be between min and max: start=%v, min=%v, max=%v", addr.Current, addr.Min, addr.Max))
	}
	if !wasIncr {
		addr.Incr = []byte{1}
	}
	return addr, nil
}

func parsePort(in interface{}) (PortRange, error) {
	switch v2 := in.(type) {
	case map[string]interface{}:
		for k, v := range v2 {
			switch strings.ToLower(k) {
			case "range":
				portRng, err := parsePortRange(v.(map[string]interface{}))
				if err != nil {
					return PortRange{}, fmt.Errorf("parseAddrRange returned: %v", err)
				}
				return portRng, nil
			default:
				return PortRange{}, fmt.Errorf("unknown key in port: %s", k)
			}
		}
	case float64:
		if in.(float64) < 0 {
			return PortRange{}, fmt.Errorf("Port should be positive")
		}
		port := (uint16)(in.(float64))
		return PortRange{Min: port, Max: port, Current: port, Incr: 0}, nil
	}
	return PortRange{}, fmt.Errorf("unknown type")
}

func parsePortRange(in map[string]interface{}) (PortRange, error) {
	portRng := PortRange{Incr: 0}
	wasMin, wasMax, wasStart, wasIncr := false, false, false, false
	for k, v := range in {
		switch strings.ToLower(k) {
		case "min":
			if v.(float64) < 0 {
				return PortRange{}, fmt.Errorf("Min should be positive")
			}
			portRng.Min = (uint16)(v.(float64))
			wasMin = true
		case "max":
			if v.(float64) < 0 {
				return PortRange{}, fmt.Errorf("Max should be positive")
			}
			portRng.Max = (uint16)(v.(float64))
			wasMax = true
		case "start":
			if v.(float64) < 0 {
				return PortRange{}, fmt.Errorf("Start should be positive")
			}
			portRng.Current = (uint16)(v.(float64))
			wasStart = true
		case "incr":
			if v.(float64) < 0 {
				return PortRange{}, fmt.Errorf("Incr should be positive")
			}
			portRng.Incr = (uint16)(v.(float64))
			wasIncr = true
		default:
			return PortRange{}, fmt.Errorf("unknown key %s", k)
		}
	}
	if !wasMax || !wasMin {
		return PortRange{}, fmt.Errorf("Min and max values should be given for range")
	}
	if portRng.Max < portRng.Min {
		return PortRange{}, fmt.Errorf("Min value should be <= Max value")
	}
	if !wasStart {
		portRng.Current = portRng.Min
	}
	if portRng.Current < portRng.Min || portRng.Current > portRng.Max {
		return PortRange{}, fmt.Errorf("Start should be in range of min and max")
	}
	if !wasIncr {
		portRng.Incr = 1
	}
	return portRng, nil
}
