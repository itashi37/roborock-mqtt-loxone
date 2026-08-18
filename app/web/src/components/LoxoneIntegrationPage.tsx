import { useEffect, useMemo, useState } from 'react';
import { AlertTriangle, ArrowLeft, Battery, CheckCircle2, Download, Home, Loader2, MapPin, Pause, Play, Radio, RefreshCw, Save, Trash2, Wifi, WifiOff } from 'lucide-react';
import { Link } from 'react-router-dom';
import {
  deleteLoxoneRoomOverride,
  downloadLoxoneIntegration,
  fetchLoxoneIntegration,
  saveLoxoneRoomOverride,
  testLoxoneCommand,
  testLoxoneMQTT,
} from '@/lib/api';
import { defaultLoxoneSelection, subscriptionBudget, updateSelectedID } from '@/lib/loxone';
import type { LoxoneActivity, LoxoneExportSelection, LoxoneIntegration, LoxoneRoom } from '@/types/loxone';

export function LoxoneIntegrationPage({ liveActivities }: { liveActivities: Record<string, LoxoneActivity> }) {
  const [integration, setIntegration] = useState<LoxoneIntegration | null>(null);
  const [selection, setSelection] = useState<LoxoneExportSelection[]>([]);
  const [roomDrafts, setRoomDrafts] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState<string | null>(null);
  const [message, setMessage] = useState<{ kind: 'error' | 'success'; text: string } | null>(null);

  const load = async () => {
    setBusy('load');
    try {
      const data = await fetchLoxoneIntegration();
      setIntegration(data);
      setSelection(defaultLoxoneSelection(data));
      setRoomDrafts(Object.fromEntries(data.robots.flatMap(robot => robot.rooms.map(room => [`${robot.slug}:${room.id}`, room.override_name ?? '']))));
    } catch (error) {
      setMessage({ kind: 'error', text: error instanceof Error ? error.message : 'Failed to load integration' });
    } finally {
      setBusy(null);
    }
  };

  useEffect(() => { void load(); }, []);

  const budget = useMemo(
    () => subscriptionBudget(selection.length, integration?.subscriptions_per_robot ?? 2, integration?.subscription_limit ?? 16),
    [selection.length, integration],
  );

  const run = async (key: string, action: () => Promise<void>, success: string) => {
    setBusy(key);
    setMessage(null);
    try {
      await action();
      setMessage({ kind: 'success', text: success });
    } catch (error) {
      setMessage({ kind: 'error', text: error instanceof Error ? error.message : 'Action failed' });
    } finally {
      setBusy(null);
    }
  };

  const replaceRooms = (slug: string, rooms: LoxoneRoom[]) => {
    setIntegration(current => current ? {
      ...current,
      robots: current.robots.map(robot => robot.slug === slug ? { ...robot, rooms } : robot),
    } : current);
  };

  const saveRoom = async (slug: string, roomId: number) => {
    const key = `${slug}:${roomId}`;
    await run(`room:${key}`, async () => {
      const rooms = await saveLoxoneRoomOverride(slug, roomId, roomDrafts[key] ?? '');
      replaceRooms(slug, rooms);
    }, 'Room name saved and MQTT state refreshed.');
  };

  const deleteRoom = async (slug: string, roomId: number) => {
    const key = `${slug}:${roomId}`;
    await run(`room:${key}`, async () => {
      const rooms = await deleteLoxoneRoomOverride(slug, roomId);
      replaceRooms(slug, rooms);
      setRoomDrafts(current => ({ ...current, [key]: '' }));
    }, 'Room override removed.');
  };

  const setRobotSelected = (slug: string, selected: boolean) => {
    setSelection(current => {
      if (!selected) return current.filter(item => item.slug !== slug);
      if (current.some(item => item.slug === slug) || !integration) return current;
      const robot = integration.robots.find(item => item.slug === slug);
      return robot ? [...current, {
        slug,
        room_ids: robot.rooms.map(room => room.id),
        scene_ids: robot.scenes.map(scene => scene.id),
      }] : current;
    });
  };

  const setItemSelected = (slug: string, field: 'room_ids' | 'scene_ids', id: number, selected: boolean) => {
    setSelection(current => current.map(item => item.slug === slug ? { ...item, [field]: updateSelectedID(item[field], id, selected) } : item));
  };

  if (!integration) {
    return <div className="min-h-screen bg-background flex items-center justify-center text-muted-foreground">
      {busy === 'load' ? <Loader2 className="h-6 w-6 animate-spin" /> : <button onClick={() => void load()} className="underline">Retry</button>}
    </div>;
  }

  const returnSlug = integration.robots[0]?.slug;

  return (
    <div className="min-h-screen bg-background p-4 md:p-8">
      <div className="max-w-6xl mx-auto space-y-6">
        <header className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-3">
            {returnSlug && <Link to={`/devices/${returnSlug}`} className="p-2 rounded-lg border border-border hover:bg-accent" aria-label="Back"><ArrowLeft className="h-5 w-5" /></Link>}
            <div>
              <h1 className="text-2xl font-bold">Loxone Integration</h1>
              <p className="text-sm text-muted-foreground">roborock-mqtt-loxone · based on {integration.upstream}</p>
            </div>
          </div>
          <button onClick={() => void load()} className="inline-flex items-center justify-center gap-2 px-3 py-2 rounded-lg border border-border hover:bg-accent">
            <RefreshCw className={`h-4 w-4 ${busy === 'load' ? 'animate-spin' : ''}`} /> Refresh
          </button>
        </header>

        {message && <div className={`p-3 rounded-lg border text-sm ${message.kind === 'error' ? 'border-red-500/30 bg-red-500/10 text-red-500' : 'border-green-500/30 bg-green-500/10 text-green-500'}`}>{message.text}</div>}

        <section className="grid gap-4 md:grid-cols-4">
          <InfoCard label="Loxone mode" value={integration.enabled ? 'Enabled' : 'Disabled'} good={integration.enabled} />
          <InfoCard label="Bridge" value={integration.bridge_started ? 'Running' : 'Stopped'} good={integration.bridge_started} />
          <InfoCard label="Robots selected" value={`${selection.length} / ${integration.robots.length}`} />
          <InfoCard label="Subscriptions" value={`${budget.required} / ${budget.limit}`} good={!budget.exceeds} />
        </section>

        {budget.exceeds && <div className="p-4 rounded-lg border border-amber-500/40 bg-amber-500/10 text-amber-600 dark:text-amber-400 flex gap-3">
          <AlertTriangle className="h-5 w-5 shrink-0" />
          <div><strong>Loxone limit exceeded.</strong> The selected pack needs {budget.required} subscriptions; Loxone documents a limit of {budget.limit}. Export remains available.</div>
        </div>}

        <section className="p-4 bg-card border border-border rounded-lg">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <h2 className="font-semibold">MQTT bridge diagnostic</h2>
              <code className="text-xs text-muted-foreground break-all">{integration.topic}</code>
              {integration.mqtt_test && <p className={`text-sm mt-1 ${integration.mqtt_test.ok ? 'text-green-500' : 'text-red-500'}`}>{integration.mqtt_test.message} · {formatTimestamp(integration.mqtt_test.tested_at)}</p>}
            </div>
            <button
              onClick={() => void run('mqtt-test', async () => {
                const result = await testLoxoneMQTT();
                setIntegration(current => current ? { ...current, mqtt_test: result } : current);
              }, 'MQTT publish/subscribe loopback succeeded.')}
              disabled={busy === 'mqtt-test'}
              className="inline-flex items-center justify-center gap-2 px-4 py-2 rounded-lg bg-primary text-primary-foreground disabled:opacity-50"
            >{busy === 'mqtt-test' ? <Loader2 className="h-4 w-4 animate-spin" /> : <Radio className="h-4 w-4" />} Test MQTT</button>
          </div>
        </section>

        {integration.robots.map(robot => {
          const selected = selection.find(item => item.slug === robot.slug);
          const activity = liveActivities[robot.slug] ?? robot.diagnostics.last_activity;
          return <section key={robot.slug} className="bg-card border border-border rounded-xl overflow-hidden">
            <div className="p-4 border-b border-border flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <div className="flex items-center gap-3">
                <input type="checkbox" checked={Boolean(selected)} onChange={event => setRobotSelected(robot.slug, event.target.checked)} className="h-4 w-4" />
                <div>
                  <h2 className="text-lg font-semibold">{robot.name}</h2>
                  <p className="text-xs text-muted-foreground">{robot.slug} · {robot.model}</p>
                </div>
              </div>
              <div className="flex flex-wrap gap-3 text-sm">
                <span className={`flex items-center gap-1 ${robot.online ? 'text-green-500' : 'text-red-500'}`}>{robot.online ? <Wifi className="h-4 w-4" /> : <WifiOff className="h-4 w-4" />}{robot.online ? 'Online' : 'Offline'}</span>
                <span className="flex items-center gap-1"><Battery className="h-4 w-4" />{robot.core.battery}%</span>
                <span className="flex items-center gap-1"><MapPin className="h-4 w-4" />{robot.core.current_room_name || 'Unknown room'}</span>
              </div>
            </div>

            <div className="grid lg:grid-cols-2 gap-6 p-4">
              <div className="space-y-5">
                <div>
                  <h3 className="text-sm font-medium uppercase tracking-wide text-muted-foreground mb-2">MQTT topics</h3>
                  <Topic label="Subscribe core" value={robot.topics.core} />
                  <Topic label="Subscribe activity" value={robot.topics.activity} />
                  <Topic label="Publish command" value={robot.topics.command} />
                </div>

                <div>
                  <h3 className="text-sm font-medium uppercase tracking-wide text-muted-foreground mb-2">Command test</h3>
                  <div className="flex flex-wrap gap-2">
                    <CommandButton icon={<Play className="h-4 w-4" />} label="Start" command="start" slug={robot.slug} busy={busy} run={run} />
                    <CommandButton icon={<Pause className="h-4 w-4" />} label="Pause" command="pause" slug={robot.slug} busy={busy} run={run} />
                    <CommandButton icon={<Home className="h-4 w-4" />} label="Dock" command="dock" slug={robot.slug} busy={busy} run={run} />
                  </div>
                  <div className="mt-3 grid gap-2 sm:grid-cols-2">
                    <select onChange={event => { const command = event.target.value; event.target.value = ''; if (command) void run(`cmd:${robot.slug}:room`, () => testLoxoneCommand(robot.slug, command), `Published ${command}`); }} defaultValue="" className="px-3 py-2 bg-background border border-border rounded-lg text-sm">
                      <option value="" disabled>Test room cleaning…</option>
                      {robot.rooms.map(room => <option key={room.id} value={room.command}>{room.effective_name} (ID {room.id})</option>)}
                    </select>
                    <select onChange={event => { const command = event.target.value; event.target.value = ''; if (command) void run(`cmd:${robot.slug}:scene`, () => testLoxoneCommand(robot.slug, command), `Published ${command}`); }} defaultValue="" className="px-3 py-2 bg-background border border-border rounded-lg text-sm">
                      <option value="" disabled>Test scene…</option>
                      {robot.scenes.map(scene => <option key={scene.id} value={scene.command}>{scene.name}</option>)}
                    </select>
                  </div>
                </div>

                <div>
                  <h3 className="text-sm font-medium uppercase tracking-wide text-muted-foreground mb-2">Diagnostic</h3>
                  <div className="rounded-lg bg-background border border-border p-3 text-sm space-y-1">
                    <p>State: <strong>{robot.core.state || 'unknown'}</strong></p>
                    <p>Error: <strong>{robot.core.error_code || 'none'}</strong></p>
                    <p>Last seen: <strong>{formatTimestamp(robot.core.last_seen)}</strong></p>
                    <p className="break-words">Last activity: <code>{activity ? describeActivity(activity) : 'none this session'}</code></p>
                    <p className="break-words">Last command: <code>{robot.diagnostics.last_command ? describeActivity(robot.diagnostics.last_command) : 'none'}</code></p>
                  </div>
                </div>
              </div>

              <div className="space-y-5">
                <div>
                  <h3 className="text-sm font-medium uppercase tracking-wide text-muted-foreground mb-2">Room mapping</h3>
                  <div className="space-y-2">
                    {robot.rooms.map(room => {
                      const key = `${robot.slug}:${room.id}`;
                      return <div key={room.id} className={`p-3 rounded-lg border ${room.conflict ? 'border-red-500/50 bg-red-500/5' : 'border-border bg-background'}`}>
                        <div className="flex gap-2 items-center mb-2 text-sm">
                          {selected && <input type="checkbox" checked={selected.room_ids.includes(room.id)} onChange={event => setItemSelected(robot.slug, 'room_ids', room.id, event.target.checked)} />}
                          <strong>ID {room.id}</strong>
                          <span className="text-muted-foreground">Roborock: {room.roborock_name}</span>
                          {room.conflict && <span className="text-red-500 ml-auto">Ambiguous</span>}
                        </div>
                        <div className="flex gap-2">
                          <input value={roomDrafts[key] ?? ''} onChange={event => setRoomDrafts(current => ({ ...current, [key]: event.target.value }))} placeholder={room.config_name || room.roborock_name} className="min-w-0 flex-1 px-3 py-2 rounded-lg border border-border bg-card text-sm" />
                          <button onClick={() => void saveRoom(robot.slug, room.id)} disabled={busy === `room:${key}`} className="p-2 rounded-lg border border-border hover:bg-accent" title="Save override"><Save className="h-4 w-4" /></button>
                          {room.override_name && <button onClick={() => void deleteRoom(robot.slug, room.id)} disabled={busy === `room:${key}`} className="p-2 rounded-lg border border-border hover:bg-accent text-red-500" title="Remove override"><Trash2 className="h-4 w-4" /></button>}
                        </div>
                        <p className="text-xs text-muted-foreground mt-1">Effective name: {room.effective_name} · command: <code>{room.command}</code></p>
                      </div>;
                    })}
                  </div>
                </div>

                {robot.scenes.length > 0 && <div>
                  <h3 className="text-sm font-medium uppercase tracking-wide text-muted-foreground mb-2">Scenes included in export</h3>
                  <div className="grid gap-2 sm:grid-cols-2">
                    {robot.scenes.map(scene => <label key={scene.id} className="flex items-center gap-2 p-2 rounded-lg border border-border bg-background text-sm">
                      {selected && <input type="checkbox" checked={selected.scene_ids.includes(scene.id)} onChange={event => setItemSelected(robot.slug, 'scene_ids', scene.id, event.target.checked)} />}
                      <span className="truncate">{scene.name}</span>
                    </label>)}
                  </div>
                </div>}
              </div>
            </div>
          </section>;
        })}

        <section className="p-5 bg-card border border-border rounded-xl flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <h2 className="font-semibold">Download Loxone Integration</h2>
            <p className="text-sm text-muted-foreground">ZIP assistant for roborock-mqtt-loxone. No MQTT or Roborock credentials are included.</p>
            <p className={`text-sm mt-1 ${budget.exceeds ? 'text-amber-500' : 'text-green-500'}`}>{budget.required} subscriptions required. Export is always available.</p>
          </div>
          <button
            onClick={() => void run('export', () => downloadLoxoneIntegration(selection), 'Integration pack downloaded.')}
            disabled={busy === 'export' || selection.length === 0}
            className="inline-flex items-center justify-center gap-2 px-4 py-3 rounded-lg bg-primary text-primary-foreground disabled:opacity-50"
          >{busy === 'export' ? <Loader2 className="h-4 w-4 animate-spin" /> : <Download className="h-4 w-4" />} Download Loxone Integration</button>
        </section>

        <footer className="text-center text-xs text-muted-foreground pb-4">roborock-mqtt-loxone preserves attribution to mqtt-home/roborock-mqtt.</footer>
      </div>
    </div>
  );
}

function InfoCard({ label, value, good }: { label: string; value: string; good?: boolean }) {
  return <div className="p-4 bg-card border border-border rounded-lg">
    <p className="text-xs uppercase tracking-wide text-muted-foreground">{label}</p>
    <p className={`mt-1 font-semibold flex items-center gap-2 ${good === false ? 'text-red-500' : good === true ? 'text-green-500' : ''}`}>
      {good === true && <CheckCircle2 className="h-4 w-4" />}{good === false && <AlertTriangle className="h-4 w-4" />}{value}
    </p>
  </div>;
}

function Topic({ label, value }: { label: string; value: string }) {
  return <div className="mb-2"><span className="text-xs text-muted-foreground">{label}</span><code className="block p-2 mt-1 bg-background border border-border rounded text-xs break-all">{value}</code></div>;
}

function CommandButton({ icon, label, command, slug, busy, run }: {
  icon: React.ReactNode;
  label: string;
  command: string;
  slug: string;
  busy: string | null;
  run: (key: string, action: () => Promise<void>, success: string) => Promise<void>;
}) {
  const key = `cmd:${slug}:${command}`;
  return <button onClick={() => void run(key, () => testLoxoneCommand(slug, command), `Published ${command}`)} disabled={busy === key} className="inline-flex items-center gap-2 px-3 py-2 rounded-lg border border-border hover:bg-accent disabled:opacity-50">{icon}{label}</button>;
}

function describeActivity(activity: LoxoneActivity): string {
  if (activity.type === 'command') return `${activity.command ?? 'command'} · ${activity.state ?? 'unknown'}${activity.error ? ` · ${activity.error}` : ''}`;
  return `${activity.event ?? 'event'}${activity.room_name ? ` · ${activity.room_name}` : ''}${activity.error_text ? ` · ${activity.error_text}` : ''}`;
}

function formatTimestamp(timestamp: number): string {
  if (!timestamp) return 'never';
  return new Date(timestamp * 1000).toLocaleString();
}
