import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Bot, CheckCircle2, Plus, RefreshCw } from 'lucide-react';
import { Link, useSearchParams } from 'react-router-dom';
import { deleteLoxoneRoomOverride, downloadLoxoneIntegration, fetchLoxoneIntegration, saveLoxoneRoomOverride, testLoxoneCommand, testLoxoneMQTT } from '@/lib/api';
import { defaultLoxoneSelection, mergeActivities, subscriptionBudget, updateSelectedID } from '@/lib/loxone';
import type { LoxoneActivity, LoxoneExportSelection, LoxoneIntegration, LoxoneRoom, LoxoneScene } from '@/types/loxone';
import type { VacuumStatus } from '@/types/status';
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
  const [searchParams] = useSearchParams();
  const [integration, setIntegration] = useState<LoxoneIntegration | null>(null);
  const [selection, setSelection] = useState<LoxoneExportSelection[]>([]);
  const [roomDrafts, setRoomDrafts] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState<Set<string>>(new Set(['load']));
  const [toast, setToast] = useState<ToastMessage | null>(null);
  const [activeSlug, setActiveSlug] = useState(() => searchParams.get('robot') ?? '');
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
      if (!selectionInitialized.current) {
        selectionInitialized.current = true;
        setSelection(defaultLoxoneSelection(data));
      } else {
        setSelection(current => current.filter(item => data.robots.some(robot => robot.slug === item.slug)));
      }
      syncDrafts(normalized);
    } catch (error) {
      notify('error', error instanceof Error ? error.message : 'Impossible de charger la connexion Loxone.');
    } finally {
      setBusyKey('load', false);
    }
  }, [notify]);

  useEffect(() => { void load(); }, [load]);
  useEffect(() => { const requested = searchParams.get('robot'); if (requested) setActiveSlug(requested); }, [searchParams]);
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

  const mqttSelectedCount = useMemo(() => selection.filter(item => integration?.robots.find(robot => robot.slug === item.slug)?.mqtt_enabled).length, [selection, integration]);
  const budget = useMemo(() => subscriptionBudget(mqttSelectedCount, integration?.subscriptions_per_robot ?? 2, integration?.subscription_limit ?? 16), [mqttSelectedCount, integration]);
  const run = async (key: string, action: () => Promise<void>, success: string) => {
    setBusyKey(key, true);
    try { await action(); notify('success', success); }
    catch (error) { notify('error', error instanceof Error ? error.message : 'L’action n’a pas abouti.'); }
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
  const saveRoom = (slug: string, roomID: number) => void run(`room:${slug}:${roomID}`, async () => replaceRooms(slug, await saveLoxoneRoomOverride(slug, roomID, roomDrafts[`${slug}:${roomID}`] ?? '')), `Le nom de la pièce ${roomID} a été enregistré.`);
  const resetRoom = (slug: string, roomID: number) => void run(`room:${slug}:${roomID}`, async () => replaceRooms(slug, await deleteLoxoneRoomOverride(slug, roomID)), `Le nom d’origine de la pièce ${roomID} a été rétabli.`);
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
  }, 'Le test de communication MQTT a réussi.');

  if (!integration) return <div className="flex min-h-96 items-center justify-center text-slate-500">{busy.has('load') ? <Spinner label="Chargement de votre connexion Loxone…" /> : <button onClick={() => void load()} className="rounded-xl border border-slate-200 bg-white px-4 py-3 text-sm">Réessayer</button>}<ToastRegion toast={toast} dismiss={() => setToast(null)} /></div>;

  const activeRobot = integration.robots.find(robot => robot.slug === activeSlug) ?? integration.robots[0];
  const onlineRobots = integration.robots.filter(robot => robot.slug in liveAvailabilities ? liveAvailabilities[robot.slug] : robot.online).length;
  const ready = integration.bridge_started && (integration.enabled || integration.direct_enabled);
  return <div className="space-y-6 text-[#15241e]">
    <header className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between"><div><p className="text-[10px] font-semibold uppercase tracking-[0.18em] text-emerald-700">Votre maison connectée</p><h1 className="mt-2 text-3xl font-semibold tracking-tight">Connexion Loxone</h1><p className="mt-2 max-w-xl text-sm leading-relaxed text-slate-500">Tout ce qu’il faut pour relier vos robots à Loxone, au même endroit.</p></div><button onClick={() => void load()} disabled={busy.has('load')} className="inline-flex min-h-10 shrink-0 items-center justify-center gap-2 self-start rounded-xl border border-slate-200 bg-white px-3.5 text-xs font-medium text-slate-500 transition hover:bg-slate-50 disabled:opacity-50"><RefreshCw className={`h-3.5 w-3.5 ${busy.has('load') ? 'animate-spin' : ''}`} />Actualiser</button></header>
    <div className="flex flex-wrap items-center gap-x-6 gap-y-2 rounded-xl border border-slate-200/80 bg-white/70 px-4 py-3 text-xs text-slate-500"><span className="flex items-center gap-2"><span className={`h-1.5 w-1.5 rounded-full ${ready ? 'bg-emerald-500' : 'bg-amber-500'}`} />{ready ? 'Passerelle active' : 'Connexion à configurer'}</span><span className="flex items-center gap-2"><Bot className="h-3.5 w-3.5 text-slate-400" />{onlineRobots} / {integration.robots.length} robot{integration.robots.length > 1 ? 's' : ''} en ligne</span><span className="ml-auto flex items-center gap-1.5 text-slate-400">{streamConnected && <CheckCircle2 className="h-3.5 w-3.5 text-emerald-600" />}{streamConnected ? 'Actualisation en direct' : 'Reconnexion du suivi en direct…'}</span></div>
    <div className="flex flex-col gap-3 sm:flex-row sm:items-center"><div className="flex min-w-0 flex-1 flex-wrap gap-2" aria-label="Choisir un robot">{integration.robots.map(robot => <button key={robot.slug} type="button" onClick={() => setActiveSlug(robot.slug)} aria-pressed={activeRobot?.slug === robot.slug} className={`inline-flex min-h-11 items-center gap-2.5 rounded-xl border px-4 text-xs font-medium transition ${activeRobot?.slug === robot.slug ? 'border-emerald-200 bg-emerald-50 text-emerald-900' : 'border-slate-200 bg-white text-slate-500 hover:border-slate-300'}`}><Bot className="h-4 w-4" />{robot.name}</button>)}</div><Link to="/setup?section=robots" className="inline-flex min-h-10 shrink-0 items-center justify-center gap-2 self-start rounded-xl border border-dashed border-slate-300 px-3.5 text-xs font-medium text-slate-500 hover:border-emerald-400 hover:bg-emerald-50 sm:self-auto"><Plus className="h-3.5 w-3.5" />Ajouter un robot</Link></div>
    {!activeRobot ? <EmptyState>Aucun robot n’est encore disponible. Connectez votre compte Roborock dans les réglages pour retrouver vos robots.</EmptyState> : [activeRobot].map(robot => {
      const selected = selection.find(item => item.slug === robot.slug);
      const liveStatus = liveStatuses[robot.slug];
      const dockErrorCode = (liveStatus as (VacuumStatus & { dock_error_status?: number }) | undefined)?.dock_error_status;
      const core = liveStatus ? { ...robot.core, state: liveStatus.state, battery: liveStatus.battery, error_code: liveStatus.error_code } : robot.core;
      const online = robot.slug in liveAvailabilities ? liveAvailabilities[robot.slug] : robot.online;
      const drafts = Object.fromEntries(robot.rooms.map(room => [String(room.id), roomDrafts[`${robot.slug}:${room.id}`] ?? room.override_name ?? room.effective_name]));
      const activities = robot.diagnostics.recent ?? [];
      return <LoxoneRobotCard key={robot.slug} robot={robot} core={core} dockErrorCode={dockErrorCode} online={online} selected={selected} drafts={drafts} activities={activities} mqttTest={integration.mqtt_test} busy={busy} directAPIUsername={integration.direct_api_username || 'loxone'} directTokenConfigured={integration.direct_token_configured} onDraft={(id, value) => setRoomDrafts(current => ({ ...current, [`${robot.slug}:${id}`]: value }))} onSaveRoom={id => saveRoom(robot.slug, id)} onResetRoom={id => resetRoom(robot.slug, id)} onSelectRoom={(id, value) => setItemSelected(robot.slug, 'room_ids', id, value)} onSelectScene={(id, value) => setItemSelected(robot.slug, 'scene_ids', id, value)} onCommand={(key, command, success) => void run(key, () => testLoxoneCommand(robot.slug, command), success)} onTestScene={(scene: LoxoneScene) => void run(`scene:${robot.slug}:${scene.id}`, () => testLoxoneCommand(robot.slug, scene.command), `Le programme « ${scene.name} » a été envoyé au robot.`)} onTestMQTT={testMQTT} />;
    })}
    {integration.robots.length > 0 && <LoxoneExportPanel robots={integration.robots} selection={selection} mqttRobotCount={mqttSelectedCount} templateStatus={integration.template_status} perRobot={integration.subscriptions_per_robot} limit={budget.limit} required={budget.required} exceeds={budget.exceeds} exporting={busy.has('export')} onRobotSelected={setRobotSelected} onDownload={() => void run('export', () => downloadLoxoneIntegration(selection), 'Votre dossier de configuration Loxone a été téléchargé.')} />}
    <ToastRegion toast={toast} dismiss={() => setToast(null)} />
  </div>;
}
