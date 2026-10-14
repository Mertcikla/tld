package store

import (
	"encoding/binary"
	"fmt"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"google.golang.org/protobuf/proto"
)

const edgeDetailsVersion byte = 1

// Edge details are storage-local: version, nullable anchor (length+1), nullable
// evidence list (count+1), then tagged evidence messages (length, protobuf).
// Tags distinguish nil entries, absent anchors, shared anchors and full anchors.
// Protobuf carries all fields, including unknown ones, without JSON overhead.
const (
	evidenceNil byte = iota
	evidenceAbsentAnchor
	evidenceSharedAnchor
	evidenceFullAnchor
)

func marshalEdgeDetails(anchor *pb.SourceAnchor, evidence []*pb.Evidence) ([]byte, error) {
	out := []byte{edgeDetailsVersion}
	if anchor == nil {
		out = binary.AppendUvarint(out, 0)
	} else {
		raw, err := proto.Marshal(anchor)
		if err != nil {
			return nil, err
		}
		out = binary.AppendUvarint(out, uint64(len(raw))+1)
		out = append(out, raw...)
	}
	if evidence == nil {
		return binary.AppendUvarint(out, 0), nil
	}
	out = binary.AppendUvarint(out, uint64(len(evidence))+1)
	for _, original := range evidence {
		if original == nil {
			out = append(out, evidenceNil)
			continue
		}
		e := proto.Clone(original).(*pb.Evidence)
		tag := evidenceFullAnchor
		switch {
		case e.Anchor == nil:
			tag = evidenceAbsentAnchor
		case anchor != nil && proto.Equal(e.Anchor, anchor):
			tag = evidenceSharedAnchor
			e.Anchor = nil
		}
		raw, err := proto.Marshal(e)
		if err != nil {
			return nil, err
		}
		out = append(out, tag)
		out = binary.AppendUvarint(out, uint64(len(raw)))
		out = append(out, raw...)
	}
	return out, nil
}

type edgeDetailsReader struct{ raw []byte }

func (r *edgeDetailsReader) number() (uint64, error) {
	n, size := binary.Uvarint(r.raw)
	if size <= 0 {
		return 0, fmt.Errorf("invalid edge details length")
	}
	r.raw = r.raw[size:]
	return n, nil
}

func (r *edgeDetailsReader) bytes(n uint64) ([]byte, error) {
	if n > uint64(len(r.raw)) {
		return nil, fmt.Errorf("truncated edge details")
	}
	raw := r.raw[:int(n)]
	r.raw = r.raw[int(n):]
	return raw, nil
}

func unmarshalEdgeDetails(raw []byte) (*pb.SourceAnchor, []*pb.Evidence, error) {
	if len(raw) == 0 || raw[0] != edgeDetailsVersion {
		return nil, nil, fmt.Errorf("unsupported edge details version")
	}
	r := edgeDetailsReader{raw: raw[1:]}
	n, err := r.number()
	if err != nil {
		return nil, nil, err
	}
	var anchor *pb.SourceAnchor
	if n != 0 {
		data, err := r.bytes(n - 1)
		if err != nil {
			return nil, nil, err
		}
		anchor = &pb.SourceAnchor{}
		if err := proto.Unmarshal(data, anchor); err != nil {
			return nil, nil, fmt.Errorf("decode edge anchor: %w", err)
		}
	}
	n, err = r.number()
	if err != nil {
		return nil, nil, err
	}
	var evidence []*pb.Evidence
	if n != 0 {
		// Every entry takes at least one byte; check before allocating from disk.
		if n-1 > uint64(len(r.raw)) {
			return nil, nil, fmt.Errorf("truncated edge evidence")
		}
		evidence = make([]*pb.Evidence, int(n-1))
		for i := range evidence {
			tags, err := r.bytes(1)
			if err != nil {
				return nil, nil, err
			}
			tag := tags[0]
			if tag == evidenceNil {
				continue
			}
			if tag > evidenceFullAnchor {
				return nil, nil, fmt.Errorf("invalid edge evidence tag %d", tag)
			}
			size, err := r.number()
			if err != nil {
				return nil, nil, err
			}
			data, err := r.bytes(size)
			if err != nil {
				return nil, nil, err
			}
			e := &pb.Evidence{}
			if err := proto.Unmarshal(data, e); err != nil {
				return nil, nil, fmt.Errorf("decode edge evidence: %w", err)
			}
			switch tag {
			case evidenceAbsentAnchor, evidenceSharedAnchor:
				if e.Anchor != nil || (tag == evidenceSharedAnchor && anchor == nil) {
					return nil, nil, fmt.Errorf("invalid shared or absent edge evidence anchor")
				}
				if tag == evidenceSharedAnchor {
					e.Anchor = proto.Clone(anchor).(*pb.SourceAnchor)
				}
			case evidenceFullAnchor:
				if e.Anchor == nil {
					return nil, nil, fmt.Errorf("missing edge evidence anchor")
				}
			}
			evidence[i] = e
		}
	}
	if len(r.raw) != 0 {
		return nil, nil, fmt.Errorf("trailing edge details bytes")
	}
	return anchor, evidence, nil
}
