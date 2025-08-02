package bgp

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"

	"github.com/im-kulikov/go-bones/logger"
	bgp "github.com/jwhited/corebgp"
)

// Attribute defines a BGP path attribute interface.
type Attribute interface {
	Encode() ([]byte, error)
	Type() uint8
}

// OriginAttrType represents an attribute type for origin.
const (
	OriginAttrType    = 1
	ASPathAttrType    = 2
	NextHopAttrType   = 3
	LocalPrefAttrType = 5
)

// ==== Реализация атрибутов ====

// AttributeOrigin represents the origin type of a BGP attribute.
type AttributeOrigin uint8

// AttributeOrigin constants represent the possible origins of a BGP attribute.
const (
	OriginIGP        AttributeOrigin = 0
	OriginEGP        AttributeOrigin = 1
	OriginINCOMPLETE AttributeOrigin = 2
)

// Encode serializes the AttributeOrigin into a byte array.
func (a AttributeOrigin) Encode() ([]byte, error) {
	return []byte{0x40, OriginAttrType, 1, byte(a)}, nil
}

// Type returns the type associated with the AttributeOrigin.
func (a AttributeOrigin) Type() uint8 { return OriginAttrType }

// AttributeASPath represents an AS path attribute containing a list of Autonomous System Numbers (ASNs).
type AttributeASPath struct {
	ASNs []uint16
}

// writeEndOfRIB Represents the end of a Route Information Base (RIB) with a value of 0x00000000.
func writeEndOfRIB(log *logger.Logger, peer string, rw bgp.UpdateMessageWriter) error {
	if err := rw.WriteUpdate([]byte{0, 0, 0, 0}); err != nil {
		log.Error(
			"could not write end-of-rib",
			logger.String("peer", peer),
			logger.Err(err),
		)

		return err
	}

	return nil
}

// Encode serializes the AttributeASPath into a byte slice according to BGP protocol specifications.
func (a *AttributeASPath) Encode() ([]byte, error) {
	if len(a.ASNs) == 0 {
		// Нормативно корректно: флаг, тип, длина=0
		return []byte{0x40, ASPathAttrType, 0x00}, nil
	}

	var segment bytes.Buffer
	segment.WriteByte(2)                 // AS_SEQUENCE
	segment.WriteByte(byte(len(a.ASNs))) // Кол-во ASN

	for _, asn := range a.ASNs {
		if err := binary.Write(&segment, binary.BigEndian, asn); err != nil {
			return nil, err
		}
	}

	data := segment.Bytes()
	length := len(data)

	var buf bytes.Buffer
	buf.WriteByte(0x40)           // Флаг (well-known, transitive)
	buf.WriteByte(ASPathAttrType) // Тип = 2
	buf.WriteByte(byte(length))   // Длина
	buf.Write(data)

	return buf.Bytes(), nil
}

// Type returns the type of the AS-Path attribute, which is a constant value.
func (a *AttributeASPath) Type() uint8 { return ASPathAttrType }

// AttributeNextHop represents the next hop IP address in a BGP path attribute.
type AttributeNextHop struct {
	IP net.IP
}

// Encode encodes the AttributeNextHop into a byte array. It only supports IPv4 addresses.
func (a *AttributeNextHop) Encode() ([]byte, error) {
	ip4 := a.IP.To4()
	if ip4 == nil {
		return nil, fmt.Errorf("only IPv4 supported in NextHop")
	}
	return []byte{0x40, NextHopAttrType, 4, ip4[0], ip4[1], ip4[2], ip4[3]}, nil
}

// Type returns the attribute type for next hop, which is a constant value.
func (a *AttributeNextHop) Type() uint8 { return NextHopAttrType }

// AttributeLocalPref represents a BGP LOCAL_PREF attribute with a preference value.
type AttributeLocalPref struct {
	Pref uint32
}

// Encode converts the AttributeLocalPref to a byte slice representation of the BGP attribute.
// It returns a byte slice containing the encoded attribute and an error, if any.
func (a *AttributeLocalPref) Encode() ([]byte, error) {
	return []byte{
		0x40, LocalPrefAttrType, 4,
		byte(a.Pref >> 24), byte(a.Pref >> 16), byte(a.Pref >> 8), byte(a.Pref),
	}, nil
}

// Type returns the type of the BGP attribute represented by AttributeLocalPref.
func (a *AttributeLocalPref) Type() uint8 { return LocalPrefAttrType }

// buildUpdateMessage creates a BGP update message with withdrawn routes,
// path attributes, and NLRI from given inputs. It serializes the data into
// a byte slice suitable for transmission over a BGP session.
func buildUpdateMessage(
	updates []net.IPNet,
	removes []net.IPNet,
	attributes ...Attribute,
) ([]byte, error) {
	var buf bytes.Buffer

	// Withdrawn Routes
	withdrawnBuf := new(bytes.Buffer)
	for _, prefix := range removes {
		if err := encodePrefix(withdrawnBuf, prefix); err != nil {
			return nil, err
		}
	}

	if err := binary.Write(&buf, binary.BigEndian, uint16(withdrawnBuf.Len())); err != nil { // nolint:gosec
		return nil, err
	}

	buf.Write(withdrawnBuf.Bytes())

	// Total Path Attributes Length
	attrBuf := new(bytes.Buffer)
	for _, attr := range attributes {
		data, err := attr.Encode()
		if err != nil {
			return nil, fmt.Errorf("encode attribute: %w", err)
		}
		attrBuf.Write(data)
	}

	if err := binary.Write(&buf, binary.BigEndian, uint16(attrBuf.Len())); err != nil { // nolint:gosec
		return nil, err
	}

	buf.Write(attrBuf.Bytes())
	for _, prefix := range updates {
		if err := encodePrefix(&buf, prefix); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

// encodePrefix writes the encoded prefix of a given IP network into a buffer.
// It supports only IPv4 addresses. The function returns an error if the
// input is not an IPv4 address or if encoding fails.
func encodePrefix(buf *bytes.Buffer, prefix net.IPNet) error {
	ones, _ := prefix.Mask.Size()
	buf.WriteByte(uint8(ones)) // nolint:gosec

	ip := prefix.IP.To4()
	if ip == nil {
		return fmt.Errorf("only IPv4 supported")
	}

	numBytes := (ones + 7) / 8
	buf.Write(ip[:numBytes])
	return nil
}
