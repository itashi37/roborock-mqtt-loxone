package roborock

import (
	"encoding/binary"
	"fmt"

	"github.com/philipparndt/go-logger"
)

// Block types in the Roborock map format.
const (
	BlockCharger       = 1
	BlockImage         = 2
	BlockVacuumPath    = 3
	BlockGoToPath      = 4
	BlockPredictedPath = 5
	BlockCleanedZones  = 6
	BlockGoToTarget    = 7
	BlockRobotPosition = 8
	BlockNoGoZones     = 9
	BlockVirtualWalls  = 10
	BlockMapBlocks     = 11
	BlockNoMopZones    = 12
)

// DebugBlock stores metadata and parsed data from a map block for debugging.
type DebugBlock struct {
	Type      int
	HeaderLen int
	DataLen   int
	Points    []MapPoint // parsed coordinate pairs (if applicable)
}

// MapData represents parsed Roborock map data.
type MapData struct {
	MajorVersion int
	MinorVersion int
	MapIndex     int
	MapSequence  int
	Image        *MapImage
	Charger      *MapPosition
	Robot        *MapPosition
	Path         []MapPoint
	DebugBlocks  []DebugBlock
}

// MapImage represents the pixel grid of the map.
type MapImage struct {
	Top    int
	Left   int
	Width  int
	Height int
	Pixels []byte // raw pixel data, 1 byte per pixel
}

// MapPosition represents a position on the map (robot, charger).
type MapPosition struct {
	X     int
	Y     int
	Angle int
}

// MapPoint represents a coordinate pair in a path.
type MapPoint struct {
	X int
	Y int
}

// ParseMapData parses the Roborock binary map format.
// The input should already be decompressed (post-gzip).
func ParseMapData(data []byte) (*MapData, error) {
	if len(data) < 20 {
		return nil, fmt.Errorf("map data too short: %d bytes", len(data))
	}

	// File header
	magic := string(data[0:2])
	if magic != "RR" && magic != "rr" {
		return nil, fmt.Errorf("invalid map magic: %q", magic)
	}

	headerLen := int(binary.LittleEndian.Uint16(data[2:4]))
	_ = binary.LittleEndian.Uint32(data[4:8]) // data length

	mapData := &MapData{
		MajorVersion: int(binary.LittleEndian.Uint16(data[8:10])),
		MinorVersion: int(binary.LittleEndian.Uint16(data[10:12])),
		MapIndex:     int(binary.LittleEndian.Uint32(data[12:16])),
		MapSequence:  int(binary.LittleEndian.Uint32(data[16:20])),
	}

	// Parse blocks
	offset := headerLen
	for offset+8 <= len(data) {
		blockType := int(binary.LittleEndian.Uint16(data[offset : offset+2]))
		blockHeaderLen := int(binary.LittleEndian.Uint16(data[offset+2 : offset+4]))
		blockDataLen := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))

		if offset+blockHeaderLen+blockDataLen > len(data) {
			logger.Debug("Map block truncated", "type", blockType, "offset", offset)
			break
		}

		blockHeader := data[offset : offset+blockHeaderLen]
		blockData := data[offset+blockHeaderLen : offset+blockHeaderLen+blockDataLen]

		logger.Debug("Map block", "type", blockType, "headerLen", blockHeaderLen, "dataLen", blockDataLen, "offset", offset)
		parseBlock(mapData, blockType, blockHeader, blockData)

		offset += blockHeaderLen + blockDataLen
	}

	if mapData.Image != nil {
		logger.Debug("Map image parsed", "width", mapData.Image.Width, "height", mapData.Image.Height, "pixels", len(mapData.Image.Pixels), "top", mapData.Image.Top, "left", mapData.Image.Left)
	} else {
		logger.Debug("No image block found in map data")
	}

	return mapData, nil
}

func parseBlock(md *MapData, blockType int, header []byte, data []byte) {
	// Record every block for debug visualization
	db := DebugBlock{
		Type:      blockType,
		HeaderLen: len(header),
		DataLen:   len(data),
	}

	switch blockType {
	case BlockCharger:
		if len(header) >= 16 {
			md.Charger = &MapPosition{
				X: int(binary.LittleEndian.Uint32(header[8:12])),
				Y: int(binary.LittleEndian.Uint32(header[12:16])),
			}
			db.Points = []MapPoint{{X: md.Charger.X, Y: md.Charger.Y}}
		}
	case BlockImage:
		parseImageBlock(md, header, data)
	case BlockVacuumPath:
		parsPathBlock(md, header, data)
	case BlockRobotPosition:
		if len(header) >= 16 {
			md.Robot = &MapPosition{
				X: int(binary.LittleEndian.Uint32(header[8:12])),
				Y: int(binary.LittleEndian.Uint32(header[12:16])),
			}
			if len(header) >= 20 {
				md.Robot.Angle = int(binary.LittleEndian.Uint32(header[16:20]))
			}
			db.Points = []MapPoint{{X: md.Robot.X, Y: md.Robot.Y}}
		}
	case BlockMapBlocks:
		// Block 11 contains map block metadata, not the authoritative list of
		// app_segment_clean IDs. Commandable rooms come from get_room_mapping.
	case BlockGoToPath, BlockPredictedPath:
		parsPathBlock(md, header, data)
	case BlockGoToTarget:
		if len(header) >= 16 {
			db.Points = []MapPoint{{
				X: int(binary.LittleEndian.Uint32(header[8:12])),
				Y: int(binary.LittleEndian.Uint32(header[12:16])),
			}}
		}
	case BlockCleanedZones, BlockNoGoZones, BlockVirtualWalls, BlockNoMopZones:
		// Zone/wall blocks: data contains int32 coordinate pairs
		db.Points = tryParseCoordinates(header, data)
	case 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 24, 25, 26, 28, 30, 31, 32:
		// Known block types we don't fully parse yet — extract coordinates if possible
		db.Points = tryParseCoordinates(header, data)
	case 1024:
		// Digest block — ignore
	default:
		db.Points = tryParseCoordinates(header, data)
		logger.Debug("Truly unknown map block type", "type", blockType)
	}

	md.DebugBlocks = append(md.DebugBlocks, db)
}

// tryParseCoordinates attempts to extract coordinate pairs from block data.
// Many Roborock blocks store int32 coordinate pairs in the header (offset 8+)
// or int16 pairs in the data section.
func tryParseCoordinates(header []byte, data []byte) []MapPoint {
	var points []MapPoint

	// Try header coordinates (int32 pairs starting at offset 8)
	if len(header) >= 16 {
		for i := 8; i+7 < len(header); i += 8 {
			x := int(int32(binary.LittleEndian.Uint32(header[i : i+4])))
			y := int(int32(binary.LittleEndian.Uint32(header[i+4 : i+8])))
			if x > -100000 && x < 100000 && y > -100000 && y < 100000 {
				points = append(points, MapPoint{X: x, Y: y})
			}
		}
	}

	// Try data as int16 coordinate pairs (common for zone/wall data)
	if len(data) >= 4 && len(data) <= 4096 && len(data)%4 == 0 {
		for i := 0; i+3 < len(data); i += 4 {
			x := int(int16(binary.LittleEndian.Uint16(data[i : i+2])))
			y := int(int16(binary.LittleEndian.Uint16(data[i+2 : i+4])))
			if x > -10000 && x < 100000 && y > -10000 && y < 100000 {
				points = append(points, MapPoint{X: x, Y: y})
			}
		}
	}

	// Try data as int32 coordinate pairs
	if len(data) >= 8 && len(data) <= 4096 && len(data)%8 == 0 && len(points) == 0 {
		for i := 0; i+7 < len(data); i += 8 {
			x := int(int32(binary.LittleEndian.Uint32(data[i : i+4])))
			y := int(int32(binary.LittleEndian.Uint32(data[i+4 : i+8])))
			if x > -100000 && x < 100000 && y > -100000 && y < 100000 {
				points = append(points, MapPoint{X: x, Y: y})
			}
		}
	}

	return points
}

func parseImageBlock(md *MapData, header []byte, data []byte) {
	if len(header) < 24 {
		return
	}

	var top, left, height, width int

	if len(header) >= 28 {
		// 28-byte header: [8:12]=flags, [12:16]=top, [16:20]=left, [20:24]=height, [24:28]=width
		top = int(binary.LittleEndian.Uint32(header[12:16]))
		left = int(binary.LittleEndian.Uint32(header[16:20]))
		height = int(binary.LittleEndian.Uint32(header[20:24]))
		width = int(binary.LittleEndian.Uint32(header[24:28]))
	} else {
		top = int(binary.LittleEndian.Uint32(header[8:12]))
		left = int(binary.LittleEndian.Uint32(header[12:16]))
		height = int(binary.LittleEndian.Uint32(header[16:20]))
		width = int(binary.LittleEndian.Uint32(header[20:24]))
	}

	md.Image = &MapImage{
		Top:    top,
		Left:   left,
		Height: height,
		Width:  width,
		Pixels: data,
	}
}

func parsPathBlock(md *MapData, header []byte, data []byte) {
	if len(header) < 20 {
		return
	}

	pointCount := int(binary.LittleEndian.Uint32(header[8:12]))
	_ = binary.LittleEndian.Uint32(header[12:16]) // point size
	_ = binary.LittleEndian.Uint32(header[16:20]) // angle

	md.Path = make([]MapPoint, 0, pointCount)
	for i := 0; i+3 < len(data); i += 4 {
		x := int(binary.LittleEndian.Uint16(data[i : i+2]))
		y := int(binary.LittleEndian.Uint16(data[i+2 : i+4]))
		md.Path = append(md.Path, MapPoint{X: x, Y: y})
	}
}

// PixelType classifies a pixel value from the map image.
type PixelType int

const (
	PixelEmpty   PixelType = 0
	PixelWall    PixelType = 1
	PixelFloor   PixelType = 2
	PixelRoom    PixelType = 3
)

// ClassifyPixel determines the type and room ID of a Roborock map pixel.
// The low three bits describe the pixel type. Room pixels use type 7 and
// store the segment ID in the upper five bits.
func ClassifyPixel(value byte) (PixelType, int) {
	switch {
	case value == 0:
		return PixelEmpty, 0
	case value == 1:
		return PixelWall, 0
	case value == 255 || value == 7:
		return PixelFloor, 0
	}

	pixelType := value & 0x07
	if pixelType == 7 {
		roomID := int(value >> 3)
		if roomID > 0 {
			return PixelRoom, roomID
		}
		return PixelFloor, 0
	}
	if pixelType == 0 || pixelType == 1 {
		return PixelWall, 0
	}
	return PixelFloor, 0
}
