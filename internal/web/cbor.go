package web

import (
	"encoding/binary"
	"fmt"
)

type cborDecoder struct {
	data []byte
	pos  int
}

func decodeCBOR(data []byte) (any, int, error) {
	d := &cborDecoder{data: data}
	value, err := d.value(0)
	return value, d.pos, err
}

func (d *cborDecoder) value(depth int) (any, error) {
	if depth > 16 || d.pos >= len(d.data) {
		return nil, fmt.Errorf("invalid CBOR")
	}
	initial := d.data[d.pos]
	d.pos++
	major, additional := initial>>5, initial&31
	length, err := d.length(additional)
	if err != nil {
		return nil, err
	}
	switch major {
	case 0:
		return int64(length), nil
	case 1:
		if length > uint64(^uint64(0)>>1) {
			return nil, fmt.Errorf("CBOR integer overflow")
		}
		return -1 - int64(length), nil
	case 2, 3:
		if length > uint64(len(d.data)-d.pos) || length > 1<<20 {
			return nil, fmt.Errorf("invalid CBOR string")
		}
		value := append([]byte(nil), d.data[d.pos:d.pos+int(length)]...)
		d.pos += int(length)
		if major == 3 {
			return string(value), nil
		}
		return value, nil
	case 4:
		if length > 1024 {
			return nil, fmt.Errorf("CBOR array too large")
		}
		items := make([]any, 0, int(length))
		for range length {
			item, err := d.value(depth + 1)
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
		return items, nil
	case 5:
		if length > 1024 {
			return nil, fmt.Errorf("CBOR map too large")
		}
		items := make(map[any]any, int(length))
		for range length {
			key, err := d.value(depth + 1)
			if err != nil {
				return nil, err
			}
			switch key.(type) {
			case string, int64:
			default:
				return nil, fmt.Errorf("unsupported CBOR map key")
			}
			value, err := d.value(depth + 1)
			if err != nil {
				return nil, err
			}
			items[key] = value
		}
		return items, nil
	case 6:
		return d.value(depth + 1)
	case 7:
		switch additional {
		case 20:
			return false, nil
		case 21:
			return true, nil
		case 22, 23:
			return nil, nil
		default:
			return nil, fmt.Errorf("unsupported CBOR simple value")
		}
	default:
		return nil, fmt.Errorf("unsupported CBOR type")
	}
}

func (d *cborDecoder) length(additional byte) (uint64, error) {
	switch {
	case additional < 24:
		return uint64(additional), nil
	case additional == 24:
		if d.pos+1 > len(d.data) {
			return 0, fmt.Errorf("truncated CBOR")
		}
		value := d.data[d.pos]
		d.pos++
		return uint64(value), nil
	case additional == 25:
		if d.pos+2 > len(d.data) {
			return 0, fmt.Errorf("truncated CBOR")
		}
		value := binary.BigEndian.Uint16(d.data[d.pos:])
		d.pos += 2
		return uint64(value), nil
	case additional == 26:
		if d.pos+4 > len(d.data) {
			return 0, fmt.Errorf("truncated CBOR")
		}
		value := binary.BigEndian.Uint32(d.data[d.pos:])
		d.pos += 4
		return uint64(value), nil
	case additional == 27:
		if d.pos+8 > len(d.data) {
			return 0, fmt.Errorf("truncated CBOR")
		}
		value := binary.BigEndian.Uint64(d.data[d.pos:])
		d.pos += 8
		return value, nil
	default:
		return 0, fmt.Errorf("indefinite CBOR is unsupported")
	}
}
