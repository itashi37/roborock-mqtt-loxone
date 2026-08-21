import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { Activity, AlertTriangle, ArrowLeft, CheckCircle2, ChevronDown, Clock3, Database, ExternalLink, RefreshCw, Server, ShieldCheck, Wifi } from 'lucide-react';
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
  const [refreshing, setRefreshing] = useState(false);

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

  const refreshStatus = async () => {
    setRefreshing(true);
    await Promise.all([load(), loadOperation()]);
    setRefreshing(false);
  };

  if (!status) return <PageShell returnSlug={returnSlug}><LoadingState error={error} onRetry={() => void refreshStatus()} /></PageShell>;
  const update = status.update;
  const healthTone = status.health.status === 'healthy' ? 'green' : status.health.status === 'degraded' ? 'amber' : 'red';
  const direct = status.transports.direct;
  const failedDirectInputs = direct?.failed_inputs || [];
  const directHas404 = direct?.last_error?.includes('404') || failedDirectInputs.some(input => input.error.includes('404'));
  const systemTitle = status.health.status === 'healthy' ? 'Everything is operational' : status.health.live ? 'Bridge running — attention required' : 'Bridge health problem';
  const systemDetail = status.health.status === 'healthy'
    ? 'Roborock, enabled integrations and the data volume are responding normally.'
    : status.health.reasons?.length
      ? `Needs attention: ${status.health.reasons.map(humanize).join(', ')}.`
      : 'Open the technical details below to identify the affected component.';

  return <PageShell returnSlug={returnSlug}>
    {error && <div className="rounded-xl border border-red-500/25 bg-red-500/10 p-4 text-sm text-red-600">{error}</div>}

    <section className={`overflow-hidden rounded-3xl border p-5 shadow-sm sm:p-6 ${healthTone === 'green' ? 'border-green-500/25 bg-gradient-to-br from-green-500/10 via-card to-card' : healthTone === 'amber' ? 'border-amber-500/30 bg-gradient-to-br from-amber-500/10 via-card to-card' : 'border-red-500/30 bg-gradient-to-br from-red-500/10 via-card to-card'}`}>
      <div className="flex flex-col gap-5 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-start gap-4">
          <div className={`flex h-12 w-12 shrink-0 items-center justify-center rounded-2xl ${healthTone === 'green' ? 'bg-green-500/15 text-green-500' : healthTone === 'amber' ? 'bg-amber-500/15 text-amber-500' : 'bg-red-500/15 text-red-500'}`}>
            {healthTone === 'green' ? <CheckCircle2 className="h-7 w-7" /> : <AlertTriangle className="h-7 w-7" />}
          </div>
          <div><div className="flex flex-wrap items-center gap-2"><h2 className="text-xl font-semibold">{systemTitle}</h2><Pill tone={healthTone}>{humanize(status.health.status)}</Pill></div><p className="mt-1 max-w-2xl text-sm text-muted-foreground">{systemDetail}</p><p className="mt-2 text-xs text-muted-foreground">Uptime {formatDuration(status.uptime_seconds)} · version {status.version} · {status.architecture}</p></div>
        </div>
        <button onClick={() => void refreshStatus()} disabled={refreshing} className="touch-target inline-flex shrink-0 items-center justify-center gap-2 rounded-xl border border-border bg-background/70 px-4 text-sm font-medium hover:bg-accent disabled:opacity-60"><RefreshCw className={`h-4 w-4 ${refreshing ? 'animate-spin' : ''}`} />{refreshing ? 'Refreshing…' : 'Refresh status'}</button>
      </div>
    </section>

    <section className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
      <ServiceCard icon={<Server />} label="Bridge" value={status.health.live ? 'Running' : 'Problem'} tone={status.health.live ? 'green' : 'red'} detail={`Ready ${yesNo(status.health.ready)} · ${formatDuration(status.uptime_seconds)}`} />
      <ServiceCard icon={<Wifi />} label="Direct Loxone" value={!direct?.enabled ? 'Disabled' : direct.connected ? 'Connected' : 'Needs attention'} tone={!direct?.enabled ? 'gray' : direct.connected ? 'green' : 'amber'} detail={!direct?.enabled ? 'Enable it in Integration Mode' : direct.last_error || (direct.last_success ? `Last sent ${formatDate(direct.last_success)}` : 'Waiting for first transmission')} />
      <ServiceCard icon={<RefreshCw />} label="Safe updater" value={updaterAvailable === null ? 'Checking…' : updaterAvailable ? 'Ready' : 'Not running'} tone={updaterAvailable === null ? 'gray' : updaterAvailable ? 'green' : 'amber'} detail={updaterAvailable ? 'Backup and rollback available' : 'Manual Docker updates still work'} />
      <ServiceCard icon={<Database />} label="Data volume" value={status.data_volume.writable ? 'Protected' : 'Problem'} tone={status.data_volume.writable ? 'green' : 'red'} detail={status.data_volume.writable ? `${formatBytes(status.data_volume.free_bytes)} available` : status.data_volume.error || 'Volume is not writable'} />
    </section>

    {directHas404 && <section className="rounded-2xl border border-amber-500/30 bg-amber-500/5 p-5 shadow-sm">
      <div className="flex items-start gap-3"><AlertTriangle className="mt-0.5 h-6 w-6 shrink-0 text-amber-500" /><div><h2 className="text-lg font-semibold">Why Direct Loxone shows HTTP 404</h2><p className="mt-1 text-sm text-muted-foreground">The Miniserver is reachable and authentication works, but it cannot find one or more Virtual Inputs. Their names are case-sensitive and must exist in Loxone Config before the bridge can send values.</p></div></div>
      <div className="mt-5 grid gap-3 md:grid-cols-3">
        <SetupStep number="1" title="Open Loxone Config">Go to <strong>Periphery → Virtual Inputs</strong>.</SetupStep>
        <SetupStep number="2" title="Create the missing inputs">Use the exact names and types listed below. Do not add spaces or rename them.</SetupStep>
        <SetupStep number="3" title="Save and resend">Save the project to the Miniserver, then open Loxone Integration and choose <strong>Resend all states</strong>.</SetupStep>
      </div>
      {failedDirectInputs.length > 0 ? <div className="mt-5 overflow-hidden rounded-xl border border-border bg-background/60"><div className="border-b border-border px-4 py-3"><strong className="text-sm">Virtual Inputs currently rejected by Loxone</strong></div><div className="divide-y divide-border">{failedDirectInputs.map(input => <div key={`${input.name}-${input.field}`} className="grid gap-2 px-4 py-3 sm:grid-cols-[minmax(0,1fr)_100px_minmax(0,1.2fr)] sm:items-center"><code className="break-all text-sm font-semibold text-primary">{input.name}</code><Pill tone="gray">{humanize(input.kind)}</Pill><span className="text-xs text-muted-foreground">{inputPurpose(input.field)}</span></div>)}</div></div> : <p className="mt-4 rounded-xl border border-border bg-background/60 p-4 text-sm text-muted-foreground">Open <Link to="/loxone" className="font-medium text-primary underline underline-offset-4">Loxone Integration</Link> and expand <strong>Virtual Inputs</strong> to copy the complete list.</p>}
      <div className="mt-4 flex flex-col gap-3 rounded-xl bg-background/60 p-4 sm:flex-row sm:items-center sm:justify-between"><p className="text-xs text-muted-foreground"><strong className="text-foreground">Important:</strong> these are monitoring inputs only. They cannot start, stop or shut down the robot, bridge or Miniserver.</p><Link to="/loxone" className="touch-target inline-flex shrink-0 items-center justify-center rounded-xl bg-primary px-4 text-sm font-medium text-primary-foreground">Open Loxone Integration</Link></div>
    </section>}

    <Card title="Updates" icon={<RefreshCw className="h-5 w-5" />}>
      <div className="grid gap-4 md:grid-cols-[minmax(0,1fr)_auto] md:items-end">
        <div><label className="text-sm font-medium">Channel</label><div className="mt-2 grid max-w-md grid-cols-2 rounded-xl bg-muted p-1"><ChannelButton active={channel === 'stable'} onClick={() => setChannel('stable')} title="Stable" subtitle="Recommended releases" /><ChannelButton active={channel === 'edge'} onClick={() => setChannel('edge')} title="Beta / Edge" subtitle="Latest main build" /></div></div>
        <button onClick={() => void check()} disabled={busy} className="touch-target inline-flex items-center justify-center gap-2 rounded-xl bg-primary px-5 font-medium text-primary-foreground disabled:opacity-60"><RefreshCw className={`h-4 w-4 ${busy ? 'animate-spin' : ''}`} />{busy ? 'Checking…' : 'Check for updates'}</button>
      </div>
      <div className={`mt-5 rounded-xl border p-4 ${update.checked_at ? update.available ? 'border-blue-500/30 bg-blue-500/5' : 'border-green-500/30 bg-green-500/5' : 'border-border bg-muted/30'}`}>
        {!update.checked_at ? <p className="text-sm text-muted-foreground">No update check has been performed since this bridge started.</p> : <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between"><div className="flex items-start gap-3">{update.available ? <AlertTriangle className="mt-0.5 h-5 w-5 text-blue-500" /> : <CheckCircle2 className="mt-0.5 h-5 w-5 text-green-500" />}<div><strong>{update.available ? `Version ${update.latest_version} is available` : 'You are up to date'}</strong><p className="mt-1 text-xs text-muted-foreground">Checked {formatDate(update.checked_at)}{update.published_at ? ` · Published ${formatDate(update.published_at)}` : ''}</p></div></div><div className="flex gap-2">{update.release_notes && <button onClick={() => setShowNotes(value => !value)} className="touch-target rounded-lg border border-border px-3 text-sm hover:bg-accent">View release notes</button>}{update.release_url && <a href={update.release_url} target="_blank" rel="noreferrer" className="touch-target inline-flex items-center gap-1 rounded-lg border border-border px-3 text-sm hover:bg-accent">GitHub <ExternalLink className="h-3.5 w-3.5" /></a>}</div></div>}
        {showNotes && update.release_notes && <pre className="mt-4 max-h-72 overflow-auto whitespace-pre-wrap rounded-lg bg-background p-4 text-sm leading-relaxed">{update.release_notes}</pre>}
      </div>
      {update.available && update.checked_at && <div className="mt-4 rounded-xl border border-amber-500/25 bg-amber-500/5 p-4">{!update.artifact_ready ? <div className="flex items-start gap-3"><RefreshCw className="mt-0.5 h-5 w-5 shrink-0 animate-spin text-blue-500" /><div><strong>Docker image is still being published</strong><p className="mt-1 text-xs text-muted-foreground">The update is visible on GitHub, but its exact multi-architecture image is not ready yet ({humanize(update.artifact_status || 'pending')}). Installation will unlock automatically after a new check confirms publication.</p></div></div> : !confirmInstall ? <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between"><div><strong>Manual installation</strong><p className="mt-1 text-xs text-muted-foreground">The isolated updater backs up the data volume, verifies commit {shortCommit(update.latest_commit || update.latest_version || '')}, and rolls back automatically if the new bridge is unhealthy or incorrect.</p></div><button onClick={() => setConfirmInstall(true)} disabled={busy || updaterAvailable !== true || Boolean(operation && activeUpdateStages.has(operation.stage))} className="touch-target rounded-xl bg-amber-500 px-5 font-medium text-black disabled:opacity-50">Install update</button></div> : <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between"><div><strong>Confirm installation of {update.latest_version}</strong><p className="mt-1 text-xs text-muted-foreground">Robot commands and the Web interface will be briefly unavailable during restart.</p></div><div className="flex gap-2"><button onClick={() => setConfirmInstall(false)} className="touch-target rounded-xl border border-border px-4">Cancel</button><button onClick={() => void install()} className="touch-target rounded-xl bg-red-600 px-4 font-medium text-white">Confirm & install</button></div></div>}</div>}
      {updaterAvailable === false && <div className="mt-4 flex flex-col gap-3 rounded-xl border border-amber-500/25 bg-amber-500/5 p-4 sm:flex-row sm:items-center sm:justify-between"><div className="flex items-start gap-3"><AlertTriangle className="mt-0.5 h-5 w-5 shrink-0 text-amber-500" /><div><strong className="text-sm">Safe updater is not running</strong><p className="mt-1 text-xs text-muted-foreground">Update checks still work. To install from this page, start the optional updater container once on your Synology.</p></div></div><code className="shrink-0 rounded-lg bg-background px-3 py-2 text-xs">docker compose --profile updates up -d</code></div>}
      {operation && operation.stage !== 'idle' && <UpdateProgress operation={operation} />}
      <div className="mt-4 flex items-start gap-2 text-xs text-muted-foreground"><ShieldCheck className="mt-0.5 h-4 w-4 shrink-0 text-green-500" /><span>The bridge only sends an authenticated allowlisted request. Docker access remains confined to the isolated updater.</span></div>
    </Card>

    {settings && <details className="group rounded-2xl border border-border bg-card shadow-sm">
      <summary className="flex cursor-pointer list-none items-center justify-between gap-4 p-5"><div className="flex items-center gap-3"><span className="text-primary"><Clock3 className="h-5 w-5" /></span><div><div className="flex flex-wrap items-center gap-2"><h2 className="font-semibold">Automatic update policy</h2><Pill tone={settings.mode === 'automatic' ? 'green' : settings.mode === 'notify' ? 'blue' : 'gray'}>{settings.mode === 'automatic' ? 'Automatic' : settings.mode === 'notify' ? 'Notify only' : 'Off'}</Pill></div><p className="mt-0.5 text-xs text-muted-foreground">{settings.channel === 'edge' ? 'Beta / Edge' : 'Stable'} · {settings.window_start}–{settings.window_end} · safety guards enabled</p></div></div><ChevronDown className="h-5 w-5 text-muted-foreground transition-transform group-open:rotate-180" /></summary>
      <div className="border-t border-border p-5"><div className="grid gap-4 lg:grid-cols-2">
        <label className="text-sm"><span className="font-medium">Mode</span><select value={settings.mode} onChange={event => setSettings({ ...settings, mode: event.target.value as UpdateSettings['mode'] })} className="mt-2 w-full rounded-xl border border-border bg-background px-3 py-2"><option value="off">Off</option><option value="notify">Notify only (recommended)</option><option value="automatic">Automatic</option></select></label>
        <label className="text-sm"><span className="font-medium">Channel</span><select value={settings.channel} onChange={event => setSettings({ ...settings, channel: event.target.value as UpdateSettings['channel'], allow_edge_automatic: event.target.value === 'edge' ? settings.allow_edge_automatic : false })} className="mt-2 w-full rounded-xl border border-border bg-background px-3 py-2"><option value="stable">Stable</option><option value="edge">Beta / Edge</option></select></label>
        <label className="text-sm"><span className="font-medium">Allowed from</span><input type="time" value={settings.window_start} onChange={event => setSettings({ ...settings, window_start: event.target.value })} className="mt-2 w-full rounded-xl border border-border bg-background px-3 py-2" /></label>
        <label className="text-sm"><span className="font-medium">Allowed until</span><input type="time" value={settings.window_end} onChange={event => setSettings({ ...settings, window_end: event.target.value })} className="mt-2 w-full rounded-xl border border-border bg-background px-3 py-2" /></label>
        <label className="text-sm"><span className="font-medium">Delay after publication</span><div className="mt-2 flex items-center gap-2"><input type="number" min="1" max="720" value={settings.delay_hours} onChange={event => setSettings({ ...settings, delay_hours: Number(event.target.value) })} className="w-28 rounded-xl border border-border bg-background px-3 py-2" /><span className="text-muted-foreground">hours</span></div></label>
        <div className="text-sm"><span className="font-medium">Allowed days</span><div className="mt-2 flex flex-wrap gap-1">{['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'].map((day, index) => <button key={day} onClick={() => setSettings({ ...settings, allowed_days: settings.allowed_days.includes(index) ? settings.allowed_days.filter(value => value !== index) : [...settings.allowed_days, index].sort() })} className={`rounded-lg px-2.5 py-2 text-xs ${settings.allowed_days.includes(index) ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground'}`}>{day}</button>)}</div></div>
      </div>
      <div className="mt-5 grid gap-2 sm:grid-cols-3"><PolicyCheck checked={settings.prevent_robot_active} label="Block while robot active" onChange={value => setSettings({ ...settings, prevent_robot_active: value })} /><PolicyCheck checked={settings.prevent_cleaning} label="Block during cleaning" onChange={value => setSettings({ ...settings, prevent_cleaning: value })} /><PolicyCheck checked={settings.prevent_command_in_progress} label="Block with command running" onChange={value => setSettings({ ...settings, prevent_command_in_progress: value })} /></div>
      {settings.mode === 'automatic' && settings.channel === 'edge' && <label className="mt-4 flex items-start gap-3 rounded-xl border border-red-500/30 bg-red-500/5 p-4 text-sm"><input type="checkbox" checked={settings.allow_edge_automatic} onChange={event => setSettings({ ...settings, allow_edge_automatic: event.target.checked })} className="mt-1" /><span><strong>I explicitly authorize automatic Edge updates.</strong><span className="mt-1 block text-xs text-muted-foreground">Edge follows every successful build from main and is never automatic without this separate consent.</span></span></label>}
      <div className="mt-5 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between"><div className="text-xs text-muted-foreground"><strong className="text-foreground">Scheduler:</strong> {status.auto_update.last_decision || 'Waiting for first evaluation'}{status.auto_update.next_window && ` · Next window ${formatDate(status.auto_update.next_window)}`}{status.auto_update.last_error && <span className="block text-red-500">{status.auto_update.last_error}</span>}</div><button onClick={() => void savePolicy()} disabled={busy || settings.allowed_days.length === 0 || (settings.mode === 'automatic' && settings.channel === 'edge' && !settings.allow_edge_automatic)} className="touch-target rounded-xl bg-primary px-5 font-medium text-primary-foreground disabled:opacity-50">{settingsSaved ? 'Saved' : 'Save update policy'}</button></div></div>
    </details>}

    <details className="group rounded-2xl border border-border bg-card shadow-sm">
      <summary className="flex cursor-pointer list-none items-center justify-between gap-4 p-5"><div className="flex items-center gap-3"><span className="text-primary"><Activity className="h-5 w-5" /></span><div><h2 className="font-semibold">Technical details</h2><p className="mt-0.5 text-xs text-muted-foreground">Build, transports, watchdog and individual health checks</p></div></div><ChevronDown className="h-5 w-5 text-muted-foreground transition-transform group-open:rotate-180" /></summary>
      <div className="border-t border-border p-5">
        <div className="grid gap-5 lg:grid-cols-2">
          <div><h3 className="mb-2 font-medium">Installed build</h3><KeyValue label="Version" value={status.version} badge={status.channel === 'edge' ? 'Beta / Edge' : 'Stable'} /><KeyValue label="Git commit" value={shortCommit(status.git_commit)} mono /><KeyValue label="Build time" value={formatDate(status.build_time)} /><KeyValue label="Architecture" value={`${status.architecture} · ${status.go_version}`} /><KeyValue label="Last restart" value={formatDate(status.last_restart)} /><KeyValue label="Watchdog" value={status.last_watchdog_reason || 'No recovery recorded'} /><KeyValue label="Supervisor" value={`${status.supervisor.kind} · ${status.supervisor.log_mode}`} /></div>
          <div><h3 className="mb-2 font-medium">Transports</h3>{Object.entries(status.transports).map(([name, transport]) => <div key={name} className="flex items-start justify-between gap-4 border-b border-border py-3 last:border-0"><div className="min-w-0"><strong className="capitalize">{name === 'direct' ? 'Direct Loxone' : name}</strong><p className="mt-1 text-xs text-muted-foreground">{transport.enabled ? (transport.last_success ? `Last success ${formatDate(transport.last_success)}` : 'Waiting for first successful transmission') : 'Disabled'}</p>{transport.last_error && <p className="mt-1 break-words text-xs text-red-500">{transport.last_error}</p>}</div><Pill tone={!transport.enabled ? 'gray' : transport.connected ? 'green' : 'red'}>{!transport.enabled ? 'Off' : transport.connected ? 'Connected' : 'Unavailable'}</Pill></div>)}</div>
        </div>
        <h3 className="mb-3 mt-6 font-medium">Health checks</h3>
        <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">{Object.entries(status.health.components || {}).map(([name, component]) => <div key={name} className="rounded-xl border border-border p-3"><div className="flex items-center justify-between gap-2"><strong className="text-sm">{humanize(name)}</strong><Pill tone={component.healthy ? 'green' : component.required ? 'red' : 'gray'}>{component.healthy ? 'OK' : component.required ? 'Problem' : 'Optional'}</Pill></div><p className="mt-2 text-xs text-muted-foreground">{component.detail || (component.last_activity ? formatDate(component.last_activity) : 'No detail')}</p></div>)}</div>
      </div>
    </details>
  </PageShell>;
}

function PageShell({ returnSlug, children }: { returnSlug?: string; children: React.ReactNode }) { return <div className="min-h-screen bg-background p-4 text-foreground md:p-8"><main className="mx-auto max-w-6xl space-y-5"><header className="flex items-center gap-3">{returnSlug && <Link to={`/devices/${returnSlug}`} className="touch-target inline-flex items-center justify-center rounded-xl border border-border hover:bg-accent"><ArrowLeft className="h-5 w-5" /></Link>}<div><h1 className="text-2xl font-bold">System & Updates</h1><p className="text-sm text-muted-foreground">Runtime health, installed build and safe update discovery</p></div></header>{children}</main></div>; }
function Card({ title, icon, children }: { title: string; icon: React.ReactNode; children: React.ReactNode }) { return <section className="rounded-2xl border border-border bg-card p-5 shadow-sm"><div className="mb-4 flex items-center gap-2"><span className="text-primary">{icon}</span><h2 className="text-lg font-semibold">{title}</h2></div>{children}</section>; }
function ServiceCard({ icon, label, value, detail, tone }: { icon: React.ReactNode; label: string; value: string; detail: string; tone: string }) { const color = tone === 'green' ? 'text-green-500 bg-green-500/10' : tone === 'amber' ? 'text-amber-500 bg-amber-500/10' : tone === 'red' ? 'text-red-500 bg-red-500/10' : 'text-muted-foreground bg-muted'; return <div className="min-w-0 rounded-2xl border border-border bg-card p-4 shadow-sm"><div className="flex items-center gap-3"><span className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-xl ${color}`}>{icon}</span><div className="min-w-0"><span className="block text-xs text-muted-foreground">{label}</span><strong className="block truncate text-base">{value}</strong></div></div><p className={`mt-3 line-clamp-2 min-h-8 text-xs ${tone === 'red' || tone === 'amber' ? 'text-foreground/75' : 'text-muted-foreground'}`} title={detail}>{detail}</p></div>; }
function LoadingState({ error, onRetry }: { error: string; onRetry: () => void }) { return <div className="rounded-3xl border border-border bg-card p-8 shadow-sm"><div className="mx-auto flex max-w-md flex-col items-center text-center">{error ? <AlertTriangle className="h-9 w-9 text-red-500" /> : <RefreshCw className="h-9 w-9 animate-spin text-primary" />}<h2 className="mt-4 text-lg font-semibold">{error ? 'Unable to load system status' : 'Loading system status'}</h2><p className="mt-2 text-sm text-muted-foreground">{error || 'Checking the bridge, integrations and updater…'}</p>{error && <button onClick={onRetry} className="touch-target mt-5 inline-flex items-center gap-2 rounded-xl bg-primary px-5 text-sm font-medium text-primary-foreground"><RefreshCw className="h-4 w-4" />Try again</button>}</div></div>; }
function SetupStep({ number, title, children }: { number: string; title: string; children: React.ReactNode }) { return <div className="rounded-xl border border-amber-500/15 bg-background/60 p-4"><div className="flex items-center gap-2"><span className="flex h-6 w-6 items-center justify-center rounded-full bg-amber-500 text-xs font-bold text-black">{number}</span><strong className="text-sm">{title}</strong></div><p className="mt-2 text-xs leading-relaxed text-muted-foreground">{children}</p></div>; }
function KeyValue({ label, value, mono = false, badge }: { label: string; value: string; mono?: boolean; badge?: string }) { return <div className="flex items-center justify-between gap-4 border-b border-border py-3 last:border-0"><span className="text-sm text-muted-foreground">{label}</span><span className={`min-w-0 truncate text-right text-sm ${mono ? 'font-mono' : ''}`}>{value}{badge && <span className="ml-2 rounded-full bg-blue-500/10 px-2 py-0.5 text-xs text-blue-600">{badge}</span>}</span></div>; }
function Pill({ tone, children }: { tone: string; children: React.ReactNode }) { const color = tone === 'green' ? 'bg-green-500/10 text-green-600' : tone === 'red' ? 'bg-red-500/10 text-red-600' : tone === 'amber' ? 'bg-amber-500/10 text-amber-600' : 'bg-muted text-muted-foreground'; return <span className={`rounded-full px-2 py-0.5 text-xs font-medium ${color}`}>{children}</span>; }
function ChannelButton({ active, onClick, title, subtitle }: { active: boolean; onClick: () => void; title: string; subtitle: string }) { return <button onClick={onClick} className={`rounded-lg p-2 text-left transition ${active ? 'bg-card shadow-sm' : 'text-muted-foreground hover:text-foreground'}`}><strong className="block text-sm">{title}</strong><span className="text-[11px]">{subtitle}</span></button>; }
function PolicyCheck({ checked, label, onChange }: { checked: boolean; label: string; onChange: (value: boolean) => void }) { return <label className="flex items-center gap-2 rounded-xl border border-border p-3 text-sm"><input type="checkbox" checked={checked} onChange={event => onChange(event.target.checked)} /><span>{label}</span></label>; }
function UpdateProgress({ operation }: { operation: UpdateOperation }) { const stages = ['preparing', 'pulling', 'backing_up', 'restarting', 'validating', 'success']; const current = stages.indexOf(operation.stage); return <div className={`mt-4 rounded-xl border p-4 ${operation.stage === 'failed' ? 'border-red-500/30 bg-red-500/5' : operation.stage === 'success' ? 'border-green-500/30 bg-green-500/5' : 'border-blue-500/30 bg-blue-500/5'}`}><div className="flex items-center justify-between gap-3"><strong>{operation.stage === 'failed' ? 'Update failed' : operation.stage === 'rollback' ? 'Rollback in progress' : operation.stage === 'success' ? 'Update installed' : 'Update in progress'}</strong><Pill tone={operation.stage === 'failed' ? 'red' : operation.stage === 'success' ? 'green' : 'gray'}>{humanize(operation.stage)}</Pill></div><div className="mt-4 grid grid-cols-3 gap-2 sm:grid-cols-6">{stages.map((stage, index) => <div key={stage} className={`rounded-lg px-2 py-2 text-center text-[11px] ${index <= current ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground'}`}>{humanize(stage)}</div>)}</div>{operation.error && <p className="mt-3 text-sm text-red-600">{operation.error}</p>}{operation.rollback_error && <p className="mt-1 text-xs text-red-600">Rollback: {operation.rollback_error}</p>}<p className="mt-3 text-xs text-muted-foreground">Operation {operation.id || 'unknown'} · Last update {formatDate(operation.updated_at)}</p></div>; }
const yesNo = (value: boolean) => value ? 'OK' : 'KO';
const humanize = (value: string) => value.replace(/_/g, ' ').replace(/\b\w/g, (letter: string) => letter.toUpperCase());
const inputPurpose = (field: string) => ({ bridge_alive: 'Bridge process running (digital)', cloud_connected: 'Roborock cloud connection (digital)', bridge_heartbeat: 'Timestamp sent every 30 seconds (analog)', robot_online: 'Robot reachable (digital)', running: 'Cleaning mission active (digital)' }[field] || `${humanize(field)} value`);
const shortCommit = (value: string) => value.length > 12 ? value.slice(0, 12) : value;
const formatDate = (value?: string) => !value || value === 'unknown' ? 'Unknown' : new Date(value).toLocaleString();
const formatDuration = (seconds: number) => { const days = Math.floor(seconds / 86400); const hours = Math.floor(seconds % 86400 / 3600); const minutes = Math.floor(seconds % 3600 / 60); return days > 0 ? `${days}d ${hours}h` : hours > 0 ? `${hours}h ${minutes}m` : `${minutes}m`; };
const formatBytes = (bytes: number) => bytes > 1024 ** 3 ? `${(bytes / 1024 ** 3).toFixed(1)} GB` : `${(bytes / 1024 ** 2).toFixed(0)} MB`;
