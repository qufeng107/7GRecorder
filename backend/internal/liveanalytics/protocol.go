package liveanalytics

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	HeaderLength      = 16
	OpHeartbeat       = 2
	OpHeartbeatReply  = 3
	OpMessage         = 5
	OpAuth            = 7
	OpAuthReply       = 8
	ProtocolPlain     = 0
	ProtocolZlib      = 2
	defaultSequenceID = 1
)

const maxDecodedBytes = 16 << 20

var ErrUnsupportedProtocol = errors.New("unsupported OpenLive protocol version")

type Packet struct {
	Version   uint16
	Operation uint32
	Body      []byte
}

func EncodePacket(operation uint32, body []byte) []byte {
	packet := make([]byte, HeaderLength+len(body))
	binary.BigEndian.PutUint32(packet[0:4], uint32(len(packet)))
	binary.BigEndian.PutUint16(packet[4:6], HeaderLength)
	binary.BigEndian.PutUint16(packet[6:8], ProtocolPlain)
	binary.BigEndian.PutUint32(packet[8:12], operation)
	binary.BigEndian.PutUint32(packet[12:16], defaultSequenceID)
	copy(packet[HeaderLength:], body)
	return packet
}

func DecodePackets(data []byte) ([]Packet, error) {
	budget := maxDecodedBytes
	return decodePackets(data, 0, &budget)
}

func decodePackets(data []byte, depth int, budget *int) ([]Packet, error) {
	if depth > 4 || len(data) > maxDecodedBytes {
		return nil, fmt.Errorf("OpenLive decode limit exceeded")
	}
	var packets []Packet
	for len(data) > 0 {
		if len(data) < HeaderLength {
			return nil, fmt.Errorf("OpenLive packet header truncated: %d bytes", len(data))
		}
		packetLength := int(binary.BigEndian.Uint32(data[0:4]))
		headerLength := int(binary.BigEndian.Uint16(data[4:6]))
		if headerLength < HeaderLength || packetLength < headerLength || packetLength > len(data) {
			return nil, fmt.Errorf("invalid OpenLive packet length=%d header=%d available=%d", packetLength, headerLength, len(data))
		}
		version := binary.BigEndian.Uint16(data[6:8])
		operation := binary.BigEndian.Uint32(data[8:12])
		body := append([]byte(nil), data[headerLength:packetLength]...)
		switch version {
		case ProtocolPlain:
			packets = append(packets, Packet{Version: version, Operation: operation, Body: body})
		case ProtocolZlib:
			reader, err := zlib.NewReader(bytes.NewReader(body))
			if err != nil {
				return nil, fmt.Errorf("open OpenLive zlib payload: %w", err)
			}
			decoded, readErr := io.ReadAll(io.LimitReader(reader, int64(*budget)+1))
			closeErr := reader.Close()
			if readErr != nil {
				return nil, fmt.Errorf("read OpenLive zlib payload: %w", readErr)
			}
			if closeErr != nil {
				return nil, fmt.Errorf("close OpenLive zlib payload: %w", closeErr)
			}
			if len(decoded) > *budget {
				return nil, fmt.Errorf("OpenLive decoded payload exceeds budget")
			}
			*budget -= len(decoded)
			nested, err := decodePackets(decoded, depth+1, budget)
			if err != nil {
				return nil, err
			}
			packets = append(packets, nested...)
		default:
			return nil, fmt.Errorf("%w: %d", ErrUnsupportedProtocol, version)
		}
		data = data[packetLength:]
	}
	return packets, nil
}
