import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { Activity, AlertTriangle, ArrowLeft, CheckCircle2, Clock3, Database, ExternalLink, HardDrive, RefreshCw, Server, ShieldCheck, Wifi } from 'lucide-react';
import { checkForUpdates, fetchSystemStatus } from '@/lib/api';
import type { SystemStatus } from '@/types/system';

export function SystemUpdatesPage({ returnSlug }: { returnSlug?: string }) {
  const [status, setStatus] = useState<SystemStatus | null>(null);
  const [channel, setChannel] = useState<'stable' | 'edge'>('stable');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [showNotes, setShowNotes] = useState(false);

  const load = async () => {
    setError('');
    try {
      const value = await fetchSystemStatus();
      setStatus(value);
      setChannel(value.channel === 'edge' ? 'edge' : 'stable');
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Unable to load system status');
    }
  };

  useEffect(() => { void load(); }, []);

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
      <div className="mt-4 flex items-start gap-2 text-xs text-muted-foreground"><ShieldCheck className="mt-0.5 h-4 w-4 shrink-0 text-green-500" /><span>Checking is read-only and uses public GitHub metadata. This bridge never receives Docker access or Registry credentials.</span></div>
    </Card>

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
const yesNo = (value: boolean) => value ? 'OK' : 'KO';
const humanize = (value: string) => value.replace(/_/g, ' ').replace(/\b\w/g, (letter: string) => letter.toUpperCase());
const shortCommit = (value: string) => value.length > 12 ? value.slice(0, 12) : value;
const formatDate = (value?: string) => !value || value === 'unknown' ? 'Unknown' : new Date(value).toLocaleString();
const formatDuration = (seconds: number) => { const days = Math.floor(seconds / 86400); const hours = Math.floor(seconds % 86400 / 3600); const minutes = Math.floor(seconds % 3600 / 60); return days > 0 ? `${days}d ${hours}h` : hours > 0 ? `${hours}h ${minutes}m` : `${minutes}m`; };
const formatBytes = (bytes: number) => bytes > 1024 ** 3 ? `${(bytes / 1024 ** 3).toFixed(1)} GB` : `${(bytes / 1024 ** 2).toFixed(0)} MB`;
