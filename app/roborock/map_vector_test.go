package roborock

import (
	"encoding/binary"
	"encoding/json"
	"reflect"
	"sort"
	"testing"
)

func positionBytes(x, y, angle int32) []byte {
	data := make([]byte, 12)
	binary.LittleEndian.PutUint32(data[0:4], uint32(x))
	binary.LittleEndian.PutUint32(data[4:8], uint32(y))
	binary.LittleEndian.PutUint32(data[8:12], uint32(angle))
	return data
}

func TestParsePositionBlocksFromPayload(t *testing.T) {
	mapData := &MapData{}
	shortHeader := make([]byte, 8)
	parseBlock(mapData, BlockRobotPosition, shortHeader, positionBytes(6450, 13450, -45))
	parseBlock(mapData, BlockCharger, shortHeader, positionBytes(6100, 13000, 90))

	if mapData.Robot == nil || mapData.Robot.X != 6450 || mapData.Robot.Y != 13450 || mapData.Robot.Angle != -45 {
		t.Fatalf("robot position = %+v, want payload coordinates", mapData.Robot)
	}
	if mapData.Charger == nil || mapData.Charger.X != 6100 || mapData.Charger.Y != 13000 || mapData.Charger.Angle != 90 {
		t.Fatalf("charger position = %+v, want payload coordinates", mapData.Charger)
	}
}

func TestParsePositionBlockRetainsLegacyHeaderFallback(t *testing.T) {
	header := append(make([]byte, 8), positionBytes(5000, 7500, 180)...)
	mapData := &MapData{}
	parseBlock(mapData, BlockRobotPosition, header, nil)

	if mapData.Robot == nil || mapData.Robot.X != 5000 || mapData.Robot.Y != 7500 || mapData.Robot.Angle != 180 {
		t.Fatalf("robot position = %+v, want legacy header coordinates", mapData.Robot)
	}
}

func TestPayloadRobotPositionResolvesCurrentRoom(t *testing.T) {
	mapData := &MapData{Image: &MapImage{
		Top: 200, Left: 100, Width: 3, Height: 1,
		Pixels: []byte{encodedRoomPixel(7), encodedRoomPixel(7), encodedRoomPixel(7)},
	}}
	parseBlock(mapData, BlockRobotPosition, make([]byte, 8), positionBytes(5050, 10000, 0))

	vectorJSON, err := MapToVectorJSON(mapData)
	if err != nil {
		t.Fatal(err)
	}
	room, err := CurrentRoomFromVectorJSON(vectorJSON, map[string]string{"7": "Cuisine"})
	if err != nil {
		t.Fatal(err)
	}
	if room == nil || room.ID != 7 || room.Name != "Cuisine" {
		t.Fatalf("current room = %+v, want Cuisine (7)", room)
	}
}

func encodedRoomPixel(segmentID int) byte {
	return byte(segmentID<<3 | 0x07)
}

func TestClassifyPixelDecodesRoborockRoomBits(t *testing.T) {
	tests := []struct {
		name     string
		value    byte
		wantType PixelType
		wantRoom int
	}{
		{name: "outside", value: 0, wantType: PixelEmpty},
		{name: "wall", value: 1, wantType: PixelWall},
		{name: "inside", value: 255, wantType: PixelFloor},
		{name: "scan", value: 7, wantType: PixelFloor},
		{name: "room 7", value: encodedRoomPixel(7), wantType: PixelRoom, wantRoom: 7},
		{name: "room 23", value: encodedRoomPixel(23), wantType: PixelRoom, wantRoom: 23},
		{name: "technical pixel", value: byte(23<<3 | 0x04), wantType: PixelFloor},
		{name: "technical wall", value: byte(23 << 3), wantType: PixelWall},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotRoom := ClassifyPixel(tt.value)
			if gotType != tt.wantType || gotRoom != tt.wantRoom {
				t.Fatalf("ClassifyPixel(%d) = (%d, %d), want (%d, %d)", tt.value, gotType, gotRoom, tt.wantType, tt.wantRoom)
			}
		})
	}
}

func TestMapToVectorJSONExcludesTechnicalPixelIDs(t *testing.T) {
	mapData := &MapData{Image: &MapImage{
		Width:  8,
		Height: 1,
		Pixels: []byte{
			encodedRoomPixel(7),
			encodedRoomPixel(8),
			encodedRoomPixel(23),
			byte(23<<3 | 0x04),
			byte(15 << 3),
			1,
			255,
			0,
		},
	}}
	data, err := MapToVectorJSON(mapData)
	if err != nil {
		t.Fatal(err)
	}
	var vector VectorMap
	if err := json.Unmarshal(data, &vector); err != nil {
		t.Fatal(err)
	}
	ids := make([]int, 0, len(vector.Rooms))
	for _, room := range vector.Rooms {
		ids = append(ids, room.ID)
	}
	sort.Ints(ids)
	if want := []int{7, 8, 23}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("rooms = %v, want %v", ids, want)
	}
}

func TestCurrentRoomCanUseGeometricSegmentOutsideCommandableInventory(t *testing.T) {
	room := FindCurrentRoom(&VectorMap{
		Robot: &VectorPosition{X: 2, Y: 3},
		Rooms: []VectorRoom{{ID: 15, Spans: []VectorSpan{{X: 0, Y: 3, W: 5}}}},
	}, map[string]string{})
	if room == nil || room.ID != 15 || room.Name != "Room 15" {
		t.Fatalf("current room = %+v, want geometric Room 15", room)
	}
}
