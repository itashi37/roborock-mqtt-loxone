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
- Browser-first setup for MQTT, Direct HTTP, or Both
- Capability-aware dock controls and fleet health diagnostics

## Quick Start

### Docker Compose (recommended)

A fresh Docker named volume is created automatically. The container creates a
minimal `config.json`, then the browser wizard stores the integration settings.

Official multiarchitecture images are published at
`ghcr.io/itashi37/roborock-mqtt-loxone`. Use `:latest` for stable releases or
`:edge` for builds from `main`:

```bash
docker pull ghcr.io/itashi37/roborock-mqtt-loxone:latest
```

Stable SemVer releases also publish immutable `:v1.2.3`, minor `:1.2`, and
major `:1` aliases. Every manifest contains both `linux/amd64` and
`linux/arm64`; `:edge` is never promoted to `:latest`.

### System & Updates

Open `/system` to inspect the installed version and commit, architecture,
uptime, data-volume writability/free space, watchdog state, and sanitized MQTT
and Direct Loxone transport diagnostics. **Check for updates** reads public
GitHub release metadata for Stable or the latest `main` commit for Beta/Edge.
It does not require or expose a GitHub/Registry credential. In this phase the
page is discovery-only and deliberately has no Docker access.

### Isolated updater security model

The optional `updates` Compose profile starts a separate
`roborock-mqtt-loxone-updater` process. The bridge container never receives the
Docker socket. The updater has no host port and accepts only authenticated
health/status/install operations on the private Compose network. Install
requests can select only a validated tag of
`ghcr.io/itashi37/roborock-mqtt-loxone`; container names, image repositories,
shell commands, Docker arguments, and arbitrary URLs are not accepted.

The updater talks to the Docker Engine API directly and therefore its socket
access is effectively root-equivalent on the NAS. `read_only`,
`no-new-privileges`, a non-root process, rate limiting, anti-replay request IDs,
and the strict allowlist reduce exposure but do not make Docker-socket access
unprivileged. Never expose port 8090 outside the private Compose network.

Generate a dedicated token and determine the Docker socket group ID before
enabling the profile:

```bash
openssl rand -hex 32
stat -c '%g' /var/run/docker.sock
```

Store the token in `deploy/updater-token` with mode `0600`, and set
`DOCKER_GID` in the Compose `.env`; the secret is mounted as a file and does
not appear in the container environment. Then start the optional service with
`docker compose --profile updates up -d`.
The updater backs up the data volume under `backups/`, pulls the allowlisted
image, recreates only `roborock-mqtt-loxone`, waits for its Docker healthcheck,
verifies the reported version, and restores the previous container on failure.
Operation state is persisted as `update-operation.json`. A Docker socket proxy
can still be placed in front of the updater, but the create/start/stop/remove
and image-pull permissions required for replacement remain highly privileged.

```bash
git clone https://github.com/itashi37/roborock-mqtt-loxone.git
cd roborock-mqtt-loxone
docker compose up -d --build
```

Open `http://NAS-IP:8080`. This baseline starts only the bridge and supports
Direct Loxone without Mosquitto. For MQTT or Both with the bundled broker:

```bash
docker compose --profile mqtt up -d --build
```

Use `tcp://mosquitto:1883` in the wizard. The sample broker permits anonymous
LAN access for first tests; add Mosquitto authentication or use an existing
secured broker before exposing port 1883 outside a trusted network.

- Direct: bridge only; no local broker dependency.
- MQTT: enable the `mqtt` profile or configure an existing broker.
- Both: enable both transports; commands still pass through one coordinator.

### Synology Container Manager

Create a writable data directory for the image's non-root UID/GID, clone the
repository into `source`, then start the Compose project:

```bash
mkdir -p /volume1/docker/roborock-mqtt-loxone/data
sudo chown -R 65532:65532 /volume1/docker/roborock-mqtt-loxone/data
cd /volume1/docker/roborock-mqtt-loxone/source
ROBOROCK_DATA_DIR=/volume1/docker/roborock-mqtt-loxone/data docker compose up -d --build
```

Container Manager can import `compose.yaml` as a Project. Put
`ROBOROCK_DATA_DIR=/volume1/docker/roborock-mqtt-loxone/data` in `.env`. For an
update that preserves all state:

```bash
cd /volume1/docker/roborock-mqtt-loxone/source
git pull --ff-only origin main
ROBOROCK_DATA_DIR=/volume1/docker/roborock-mqtt-loxone/data docker compose up -d --build
```

This deployment is independent from any existing upstream container.

### From Source

```bash
cd app
make dev
```

This builds the frontend, builds the backend, and starts the server using `production/config/config.json`.

## Configuration

The Setup Wizard is the normal configuration path. For unattended/provisioned
installs, the equivalent `config.json` structure is:

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
    "command_timeout_seconds": 90,
    "direct": {
      "enabled": false,
      "scheme": "https",
      "host": "192.168.1.20",
      "port": 443,
      "username": "roborock_bridge",
      "password": "${LOXONE_PASSWORD}",
      "timeout_seconds": 5,
      "max_retries": 3,
      "retry_delay_ms": 500,
      "input_prefix": "RR",
      "api_username": "loxone",
      "api_token": "${LOXONE_BRIDGE_API_TOKEN}",
      "allowed_cidrs": ["192.168.1.0/24"],
      "allow_get_commands": false,
      "rate_limit_per_minute": 30
    },
    "devices": {
      "ROBOROCK_DUID": {"mqtt": true, "direct": true}
    }
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
still started normally. Direct Loxone can therefore run without a local broker.

External robot slugs are persisted by Roborock DUID in `device-slugs.json`
inside the data volume. Renaming or reordering robots therefore no longer
changes their MQTT topics or API paths.

When `loxone.direct.enabled` is true, changed robot values are pushed to Loxone
Virtual Inputs using the documented `/dev/sps/io/<input>/<value>` Web Service.
The default names follow `RR_<slug>_<field>` and can be overridden per robot
with `loxone.direct.inputs`. Failed values are retried without blocking
Roborock polling; `POST /api/loxone/direct/resend` queues a full resync. Dock
service fields are added only when that robot actually reports them. Capability
detection uses status/feature evidence, never a commercial model-name guess.

### Direct Loxone command API

Virtual Outputs should use HTTP `POST` with dedicated Basic authentication
(`api_username` plus the random `api_token`). The token must contain at least
32 characters. Bearer authentication is also accepted for non-Loxone clients;
browser cookies are never accepted for this API. `allowed_cidrs` can restrict
calls to the Miniserver LAN and commands are rate-limited per source and robot.
Compatibility `GET` commands are disabled unless `allow_get_commands` is set
explicitly.

Canonical endpoint:

```text
POST /api/loxone/direct/v1/devices/{slug}/commands
{"command":"clean_room_id:23"}
```

Virtual Output adapters:

```text
/commands/start
/commands/pause
/commands/dock
/commands/locate
/commands/rooms/{segment_id}
/commands/scenes/{scene_id}
/commands/fan/{mode}
/commands/mop/{mode}
/commands/water/{level}
/commands/stop
/commands/empty_dustbin
/commands/stop_emptying
/commands/wash_mop
/commands/stop_washing
/commands/dry_mop
/commands/stop_drying
```

All routes enter the same coordinator as MQTT and the Web UI. A successful
submission returns HTTP `202` with its command ID. The latest correlated state
is available from `GET /api/loxone/direct/v1/commands/{id}` using the same API
authentication. Unsupported or unconfirmed capabilities are rejected rather
than sent speculatively to the robot.

### MQTT, Direct, or Both

The transports can be selected independently and overridden per stable
Roborock DUID:

| Mode | `mqtt.enabled` | `loxone.enabled` | `loxone.direct.enabled` |
|---|---:|---:|---:|
| MQTT only | `true` | `true` | `false` |
| Direct only | `false` | `false` | `true` |
| Both | `true` | `true` | `true` |

Per-device `mqtt` and `direct` booleans default to their global transport
setting. If the local broker is running, the bridge also reconciles a persisted
retained-topic ledger and deletes topics belonging to removed robots, changed
slugs, or disabled per-robot MQTT integrations. When MQTT is globally disabled,
cleanup is intentionally deferred until a broker is enabled again; Direct-only
startup never contacts or requires Mosquitto.

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
  integration-settings.json   # UI settings and write-only credentials (0600)
  device-slugs.json           # stable Roborock DUID to slug mapping
  device-capabilities.json    # evidence-based capability cache
  loxone-room-overrides.json
  mqtt-retained-topics.json
  .session/             # Roborock session data
  schedules/
    not-at-home.json    # Global not-at-home toggle state
    devices/            # User-created schedules (one JSON file per device)
```

In Kubernetes, mount this directory as a persistent volume.

The container reads `/var/lib/roborock-mqtt-loxone/config.json` by default. On
a fresh writable volume it creates this file automatically. Back up the entire
directory, not only `config.json`:

```bash
docker compose stop bridge
tar -C /volume1/docker/roborock-mqtt-loxone -czf roborock-mqtt-loxone-backup.tgz data
docker compose start bridge
```

### Migration from the current fork installation

1. Stop the old container and back up its whole mounted config/data directory.
2. Copy that directory to the new `ROBOROCK_DATA_DIR`; do not copy only the JSON config.
3. Ensure UID/GID `65532` has read/write access.
4. Start the new bridge with `docker compose up -d --build`.
5. Existing configurations without `mqtt.enabled` keep MQTT enabled for compatibility.
6. Open `/setup`, choose MQTT, Direct, or Both, save, then verify `/loxone` health.
7. Keep the old container stopped until state, commands, room mappings, and scenes are verified.

The updater never deletes the Roborock session, stable slugs, overrides,
integration credentials, API token, schedules, or capability cache.

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

#### Bridge and robot health

Health is published separately so `/core` stays compact and its existing
contract remains unchanged:

| Topic | Retained | Meaning |
|-------|----------|---------|
| `{loxone_topic}/_bridge/bridge_alive` | yes | `1` while the bridge MQTT client is connected; its broker-side Last Will publishes `0` after an ungraceful loss |
| `{loxone_topic}/_bridge/cloud_connected` | yes | `1` when at least one Roborock cloud transport is connected |
| `{loxone_topic}/_bridge/bridge_heartbeat` | no | Unix timestamp emitted every 30 seconds, including when no robot value changes |
| `{loxone_topic}/{slug}/robot_online` | yes | `1` only after the robot has produced status and has not accumulated repeated polling failures |

These topics are optional diagnostics and do not change the recommended two
subscriptions per robot. An installation that subscribes to the three bridge
topics uses three additional subscriptions total, not per robot. In Direct
HTTP mode the equivalent Virtual Inputs are `RR_bridge_alive`,
`RR_cloud_connected`, `RR_bridge_heartbeat`, and
`RR_{slug}_robot_online` (subject to the configured prefix/overrides).

Configure a 90-second timeout on `bridge_heartbeat` in Loxone. Direct HTTP
cannot send a final value after the process, NAS, or network has already
failed, so an absent heartbeat is the only reliable Direct-mode indication of
a completely unavailable bridge. Interpretation:

- heartbeat younger than 90 seconds, cloud `1`, robot `1`: all paths work;
- heartbeat missing: bridge, NAS, or network probably unavailable;
- heartbeat current, cloud `0`: Roborock cloud or Internet unavailable;
- heartbeat current, cloud `1`, robot `0`: robot Wi-Fi, power, or response problem.

#### Internal watchdog and HTTP probes

The internal watchdog observes the Roborock polling loop, last cloud and robot
updates, command dispatcher, Direct HTTP queue, and every enabled transport. It
first reconnects, then rebuilds connections and finally resets the affected
subsystems. It exits with a non-zero code only when an internal loop or queue
remains blocked for the configured fatal threshold; a temporary cloud outage
alone never requests a process restart. Restart attempts are persisted in the
data volume and limited to three per hour by default.

| Endpoint | Success condition |
|----------|-------------------|
| `GET /api/live` | Process, polling loop, dispatcher and queues are responsive; use this for Docker `HEALTHCHECK` |
| `GET /api/livez` | Backwards-compatible alias of `/api/live` |
| `GET /api/ready` | Live, authenticated, bridge started, cloud connected, and every enabled transport ready |
| `GET /api/health` | The complete report is `healthy`; otherwise returns 503 with component diagnostics and the last watchdog reason |

Optional `watchdog` configuration values are expressed in seconds:

```json
{
  "watchdog": {
    "enabled": true,
    "check_interval_seconds": 30,
    "stale_after_seconds": 120,
    "reconnect_after_seconds": 120,
    "rebuild_after_seconds": 300,
    "reset_after_seconds": 480,
    "restart_after_seconds": 900,
    "recovery_hysteresis_checks": 2,
    "max_restarts_per_hour": 3,
    "max_queue_depth": 256
  }
}
```

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
- `direct-inputs.csv` — Virtual Input names, fields and types for Direct mode;
- `direct-outputs.csv` — credential-free POST paths for safe and supported advanced commands;
- `template-status.json` — machine-readable native-template validation status;
- `TEMPLATE-SAMPLES-NEEDED.md` — exact exports still required from your Loxone Config version;
- `SETUP.md` — step-by-step setup and official Loxone references.

For Direct HTTP installations, each robot card also contains a collapsed
**Ready-to-copy Loxone Config setup** guide. It shows the exact Virtual Input
names and types, the authenticated Virtual Output connector address, and every
supported `POST` command path for core controls, selected rooms, scenes and
confirmed advanced capabilities. The API token is entered only in the current
browser tab so the UI can assemble copy-ready fields; it is never read back
from the bridge, persisted by the browser, logged, or added to an export.

No MQTT username, password, Roborock session, token, local key, or device
secret is exported. The archive is deliberately a configuration assistant,
not an undocumented XML or `.LoxPLAN` file. Loxone documents a limit of 16
MQTT subscriptions; the page warns when the selection exceeds that budget but
still allows the pack to be generated.

Native XML generation remains disabled until five minimal, real exports from
the exact target Loxone Config version are available: Virtual Input digital,
Virtual Input analog, Virtual Text Input, Virtual Output HTTP, and Virtual
Output HTTP POST (if supported). The project will validate namespaces,
identifiers, encoding and round-trip import before enabling any generator.

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
| `GET` | `/api/health` | Complete internal health report |
| `GET` | `/api/live` | Process liveness probe used by Docker |
| `GET` | `/api/ready` | Dependency readiness probe |
| `GET` | `/api/system/status` | Sanitized runtime, volume, transport and version status |
| `POST` | `/api/system/updates/check` | Read-only Stable/Edge update metadata check |
| `GET` | `/api/fleet/health` | Fleet health, latency and polling backoff |
| `GET` | `/api/setup/status` | Sanitized setup/integration status |
| `PUT` | `/api/setup/settings` | Persist and live-apply integration settings |
| `GET` | `/api/auth/status` | Authentication status |
| `POST` | `/api/auth/request-code` | Request verification code |
| `POST` | `/api/auth/login` | Login with code |
| `POST` | `/api/auth/logout` | Logout |
| `GET` | `/api/devices` | List devices |
| `GET` | `/api/devices/{slug}/status` | Device status |
| `GET` | `/api/devices/{slug}/advanced-diagnostics` | Sanitized capability diagnostic |
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
