import { AlertCircle, Clock, Home, MapPin, Pause, Play, Wifi, WifiOff } from 'lucide-react';
import type { LoxoneActivity, LoxoneCore, LoxoneExportSelection, LoxoneMQTTTest, LoxoneRobot, LoxoneScene } from '@/types/loxone';
import { formatActivity, validateRoomDrafts } from '@/lib/loxone';
import { BatteryGauge, formatTimestamp, Spinner, StatusBadge } from './LoxoneUI';
import { RoomMappingTable } from './RoomMappingTable';
import { ScenesPanel } from './ScenesPanel';
import { MqttTopicsPanel } from './MqttTopicsPanel';
import { DiagnosticsPanel } from './DiagnosticsPanel';
import { AdvancedDiagnosticsPanel } from './AdvancedDiagnosticsPanel';
import { DirectHttpSetupPanel } from './DirectHttpSetupPanel';

interface Props {
  robot: LoxoneRobot;
  core: LoxoneCore;
  online: boolean;
  selected?: LoxoneExportSelection;
  drafts: Record<string, string>;
  activities: LoxoneActivity[];
  mqttTest?: LoxoneMQTTTest;
  busy: Set<string>;
  onDraft: (roomID: number, value: string) => void;
  onSaveRoom: (roomID: number) => void;
  onResetRoom: (roomID: number) => void;
  onSelectRoom: (roomID: number, selected: boolean) => void;
  onSelectScene: (sceneID: number, selected: boolean) => void;
  onCommand: (key: string, command: string, success: string) => void;
  onTestScene: (scene: LoxoneScene) => void;
  onTestMQTT: () => void;
  directAPIUsername: string;
  directTokenConfigured: boolean;
}

export function LoxoneRobotCard(props: Props) {
  const { robot, core, online, selected, drafts, activities, mqttTest, busy } = props;
  const validation = validateRoomDrafts(robot.rooms, drafts);
  const lastActivity = activities[0] ?? robot.diagnostics.last_activity;
  const lastCommand = activities.find(item => item.type === 'command') ?? robot.diagnostics.last_command;
  return <article className="overflow-hidden rounded-2xl border border-border bg-card shadow-sm">
    <header className="border-b border-border p-4 md:p-5">
      <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2"><h2 className="truncate text-xl font-semibold">{robot.name}</h2><StatusBadge tone={online ? 'green' : 'red'}>{online ? <Wifi className="h-3 w-3" /> : <WifiOff className="h-3 w-3" />}{online ? 'Online' : 'Offline'}</StatusBadge><StatusBadge tone={stateTone(core.state)}>{displayState(core.state)}</StatusBadge>{robot.mqtt_enabled && <StatusBadge tone="blue">MQTT</StatusBadge>}{robot.direct_enabled && <StatusBadge tone="blue">Direct</StatusBadge>}</div>
          <p className="mt-1 text-xs text-muted-foreground">{robot.model || 'Unknown model'} · {robot.slug}</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <CommandButton icon={<Play className="h-4 w-4" />} label="Start" command="start" robot={robot.slug} busy={busy} onCommand={props.onCommand} />
          <CommandButton icon={<Pause className="h-4 w-4" />} label="Pause" command="pause" robot={robot.slug} busy={busy} onCommand={props.onCommand} />
          <CommandButton icon={<Home className="h-4 w-4" />} label="Dock" command="dock" robot={robot.slug} busy={busy} onCommand={props.onCommand} />
        </div>
      </div>
      <div className="mt-5 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Metric label="Battery"><BatteryGauge value={core.battery} /></Metric>
        <Metric label="Current room"><span className="flex items-center gap-2"><MapPin className="h-4 w-4 text-muted-foreground" />{core.current_room_name || 'Unknown'}{core.current_room_id ? <span className="text-xs text-muted-foreground">ID {core.current_room_id}</span> : null}</span></Metric>
        <Metric label="Error"><span className={`flex items-center gap-2 ${core.error_code ? 'text-red-500' : ''}`}><AlertCircle className="h-4 w-4" />{core.error_code ? `Code ${core.error_code}` : 'None'}</span></Metric>
        <Metric label="Last seen"><span className="flex items-center gap-2"><Clock className="h-4 w-4 text-muted-foreground" />{formatTimestamp(core.last_seen)}</span></Metric>
      </div>
      <div className="mt-4 rounded-lg bg-muted/60 px-3 py-2 text-sm"><span className="text-muted-foreground">Last command:</span> <strong>{lastCommand ? formatActivity(lastCommand) : 'None recorded'}</strong></div>
      {robot.active_program && <div className="mt-2 rounded-lg bg-blue-500/10 px-3 py-2 text-sm"><span className="text-muted-foreground">Active program:</span> <strong>{robot.active_scene_name || (robot.active_scene_id ? `Scene #${robot.active_scene_id}` : displayState(robot.active_program))}</strong></div>}
      <div className="mt-2 flex flex-wrap gap-x-5 gap-y-1 rounded-lg bg-muted/40 px-3 py-2 text-xs text-muted-foreground"><span>Dock: <strong className="text-foreground">{robot.health.dock_state || 'unknown'}</strong></span><span>Status API: <strong className="text-foreground">{robot.health.status_latency_ms || 0} ms</strong></span><span>Poll failures: <strong className={robot.health.consecutive_failures ? 'text-red-500' : 'text-foreground'}>{robot.health.consecutive_failures || 0}</strong></span><span>Backoff: <strong className="text-foreground">{robot.health.backoff_seconds || 0}s</strong></span></div>
    </header>

    <div className="space-y-7 p-4 md:p-5">
      <Section title="Commandable rooms" count={`${robot.rooms.length} rooms`} description="Only active segments returned by get_room_mapping are shown.">
        <RoomMappingTable slug={robot.slug} rooms={robot.rooms} selected={selected} drafts={drafts} validation={validation} busy={busy} onDraft={props.onDraft} onSave={props.onSaveRoom} onReset={props.onResetRoom} onSelect={props.onSelectRoom} />
      </Section>
      <Section title="Scenes" count={`${robot.scenes.length} scenes`}>
        <ScenesPanel slug={robot.slug} scenes={robot.scenes} activities={activities} selected={selected} busy={busy} onSelect={props.onSelectScene} onTest={props.onTestScene} />
      </Section>
      <Section title="Direct HTTP configuration" description="Copy-ready Loxone Config fields, kept in one compact guide."><DirectHttpSetupPanel robot={robot} apiUsername={props.directAPIUsername} tokenConfigured={props.directTokenConfigured} /></Section>
      <Section title="MQTT topics" description="The standard integration consumes two subscriptions per selected robot."><MqttTopicsPanel topics={robot.topics} /></Section>
      <Section title="Diagnostics"><DiagnosticsPanel core={core} activity={lastActivity} lastCommand={lastCommand} activities={activities} mqttTest={mqttTest} testing={busy.has('mqtt-test')} onTestMQTT={props.onTestMQTT} /></Section>
      <Section title="Capabilities & advanced controls" description="Advanced/Admin controls appear only after evidence from this robot confirms support."><AdvancedDiagnosticsPanel slug={robot.slug} initialCapabilities={robot.capabilities} busy={busy} onCommand={props.onCommand} /></Section>
    </div>
  </article>;
}

function Section({ title, count, description, children }: { title: string; count?: string; description?: string; children: React.ReactNode }) {
  return <section><div className="mb-3 flex flex-wrap items-end justify-between gap-2"><div><h3 className="font-semibold">{title}</h3>{description && <p className="mt-0.5 text-xs text-muted-foreground">{description}</p>}</div>{count && <span className="text-xs text-muted-foreground">{count}</span>}</div>{children}</section>;
}

function Metric({ label, children }: { label: string; children: React.ReactNode }) {
  return <div><p className="mb-1 text-xs font-medium uppercase tracking-wide text-muted-foreground">{label}</p><div className="text-sm font-medium">{children}</div></div>;
}

function CommandButton({ icon, label, command, robot, busy, onCommand }: { icon: React.ReactNode; label: string; command: string; robot: string; busy: Set<string>; onCommand: (key: string, command: string, success: string) => void }) {
  const key = `cmd:${robot}:${command}`;
  return <button onClick={() => onCommand(key, command, `${label} command published. Waiting for /activity confirmation.`)} disabled={busy.has(key)} className="touch-target inline-flex items-center justify-center gap-2 rounded-lg border border-border px-3 text-sm hover:bg-accent disabled:opacity-50">{busy.has(key) ? <Spinner label={label} /> : <>{icon}{label}</>}</button>;
}

function displayState(state: string): string {
  return (state || 'unknown').replace(/_/g, ' ').replace(/\b\w/g, (value: string) => value.toUpperCase());
}

function stateTone(state: string): 'green' | 'red' | 'amber' | 'blue' | 'neutral' {
  if (state === 'error' || state === 'stuck' || state === 'offline') return 'red';
  if (['cleaning', 'segment_cleaning', 'spot_cleaning', 'zoned_cleaning', 'washing_mop', 'servicing_dock'].includes(state)) return 'blue';
  if (['paused', 'returning_home'].includes(state)) return 'amber';
  if (['idle', 'charging', 'docked'].includes(state)) return 'green';
  return 'neutral';
}
