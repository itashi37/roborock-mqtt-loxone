import { useState } from 'react';
import { AlertCircle, Bot, ChevronDown, Clock3, Home, Link2, MapPin, Pause, Play, Settings2, Shapes, Wifi, WifiOff } from 'lucide-react';
import type { LoxoneActivity, LoxoneCore, LoxoneExportSelection, LoxoneMQTTTest, LoxoneRobot, LoxoneScene } from '@/types/loxone';
import { validateRoomDrafts } from '@/lib/loxone';
import { BatteryGauge, displayActivity, displayRobotState, relativeTimestamp, Spinner, StatusBadge } from './LoxoneUI';
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
  dockErrorCode?: number;
}

type Tab = 'loxone' | 'rooms' | 'diagnostics';

export function LoxoneRobotCard(props: Props) {
  const { robot, core, online, selected, drafts, activities, mqttTest, busy } = props;
  const [tab, setTab] = useState<Tab>('loxone');
  const validation = validateRoomDrafts(robot.rooms, drafts);
  const lastActivity = activities[0] ?? robot.diagnostics.last_activity;
  const lastCommand = activities.find(item => item.type === 'command') ?? robot.diagnostics.last_command;
  const stationError = props.dockErrorCode !== undefined ? props.dockErrorCode !== 0 : robot.health?.dock_state === 'error';
  const needsAttention = Boolean(core.error_code || stationError);
  const stationState = stationError ? 'Station à vérifier' : props.dockErrorCode === 0 && robot.health?.dock_state === 'error' ? 'État en cours d’actualisation' : displayRobotState(robot.health?.dock_state || 'unknown');
  return <article className="overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-[0_2px_6px_rgba(21,36,30,0.025)]">
    <header className="p-5 sm:p-7">
      <div className="flex flex-col gap-5 xl:flex-row xl:items-center xl:justify-between">
        <div className="flex items-center gap-4"><div className="flex h-14 w-14 shrink-0 items-center justify-center rounded-2xl border border-emerald-100 bg-gradient-to-br from-emerald-50 to-white text-emerald-700"><Bot className="h-7 w-7" strokeWidth={1.4} /></div><div className="min-w-0"><div className="flex flex-wrap items-center gap-2.5"><h2 className="truncate text-xl font-semibold tracking-tight text-[#15241e]">{robot.name}</h2><StatusBadge tone={online ? 'green' : 'neutral'}>{online ? <Wifi className="h-3 w-3" /> : <WifiOff className="h-3 w-3" />}{online ? 'En ligne' : 'Hors ligne'}</StatusBadge></div><p className="mt-1 text-xs text-slate-400">{robot.model || 'Robot Roborock'}<span className="mx-2 text-slate-200">/</span>{displayRobotState(core.state)}</p></div></div>
        <div className="flex flex-wrap gap-2"><CommandButton icon={<Play className="h-3.5 w-3.5" fill="currentColor" />} label="Démarrer" command="start" robot={robot.slug} busy={busy} onCommand={props.onCommand} primary /><CommandButton icon={<Pause className="h-3.5 w-3.5" />} label="Pause" command="pause" robot={robot.slug} busy={busy} onCommand={props.onCommand} /><CommandButton icon={<Home className="h-3.5 w-3.5" />} label="À la station" command="dock" robot={robot.slug} busy={busy} onCommand={props.onCommand} /></div>
      </div>
      <div className="mt-6 grid grid-cols-2 gap-x-4 gap-y-5 rounded-xl border border-slate-100 bg-slate-50/60 p-4 lg:grid-cols-4 lg:divide-x lg:divide-slate-200">
        <Metric label="Batterie"><BatteryGauge value={core.battery} /></Metric>
        <Metric label="Pièce actuelle"><span className="flex items-center gap-1.5"><MapPin className="h-3.5 w-3.5 text-slate-400" />{core.current_room_name || 'Aucune pièce active'}</span></Metric>
        <Metric label="Robot et station"><span className={`flex items-center gap-1.5 ${needsAttention ? 'text-rose-600' : 'text-emerald-700'}`}><span className={`h-1.5 w-1.5 rounded-full ${needsAttention ? 'bg-rose-500' : 'bg-emerald-500'}`} />{core.error_code ? `Erreur robot ${core.error_code}` : stationError ? 'Station à vérifier' : 'Aucune erreur signalée'}</span></Metric>
        <Metric label="Dernière réception"><span className="flex items-center gap-1.5"><Clock3 className="h-3.5 w-3.5 text-slate-400" />{relativeTimestamp(core.last_seen)}</span></Metric>
      </div>
      {core.error_code > 0 && <div className="mt-4 flex items-center gap-2 rounded-xl bg-rose-50 p-3 text-xs text-rose-700"><AlertCircle className="h-4 w-4 shrink-0" />Le robot demande une intervention (code {core.error_code}). Le détail est disponible dans l’application Roborock et dans le diagnostic.</div>}
      {stationError && <div className="mt-3 flex items-center gap-2 rounded-xl bg-amber-50 p-3 text-xs text-amber-800"><AlertCircle className="h-4 w-4 shrink-0" />La station signale une erreur{props.dockErrorCode ? ` (code ${props.dockErrorCode})` : ''}. Vérifiez la station et le détail dans l’application Roborock.</div>}
      {robot.active_program && <p className="mt-4 text-xs text-slate-500">Programme actif : <span className="font-medium text-slate-700">{robot.active_scene_name || (robot.active_scene_id ? `Programme n° ${robot.active_scene_id}` : displayRobotState(robot.active_program))}</span></p>}
    </header>
    <div className="flex gap-6 overflow-x-auto border-y border-slate-100 bg-slate-50/40 px-5 sm:gap-7 sm:px-7" role="tablist" aria-label="Informations du robot">
      {([{ id: 'loxone', label: 'Connexion Loxone', icon: Link2 }, { id: 'rooms', label: 'Pièces et programmes', icon: Shapes }, { id: 'diagnostics', label: 'Diagnostic', icon: Settings2 }] as const).map(item => <button key={item.id} type="button" role="tab" aria-selected={tab === item.id} aria-controls={`robot-panel-${robot.slug}`} onClick={() => setTab(item.id)} className={`relative flex min-h-14 shrink-0 items-center gap-2 px-1 text-xs font-medium transition ${tab === item.id ? 'text-emerald-800 after:absolute after:inset-x-0 after:bottom-0 after:h-0.5 after:rounded-t-full after:bg-emerald-600' : 'text-slate-400 hover:text-slate-700'}`}><item.icon className="h-4 w-4" />{item.label}</button>)}
    </div>
    <div id={`robot-panel-${robot.slug}`} role="tabpanel" className="p-5 sm:p-7">
      {tab === 'loxone' && <DirectHttpSetupPanel robot={robot} apiUsername={props.directAPIUsername} tokenConfigured={props.directTokenConfigured} />}
      {tab === 'rooms' && <div className="space-y-8"><Section title="Vos pièces" count={`${robot.rooms.length} pièces`} description="Personnalisez les noms utilisés dans la passerelle et choisissez les pièces incluses dans votre dossier Loxone."><RoomMappingTable slug={robot.slug} rooms={robot.rooms} selected={selected} drafts={drafts} validation={validation} busy={busy} onDraft={props.onDraft} onSave={props.onSaveRoom} onReset={props.onResetRoom} onSelect={props.onSelectRoom} /></Section><Section title="Vos programmes Roborock" count={`${robot.scenes.length} programmes`} description="Retrouvez les programmes créés dans l’application Roborock et lancez-les depuis la passerelle."><ScenesPanel slug={robot.slug} scenes={robot.scenes} activities={activities} selected={selected} busy={busy} onSelect={props.onSelectScene} onTest={props.onTestScene} /></Section></div>}
      {tab === 'diagnostics' && <div className="space-y-6">
        <div><h3 className="font-semibold text-slate-800">Suivre les échanges</h3><p className="mt-1 text-sm text-slate-500">Les dernières commandes et les événements utiles de ce robot.</p></div>
        {lastCommand && <div className="rounded-xl border border-slate-200 bg-slate-50/50 px-4 py-3 text-sm"><span className="text-slate-400">Dernière commande</span><p className="mt-1 font-medium text-slate-700">{displayActivity(lastCommand)}</p></div>}
        <DiagnosticsPanel core={core} activity={lastActivity} lastCommand={lastCommand} activities={activities} mqttTest={robot.mqtt_enabled ? mqttTest : undefined} testing={busy.has('mqtt-test')} onTestMQTT={props.onTestMQTT} mqttEnabled={robot.mqtt_enabled} />
        <details className="group rounded-xl border border-slate-200"><summary className="flex cursor-pointer list-none items-center justify-between px-4 py-4 text-sm font-medium text-slate-600">Informations techniques et commandes avancées<ChevronDown className="h-4 w-4 transition-transform group-open:rotate-180" /></summary><div className="space-y-5 border-t border-slate-100 p-4"><div className="flex flex-wrap gap-x-6 gap-y-2 text-xs text-slate-500"><span>Identifiant : <code>{robot.slug}</code></span><span>Station : {stationState}</span><span>Réponse : {robot.health?.status_latency_ms || 0} ms</span><span>Échecs successifs : {robot.health?.consecutive_failures || 0}</span></div><AdvancedDiagnosticsPanel slug={robot.slug} initialCapabilities={robot.capabilities} busy={busy} onCommand={props.onCommand} />{robot.mqtt_enabled && <Section title="Connexion MQTT existante" description="Ces paramètres restent disponibles pour les installations qui utilisent ce transport."><MqttTopicsPanel topics={robot.topics} /></Section>}</div></details>
      </div>}
    </div>
  </article>;
}

function Section({ title, count, description, children }: { title: string; count?: string; description?: string; children: React.ReactNode }) {
  return <section><div className="mb-4 flex flex-wrap items-start justify-between gap-2"><div><h3 className="font-semibold text-slate-800">{title}</h3>{description && <p className="mt-1 max-w-2xl text-sm leading-relaxed text-slate-500">{description}</p>}</div>{count && <StatusBadge>{count}</StatusBadge>}</div>{children}</section>;
}

function Metric({ label, children }: { label: string; children: React.ReactNode }) {
  return <div className="min-w-0 lg:pl-5 lg:first:pl-0"><p className="mb-2 text-[10px] font-medium uppercase tracking-wider text-slate-400">{label}</p><div className="text-xs font-medium text-slate-700">{children}</div></div>;
}

function CommandButton({ icon, label, command, robot, busy, onCommand, primary = false }: { icon: React.ReactNode; label: string; command: string; robot: string; busy: Set<string>; onCommand: (key: string, command: string, success: string) => void; primary?: boolean }) {
  const key = `cmd:${robot}:${command}`;
  return <button onClick={() => onCommand(key, command, `Commande « ${label} » envoyée au robot.`)} disabled={busy.has(key)} className={`inline-flex min-h-10 items-center justify-center gap-2 rounded-xl border px-3.5 text-xs font-medium transition disabled:opacity-50 ${primary ? 'border-emerald-700 bg-emerald-700 text-white shadow-sm hover:bg-emerald-800' : 'border-slate-200 bg-white text-slate-600 hover:border-emerald-200 hover:bg-emerald-50/50'}`}>{busy.has(key) ? <Spinner label={label} /> : <>{icon}{label}</>}</button>;
}
