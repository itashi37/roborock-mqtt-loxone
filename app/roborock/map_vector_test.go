package roborock

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"
)

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
