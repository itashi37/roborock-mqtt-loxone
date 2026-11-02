import { useState } from 'react';
import { LocateFixed, Octagon, RefreshCw, ShieldAlert, Trash2, Waves, Wind } from 'lucide-react';
import { fetchAdvancedDiagnostics } from '@/lib/api';
import type { AdvancedDiagnosticsResponse, RobotCapabilities } from '@/types/loxone';
import { CopyButton, StatusBadge } from './LoxoneUI';

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
    catch (reason) { setError(reason instanceof Error ? reason.message : 'Le diagnostic n’a pas pu être récupéré.'); }
    finally { setLoading(false); }
  };
  const command = (name: string, label: string) => onCommand(`cmd:${slug}:${name}`, name, `Commande « ${label} » envoyée au robot.`);
  const supported = (name: keyof RobotCapabilities) => capabilities[name]?.supported === true;
  return <div className="space-y-4 rounded-xl border border-slate-200 p-4">
    <div className="flex flex-wrap items-center justify-between gap-3"><div><h4 className="text-sm font-medium text-slate-700">Fonctions disponibles sur ce robot</h4><p className="mt-1 text-xs text-slate-400">Seules les fonctions confirmées par le robot sont proposées.</p></div><button onClick={query} disabled={loading} className="inline-flex min-h-9 items-center gap-2 rounded-lg border border-slate-200 px-3 text-xs text-slate-500 hover:bg-slate-50 disabled:opacity-50"><RefreshCw className={`h-3.5 w-3.5 ${loading ? 'animate-spin' : ''}`} />Interroger le robot</button></div>
    {error && <div className="rounded-lg bg-red-500/10 p-3 text-sm text-red-600">{error}</div>}
    <div className="flex flex-wrap gap-2">
      {supported('stop') && <AdvancedButton icon={<Octagon />} label="Arrêter" busy={busy.has(`cmd:${slug}:stop`)} onClick={() => command('stop', 'Arrêter')} />}
      {supported('locate') && <AdvancedButton icon={<LocateFixed />} label="Localiser" busy={busy.has(`cmd:${slug}:locate`)} onClick={() => command('locate', 'Localiser')} />}
      {supported('dock_empty') && <><AdvancedButton icon={<Trash2 />} label="Vider le bac" busy={busy.has(`cmd:${slug}:empty_dustbin`)} onClick={() => command('empty_dustbin', 'Vider le bac')} /><AdvancedButton label="Arrêter le vidage" busy={busy.has(`cmd:${slug}:stop_emptying`)} onClick={() => command('stop_emptying', 'Arrêter le vidage')} /></>}
      {supported('mop_wash') && <><AdvancedButton icon={<Waves />} label="Laver les serpillières" busy={busy.has(`cmd:${slug}:wash_mop`)} onClick={() => command('wash_mop', 'Laver les serpillières')} /><AdvancedButton label="Arrêter le lavage" busy={busy.has(`cmd:${slug}:stop_washing`)} onClick={() => command('stop_washing', 'Arrêter le lavage')} /></>}
      {supported('mop_dry') && <><AdvancedButton icon={<Wind />} label="Sécher les serpillières" busy={busy.has(`cmd:${slug}:dry_mop`)} onClick={() => command('dry_mop', 'Sécher les serpillières')} /><AdvancedButton label="Arrêter le séchage" busy={busy.has(`cmd:${slug}:stop_drying`)} onClick={() => command('stop_drying', 'Arrêter le séchage')} /></>}
      {!supported('stop') && !supported('locate') && !supported('dock_empty') && !supported('mop_wash') && !supported('mop_dry') && <span className="text-xs text-slate-400">Aucune commande avancée n’est encore confirmée. Interrogez le robot pour actualiser les fonctions disponibles.</span>}
    </div>
    <details className="rounded-lg bg-slate-50 p-3"><summary className="cursor-pointer text-xs text-slate-500">Toutes les capacités détectées</summary><div className="mt-3 flex flex-wrap gap-1">{Object.entries(capabilities).map(([name, capability]) => <StatusBadge key={name} tone={capability.supported === true ? 'green' : capability.supported === false ? 'neutral' : 'neutral'}>{name} : {capability.supported === null ? 'inconnu' : capability.supported ? 'disponible' : 'indisponible'}</StatusBadge>)}</div></details>
    {result && <details className="rounded-lg bg-slate-50 p-3"><summary className="cursor-pointer text-xs font-medium text-slate-500"><ShieldAlert className="mr-2 inline h-3.5 w-3.5" />Données techniques du robot</summary><div className="mt-3 flex justify-end"><CopyButton value={JSON.stringify(result.diagnostics, null, 2)} label="Copier les données JSON" /></div><pre className="mt-2 max-h-80 overflow-auto whitespace-pre-wrap break-all text-[10px] text-slate-500">{JSON.stringify(result.diagnostics, null, 2)}</pre></details>}
  </div>;
}

function AdvancedButton({ icon, label, busy, onClick }: { icon?: React.ReactNode; label: string; busy: boolean; onClick: () => void }) {
  return <button disabled={busy} onClick={onClick} className="inline-flex min-h-9 items-center gap-2 rounded-lg border border-slate-200 px-3 text-xs text-slate-600 hover:border-blue-200 hover:bg-blue-50 disabled:opacity-50">{icon && <span className="h-4 w-4 [&>svg]:h-4 [&>svg]:w-4">{icon}</span>}{label}</button>;
}
