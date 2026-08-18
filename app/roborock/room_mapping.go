package roborock

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// RoomMapping links the segment ID accepted by app_segment_clean to the
// home-level room ID used by the Roborock cloud for the room name.
type RoomMapping struct {
	SegmentID  int    `json:"segment_id"`
	HomeRoomID string `json:"home_room_id"`
	RoomType   int    `json:"room_type,omitempty"`
}

// ParseRoomMappings parses get_room_mapping responses. Roborock models return
// rows with either two or three fields: [segmentID, homeRoomID, roomType].
func ParseRoomMappings(data []byte) ([]RoomMapping, error) {
	var rows [][]json.RawMessage
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, fmt.Errorf("parse room mapping: %w", err)
	}

	result := make([]RoomMapping, 0, len(rows))
	seen := make(map[int]bool)
	for index, row := range rows {
		if len(row) < 2 {
			return nil, fmt.Errorf("parse room mapping row %d: expected at least 2 fields", index)
		}
		segmentID, err := rawPositiveInt(row[0])
		if err != nil {
			return nil, fmt.Errorf("parse room mapping row %d segment: %w", index, err)
		}
		homeRoomID, err := rawIDString(row[1])
		if err != nil {
			return nil, fmt.Errorf("parse room mapping row %d home room: %w", index, err)
		}
		roomType := 0
		if len(row) >= 3 && !bytes.Equal(bytes.TrimSpace(row[2]), []byte("null")) {
			roomType, err = rawNonNegativeInt(row[2])
			if err != nil {
				return nil, fmt.Errorf("parse room mapping row %d type: %w", index, err)
			}
		}
		if seen[segmentID] {
			continue
		}
		seen[segmentID] = true
		result = append(result, RoomMapping{SegmentID: segmentID, HomeRoomID: homeRoomID, RoomType: roomType})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].SegmentID < result[j].SegmentID })
	return result, nil
}

func rawPositiveInt(raw json.RawMessage) (int, error) {
	value, err := rawNonNegativeInt(raw)
	if err != nil {
		return 0, err
	}
	if value <= 0 {
		return 0, fmt.Errorf("ID must be positive")
	}
	return value, nil
}

func rawNonNegativeInt(raw json.RawMessage) (int, error) {
	var number int
	if err := json.Unmarshal(raw, &number); err == nil {
		if number < 0 {
			return 0, fmt.Errorf("value must not be negative")
		}
		return number, nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return 0, fmt.Errorf("expected integer or numeric string")
	}
	number, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || number < 0 {
		return 0, fmt.Errorf("invalid integer %q", text)
	}
	return number, nil
}

func rawIDString(raw json.RawMessage) (string, error) {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		text = strings.TrimSpace(text)
		if text == "" {
			return "", fmt.Errorf("ID is empty")
		}
		return text, nil
	}
	var number json.Number
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&number); err != nil {
		return "", fmt.Errorf("expected string or integer ID")
	}
	if _, err := strconv.ParseInt(number.String(), 10, 64); err != nil {
		return "", fmt.Errorf("invalid ID %q", number.String())
	}
	return number.String(), nil
}

// CommandableRoomNames builds a segment-ID keyed name map exclusively from
// the active get_room_mapping result. Home names are joined through HomeRoomID;
// config and UI overrides are keyed by the commandable segment ID.
func CommandableRoomNames(mappings []RoomMapping, homeNames, configuredNames, overrides map[string]string) map[string]string {
	names := make(map[string]string, len(mappings))
	for _, mapping := range mappings {
		segmentKey := strconv.Itoa(mapping.SegmentID)
		name := strings.TrimSpace(homeNames[mapping.HomeRoomID])
		if configured := strings.TrimSpace(configuredNames[segmentKey]); configured != "" {
			name = configured
		}
		if override := strings.TrimSpace(overrides[segmentKey]); override != "" {
			name = override
		}
		if name == "" {
			name = fmt.Sprintf("Room %d", mapping.SegmentID)
		}
		names[segmentKey] = name
	}
	return names
}
