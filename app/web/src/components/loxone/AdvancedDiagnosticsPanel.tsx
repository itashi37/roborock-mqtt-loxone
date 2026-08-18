import { useState } from 'react';
import { Clipboard, LocateFixed, Octagon, RefreshCw, ShieldAlert, Trash2, Waves, Wind } from 'lucide-react';
import { fetchAdvancedDiagnostics } from '@/lib/api';
import type { AdvancedDiagnosticsResponse, RobotCapabilities } from '@/types/loxone';
import { StatusBadge } from './LoxoneUI';

export function AdvancedDiagnosticsPanel({ slug, initialCapabilities, busy, onCommand }: {
  slug: string;
  initialCapabilities: RobotCapabilities;
  busy: Set<string>;
  onCommand: (key: string, command: string, success: string) => void;
}) {
  const [result, setResult] = useState<AdvancedDiagnosticsResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const capabilities = result?.capabilities ?? initialCapabilities;
  const query = async () => {
    setLoading(true); setError('');
    try { setResult(await fetchAdvancedDiagnostics(slug)); }
    catch (reason) { setError(reason instanceof Error ? reason.message : 'Diagnostic query failed'); }
    finally { setLoading(false); }
  };
  const command = (name: string, label: string) => onCommand(`cmd:${slug}:${name}`, name, `${label} accepted. Waiting for reliable confirmation.`);
  const supported = (name: keyof RobotCapabilities) => capabilities[name]?.supported === true;
  return <div className="space-y-4 rounded-xl border border-border p-4">
    <div className="flex flex-wrap items-center justify-between gap-2"><div><h4 className="font-medium">Advanced Diagnostics</h4><p className="text-xs text-muted-foreground">Admin level · allowlisted app_get_init_status fields only; credentials and protocol secrets are removed.</p></div><button onClick={query} disabled={loading} className="touch-target inline-flex items-center gap-2 rounded-lg border px-3 text-sm"><RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} /> Query robot</button></div>
    {error && <div className="rounded-lg bg-red-500/10 p-3 text-sm text-red-600">{error}</div>}
    <div className="flex flex-wrap gap-2">
      {supported('stop') && <AdvancedButton icon={<Octagon />} label="Stop" busy={busy.has(`cmd:${slug}:stop`)} onClick={() => command('stop', 'Stop')} />}
      {supported('locate') && <AdvancedButton icon={<LocateFixed />} label="Locate" busy={busy.has(`cmd:${slug}:locate`)} onClick={() => command('locate', 'Locate')} />}
      {supported('dock_empty') && <><AdvancedButton icon={<Trash2 />} label="Empty bin" busy={busy.has(`cmd:${slug}:empty_dustbin`)} onClick={() => command('empty_dustbin', 'Empty bin')} /><AdvancedButton label="Stop emptying" busy={busy.has(`cmd:${slug}:stop_emptying`)} onClick={() => command('stop_emptying', 'Stop emptying')} /></>}
      {supported('mop_wash') && <><AdvancedButton icon={<Waves />} label="Wash mop" busy={busy.has(`cmd:${slug}:wash_mop`)} onClick={() => command('wash_mop', 'Wash mop')} /><AdvancedButton label="Stop washing" busy={busy.has(`cmd:${slug}:stop_washing`)} onClick={() => command('stop_washing', 'Stop washing')} /></>}
      {supported('mop_dry') && <><AdvancedButton icon={<Wind />} label="Dry mop" busy={busy.has(`cmd:${slug}:dry_mop`)} onClick={() => command('dry_mop', 'Dry mop')} /><AdvancedButton label="Stop drying" busy={busy.has(`cmd:${slug}:stop_drying`)} onClick={() => command('stop_drying', 'Stop drying')} /></>}
      {!supported('stop') && !supported('locate') && !supported('dock_empty') && !supported('mop_wash') && !supported('mop_dry') && <span className="text-sm text-muted-foreground">No advanced command is confirmed yet. Query the robot, then refresh if explicit support flags are reported.</span>}
    </div>
    <div className="flex flex-wrap gap-1">{Object.entries(capabilities).map(([name, capability]) => <StatusBadge key={name} tone={capability.supported === true ? 'green' : capability.supported === false ? 'red' : 'neutral'}>{name}: {capability.supported === null ? 'unknown' : capability.supported ? 'supported' : 'unsupported'}</StatusBadge>)}</div>
    {result && <details className="rounded-lg bg-muted/60 p-3"><summary className="cursor-pointer text-sm font-medium"><ShieldAlert className="mr-2 inline h-4 w-4" />Sanitized Qrevo data</summary><div className="mt-3 flex justify-end"><button className="inline-flex items-center gap-1 text-xs underline" onClick={() => navigator.clipboard.writeText(JSON.stringify(result.diagnostics, null, 2))}><Clipboard className="h-3 w-3" /> Copy JSON</button></div><pre className="mt-2 max-h-80 overflow-auto whitespace-pre-wrap break-all text-xs">{JSON.stringify(result.diagnostics, null, 2)}</pre></details>}
  </div>;
}

function AdvancedButton({ icon, label, busy, onClick }: { icon?: React.ReactNode; label: string; busy: boolean; onClick: () => void }) {
  return <button disabled={busy} onClick={onClick} className="touch-target inline-flex items-center gap-2 rounded-lg border border-amber-500/40 px-3 text-sm hover:bg-amber-500/10 disabled:opacity-50">{icon && <span className="h-4 w-4">{icon}</span>}{label}</button>;
}
