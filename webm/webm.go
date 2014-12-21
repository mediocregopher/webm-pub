// A small package to supplement ebmlstream with webm specfic functionality
package webm

import (
	"github.com/mediocregopher/ebmlstream/edtd"
	"github.com/mediocregopher/ebmlstream/varint"
)

type LacingMode byte
const (
	None LacingMode = iota
	Xiph
	FixedSize
	EBML
)

// Describes some of the data which can be found in a simple block
type SimpleBlock struct {
	TrackNumber uint64
	Timecode    int16
	Keyframe    bool
	Invisible   bool
	Discardable bool
	Lacing      LacingMode
}

// Assuming the given ebml element is a SimpleBlock, parses its contents out
// into the SimpleBlock structure
func NewSimpleBlock(e *edtd.Elem) (*SimpleBlock, error) {
	data, err := e.Bytes()
	if err != nil {
		return nil, err
	}
	
	return parseAsSimpleBlock(data)
}

func parseAsSimpleBlock(data []byte) (*SimpleBlock, error) {
	trackNumber, err := varint.Parse(data)
	if err != nil {
		return nil, err
	}

	trackNumber64, err := trackNumber.Uint64()
	if err != nil {
		return nil, err
	}

	trackNumberSize, err := trackNumber.Size()
	if err != nil {
		return nil, err
	}
	data = data[trackNumberSize:]

	timecode := int16(data[1]) | (int16(data[0]) << 8)
	data = data[2:]

	flags := data[0]

	lacingMode := LacingMode((flags >> 4) & 3)

	return &SimpleBlock{
		TrackNumber: trackNumber64,
		Timecode:    timecode,
		Keyframe:    flags & 0x80 == 0x80,
		Invisible:   flags & 0x08 == 0x08,
		Discardable: flags & 0x01 == 0x01,
		Lacing:      lacingMode,
	}, nil
}

// Assuming the given ebml element is a Block, parses its contents out into the
// Block structure
type Block struct {
	TrackNumber uint64
	Timecode    int16
	Keyframe    bool
	Invisible   bool
	Lacing      LacingMode
}

func NewBlock(e *edtd.Elem) (*Block, error) {
	data, err := e.Bytes()
	if err != nil {
		return nil, err
	}

	return parseAsBlock(data)
}

func parseAsBlock(data []byte) (*Block, error) {
	trackNumber, err := varint.Parse(data)
	if err != nil {
		return nil, err
	}

	trackNumber64, err := trackNumber.Uint64()
	if err != nil {
		return nil, err
	}

	trackNumberSize, err := trackNumber.Size()
	if err != nil {
		return nil, err
	}
	data = data[trackNumberSize:]

	timecode := int16(data[1]) | (int16(data[0]) << 8)
	data = data[2:]

	flags := data[0]
	lacingMode := LacingMode((flags >> 4) & 3)
	b := Block{
		TrackNumber: trackNumber64,
		Timecode:    timecode,
		Invisible:   flags & 0x08 == 0x08,
		Lacing:      lacingMode,
	}
	data = data[1:]

	if b.Lacing > 0 {
		numLaceFrames := uint8(data[0])
		data = data[1:]

		if b.Lacing == Xiph {
			for i := byte(0); i < numLaceFrames; i++ {
				for {
					throwaway := data[0]
					data = data[1:]
					if throwaway < 255 {
						break
					}
				}
			}
		} else if b.Lacing == EBML {
			for i := byte(0); i < numLaceFrames; i++ {
				throwaway, err := varint.Parse(data)
				if err != nil {
					return nil, err
				}
				throwawaySize, err := throwaway.Size()
				if err != nil {
					return nil, err
				}
				data = data[throwawaySize:]
			}
		}
	}

	b.Keyframe = data[2] & 0x01 == 0x00
	return &b, nil
}

// If the given element is a Block or a SimpleBlock, returns the track number it
// is for and whether or not it is a keyframe for that track
func AsKeyBlock(e *edtd.Elem) (uint64, bool, error) {
	if e.Name == "SimpleBlock" {
		s, err := NewSimpleBlock(e)
		if err != nil {
			return 0, false, err
		}
		return s.TrackNumber, s.Keyframe, nil
	}

	if e.Name == "Block" {
		b, err := NewBlock(e)
		if err != nil {
			return 0, false, err
		}
		return b.TrackNumber, b.Keyframe, nil
	}

	return 0, false, nil
}
