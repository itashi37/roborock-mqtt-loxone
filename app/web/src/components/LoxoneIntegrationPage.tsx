import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { ArrowLeft, RefreshCw, Settings } from 'lucide-react';
import { Link } from 'react-router-dom';
import { deleteLoxoneRoomOverride, downloadLoxoneIntegration, fetchLoxoneIntegration, saveLoxoneRoomOverride, testLoxoneCommand, testLoxoneMQTT } from '@/lib/api';
import { defaultLoxoneSelection, latestActivity, mergeActivities, subscriptionBudget, updateSelectedID } from '@/lib/loxone';
import type { LoxoneActivity, LoxoneExportSelection, LoxoneIntegration, LoxoneRoom, LoxoneScene } from '@/types/loxone';
import type { VacuumStatus } from '@/types/status';
import { LoxoneDashboard } from './loxone/LoxoneDashboard';
import { LoxoneRobotCard } from './loxone/LoxoneRobotCard';
import { LoxoneExportPanel } from './loxone/LoxoneExportPanel';
import { EmptyState, Spinner, ToastRegion, type ToastMessage } from './loxone/LoxoneUI';

interface Props {
  liveActivities: Record<string, LoxoneActivity>;
  liveStatuses: Record<string, VacuumStatus>;
  liveAvailabilities: Record<string, boolean>;
  streamConnected: boolean;
}

export function LoxoneIntegrationPage({ liveActivities, liveStatuses, liveAvailabilities, streamConnected }: Props) {
  const [integration, setIntegration] = useState<LoxoneIntegration | null>(null);
  const [selection, setSelection] = useState<LoxoneExportSelection[]>([]);
  const [roomDrafts, setRoomDrafts] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState<Set<string>>(new Set(['load']));
  const [toast, setToast] = useState<ToastMessage | null>(null);
  const selectionInitialized = useRef(false);

  const notify = useCallback((kind: ToastMessage['kind'], text: string) => setToast({ id: Date.now(), kind, text }), []);
  const setBusyKey = (key: string, value: boolean) => setBusy(current => {
    const next = new Set(current);
    if (value) next.add(key); else next.delete(key);
    return next;
  });
  const syncDrafts = (data: LoxoneIntegration) => setRoomDrafts(current => {
    const next = { ...current };
    for (const robot of data.robots) for (const room of robot.rooms) {
      const key = `${robot.slug}:${room.id}`;
      if (!(key in next)) next[key] = room.override_name ?? room.effective_name;
    }
    return next;
  });

  const load = useCallback(async () => {
    setBusyKey('load', true);
    try {
      const data = await fetchLoxoneIntegration();
      const normalized = { ...data, robots: data.robots.map(robot => ({ ...robot, diagnostics: { ...robot.diagnostics, recent: mergeActivities([], robot.diagnostics.recent ?? []) } })) };
      setIntegration(normalized);
      setSelection(current => {
        if (!selectionInitialized.current) {
          selectionInitialized.current = true;
          return defaultLoxoneSelection(data);
        }
        return current.filter(item => data.robots.some(robot => robot.slug === item.slug));
      });
      syncDrafts(normalized);
    } catch (error) {
      notify('error', error instanceof Error ? error.message : 'Failed to load the Loxone integration.');
    } finally {
      setBusyKey('load', false);
    }
  }, [notify]);

  useEffect(() => { void load(); }, [load]);
  useEffect(() => {
    if (Object.keys(liveActivities).length === 0) return;
    setIntegration(current => current ? {
      ...current,
      robots: current.robots.map(robot => {
        const activity = liveActivities[robot.slug];
        if (!activity) return robot;
        const recent = mergeActivities(robot.diagnostics.recent ?? [], [activity]);
        const core = activity.type === 'event' && activity.event === 'room_entered' ? { ...robot.core, current_room_id: activity.room_id ?? robot.core.current_room_id, current_room_name: activity.room_name ?? robot.core.current_room_name } : robot.core;
        return { ...robot, core, diagnostics: { ...robot.diagnostics, recent, last_activity: activity, last_command: activity.type === 'command' ? activity : robot.diagnostics.last_command } };
      }),
    } : current);
  }, [liveActivities]);

  const budget = useMemo(() => subscriptionBudget(selection.length, integration?.subscriptions_per_robot ?? 2, integration?.subscription_limit ?? 16), [selection.length, integration]);
  const latest = useMemo(() => integration ? latestActivity(integration) : undefined, [integration]);
  const run = async (key: string, action: () => Promise<void>, success: string) => {
    setBusyKey(key, true);
    try { await action(); notify('success', success); }
    catch (error) { notify('error', error instanceof Error ? error.message : 'Action failed.'); }
    finally { setBusyKey(key, false); }
  };
  const replaceRooms = (slug: string, rooms: LoxoneRoom[]) => {
    setIntegration(current => current ? { ...current, robots: current.robots.map(robot => robot.slug === slug ? { ...robot, rooms } : robot) } : current);
    setRoomDrafts(current => {
      const next = { ...current };
      for (const room of rooms) next[`${slug}:${room.id}`] = room.override_name ?? room.effective_name;
      return next;
    });
  };
  const saveRoom = (slug: string, roomID: number) => void run(`room:${slug}:${roomID}`, async () => replaceRooms(slug, await saveLoxoneRoomOverride(slug, roomID, roomDrafts[`${slug}:${roomID}`] ?? '')), `Room ${roomID} saved. MQTT state has been refreshed.`);
  const resetRoom = (slug: string, roomID: number) => void run(`room:${slug}:${roomID}`, async () => replaceRooms(slug, await deleteLoxoneRoomOverride(slug, roomID)), `Room ${roomID} reset to its Roborock/configured name.`);
  const setRobotSelected = (slug: string, selected: boolean) => setSelection(current => {
    if (!selected) return current.filter(item => item.slug !== slug);
    if (current.some(item => item.slug === slug) || !integration) return current;
    const robot = integration.robots.find(item => item.slug === slug);
    return robot ? [...current, { slug, room_ids: robot.rooms.map(room => room.id), scene_ids: robot.scenes.map(scene => scene.id) }] : current;
  });
  const setItemSelected = (slug: string, field: 'room_ids' | 'scene_ids', id: number, selected: boolean) => setSelection(current => current.map(item => item.slug === slug ? { ...item, [field]: updateSelectedID(item[field], id, selected) } : item));
  const testMQTT = () => void run('mqtt-test', async () => {
    const result = await testLoxoneMQTT();
    setIntegration(current => current ? { ...current, mqtt_test: result } : current);
    if (!result.ok) throw new Error(result.message);
  }, 'MQTT publish/subscribe loopback succeeded.');

  if (!integration) return <div className="flex min-h-screen items-center justify-center bg-background text-muted-foreground">{busy.has('load') ? <Spinner label="Loading Loxone integration" /> : <button onClick={() => void load()} className="underline">Retry</button>}<ToastRegion toast={toast} dismiss={() => setToast(null)} /></div>;

  const returnSlug = integration.robots[0]?.slug;
  const onlineRobots = integration.robots.filter(robot => robot.slug in liveAvailabilities ? liveAvailabilities[robot.slug] : robot.online).length;
  return <div className="min-h-screen bg-background p-4 md:p-8"><div className="mx-auto max-w-7xl space-y-6">
    <header className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between"><div className="flex items-center gap-3">{returnSlug && <Link to={`/devices/${returnSlug}`} className="touch-target inline-flex items-center justify-center rounded-lg border border-border hover:bg-accent" aria-label="Back"><ArrowLeft className="h-5 w-5" /></Link>}<div><h1 className="text-2xl font-bold">Loxone Integration</h1><p className="text-sm text-muted-foreground">roborock-mqtt-loxone · based on {integration.upstream}</p></div></div><div className="flex gap-2"><Link to="/setup" className="touch-target inline-flex items-center justify-center gap-2 rounded-lg border border-border px-3 hover:bg-accent"><Settings className="h-4 w-4" /> Modes & settings</Link><button onClick={() => void load()} disabled={busy.has('load')} className="touch-target inline-flex items-center justify-center gap-2 rounded-lg border border-border px-3 hover:bg-accent disabled:opacity-50"><RefreshCw className={`h-4 w-4 ${busy.has('load') ? 'animate-spin' : ''}`} /> Refresh</button></div></header>
    <LoxoneDashboard bridgeStarted={integration.bridge_started && (integration.enabled || integration.direct_enabled)} streamConnected={streamConnected} mqttTest={integration.mqtt_test} onlineRobots={onlineRobots} robotCount={integration.robots.length} subscriptions={budget.required} limit={budget.limit} latest={latest} fleet={integration.fleet} />
    {integration.robots.length === 0 ? <EmptyState>No Roborock robot is currently detected by the bridge.</EmptyState> : integration.robots.map(robot => {
      const selected = selection.find(item => item.slug === robot.slug);
      const liveStatus = liveStatuses[robot.slug];
      const core = liveStatus ? { ...robot.core, state: liveStatus.state, battery: liveStatus.battery, error_code: liveStatus.error_code } : robot.core;
      const online = robot.slug in liveAvailabilities ? liveAvailabilities[robot.slug] : robot.online;
      const drafts = Object.fromEntries(robot.rooms.map(room => [String(room.id), roomDrafts[`${robot.slug}:${room.id}`] ?? room.override_name ?? room.effective_name]));
      const activities = robot.diagnostics.recent ?? [];
      return <LoxoneRobotCard key={robot.slug} robot={robot} core={core} online={online} selected={selected} drafts={drafts} activities={activities} mqttTest={integration.mqtt_test} busy={busy} onDraft={(id, value) => setRoomDrafts(current => ({ ...current, [`${robot.slug}:${id}`]: value }))} onSaveRoom={id => saveRoom(robot.slug, id)} onResetRoom={id => resetRoom(robot.slug, id)} onSelectRoom={(id, value) => setItemSelected(robot.slug, 'room_ids', id, value)} onSelectScene={(id, value) => setItemSelected(robot.slug, 'scene_ids', id, value)} onCommand={(key, command, success) => void run(key, () => testLoxoneCommand(robot.slug, command), success)} onTestScene={(scene: LoxoneScene) => void run(`scene:${robot.slug}:${scene.id}`, () => testLoxoneCommand(robot.slug, scene.command), `${scene.name} published. Waiting for /activity confirmation.`)} onTestMQTT={testMQTT} />;
    })}
    <LoxoneExportPanel robots={integration.robots} selection={selection} perRobot={integration.subscriptions_per_robot} limit={budget.limit} required={budget.required} exceeds={budget.exceeds} exporting={busy.has('export')} onRobotSelected={setRobotSelected} onDownload={() => void run('export', () => downloadLoxoneIntegration(selection), 'Loxone integration pack downloaded.')} />
    <footer className="pb-4 text-center text-xs text-muted-foreground">roborock-mqtt-loxone preserves attribution to mqtt-home/roborock-mqtt.</footer>
  </div><ToastRegion toast={toast} dismiss={() => setToast(null)} /></div>;
}
