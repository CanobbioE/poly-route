package codec

import (
	"fmt"

	"google.golang.org/protobuf/proto"
)

// PassThrough defines a codec that does not have knowledge of what is being sent or received.
// Its only job is to pass data as a stream of bytes.
type PassThrough struct{}

// Marshal returns the wire format of v.
func (*PassThrough) Marshal(v any) ([]byte, error) {
	switch t := v.(type) {
	case proto.Message:
		return proto.Marshal(t)
	case []byte:
		return t, nil
	case *[]byte:
		return *t, nil
	}
	return nil, fmt.Errorf("codec: unsupported type: %T", v)
}

// Unmarshal parses the wire format into v.
func (*PassThrough) Unmarshal(data []byte, v any) error {
	switch t := v.(type) {
	case proto.Message:
		return proto.Unmarshal(data, v.(proto.Message))
	case []byte:
		return nil
	case *[]byte:
		*t = data
		return nil
	}
	return fmt.Errorf("codec: unsupported type: %T", v)
}

// Name returns the codec name.
func (*PassThrough) Name() string {
	return "passthrough"
}
