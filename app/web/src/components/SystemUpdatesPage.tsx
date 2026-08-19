import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { Activity, AlertTriangle, ArrowLeft, CheckCircle2, Clock3, Database, ExternalLink, HardDrive, RefreshCw, Server, ShieldCheck, Wifi } from 'lucide-react';
import { checkForUpdates, fetchSystemStatus, fetchUpdateOperation, installUpdate, saveUpdateSettings } from '@/lib/api';
import type { SystemStatus, UpdateOperation, UpdateSettings } from '@/types/system';

const activeUpdateStages = new Set(['preparing', 'pulling', 'backing_up', 'restarting', 'validating', 'rollback']);

export function SystemUpdatesPage({ returnSlug }: { returnSlug?: string }) {
  const [status, setStatus] = useState<SystemStatus | null>(null);
  const [channel, setChannel] = useState<'stable' | 'edge'>('stable');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [showNotes, setShowNotes] = useState(false);
  const [operation, setOperation] = useState<UpdateOperation | null>(null);
  const [updaterAvailable, setUpdaterAvailable] = useState<boolean | null>(null);
  const [confirmInstall, setConfirmInstall] = useState(false);
  const [settings, setSettings] = useState<UpdateSettings | null>(null);
  const [settingsSaved, setSettingsSaved] = useState(false);

  const load = async () => {
    setError('');
    try {
      const value = await fetchSystemStatus();
      setStatus(value);
      setSettings(value.update_settings);
      setChannel(value.channel === 'edge' ? 'edge' : 'stable');
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Unable to load system status');
    }
  };

  const loadOperation = async () => {
    try {
      const value = await fetchUpdateOperation();
      setOperation(value); setUpdaterAvailable(true);
      if (value.stage === 'success') void load();
    } catch {
      setUpdaterAvailable(false);
    }
  };

  useEffect(() => { void load(); void loadOperation(); }, []);
  useEffect(() => {
    if (!operation || !activeUpdateStages.has(operation.stage)) return;
    const timer = window.setInterval(() => void loadOperation(), 2000);
    return () => window.clearInterval(timer);
  }, [operation?.stage]);

  const check = async () => {
    setBusy(true); setError(''); setShowNotes(false);
    try {
      const update = await checkForUpdates(channel);
      setStatus(current => current ? { ...current, update } : current);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Update check failed');
    } finally {
      setBusy(false);
    }
  };

  const install = async () => {
    setBusy(true); setError(''); setConfirmInstall(false);
    try {
      const next = await installUpdate(channel);
      setOperation(next); setUpdaterAvailable(true);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Update installation failed to start');
    } finally {
      setBusy(false);
    }
  };

  const savePolicy = async () => {
    if (!settings) return;
    setBusy(true); setError(''); setSettingsSaved(false);
    try {
      const saved = await saveUpdateSettings(settings);
      setSettings(saved); setSettingsSaved(true);
      window.setTimeout(() => setSettingsSaved(false), 3000);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Failed to save update policy');
    } finally {
      setBusy(false);
    }
  };

  if (!status) return <PageShell returnSlug={returnSlug}><div className="rounded-2xl border border-border bg-card p-8 text-center text-muted-foreground">{error || 'Loading system status…'}</div></PageShell>;
  const update = status.update;
  const healthTone = status.health.status === 'healthy' ? 'green' : status.health.status === 'degraded' ? 'amber' : 'red';

  return <PageShell returnSlug={returnSlug}>
    {error && <div className="rounded-xl border border-red-500/25 bg-red-500/10 p-4 text-sm text-red-600">{error}</div>}

    <section className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
      <SummaryCard icon={<Activity />} label="Health" value={status.health.status} tone={healthTone} detail={`Live ${yesNo(status.health.live)} · Ready ${yesNo(status.health.ready)}`} />
      <SummaryCard icon={<Clock3 />} label="Uptime" value={formatDuration(status.uptime_seconds)} detail={`Started ${formatDate(status.started_at)}`} />
      <SummaryCard icon={<HardDrive />} label="Architecture" value={status.architecture} detail={status.go_version} />
      <SummaryCard icon={<Database />} label="Data volume" value={status.data_volume.writable ? 'Writable' : 'Error'} tone={status.data_volume.writable ? 'green' : 'red'} detail={`${formatBytes(status.data_volume.free_bytes)} free`} />
    </section>

    <section className="grid gap-4 lg:grid-cols-2">
      <Card title="Installed build" icon={<Server className="h-5 w-5" />}>
        <KeyValue label="Version" value={status.version} badge={status.channel === 'edge' ? 'Beta / Edge' : 'Stable'} />
        <KeyValue label="Git commit" value={shortCommit(status.git_commit)} mono />
        <KeyValue label="Build time" value={formatDate(status.build_time)} />
        <KeyValue label="Last restart" value={formatDate(status.last_restart)} />
        <KeyValue label="Watchdog" value={status.last_watchdog_reason || 'No recovery recorded'} />
        <KeyValue label="Supervisor" value={`${status.supervisor.kind} · ${status.supervisor.log_mode}`} />
      </Card>

      <Card title="Transports" icon={<Wifi className="h-5 w-5" />}>
        {Object.entries(status.transports).map(([name, transport]) => <div key={name} className="flex items-start justify-between gap-4 border-b border-border py-3 last:border-0">
          <div><strong className="capitalize">{name === 'direct' ? 'Direct Loxone' : name}</strong><p className="mt-1 text-xs text-muted-foreground">{transport.enabled ? (transport.last_success ? `Last success ${formatDate(transport.last_success)}` : 'Waiting for first successful transmission') : 'Disabled'}</p>{transport.last_error && <p className="mt-1 text-xs text-red-500">{transport.last_error}</p>}</div>
          <Pill tone={!transport.enabled ? 'gray' : transport.connected ? 'green' : 'red'}>{!transport.enabled ? 'Off' : transport.connected ? 'Connected' : 'Unavailable'}</Pill>
        </div>)}
      </Card>
    </section>

    <Card title="Updates" icon={<RefreshCw className="h-5 w-5" />}>
      <div className="grid gap-4 md:grid-cols-[minmax(0,1fr)_auto] md:items-end">
        <div><label className="text-sm font-medium">Channel</label><div className="mt-2 grid max-w-md grid-cols-2 rounded-xl bg-muted p-1"><ChannelButton active={channel === 'stable'} onClick={() => setChannel('stable')} title="Stable" subtitle="Recommended releases" /><ChannelButton active={channel === 'edge'} onClick={() => setChannel('edge')} title="Beta / Edge" subtitle="Latest main build" /></div></div>
        <button onClick={() => void check()} disabled={busy} className="touch-target inline-flex items-center justify-center gap-2 rounded-xl bg-primary px-5 font-medium text-primary-foreground disabled:opacity-60"><RefreshCw className={`h-4 w-4 ${busy ? 'animate-spin' : ''}`} />{busy ? 'Checking…' : 'Check for updates'}</button>
      </div>
      <div className={`mt-5 rounded-xl border p-4 ${update.checked_at ? update.available ? 'border-blue-500/30 bg-blue-500/5' : 'border-green-500/30 bg-green-500/5' : 'border-border bg-muted/30'}`}>
        {!update.checked_at ? <p className="text-sm text-muted-foreground">No update check has been performed since this bridge started.</p> : <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between"><div className="flex items-start gap-3">{update.available ? <AlertTriangle className="mt-0.5 h-5 w-5 text-blue-500" /> : <CheckCircle2 className="mt-0.5 h-5 w-5 text-green-500" />}<div><strong>{update.available ? `Version ${update.latest_version} is available` : 'You are up to date'}</strong><p className="mt-1 text-xs text-muted-foreground">Checked {formatDate(update.checked_at)}{update.published_at ? ` · Published ${formatDate(update.published_at)}` : ''}</p></div></div><div className="flex gap-2">{update.release_notes && <button onClick={() => setShowNotes(value => !value)} className="touch-target rounded-lg border border-border px-3 text-sm hover:bg-accent">View release notes</button>}{update.release_url && <a href={update.release_url} target="_blank" rel="noreferrer" className="touch-target inline-flex items-center gap-1 rounded-lg border border-border px-3 text-sm hover:bg-accent">GitHub <ExternalLink className="h-3.5 w-3.5" /></a>}</div></div>}
        {showNotes && update.release_notes && <pre className="mt-4 max-h-72 overflow-auto whitespace-pre-wrap rounded-lg bg-background p-4 text-sm leading-relaxed">{update.release_notes}</pre>}
      </div>
      {update.available && update.checked_at && <div className="mt-4 rounded-xl border border-amber-500/25 bg-amber-500/5 p-4">{!confirmInstall ? <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between"><div><strong>Manual installation</strong><p className="mt-1 text-xs text-muted-foreground">The isolated updater backs up the data volume and rolls back automatically if the new bridge is unhealthy.</p></div><button onClick={() => setConfirmInstall(true)} disabled={busy || updaterAvailable !== true || Boolean(operation && activeUpdateStages.has(operation.stage))} className="touch-target rounded-xl bg-amber-500 px-5 font-medium text-black disabled:opacity-50">Install update</button></div> : <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between"><div><strong>Confirm installation of {update.latest_version}</strong><p className="mt-1 text-xs text-muted-foreground">Robot commands and the Web interface will be briefly unavailable during restart.</p></div><div className="flex gap-2"><button onClick={() => setConfirmInstall(false)} className="touch-target rounded-xl border border-border px-4">Cancel</button><button onClick={() => void install()} className="touch-target rounded-xl bg-red-600 px-4 font-medium text-white">Confirm & install</button></div></div>}</div>}
      {updaterAvailable === false && <p className="mt-3 text-xs text-amber-600">The isolated updater is unavailable. Start the Compose <code>updates</code> profile to enable installation.</p>}
      {operation && operation.stage !== 'idle' && <UpdateProgress operation={operation} />}
      <div className="mt-4 flex items-start gap-2 text-xs text-muted-foreground"><ShieldCheck className="mt-0.5 h-4 w-4 shrink-0 text-green-500" /><span>The bridge only sends an authenticated allowlisted request. Docker access remains confined to the isolated updater.</span></div>
    </Card>

    {settings && <Card title="Automatic update policy" icon={<Clock3 className="h-5 w-5" />}>
      <div className="grid gap-4 lg:grid-cols-2">
        <label className="text-sm"><span className="font-medium">Mode</span><select value={settings.mode} onChange={event => setSettings({ ...settings, mode: event.target.value as UpdateSettings['mode'] })} className="mt-2 w-full rounded-xl border border-border bg-background px-3 py-2"><option value="off">Off</option><option value="notify">Notify only (recommended)</option><option value="automatic">Automatic</option></select></label>
        <label className="text-sm"><span className="font-medium">Channel</span><select value={settings.channel} onChange={event => setSettings({ ...settings, channel: event.target.value as UpdateSettings['channel'], allow_edge_automatic: event.target.value === 'edge' ? settings.allow_edge_automatic : false })} className="mt-2 w-full rounded-xl border border-border bg-background px-3 py-2"><option value="stable">Stable</option><option value="edge">Beta / Edge</option></select></label>
        <label className="text-sm"><span className="font-medium">Allowed from</span><input type="time" value={settings.window_start} onChange={event => setSettings({ ...settings, window_start: event.target.value })} className="mt-2 w-full rounded-xl border border-border bg-background px-3 py-2" /></label>
        <label className="text-sm"><span className="font-medium">Allowed until</span><input type="time" value={settings.window_end} onChange={event => setSettings({ ...settings, window_end: event.target.value })} className="mt-2 w-full rounded-xl border border-border bg-background px-3 py-2" /></label>
        <label className="text-sm"><span className="font-medium">Delay after publication</span><div className="mt-2 flex items-center gap-2"><input type="number" min="1" max="720" value={settings.delay_hours} onChange={event => setSettings({ ...settings, delay_hours: Number(event.target.value) })} className="w-28 rounded-xl border border-border bg-background px-3 py-2" /><span className="text-muted-foreground">hours</span></div></label>
        <div className="text-sm"><span className="font-medium">Allowed days</span><div className="mt-2 flex flex-wrap gap-1">{['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'].map((day, index) => <button key={day} onClick={() => setSettings({ ...settings, allowed_days: settings.allowed_days.includes(index) ? settings.allowed_days.filter(value => value !== index) : [...settings.allowed_days, index].sort() })} className={`rounded-lg px-2.5 py-2 text-xs ${settings.allowed_days.includes(index) ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground'}`}>{day}</button>)}</div></div>
      </div>
      <div className="mt-5 grid gap-2 sm:grid-cols-3"><PolicyCheck checked={settings.prevent_robot_active} label="Block while robot active" onChange={value => setSettings({ ...settings, prevent_robot_active: value })} /><PolicyCheck checked={settings.prevent_cleaning} label="Block during cleaning" onChange={value => setSettings({ ...settings, prevent_cleaning: value })} /><PolicyCheck checked={settings.prevent_command_in_progress} label="Block with command running" onChange={value => setSettings({ ...settings, prevent_command_in_progress: value })} /></div>
      {settings.mode === 'automatic' && settings.channel === 'edge' && <label className="mt-4 flex items-start gap-3 rounded-xl border border-red-500/30 bg-red-500/5 p-4 text-sm"><input type="checkbox" checked={settings.allow_edge_automatic} onChange={event => setSettings({ ...settings, allow_edge_automatic: event.target.checked })} className="mt-1" /><span><strong>I explicitly authorize automatic Edge updates.</strong><span className="mt-1 block text-xs text-muted-foreground">Edge follows every successful build from main and is never automatic without this separate consent.</span></span></label>}
      <div className="mt-5 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between"><div className="text-xs text-muted-foreground"><strong className="text-foreground">Scheduler:</strong> {status.auto_update.last_decision || 'Waiting for first evaluation'}{status.auto_update.next_window && ` · Next window ${formatDate(status.auto_update.next_window)}`}{status.auto_update.last_error && <span className="block text-red-500">{status.auto_update.last_error}</span>}</div><button onClick={() => void savePolicy()} disabled={busy || settings.allowed_days.length === 0 || (settings.mode === 'automatic' && settings.channel === 'edge' && !settings.allow_edge_automatic)} className="touch-target rounded-xl bg-primary px-5 font-medium text-primary-foreground disabled:opacity-50">{settingsSaved ? 'Saved' : 'Save update policy'}</button></div>
    </Card>}

    <Card title="Health details" icon={<Activity className="h-5 w-5" />}>
      <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">{Object.entries(status.health.components || {}).map(([name, component]) => <div key={name} className="rounded-xl border border-border p-3"><div className="flex items-center justify-between gap-2"><strong className="text-sm">{humanize(name)}</strong><Pill tone={component.healthy ? 'green' : component.required ? 'red' : 'gray'}>{component.healthy ? 'OK' : component.required ? 'Problem' : 'Optional'}</Pill></div><p className="mt-2 text-xs text-muted-foreground">{component.detail || (component.last_activity ? formatDate(component.last_activity) : 'No detail')}</p></div>)}</div>
    </Card>
  </PageShell>;
}

function PageShell({ returnSlug, children }: { returnSlug?: string; children: React.ReactNode }) { return <div className="min-h-screen bg-background p-4 text-foreground md:p-8"><main className="mx-auto max-w-6xl space-y-5"><header className="flex items-center gap-3">{returnSlug && <Link to={`/devices/${returnSlug}`} className="touch-target inline-flex items-center justify-center rounded-xl border border-border hover:bg-accent"><ArrowLeft className="h-5 w-5" /></Link>}<div><h1 className="text-2xl font-bold">System & Updates</h1><p className="text-sm text-muted-foreground">Runtime health, installed build and safe update discovery</p></div></header>{children}</main></div>; }
function Card({ title, icon, children }: { title: string; icon: React.ReactNode; children: React.ReactNode }) { return <section className="rounded-2xl border border-border bg-card p-5 shadow-sm"><div className="mb-4 flex items-center gap-2"><span className="text-primary">{icon}</span><h2 className="text-lg font-semibold">{title}</h2></div>{children}</section>; }
function SummaryCard({ icon, label, value, detail, tone = 'blue' }: { icon: React.ReactNode; label: string; value: string; detail: string; tone?: string }) { return <div className="rounded-2xl border border-border bg-card p-4 shadow-sm"><div className="flex items-center justify-between"><span className="text-sm text-muted-foreground">{label}</span><span className={tone === 'green' ? 'text-green-500' : tone === 'amber' ? 'text-amber-500' : tone === 'red' ? 'text-red-500' : 'text-primary'}>{icon}</span></div><strong className="mt-3 block truncate text-xl capitalize">{value}</strong><span className="mt-1 block truncate text-xs text-muted-foreground">{detail}</span></div>; }
function KeyValue({ label, value, mono = false, badge }: { label: string; value: string; mono?: boolean; badge?: string }) { return <div className="flex items-center justify-between gap-4 border-b border-border py-3 last:border-0"><span className="text-sm text-muted-foreground">{label}</span><span className={`min-w-0 truncate text-right text-sm ${mono ? 'font-mono' : ''}`}>{value}{badge && <span className="ml-2 rounded-full bg-blue-500/10 px-2 py-0.5 text-xs text-blue-600">{badge}</span>}</span></div>; }
function Pill({ tone, children }: { tone: string; children: React.ReactNode }) { const color = tone === 'green' ? 'bg-green-500/10 text-green-600' : tone === 'red' ? 'bg-red-500/10 text-red-600' : tone === 'amber' ? 'bg-amber-500/10 text-amber-600' : 'bg-muted text-muted-foreground'; return <span className={`rounded-full px-2 py-0.5 text-xs font-medium ${color}`}>{children}</span>; }
function ChannelButton({ active, onClick, title, subtitle }: { active: boolean; onClick: () => void; title: string; subtitle: string }) { return <button onClick={onClick} className={`rounded-lg p-2 text-left transition ${active ? 'bg-card shadow-sm' : 'text-muted-foreground hover:text-foreground'}`}><strong className="block text-sm">{title}</strong><span className="text-[11px]">{subtitle}</span></button>; }
function PolicyCheck({ checked, label, onChange }: { checked: boolean; label: string; onChange: (value: boolean) => void }) { return <label className="flex items-center gap-2 rounded-xl border border-border p-3 text-sm"><input type="checkbox" checked={checked} onChange={event => onChange(event.target.checked)} /><span>{label}</span></label>; }
function UpdateProgress({ operation }: { operation: UpdateOperation }) { const stages = ['preparing', 'pulling', 'backing_up', 'restarting', 'validating', 'success']; const current = stages.indexOf(operation.stage); return <div className={`mt-4 rounded-xl border p-4 ${operation.stage === 'failed' ? 'border-red-500/30 bg-red-500/5' : operation.stage === 'success' ? 'border-green-500/30 bg-green-500/5' : 'border-blue-500/30 bg-blue-500/5'}`}><div className="flex items-center justify-between gap-3"><strong>{operation.stage === 'failed' ? 'Update failed' : operation.stage === 'rollback' ? 'Rollback in progress' : operation.stage === 'success' ? 'Update installed' : 'Update in progress'}</strong><Pill tone={operation.stage === 'failed' ? 'red' : operation.stage === 'success' ? 'green' : 'gray'}>{humanize(operation.stage)}</Pill></div><div className="mt-4 grid grid-cols-3 gap-2 sm:grid-cols-6">{stages.map((stage, index) => <div key={stage} className={`rounded-lg px-2 py-2 text-center text-[11px] ${index <= current ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground'}`}>{humanize(stage)}</div>)}</div>{operation.error && <p className="mt-3 text-sm text-red-600">{operation.error}</p>}{operation.rollback_error && <p className="mt-1 text-xs text-red-600">Rollback: {operation.rollback_error}</p>}<p className="mt-3 text-xs text-muted-foreground">Operation {operation.id || 'unknown'} · Last update {formatDate(operation.updated_at)}</p></div>; }
const yesNo = (value: boolean) => value ? 'OK' : 'KO';
const humanize = (value: string) => value.replace(/_/g, ' ').replace(/\b\w/g, (letter: string) => letter.toUpperCase());
const shortCommit = (value: string) => value.length > 12 ? value.slice(0, 12) : value;
const formatDate = (value?: string) => !value || value === 'unknown' ? 'Unknown' : new Date(value).toLocaleString();
const formatDuration = (seconds: number) => { const days = Math.floor(seconds / 86400); const hours = Math.floor(seconds % 86400 / 3600); const minutes = Math.floor(seconds % 3600 / 60); return days > 0 ? `${days}d ${hours}h` : hours > 0 ? `${hours}h ${minutes}m` : `${minutes}m`; };
const formatBytes = (bytes: number) => bytes > 1024 ** 3 ? `${(bytes / 1024 ** 3).toFixed(1)} GB` : `${(bytes / 1024 ** 2).toFixed(0)} MB`;
