import { Bug, Radio } from 'lucide-react';
import type { LoxoneActivity, LoxoneCore, LoxoneMQTTTest } from '@/types/loxone';
import { displayActivity, EmptyState, formatTimestamp, relativeTimestamp, Spinner, StatusBadge } from './LoxoneUI';

interface Props {
  core: LoxoneCore;
  activity?: LoxoneActivity;
  lastCommand?: LoxoneActivity;
  activities: LoxoneActivity[];
  mqttTest?: LoxoneMQTTTest;
  testing: boolean;
  onTestMQTT: () => void;
  mqttEnabled?: boolean;
}

export function DiagnosticsPanel({ core, activity, activities, mqttTest, testing, onTestMQTT, mqttEnabled = false }: Props) {
  return <div className="space-y-5">
    {mqttEnabled && <div className="flex flex-wrap items-center gap-3 rounded-xl border border-slate-200 p-4"><Radio className="h-4 w-4 text-slate-400" /><div className="flex-1"><p className="text-sm font-medium text-slate-700">Test de la connexion MQTT</p><p className="mt-1 text-xs text-slate-400">{mqttTest ? `${mqttTest.message} · ${relativeTimestamp(mqttTest.tested_at)}` : 'Disponible pour votre connexion MQTT existante.'}</p></div>{mqttTest && <StatusBadge tone={mqttTest.ok ? 'green' : 'red'}>{mqttTest.ok ? 'Réussi' : 'Échec'}</StatusBadge>}<button onClick={onTestMQTT} disabled={testing} className="inline-flex min-h-9 items-center gap-2 rounded-lg border border-slate-200 px-3 text-xs font-medium text-slate-600 hover:bg-slate-50 disabled:opacity-50">{testing ? <Spinner label="Test en cours" /> : 'Tester'}</button></div>}
    <div><h4 className="mb-3 text-xs font-medium text-slate-500">Historique récent</h4>{!activities.length ? <EmptyState>Les commandes et les événements du robot apparaîtront ici.</EmptyState> : <ol className="max-h-96 divide-y divide-slate-100 overflow-y-auto rounded-xl border border-slate-200">{activities.map((item, index) => <li key={`${item.type}-${item.ts}-${index}`} className="flex items-start gap-3 p-4"><span className={`mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full ${item.event === 'error' || item.event === 'stuck' || item.state === 'failed' ? 'bg-rose-400' : item.type === 'command' ? 'bg-blue-500' : 'bg-slate-300'}`} /><div className="min-w-0 flex-1"><p className="break-words text-xs font-medium text-slate-700">{displayActivity(item)}</p><p className="mt-1 text-[10px] text-slate-400">{item.type === 'command' ? 'Commande' : 'Événement'}</p></div><time className="shrink-0 text-right text-[10px] leading-relaxed text-slate-400" title={formatTimestamp(item.ts)} dateTime={item.ts ? new Date(item.ts * 1000).toISOString() : undefined}>{relativeTimestamp(item.ts)}</time></li>)}</ol>}</div>
    <details className="rounded-xl border border-slate-200 bg-slate-50/50"><summary className="flex cursor-pointer items-center gap-2 px-4 py-3 text-xs font-medium text-slate-500"><Bug className="h-3.5 w-3.5" />Données brutes pour le dépannage</summary><div className="grid gap-4 border-t border-slate-100 p-4 lg:grid-cols-2"><RawPayload label="Dernier état reçu" value={core} /><RawPayload label="Dernier événement reçu" value={activity ?? null} /></div></details>
  </div>;
}

function RawPayload({ label, value }: { label: string; value: unknown }) {
  return <div className="min-w-0"><p className="mb-2 text-xs font-medium text-slate-500">{label}</p><pre className="max-h-64 overflow-auto rounded-xl border border-slate-200 bg-white p-3 text-[10px] leading-relaxed text-slate-500">{JSON.stringify(value, null, 2)}</pre></div>;
}
