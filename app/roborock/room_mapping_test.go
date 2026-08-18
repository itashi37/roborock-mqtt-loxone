package roborock

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestBuildGetRoomMappingPayloadUsesExpectedRPCMethod(t *testing.T) {
	payload, requestID, err := BuildGetRoomMappingPayload()
	if err != nil {
		t.Fatal(err)
	}
	var message MQTTMessage
	if err := json.Unmarshal(payload, &message); err != nil {
		t.Fatal(err)
	}
	encoded, ok := message.DPS["101"].(string)
	if !ok {
		t.Fatalf("missing DPS 101 string: %#v", message.DPS)
	}
	var request IPCRequest
	if err := json.Unmarshal([]byte(encoded), &request); err != nil {
		t.Fatal(err)
	}
	if request.Method != "get_room_mapping" || request.ID != requestID {
		t.Fatalf("request = %+v, generated ID = %d", request, requestID)
	}
}

func TestParseRoomMappingsSupportsRealResponses(t *testing.T) {
	data := []byte(`[[23,"23364799",12],[7,"2219685"],[8,2219691,9],[23,"duplicate",1]]`)
	got, err := ParseRoomMappings(data)
	if err != nil {
		t.Fatal(err)
	}
	want := []RoomMapping{
		{SegmentID: 7, HomeRoomID: "2219685"},
		{SegmentID: 8, HomeRoomID: "2219691", RoomType: 9},
		{SegmentID: 23, HomeRoomID: "23364799", RoomType: 12},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestCommandableRoomNamesJoinsCloudIDsAndExcludesHistoricalRooms(t *testing.T) {
	mappings := []RoomMapping{
		{SegmentID: 7, HomeRoomID: "23364799"},
		{SegmentID: 8, HomeRoomID: "23364800"},
		{SegmentID: 23, HomeRoomID: "23364801"},
	}
	homeNames := map[string]string{
		"23364799": "Cuisine",
		"23364800": "Salon",
		"23364801": "Entrée",
		"99999999": "Ancienne pièce",
	}
	got := CommandableRoomNames(
		mappings,
		homeNames,
		map[string]string{"8": "Séjour", "15": "Ancien segment"},
		map[string]string{"23": "Hall", "99999999": "Parasite"},
	)
	want := map[string]string{"7": "Cuisine", "8": "Séjour", "23": "Hall"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestCommandableRoomNamesFailsClosedWithoutMapping(t *testing.T) {
	got := CommandableRoomNames(nil, map[string]string{"23364799": "Cuisine"}, map[string]string{"23": "Cuisine"}, nil)
	if len(got) != 0 {
		t.Fatalf("got %#v, want no commandable rooms", got)
	}
}
