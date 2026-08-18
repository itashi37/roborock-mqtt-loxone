package roborock

import (
	"encoding/json"
	"fmt"
)

// VectorDebugBlock contains debug info for a single map block.
type VectorDebugBlock struct {
	Type      int      `json:"type"`
	Label     string   `json:"label"`
	HeaderLen int      `json:"header_len"`
	DataLen   int      `json:"data_len"`
	Points    [][2]int `json:"points,omitempty"`
}

// VectorMap is the JSON-serializable vector representation of a map.
type VectorMap struct {
	Width       int                `json:"width"`
	Height      int                `json:"height"`
	Rooms       []VectorRoom       `json:"rooms"`
	Walls       []VectorSpan       `json:"walls"`
	Floor       []VectorSpan       `json:"floor"`
	Path        [][2]int           `json:"path"`
	Charger     *VectorPosition    `json:"charger,omitempty"`
	Robot       *VectorPosition    `json:"robot,omitempty"`
	RoomNames   map[string]string  `json:"room_names,omitempty"`
	DebugBlocks []VectorDebugBlock `json:"debug_blocks,omitempty"`
}

// VectorEdge represents a line segment for room outlines.
type VectorEdge struct {
	X1 int `json:"x1"`
	Y1 int `json:"y1"`
	X2 int `json:"x2"`
	Y2 int `json:"y2"`
}

// VectorRoom groups run-length encoded spans for a single room.
type VectorRoom struct {
	ID      int           `json:"id"`
	Color   string        `json:"color"`
	Center  [2]int        `json:"center"`
	Spans   []VectorSpan  `json:"spans"`
	Outline []VectorEdge  `json:"outline,omitempty"`
}

// VectorSpan is a horizontal run of pixels: row Y, from X to X+W.
type VectorSpan struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
}

// VectorPosition represents a point with optional angle.
type VectorPosition struct {
	X     int `json:"x"`
	Y     int `json:"y"`
	Angle int `json:"angle,omitempty"`
}

// hexColors maps room color palette — bright colors for dark backgrounds.
var hexColors = []string{
	"#00D4AA", "#FF8C42", "#A855F7", "#3B82F6",
	"#22C55E", "#EC4899", "#06B6D4", "#F59E0B",
	"#14B8A6", "#F43F5E", "#8B5CF6", "#84CC16",
	"#0EA5E9", "#D946EF", "#10B981", "#FB923C",
}

// MapToVectorJSON converts parsed map data to a JSON vector representation.
func MapToVectorJSON(md *MapData) ([]byte, error) {
	if md.Image == nil || md.Image.Width == 0 || md.Image.Height == 0 {
		return nil, nil
	}

	w, h := md.Image.Width, md.Image.Height
	vm := VectorMap{
		Width:  w,
		Height: h,
	}

	// Collect spans per room ID
	roomSpans := make(map[int][]VectorSpan)

	for y := 0; y < h; y++ {
		// Flip Y for display (bottom-up to top-down)
		srcY := h - 1 - y
		x := 0
		for x < w {
			idx := srcY*w + x
			if idx >= len(md.Image.Pixels) {
				x++
				continue
			}

			pType, roomID := ClassifyPixel(md.Image.Pixels[idx])
			if pType == PixelEmpty {
				x++
				continue
			}

			// Run-length: find how far this same type+room extends
			startX := x
			for x < w {
				idx2 := srcY*w + x
				if idx2 >= len(md.Image.Pixels) {
					break
				}
				pType2, roomID2 := ClassifyPixel(md.Image.Pixels[idx2])
				if pType2 != pType || roomID2 != roomID {
					break
				}
				x++
			}

			span := VectorSpan{X: startX, Y: y, W: x - startX}

			switch pType {
			case PixelWall:
				vm.Walls = append(vm.Walls, span)
			case PixelFloor:
				vm.Floor = append(vm.Floor, span)
			case PixelRoom:
				roomSpans[roomID] = append(roomSpans[roomID], span)
			}
		}
	}

	// Convert room spans map to sorted list with centroids and outlines
	for id, spans := range roomSpans {
		var sumX, sumY, totalW int
		for _, s := range spans {
			sumX += (s.X + s.X + s.W) * s.W / 2
			sumY += s.Y * s.W
			totalW += s.W
		}
		cx, cy := 0, 0
		if totalW > 0 {
			cx = sumX / totalW
			cy = sumY / totalW
		}

		outline := computeRoomOutline(spans, id, roomSpans)

		vm.Rooms = append(vm.Rooms, VectorRoom{
			ID:      id,
			Color:   hexColors[id%len(hexColors)],
			Center:  [2]int{cx, cy},
			Spans:   spans,
			Outline: outline,
		})
	}

	// Path coordinates (flipped Y)
	for _, p := range md.Path {
		px := (p.X / 50) - md.Image.Left
		py := h - 1 - ((p.Y / 50) - md.Image.Top)
		vm.Path = append(vm.Path, [2]int{px, py})
	}

	// Positions (flipped Y)
	if md.Charger != nil {
		cx := (md.Charger.X / 50) - md.Image.Left
		cy := h - 1 - ((md.Charger.Y / 50) - md.Image.Top)
		vm.Charger = &VectorPosition{X: cx, Y: cy}
	}
	if md.Robot != nil {
		rx := (md.Robot.X / 50) - md.Image.Left
		ry := h - 1 - ((md.Robot.Y / 50) - md.Image.Top)
		vm.Robot = &VectorPosition{X: rx, Y: ry, Angle: md.Robot.Angle}
	}

	// Debug blocks
	for _, db := range md.DebugBlocks {
		vdb := VectorDebugBlock{
			Type:      db.Type,
			Label:     blockTypeLabel(db.Type),
			HeaderLen: db.HeaderLen,
			DataLen:   db.DataLen,
		}
		for _, p := range db.Points {
			px := (p.X / 50) - md.Image.Left
			py := h - 1 - ((p.Y / 50) - md.Image.Top)
			vdb.Points = append(vdb.Points, [2]int{px, py})
		}
		vm.DebugBlocks = append(vm.DebugBlocks, vdb)
	}

	return json.Marshal(vm)
}

// computeRoomOutline finds the boundary edges of a room by checking
// which span edges border non-room pixels.
func computeRoomOutline(spans []VectorSpan, roomID int, allRoomSpans map[int][]VectorSpan) []VectorEdge {
	// Build a set of all pixels belonging to this room for fast lookup
	type pixel struct{ x, y int }
	roomPixels := make(map[pixel]bool)
	for _, s := range spans {
		for x := s.X; x < s.X+s.W; x++ {
			roomPixels[pixel{x, s.Y}] = true
		}
	}

	var edges []VectorEdge

	for _, s := range spans {
		// Top edge: if pixel above is not this room
		if !roomPixels[pixel{s.X, s.Y - 1}] {
			// Find how far the top edge extends
			endX := s.X
			for endX < s.X+s.W && !roomPixels[(pixel{endX, s.Y - 1})] {
				endX++
			}
			if endX > s.X {
				edges = append(edges, VectorEdge{s.X, s.Y, endX, s.Y})
			}
		}
		// Bottom edge
		if !roomPixels[pixel{s.X, s.Y + 1}] {
			endX := s.X
			for endX < s.X+s.W && !roomPixels[(pixel{endX, s.Y + 1})] {
				endX++
			}
			if endX > s.X {
				edges = append(edges, VectorEdge{s.X, s.Y + 1, endX, s.Y + 1})
			}
		}
		// Left edge
		if !roomPixels[pixel{s.X - 1, s.Y}] {
			edges = append(edges, VectorEdge{s.X, s.Y, s.X, s.Y + 1})
		}
		// Right edge
		endX := s.X + s.W
		if !roomPixels[pixel{endX, s.Y}] {
			edges = append(edges, VectorEdge{endX, s.Y, endX, s.Y + 1})
		}
	}

	return edges
}

func blockTypeLabel(id int) string {
	switch id {
	case BlockCharger:
		return "Charger"
	case BlockImage:
		return "Image"
	case BlockVacuumPath:
		return "Vacuum Path"
	case BlockGoToPath:
		return "Go-To Path"
	case BlockPredictedPath:
		return "Predicted Path"
	case BlockCleanedZones:
		return "Cleaned Zones"
	case BlockGoToTarget:
		return "Go-To Target"
	case BlockRobotPosition:
		return "Robot Position"
	case BlockNoGoZones:
		return "No-Go Zones"
	case BlockVirtualWalls:
		return "Virtual Walls"
	case BlockMapBlocks:
		return "Map Blocks"
	case BlockNoMopZones:
		return "No-Mop Zones"
	case 13:
		return "Active Segments"
	case 14:
		return "No-Go Zone V2"
	case 15:
		return "No-Mop Zone V2"
	case 16:
		return "Carpet Map"
	case 17:
		return "Secondary Image"
	case 18:
		return "Furniture"
	case 19:
		return "Dock Type"
	case 20:
		return "Enemy Heatmap"
	case 21:
		return "Smart Zone"
	case 22:
		return "Custom Carpet"
	case 24:
		return "Carpet Clean Area"
	case 25:
		return "Carpet Forbidden"
	case 26:
		return "Carpet Mode"
	case 28:
		return "Floor Material"
	case 30:
		return "Mop Path"
	case 31:
		return "Obstacle Info"
	case 32:
		return "AI Object Detection"
	case 1024:
		return "Digest"
	}
	return fmt.Sprintf("Block %d", id)
}
