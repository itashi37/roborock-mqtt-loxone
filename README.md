# roborock-mqtt-loxone

A Roborock-to-Loxone MQTT bridge with a built-in integration assistant, web UI,
multi-robot support, command tracking, and room-aware automation.

This project is a dedicated fork of
[`mqtt-home/roborock-mqtt`](https://github.com/mqtt-home/roborock-mqtt). It
preserves the upstream Git history and attribution while adding an optional,
additive Loxone contract. Existing upstream MQTT topics remain available.

## Features

- Bridges Roborock devices to a local MQTT broker
- Web UI for device control, cleaning programs, and schedule management
- Context-aware cleaning schedules with four day types (normal, weekend, free day, not at home)
- MQTT signal integration for public holidays and vacation detection
- Per-device scene (program) execution
- Live device status, map visualization, and SSE real-time updates
- Multi-device support
- Compact Loxone `/core` and `/activity` topics (2 subscriptions per robot)
- Reliable command lifecycle and robot events
- Loxone Integration web page with room-name overrides and diagnostics
- Safe downloadable Loxone setup pack generated from detected robots

## Quick Start

### Docker

Clone the dedicated repository and build the image locally:

```bash
git clone https://github.com/itashi37/roborock-mqtt-loxone.git
cd roborock-mqtt-loxone
docker build -t roborock-mqtt-loxone:local ./app
docker run -d --name roborock-mqtt-loxone \
  --restart unless-stopped \
  -v /path/to/config:/var/lib/roborock-mqtt-loxone \
  -p 8080:8080 \
  roborock-mqtt-loxone:local
```

The mounted directory must contain `config.json`. Application sessions,
schedules, and Loxone room-name overrides are stored alongside it.

### Synology update

When the repository is stored in
`/volume1/docker/roborock-mqtt-loxone/source`, rebuild and replace the dedicated
container with:

```bash
cd /volume1/docker/roborock-mqtt-loxone/source
git pull --ff-only origin main
docker build -t roborock-mqtt-loxone:local ./app
docker stop roborock-mqtt-loxone
docker rm roborock-mqtt-loxone
docker run -d --name roborock-mqtt-loxone \
  --restart unless-stopped \
  -v /volume1/docker/roborock-mqtt-loxone/config:/var/lib/roborock-mqtt-loxone \
  -p 8080:8080 \
  roborock-mqtt-loxone:local
```

This deployment is independent from any existing upstream
`roborock-mqtt` container.

### From Source

```bash
cd app
make dev
```

This builds the frontend, builds the backend, and starts the server using `production/config/config.json`.

## Configuration

Create a `config.json` file:

```json
{
  "mqtt": {
    "enabled": true,
    "url": "tcp://localhost:1883",
    "topic": "home/roborock",
    "qos": 2,
    "retain": true
  },
  "roborock": {
    "username": "your-email@example.com",
    "password": "your-password",
    "client_id": "<client-id>",
    "base_url": "https://euiot.roborock.com",
    "polling_interval": 30
  },
  "loxone": {
    "enabled": false,
    "topic": "loxone/roborock",
    "command_debounce_ms": 2000,
    "command_timeout_seconds": 90
  },
  "web": {
    "enabled": true,
    "port": 8080
  },
  "loglevel": "info"
}
```

Environment variables can be used in the config file with `${VAR_NAME}` syntax.

`mqtt.enabled` controls only the local home-automation broker. It defaults to
`true` when omitted so existing installations keep the same behaviour. Setting
it to `false` lets the Roborock bridge and Web UI run without Mosquitto; the
separate Roborock Cloud MQTT connection used to communicate with the robots is
still started normally. Direct Loxone transport is added in the next phase.

External robot slugs are persisted by Roborock DUID in `device-slugs.json`
inside the data volume. Renaming or reordering robots therefore no longer
changes their MQTT topics or API paths.

### Schedules

Schedules can be provisioned via the config file (read-only) or created in the web UI (persisted in the data directory).

Add a `schedules` section under `roborock` to provision schedules:

```json
{
  "roborock": {
    "schedules": {
      "My Vacuum": {
        "normal": [
          { "time": "09:00", "action": "scene", "scene_id": 12345 }
        ],
        "weekend": [
          { "time": "11:00", "action": "scene", "scene_id": 12345 }
        ],
        "free": [
          { "time": "10:00", "action": "start" }
        ],
        "notAtHome": []
      }
    },
    "schedule_signals": {
      "public_holiday": "rules/public-holiday",
      "vacation": "rules/free-day"
    }
  }
}
```

**Day type priority** (highest first): Not at Home > Weekend / Holiday > Free Day > Normal

- **Not at Home** is a manual toggle in the web UI, persisted in the data directory
- **Weekend** includes Saturday, Sunday, and days where the public holiday MQTT signal is `true`
- **Free Day** is active when the vacation MQTT signal is `true`
- **Normal** is the fallback for regular weekdays

All schedule times use the `Europe/Berlin` timezone.

### Data Directory

The application stores persistent state in the config file's parent directory:

```
config-dir/
  config.json
  .session/             # Roborock session data
  schedules/
    not-at-home.json    # Global not-at-home toggle state
    devices/            # User-created schedules (one JSON file per device)
```

In Kubernetes, mount this directory as a persistent volume.

The container reads `/var/lib/roborock-mqtt-loxone/config.json` by default.

## MQTT Topics

Published topics (per device):

| Topic | Description |
|-------|-------------|
| `{topic}/{slug}/status` | Device status (JSON) |
| `{topic}/{slug}/map` | Map image (PNG) |
| `{topic}/{slug}/map.json` | Vector map data (JSON) |
| `{topic}/{slug}/current_room` | Current robot room (`{"id":23,"name":"Cuisine"}` or `null` when unknown) |
| `{topic}/{slug}/scenes` | Available cleaning programs (JSON) |
| `{topic}/{slug}/schedule` | Schedule state (JSON) |

Command topic (per device):

| Topic | Description |
|-------|-------------|
| `{topic}/{slug}/set` | Send commands (JSON) |

Command payload format:

```json
{ "action": "start" }
{ "action": "pause" }
{ "action": "dock" }
{ "action": "segment_clean", "segments": [1, 2] }
{ "action": "set_fan_speed", "speed": "quiet|balanced|turbo|max" }
{ "action": "set_mop_mode", "mode": "standard|deep|deep_plus" }
{ "action": "set_water_box", "level": "off|mild|moderate|intense" }
{ "action": "scene", "scene_id": 12345 }
```

### Loxone MQTT mode

The optional Loxone mode adds a scalar, retained MQTT contract alongside the
existing topics. It does not replace or alter the standard MQTT API. Enable it
in `config.json`:

```json
{
  "loxone": {
    "enabled": true,
    "topic": "loxone/roborock",
    "command_debounce_ms": 2000,
    "command_timeout_seconds": 90
  }
}
```

The topic defaults to `loxone/roborock` when omitted. Command debounce defaults
to 2 seconds and confirmation timeout to 90 seconds.

The bridge publishes one compact retained core topic plus the existing 14
retained scalar topics per device.

#### Recommended standard subscriptions (2 per device)

Subscribe to these two topics for a normal Loxone integration:

```text
{loxone_topic}/{slug}/core
{loxone_topic}/{slug}/activity
```

Example compact JSON payload:

```json
{"online":1,"state":"cleaning","battery":82,"current_room_id":23,"current_room_name":"Cuisine","error_code":0,"last_seen":1700000000}
```

`/core` is republished whenever availability, status, or current room changes.
`/activity` is the non-retained stream of command progress and reliable robot
events described below. A standard installation therefore uses two
subscriptions per robot: three robots use six of the MQTT plugin's 16
subscriptions, and up to eight robots fit within that limit.

#### Optional individual scalar subscriptions (14 per device)

All scalar topics remain published and retained for projects that prefer
individual values. The first seven mirror the fields in `/core`:

| Topic | Value |
|-------|-------|
| `{loxone_topic}/{slug}/online` | `0` or `1` |
| `{loxone_topic}/{slug}/state` | Normalized robot state |
| `{loxone_topic}/{slug}/battery` | Battery percentage (`0`–`100`) |
| `{loxone_topic}/{slug}/current_room_id` | Room ID, or `0` when unknown |
| `{loxone_topic}/{slug}/current_room_name` | Room name, or an empty string when unknown |
| `{loxone_topic}/{slug}/error_code` | Roborock error code, or `0` |
| `{loxone_topic}/{slug}/last_seen` | Unix timestamp of the last status update |

The remaining seven are intended for statistics, diagnostics, and maintenance
screens:

| Topic | Value |
|-------|-------|
| `{loxone_topic}/{slug}/clean_area` | Cleaned area in m² |
| `{loxone_topic}/{slug}/clean_time_seconds` | Cleaning time in seconds |
| `{loxone_topic}/{slug}/error_text` | Error text, or an empty string |
| `{loxone_topic}/{slug}/maintenance/main_brush` | Remaining percentage |
| `{loxone_topic}/{slug}/maintenance/side_brush` | Remaining percentage |
| `{loxone_topic}/{slug}/maintenance/filter` | Remaining percentage |
| `{loxone_topic}/{slug}/maintenance/sensor` | Remaining percentage |

Stable state values include `offline`, `unknown`, `starting`, `idle`, `manual`,
`cleaning`, `paused`, `returning`, `charging`, `docked`, `error`,
`shutting_down`, `updating`, `going_to_target`, `emptying_dustbin`,
`washing_mop`, `servicing_dock`, and `mapping`.

Publish text commands to:

```text
{loxone_topic}/{slug}/command
```

This is a Loxone MQTT publish output, not a subscription. It therefore does
not count toward the recommended `/core` subscription (it uses one of the
plugin's publish outputs per robot).

Supported commands:

```text
start
pause
dock
clean_room:Cuisine
clean_rooms:Cuisine,Salon
clean_room_id:23
clean_room_ids:23,24
scene:Après les repas
scene_id:12345
fan:quiet
fan:balanced
fan:turbo
fan:max
mop:standard
mop:deep
mop:deep_plus
```

Room and scene name matching ignores case and surrounding spaces. Configured
room names override names discovered through the Roborock API. Ambiguous or
unknown names are rejected; ID-based commands remain available as an explicit
fallback.

The room inventory uses the active robot response from `get_room_mapping`.
Only segment IDs accepted by `app_segment_clean` are exposed or accepted by
room commands. Large home-level room IDs and historical rooms are used only to
resolve names and are never treated as cleaning segments. If the active mapping
is unavailable, the bridge exposes no commandable rooms instead of falling back
to map pixels or the unfiltered home room catalogue.

Commands must be published with `retain=false`. On startup the bridge clears
any retained `/command` value before subscribing, preventing a stale command
from being replayed after a restart.

#### Activity stream

The single Phase 2 activity subscription is:

```text
{loxone_topic}/{slug}/activity
```

It is never retained. Every payload has a Unix timestamp in seconds. Command
progress uses the same ID from acceptance through completion or failure:

```json
{"type":"command","ts":1700000000,"id":"cmd-1700000000000-1","command":"clean_room:Cuisine","state":"accepted","error":""}
{"type":"command","ts":1700000001,"id":"cmd-1700000000000-1","command":"clean_room:Cuisine","state":"running","error":""}
{"type":"command","ts":1700000004,"id":"cmd-1700000000000-1","command":"clean_room:Cuisine","state":"completed","error":""}
```

`accepted` means the command passed local validation, `running` means it was
successfully handed to the Roborock API, and `completed` means a later robot
status confirmed its requested effect. A command is `failed` when it is
invalid, ambiguous, incompatible with the current state, received while the
robot is offline, rejected by the Roborock API, duplicated during the debounce
window, or not confirmed before the timeout.

Reliable robot transitions are emitted on the same topic:

```json
{"type":"event","ts":1700000010,"event":"room_entered","room_id":23,"room_name":"Cuisine"}
{"type":"event","ts":1700000020,"event":"error","error_code":12,"error_text":"error_12"}
```

Supported events are `cleaning_started`, `room_entered`, `cleaning_completed`,
`returned_to_dock`, `paused`, `resumed`, and `error`. No event is emitted from
the initial status baseline after startup. `room_completed` and `stuck` are not
published because the currently available Roborock data cannot identify them
reliably. A `dock` command is confirmed only by a subsequent charging or fully
charged status; a dock-service state such as `washing_mop` is not sufficient.

The last command activity is also available as an optional retained diagnostic
topic:

```text
{loxone_topic}/{slug}/last_command
```

It is not part of the recommended Loxone subscriptions. The 14 retained scalar
topics remain optional and unchanged.

## Web UI

The web UI is available at `http://localhost:8080` (default port).

On first launch, you authenticate with your Roborock account via email verification code. The session is persisted so you don't need to re-authenticate on restart.

The main view shows:
- Device status with battery, fan speed, and mop mode
- Cleaning programs (scenes) as the primary action
- Pause/Dock buttons during active cleaning
- Controls page for manual start, fan speed, and mop mode settings
- Schedule summary with link to the full schedule page
- Interactive map
- Dedicated `/loxone` integration and diagnostic page

### Loxone Integration assistant

Open `http://localhost:8080/loxone` or use the **Loxone Integration** link in
the device page. The assistant lists every detected robot, its compact MQTT
topics, live core state, current room, recent command activity, and scenes.

Room-name overrides created in this page are stored in
`loxone-room-overrides.json` next to `config.json`. They do not rewrite the
configuration file. UI overrides take precedence over API and `config.json`
names and are rejected when they would make two room names ambiguous.

The **Download Loxone Integration** button generates a
`roborock-mqtt-loxone-integration-*.zip` archive containing:

- `integration.json` — detected robots, selected rooms/scenes and topic contract;
- `topics.csv` — the two subscriptions and one publish per selected robot;
- `command-recognition.csv` — suggested Loxone recognition expressions;
- `SETUP.md` — step-by-step setup and official Loxone references.

No MQTT username, password, Roborock session, token, local key, or device
secret is exported. The archive is deliberately a configuration assistant,
not an undocumented XML or `.LoxPLAN` file. Loxone documents a limit of 16
MQTT subscriptions; the page warns when the selection exceeds that budget but
still allows the pack to be generated.

## Development

```bash
cd app

# Full build + run
make dev

# Frontend dev server (with hot reload)
make dev-frontend

# Backend only
make dev-backend

# Build Docker image
make docker
```

## REST API

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/health` | Health check |
| `GET` | `/api/auth/status` | Authentication status |
| `POST` | `/api/auth/request-code` | Request verification code |
| `POST` | `/api/auth/login` | Login with code |
| `POST` | `/api/auth/logout` | Logout |
| `GET` | `/api/devices` | List devices |
| `GET` | `/api/devices/{slug}/status` | Device status |
| `POST` | `/api/devices/{slug}/start` | Start cleaning |
| `POST` | `/api/devices/{slug}/pause` | Pause cleaning |
| `POST` | `/api/devices/{slug}/dock` | Return to dock |
| `POST` | `/api/devices/{slug}/fan-speed` | Set fan speed |
| `POST` | `/api/devices/{slug}/mop-mode` | Set mop mode |
| `GET` | `/api/devices/{slug}/scenes` | List scenes |
| `POST` | `/api/devices/{slug}/scenes/{id}/execute` | Execute scene |
| `GET` | `/api/devices/{slug}/map` | Map PNG |
| `GET` | `/api/devices/{slug}/map.json` | Vector map JSON |
| `GET` | `/api/devices/{slug}/schedule` | Schedule state |
| `POST` | `/api/devices/{slug}/schedule` | Save user schedule |
| `DELETE` | `/api/devices/{slug}/schedule` | Delete user schedule |
| `PUT` | `/api/not-at-home` | Toggle not-at-home |
| `GET` | `/api/schedule/status` | Global schedule status |
| `GET` | `/api/events` | SSE event stream |
| `GET` | `/api/loxone/integration` | Loxone inventory and diagnostics |
| `PUT` | `/api/loxone/devices/{slug}/rooms/{id}` | Save a Loxone room-name override |
| `DELETE` | `/api/loxone/devices/{slug}/rooms/{id}` | Remove a room-name override |
| `POST` | `/api/loxone/devices/{slug}/command` | Publish a non-retained test command |
| `POST` | `/api/loxone/mqtt-test` | Run an MQTT loopback diagnostic |
| `POST` | `/api/loxone/export` | Download the Loxone integration ZIP |
