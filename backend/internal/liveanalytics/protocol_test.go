package liveanalytics

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"testing"
)

func TestDecodePacketsPlainAndZlibBatch(t *testing.T) {
	first := EncodePacket(OpMessage, []byte(`{"cmd":"LIVE_OPEN_PLATFORM_DM"}`))
	second := EncodePacket(OpMessage, []byte(`{"cmd":"FUTURE_EVENT"}`))
	var compressed bytes.Buffer
	writer := zlib.NewWriter(&compressed)
	if _, err := writer.Write(append(first, second...)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	outer := EncodePacket(OpMessage, compressed.Bytes())
	binary.BigEndian.PutUint16(outer[6:8], ProtocolZlib)

	packets, err := DecodePackets(outer)
	if err != nil {
		t.Fatal(err)
	}
	if len(packets) != 2 || string(packets[0].Body) != `{"cmd":"LIVE_OPEN_PLATFORM_DM"}` || string(packets[1].Body) != `{"cmd":"FUTURE_EVENT"}` {
		t.Fatalf("unexpected packets: %#v", packets)
	}
}

func TestDecodePacketsRejectsTruncatedAndUnknownProtocol(t *testing.T) {
	if _, err := DecodePackets([]byte{1, 2, 3}); err == nil {
		t.Fatal("expected truncated packet error")
	}
	packet := EncodePacket(OpMessage, nil)
	binary.BigEndian.PutUint16(packet[6:8], 3)
	if _, err := DecodePackets(packet); !errors.Is(err, ErrUnsupportedProtocol) {
		t.Fatalf("expected unsupported protocol error, got %v", err)
	}
}

func TestDecodePacketsBoundsExpansionAndNesting(t *testing.T) {
	compress := func(body []byte) []byte {
		var buf bytes.Buffer
		w := zlib.NewWriter(&buf)
		if _, err := w.Write(body); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		packet := EncodePacket(OpMessage, buf.Bytes())
		binary.BigEndian.PutUint16(packet[6:8], ProtocolZlib)
		return packet
	}
	oversized := compress(bytes.Repeat([]byte{0}, maxDecodedBytes+1))
	if _, err := DecodePackets(oversized); err == nil {
		t.Fatal("unbounded expansion")
	}
	nested := EncodePacket(OpMessage, []byte("{}"))
	for i := 0; i < 6; i++ {
		nested = compress(nested)
	}
	if _, err := DecodePackets(nested); err == nil {
		t.Fatal("unbounded nesting")
	}
}
